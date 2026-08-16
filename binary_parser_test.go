package plist

import (
	"bytes"
	"encoding/binary"
	"io"
	"strings"
	"testing"
)

//  1. Self-reference: object 0 is an array whose only element points back at 0.
//     Must error ("cyclic"), never overflow the stack.
func TestBinaryParserSelfReference(t *testing.T) {
	data := buildBinaryPlist([][]byte{arrayObj([]uint64{0}, 1)}, 0, 1)
	var out interface{}
	err := Unmarshal(data, &out)
	if err == nil || !strings.Contains(err.Error(), "cyclic") {
		t.Fatalf("want cyclic-reference error, got %v", err)
	}
}

//  2. Exponential doubling chain: obj i = [i+1, i+1]; would expand to 2^40 nodes.
//     The node budget must reject it QUICKLY (finish well under the timeout AND
//     return an error). This is the case parse-layer memoization alone fails.
func TestBinaryParserExponentialFanout(t *testing.T) {
	// Shrink the budget so the guard trips almost immediately — we're
	// verifying the guard fires, not how fast it counts to 2 million.
	defer func(n int) { maxObjectNodes = n }(maxObjectNodes)
	maxObjectNodes = 1000

	const n = 40
	objs := make([][]byte, n+1)
	for i := 0; i < n; i++ {
		objs[i] = arrayObj([]uint64{uint64(i + 1), uint64(i + 1)}, 2)
	}
	objs[n] = asciiObj("leaf")
	data := buildBinaryPlist(objs, 0, 2)

	var out interface{}
	err := Unmarshal(data, &out)
	if err == nil || !strings.Contains(err.Error(), "object graph exceeds maximum size") {
		t.Fatalf("want maximum-size error, got %v", err)
	}
}

// 3. Deep linear chain deeper than maxObjectDepth: must error ("depth"), not overflow.
func TestBinaryParserMaxDepth(t *testing.T) {
	const n = maxObjectDepth + 5
	objs := make([][]byte, n+1)
	for i := 0; i < n; i++ {
		objs[i] = arrayObj([]uint64{uint64(i + 1)}, 2)
	}
	objs[n] = asciiObj("leaf")
	data := buildBinaryPlist(objs, 0, 2)
	var out interface{}
	err := Unmarshal(data, &out)
	if err == nil || !strings.Contains(err.Error(), "depth") {
		t.Fatalf("want max-depth error, got %v", err)
	}
}

//  4. Valid, modestly-shared plist (regression): a dict decodes correctly.
//     Ensures the guards don't reject legitimate input.
func TestBinaryParserValidDict(t *testing.T) {
	dict := []byte{0xd2, 1, 2, 3, 4} // count=2; key refs 1,2; value refs 3,4 (refSize 1)
	objs := [][]byte{dict, asciiObj("a"), asciiObj("b"), asciiObj("x"), asciiObj("y")}
	data := buildBinaryPlist(objs, 0, 1)
	var out map[string]string
	if err := Unmarshal(data, &out); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out["a"] != "x" || out["b"] != "y" {
		t.Fatalf("decoded wrong: %#v", out)
	}
}

// 5. Off-by-one: RootObject == NumObjects (one past the table).
func TestBinaryParserRootOutOfRange(t *testing.T) {
	data := buildBinaryPlist([][]byte{asciiObj("x")}, 1, 1)
	var out interface{}
	err := Unmarshal(data, &out)
	if err == nil || !strings.Contains(err.Error(), "out of range") {
		t.Fatalf("want out-of-range error, got %v", err)
	}
}

func TestBinaryParserChildRefOutOfRange(t *testing.T) {
	// Only object 0 exists; its array element points at nonexistent index 1.
	data := buildBinaryPlist([][]byte{arrayObj([]uint64{1}, 1)}, 0, 1)
	var out interface{}
	err := Unmarshal(data, &out)
	if err == nil || !strings.Contains(err.Error(), "offset too large") {
		t.Fatalf("want offset-too-large error, got %v", err)
	}
}

//  6. Issue #28 allocation guard: tiny buffer whose trailer claims a huge NumObjects.
//     Must error cleanly with no large allocation. (Requires the #28 hardening.)
func TestBinaryParserInflatedNumObjects(t *testing.T) {
	data := buildBinaryPlist([][]byte{asciiObj("x")}, 0, 1)
	// Overwrite NumObjects in the trailer (bytes [len-24, len-16)).
	binary.BigEndian.PutUint64(data[len(data)-24:len(data)-16], 1<<40)
	var out interface{}
	if err := Unmarshal(data, &out); err == nil || !strings.Contains(err.Error(), "exceeds available data") {
		t.Fatal("want error for inflated NumObjects")
	}
}

func TestBinaryParserZeroOffsetIntSize(t *testing.T) {
	data := buildBinaryPlist([][]byte{asciiObj("x")}, 0, 1)
	data[len(data)-32+6] = 0 // OffsetIntSize byte in the trailer
	var out interface{}
	if err := Unmarshal(data, &out); err == nil {
		t.Fatal("want error, got nil")
	}
}
func TestBinaryParserHugeObjectRefSize(t *testing.T) {
	data := buildBinaryPlist([][]byte{arrayObj([]uint64{0}, 1)}, 0, 1)
	data[len(data)-32+7] = 9 // ObjectRefSize byte in the trailer
	var out interface{}
	if err := Unmarshal(data, &out); err == nil {
		t.Fatal("want error, got nil")
	}
}

// Byte amplification: a small array whose many refs all point at one large
// object. The node budget (object COUNT) never trips, but the aggregate SIZE
// budget must, so the decode cannot be inflated into unbounded memory.
func TestBinaryParserByteAmplification(t *testing.T) {
	defer func(n uint64) { maxObjectBytes = n }(maxObjectBytes)
	maxObjectBytes = 25

	// obj0: array of 5 refs, all -> obj1. obj1: a 10-byte string.
	// Re-materializing obj1 five times is 50 bytes > the 25-byte budget.
	data := buildBinaryPlist([][]byte{
		arrayObj([]uint64{1, 1, 1, 1, 1}, 1),
		asciiObj("0123456789"),
	}, 0, 1)

	var out interface{}
	err := Unmarshal(data, &out)
	if err == nil || !strings.Contains(err.Error(), "exceeds maximum size") {
		t.Fatalf("want size-budget error, got %v", err)
	}
}

// An offset-table entry that points outside the object region must be rejected.
// A scalar object at such an offset would otherwise decode to silent garbage
// (it never reaches the checkCount guard that catches collections/strings).
func TestBinaryParserOffsetOutOfRange(t *testing.T) {
	data := buildBinaryPlist([][]byte{asciiObj("x")}, 0, 1)
	// OffsetTableOffset is the trailer's last 8 bytes; offset entry 0 sits there
	// (buildBinaryPlist writes 8-byte offsets). Point it at the table itself.
	otoff := binary.BigEndian.Uint64(data[len(data)-8:])
	binary.BigEndian.PutUint64(data[otoff:otoff+8], otoff)
	var out interface{}
	err := Unmarshal(data, &out)
	if err == nil || !strings.Contains(err.Error(), "out of range") {
		t.Fatalf("want offset-out-of-range error, got %v", err)
	}
}

// NumObjects inflated past the object region (more objects than bytes that can
// hold their markers) must be rejected before the offset table is allocated,
// even when NumObjects still fits the raw file (the make([]uint64, N) OOM path).
func TestBinaryParserNumObjectsExceedsObjectRegion(t *testing.T) {
	// Hand-built: 8-byte header, one 1-byte object (bool true) at offset 8, then
	// a 100-entry 1-byte offset table, then the trailer. NumObjects=100 fits the
	// file but far exceeds the 1-byte object region.
	const numObjects = 100
	buf := []byte("bplist00")
	buf = append(buf, 0x09) // object 0: boolean true, at offset 8
	otoff := uint64(len(buf))
	for i := 0; i < numObjects; i++ {
		buf = append(buf, 8) // every 1-byte offset -> the boolean at offset 8
	}
	var tr [32]byte
	tr[6] = 1 // OffsetIntSize
	tr[7] = 1 // ObjectRefSize
	binary.BigEndian.PutUint64(tr[8:16], numObjects)
	binary.BigEndian.PutUint64(tr[16:24], 0) // RootObject
	binary.BigEndian.PutUint64(tr[24:32], otoff)
	buf = append(buf, tr[:]...)

	var out interface{}
	err := Unmarshal(buf, &out)
	if err == nil || !strings.Contains(err.Error(), "exceeds object region") {
		t.Fatalf("want object-region error, got %v", err)
	}
}

// oneByteReader wraps an io.ReadSeeker but returns at most one byte per Read,
// which io.Reader permits. It models a streaming/custom ReadSeeker (not a
// bytes.Reader) to prove multi-byte fields are read with io.ReadFull rather than
// a single Read that would silently zero-fill the unread tail.
type oneByteReader struct{ rs io.ReadSeeker }

func (r oneByteReader) Read(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	return r.rs.Read(p[:1])
}

func (r oneByteReader) Seek(offset int64, whence int) (int64, error) {
	return r.rs.Seek(offset, whence)
}

// A valid plist must decode correctly even through a reader that never fills a
// multi-byte buffer in one call: 8-byte offsets, the 2-byte integer, and the
// multi-char strings all depend on the full buffer being read.
func TestBinaryParserShortReads(t *testing.T) {
	objs := [][]byte{
		{0xd2, 1, 2, 3, 4}, // dict: key refs 1,2; value refs 3,4 (refSize 1)
		asciiObj("num"),    // key 0
		asciiObj("str"),    // key 1
		{0x11, 0x01, 0x02}, // integer 258, encoded in 2 bytes
		asciiObj("hello"),  // value 1
	}
	data := buildBinaryPlist(objs, 0, 1)

	var out struct {
		Num int    `plist:"num"`
		Str string `plist:"str"`
	}
	dec := NewBinaryDecoder(oneByteReader{bytes.NewReader(data)})
	if err := dec.Decode(&out); err != nil {
		t.Fatalf("decode through short reader failed: %v", err)
	}
	if out.Num != 258 || out.Str != "hello" {
		t.Fatalf("decoded wrong through short reader: %+v", out)
	}
}

func buildBinaryPlist(objects [][]byte, root uint64, refSize uint8) []byte {
	buf := []byte("bplist00")
	offsets := make([]uint64, len(objects))
	for i, o := range objects {
		offsets[i] = uint64(len(buf))
		buf = append(buf, o...)
	}
	offsetTableOffset := uint64(len(buf))
	for _, off := range offsets {
		var b [8]byte
		binary.BigEndian.PutUint64(b[:], off)
		buf = append(buf, b[:]...)
	}
	var tr [32]byte
	tr[6] = 8                                                  // OffsetIntSize
	tr[7] = refSize                                            // ObjectRefSize
	binary.BigEndian.PutUint64(tr[8:16], uint64(len(objects))) // NumObjects
	binary.BigEndian.PutUint64(tr[16:24], root)                // RootObject
	binary.BigEndian.PutUint64(tr[24:32], offsetTableOffset)   // OffsetTableOffset
	return append(buf, tr[:]...)
}

// arrayObj encodes an array object (<15 elements) with the given element refs.
func arrayObj(refs []uint64, refSize uint8) []byte {
	o := []byte{0xa0 | byte(len(refs))}
	for _, r := range refs {
		var b [8]byte
		binary.BigEndian.PutUint64(b[:], r)
		o = append(o, b[8-refSize:]...)
	}
	return o
}

// asciiObj encodes a short (<15 char) ASCII string object.
func asciiObj(s string) []byte { return append([]byte{0x50 | byte(len(s))}, s...) }
