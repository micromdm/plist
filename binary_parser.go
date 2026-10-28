package plist

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"time"
	"unicode/utf16"
)

// plistTrailer is the last 32 bytes of a binary plist
// See definition of CFBinaryPlistTrailer here
// https://opensource.apple.com/source/CF/CF-550.29/ForFoundationOnly.h
type plistTrailer struct {
	_                 [5]byte // unused padding
	SortVersion       uint8   // seems to be unused (always zero)
	OffsetIntSize     uint8   // byte size of offset ints in offset table
	ObjectRefSize     uint8   // byte size of object refs in arrays and dicts
	NumObjects        uint64  // number of objects (also number of offsets in offset table)
	RootObject        uint64  // object ref of top level object
	OffsetTableOffset uint64  // offset of the offset table
}

type binaryParser struct {
	OffsetTable   []uint64        // array of offsets for each object in plist
	plistTrailer                  // last 32 bytes of plist
	io.ReadSeeker                 // reader for plist data
	inProgress    map[uint64]bool // indices currently on the recursion stack (cycle detection)
	depth         int             // current recursion depth
	nodes         int             // total materialized objects so far (expansion budget)
	bytes         uint64          // total data/string bytes materialized so far (size budget)
}

// maxObjectDepth bounds binary-plist reference nesting to prevent
// stack exhaustion.
const maxObjectDepth = 128

// maxObjectNodes bounds the TOTAL number of objects materialized while
// decoding, defeating exponential-expansion inputs.
var maxObjectNodes = 1 << 21 // ~2.1M

// headerSize is the fixed "bplist00" magic; trailerSize is the fixed 32-byte
// trailer. Object offsets live in [headerSize, OffsetTableOffset).
const (
	headerSize  = 8
	trailerSize = 32
)

// maxObjectBytes bounds the TOTAL bytes materialized into data and string
// objects while decoding. maxObjectNodes bounds object COUNT; this bounds their
// aggregate SIZE, so a small input that references a large object many times
// (byte amplification) cannot be inflated into unbounded memory the node budget
// alone would not catch.
var maxObjectBytes uint64 = 1 << 26 // 64MB

// newBinaryParser takes in a ReadSeeker for the bytes of a binary plist and
// returns a parser after reading the offset table and trailer.
func newBinaryParser(r io.ReadSeeker) (*binaryParser, error) {
	var bp binaryParser
	bp.ReadSeeker = r

	// Read the trailer.
	if _, err := bp.Seek(-32, io.SeekEnd); err != nil {
		return nil, fmt.Errorf("plist: couldn't seek to start of trailer: %v", err)
	}
	if err := binary.Read(bp, binary.BigEndian, &bp.plistTrailer); err != nil {
		return nil, fmt.Errorf("plist: couldn't read trailer: %v", err)
	}

	if bp.OffsetIntSize == 0 || bp.OffsetIntSize > 8 {
		return nil, fmt.Errorf("plist: invalid offset int size %d", bp.OffsetIntSize)
	}
	if bp.ObjectRefSize == 0 || bp.ObjectRefSize > 8 {
		return nil, fmt.Errorf("plist: invalid object ref size %d", bp.ObjectRefSize)
	}

	fileLen, err := bp.Seek(0, io.SeekEnd)
	if err != nil {
		return nil, err
	}
	// The offset table lives after the header and before the trailer.
	if bp.OffsetTableOffset < headerSize || bp.OffsetTableOffset > uint64(fileLen)-trailerSize {
		return nil, fmt.Errorf("plist: offset table offset (%d) out of range", bp.OffsetTableOffset)
	}
	// The table holds NumObjects entries of OffsetIntSize bytes each and must fit
	// between the object region and the trailer.
	if bp.NumObjects > (uint64(fileLen)-trailerSize-bp.OffsetTableOffset)/uint64(bp.OffsetIntSize) {
		return nil, fmt.Errorf("plist: NumObjects (%d) exceeds available data", bp.NumObjects)
	}
	// Every object occupies at least its 1-byte marker in the object region
	// [headerSize, OffsetTableOffset), so there cannot be more objects than
	// object-region bytes. This bounds the offset-table allocation below to the
	// file size regardless of the OffsetIntSize amplification factor.
	if bp.NumObjects > bp.OffsetTableOffset-headerSize {
		return nil, fmt.Errorf("plist: NumObjects (%d) exceeds object region", bp.NumObjects)
	}
	if bp.RootObject >= bp.NumObjects {
		return nil, fmt.Errorf("plist: root object ref out of range")
	}

	// Read the offset table.
	if _, err := bp.Seek(int64(bp.OffsetTableOffset), io.SeekStart); err != nil {
		return nil, fmt.Errorf("plist: couldn't seek to start of offset table: %v", err)
	}

	bp.OffsetTable = make([]uint64, bp.NumObjects)
	for i := uint64(0); i < bp.NumObjects; i++ {
		buf := make([]byte, 8)
		if _, err := io.ReadFull(bp, buf[8-bp.OffsetIntSize:]); err != nil {
			return nil, fmt.Errorf("plist: couldn't read offset table: %v", err)
		}
		// Each entry must point into the object region; a scalar object at an
		// out-of-region offset would otherwise decode to silent garbage.
		off := binary.BigEndian.Uint64(buf)
		if off < headerSize || off >= bp.OffsetTableOffset {
			return nil, fmt.Errorf("plist: object offset %d out of range", off)
		}
		bp.OffsetTable[i] = off
	}

	bp.inProgress = make(map[uint64]bool)

	return &bp, nil
}

// parseDocument parses the entire binary plist starting from the root object
// and returns a plistValue representing the root object.
func (bp *binaryParser) parseDocument() (*plistValue, error) {
	// Decode and return the root object.
	return bp.parseObjectRef(bp.RootObject)
}

// parseObjectRef decodes and returns the plist object with the given index.
// Index 0 is the first object in the object table, 1 is the second, etc.
// This function restores the current plist offset when it's done so that you
// may call it while decoding a collection object without losing your place.
func (bp *binaryParser) parseObjectRef(index uint64) (val *plistValue, err error) {

	if index >= uint64(len(bp.OffsetTable)) {
		return nil, fmt.Errorf("plist: offset too large: %d", index)
	}

	// cyclic references
	if bp.inProgress[index] {
		return nil, fmt.Errorf("plist: cyclic object reference at index %d", index)
	}

	// total expansion budget
	bp.nodes++
	if bp.nodes > maxObjectNodes {
		return nil, fmt.Errorf("plist: object graph exceeds maximum size (%d nodes)", maxObjectNodes)
	}

	// max depth check
	if bp.depth >= maxObjectDepth {
		return nil, fmt.Errorf("plist: object nesting exceeds maximum depth %d", maxObjectDepth)
	}
	bp.inProgress[index] = true
	bp.depth++
	defer func() {
		delete(bp.inProgress, index)
		bp.depth--
	}()

	// Save the current offset.
	offset, err := bp.Seek(0, io.SeekCurrent)
	if err != nil {
		return nil, err
	}
	// Then restore the original offset in a defer.
	defer func() {
		_, err2 := bp.Seek(offset, io.SeekStart)
		if err2 != nil {
			err = err2
		}
	}()

	// Move to the start of the object we want to decode.
	if _, err := bp.Seek(int64(bp.OffsetTable[index]), io.SeekStart); err != nil {
		return nil, err
	}
	// The first byte of the object is its marker byte.
	// High 4 bits of marker byte indicates the object type.
	// Low 4 bits contain additional info, typically a count.
	// Defined here: https://opensource.apple.com/source/CF/CF-550.29/CFBinaryPList.c
	// (using Read instead of ReadByte so that we can accept a ReadSeeker)
	b := make([]byte, 1)
	if _, err := bp.Read(b); err != nil {
		return nil, err
	}
	marker := b[0]
	switch marker >> 4 {
	case 0x0: // null, bool, or fill
		return bp.parseSingleton(marker)
	case 0x1: // integer
		return bp.parseInteger(marker)
	case 0x2: // real
		return bp.parseReal(marker)
	case 0x3: // date
		return bp.parseDate(marker)
	case 0x4: // data
		return bp.parseData(marker)
	case 0x5: // ascii string
		return bp.parseASCII(marker)
	case 0x6: // unicode (utf-16) string
		return bp.parseUTF16(marker)
	case 0x8: // uid
		return bp.parseUID(marker)
	case 0xa: // array
		return bp.parseArray(marker)
	case 0xc: // set (not supported)
		return &plistValue{Invalid, nil}, nil
	case 0xd: // dictionary
		return bp.parseDict(marker)
	}
	return nil, fmt.Errorf("plist: unknown object type %x", marker>>4)
}

func (bp *binaryParser) parseSingleton(marker byte) (*plistValue, error) {
	switch marker & 0xf {
	case 0x0: // null (not supported)
		return &plistValue{Invalid, nil}, nil
	case 0x8: // bool false
		return &plistValue{Boolean, false}, nil
	case 0x9: // bool true
		return &plistValue{Boolean, true}, nil
	case 0xf: // fill (not supported)
		return &plistValue{Invalid, nil}, nil
	}
	return nil, fmt.Errorf("plist: unrecognized singleton type %x", marker&0xf)
}

func (bp *binaryParser) parseInteger(marker byte) (*plistValue, error) {
	// CoreFoundation writes 1-, 2- and 4-byte integers as unsigned, 8-byte
	// integers as signed 64-bit, and 16-byte integers for unsigned values
	// above math.MaxInt64 (upper 8 bytes zero). Negative values are therefore
	// always 8 bytes. When reading, CF ignores the upper 8 bytes of a 16-byte
	// integer; so do we.
	// See: https://opensource.apple.com/source/CF/CF-550.29/CFBinaryPList.c
	//
	// There is some discussion regarding 128-bit number support in the Python
	// bug report below.  The conclusion was that public APIs only allow plists
	// to contain up to 64-bit values.
	// See: https://bugs.python.org/issue14455
	nbytes := 1 << (marker & 0xf)
	if nbytes > 16 {
		return nil, fmt.Errorf("plist: cannot decode integers longer than 16 bytes (%d)", nbytes)
	}
	// Read into the right-most bytes of a 16-byte zero-valued buffer.
	buf := make([]byte, 16)
	_, err := io.ReadFull(bp, buf[16-nbytes:])
	if err != nil {
		return nil, err
	}
	// Truncate values to 64 bits (8 bytes). Only 8-byte integers are signed.
	value := binary.BigEndian.Uint64(buf[8:])
	result := signedInt{value, nbytes == 8 && int64(value) < 0}

	return &plistValue{Integer, result}, nil
}

func (bp *binaryParser) parseReal(marker byte) (*plistValue, error) {
	nbytes := 1 << (marker & 0xf)
	buf := make([]byte, nbytes)
	if _, err := io.ReadFull(bp, buf); err != nil {
		return nil, err
	}
	switch len(buf) {
	case 4:
		var r float32
		if err := binary.Read(bytes.NewReader(buf), binary.BigEndian, &r); err != nil {
			return nil, err
		}
		return &plistValue{Real, sizedFloat{float64(r), nbytes * 8}}, nil
	case 8:
		var r float64
		if err := binary.Read(bytes.NewReader(buf), binary.BigEndian, &r); err != nil {
			return nil, err
		}
		return &plistValue{Real, sizedFloat{r, nbytes * 8}}, nil
	default:
		return nil, fmt.Errorf("plist: invalid length of real number: expected: 4 or 8, got: %d", len(buf))
	}
}

func (bp *binaryParser) parseDate(marker byte) (*plistValue, error) {
	if marker&0xf != 0x3 {
		return nil, fmt.Errorf("plist: invalid marker byte for date: %x", marker)
	}
	buf := make([]byte, 8)
	if _, err := io.ReadFull(bp, buf); err != nil {
		return nil, err
	}
	var t float64
	if err := binary.Read(bytes.NewReader(buf), binary.BigEndian, &t); err != nil {
		return nil, err
	}
	// The float time is Apple Epoch time (secs since Jan 1, 2001 GMT) but we
	// need to convert it to Unix Epoch time (secs since Jan 1, 1970 GMT)
	t += 978307200
	secs := int64(t)
	nsecs := int64((t - float64(secs)) * 1e9)
	return &plistValue{Date, time.Unix(secs, nsecs)}, nil
}

func (bp *binaryParser) parseUID(marker byte) (*plistValue, error) {
	nbytes := int(marker&0xf) + 1
	if nbytes > 8 {
		return nil, fmt.Errorf("plist: UID too large: %d bytes", nbytes)
	}
	buf := make([]byte, 8)
	if _, err := io.ReadFull(bp, buf[8-nbytes:]); err != nil {
		return nil, err
	}
	return &plistValue{CFUID, UID(binary.BigEndian.Uint64(buf))}, nil
}

// accountBytes adds n to the running total of bytes materialized into data and
// string objects and rejects the document once it exceeds maxObjectBytes. The
// node budget bounds how many objects are decoded; this bounds their aggregate
// size, so a small input referencing a large object many times cannot be
// amplified into unbounded memory.
func (bp *binaryParser) accountBytes(n uint64) error {
	bp.bytes += n
	if bp.bytes > maxObjectBytes {
		return fmt.Errorf("plist: decoded data exceeds maximum size (%d bytes)", maxObjectBytes)
	}
	return nil
}

func (bp *binaryParser) parseData(marker byte) (*plistValue, error) {
	count, err := bp.readCount(marker)
	if err != nil {
		return nil, err
	}
	if err := bp.checkCount(count, 1); err != nil {
		return nil, err
	}
	if err := bp.accountBytes(count); err != nil {
		return nil, err
	}
	buf := make([]byte, count)
	if _, err := io.ReadFull(bp, buf); err != nil {
		return nil, err
	}
	return &plistValue{Data, buf}, nil
}

func (bp *binaryParser) parseASCII(marker byte) (*plistValue, error) {
	count, err := bp.readCount(marker)
	if err != nil {
		return nil, err
	}
	if err := bp.checkCount(count, 1); err != nil {
		return nil, err
	}
	if err := bp.accountBytes(count); err != nil {
		return nil, err
	}
	buf := make([]byte, count)
	if _, err := io.ReadFull(bp, buf); err != nil {
		return nil, err
	}
	return &plistValue{String, string(buf)}, nil
}

func (bp *binaryParser) parseUTF16(marker byte) (*plistValue, error) {
	count, err := bp.readCount(marker)
	if err != nil {
		return nil, err
	}
	if err := bp.checkCount(count, 2); err != nil {
		return nil, err
	}
	if err := bp.accountBytes(2 * count); err != nil {
		return nil, err
	}
	// Each character in the UTF16 string is 2 bytes.  First we read everything
	// into a byte slice, then convert this into a slice of uint16, then this
	// gets converted into a slice of rune, which gets converted to a string.
	buf := make([]byte, 2*count)
	if _, err := io.ReadFull(bp, buf); err != nil {
		return nil, err
	}
	uni := make([]uint16, count)
	if err := binary.Read(bytes.NewReader(buf), binary.BigEndian, uni); err != nil {
		return nil, err
	}
	return &plistValue{String, string(utf16.Decode(uni))}, nil
}

func (bp *binaryParser) parseArray(marker byte) (*plistValue, error) {
	count, err := bp.readCount(marker)
	if err != nil {
		return nil, err
	}
	// A list of count object refs representing the items in the array follow.
	list, err := bp.readObjectList(count)
	if err != nil {
		return nil, err
	}
	return &plistValue{Array, list}, nil
}

func (bp *binaryParser) parseDict(marker byte) (*plistValue, error) {
	count, err := bp.readCount(marker)
	if err != nil {
		return nil, err
	}
	// A list of 2*count object refs follow.  All of the keys are listed first,
	// followed by all of the values.
	keys, err := bp.readObjectList(count)
	if err != nil {
		return nil, err
	}
	vals, err := bp.readObjectList(count)
	if err != nil {
		return nil, err
	}
	m := make(map[string]*plistValue)
	for i := uint64(0); i < count; i++ {
		if keys[i].kind != String {
			return nil, fmt.Errorf("plist: dictionary key is not a string: %v", keys[i])
		}
		m[keys[i].value.(string)] = vals[i]
	}
	return &plistValue{Dictionary, &dictionary{m: m}}, nil
}

// readCount reads the variable-length encoded integer count
// used by data, strings, arrays, and dicts
func (bp *binaryParser) readCount(marker byte) (uint64, error) {
	// Check marker for count < 15 in lower 4 bits.
	if marker&0xf != 0xf {
		return uint64(marker & 0xf), nil
	}
	// Otherwise must read additional bytes to get count.  Read first byte:
	// (using Read instead of ReadByte so that we can accept a ReadSeeker)
	b := make([]byte, 1)
	if _, err := bp.Read(b); err != nil {
		return 0, err
	}
	first := b[0]
	// The lower 4 bits of indicate how many additional bytes to read:
	//   0 means 1 additional byte
	//   1 means 2 additional bytes
	//   2 means 4 additional bytes
	//   3 means 8 additional bytes
	nbytes := 1 << (first & 0x0f)
	// Number of bytes in count should be at most 8.
	if nbytes > 8 {
		return 0, fmt.Errorf("plist: invalid nbytes (%d) in readCount", nbytes)
	}
	buf := make([]byte, 8)
	// Shove these bytes into the low end of an 8-byte buffer.
	if _, err := io.ReadFull(bp, buf[8-nbytes:]); err != nil {
		return 0, err
	}
	return binary.BigEndian.Uint64(buf), nil
}

// readObjectRef reads the next ObjectRefSize bytes from the binary plist
// and returns the bytes decoded into an integer value.
func (bp *binaryParser) readObjectRef() (uint64, error) {
	buf := make([]byte, 8)
	if _, err := io.ReadFull(bp, buf[8-bp.ObjectRefSize:]); err != nil {
		return 0, err
	}
	return binary.BigEndian.Uint64(buf), nil
}

// readObjectList is a helper function for parseArray and parseDict.
// It decodes a sequence of object refs from the current offset in the plist
// and returns the decoded objects in a slice.
func (bp *binaryParser) readObjectList(count uint64) ([]*plistValue, error) {
	if err := bp.checkCount(count, uint64(bp.ObjectRefSize)); err != nil {
		return nil, err
	}
	list := make([]*plistValue, count)
	for i := uint64(0); i < count; i++ {
		// Read index of object in offset table.
		ref, err := bp.readObjectRef()
		if err != nil {
			return nil, err
		}
		// Find and decode the object in object table, then add it to list.
		v, err := bp.parseObjectRef(ref)
		if err != nil {
			return nil, err
		}
		list[i] = v
	}
	return list, nil
}

// remainingInObjects reports the bytes between the current read position and the
// start of the offset table (the end of the object region).
func (bp *binaryParser) remainingInObjects() (uint64, error) {
	pos, err := bp.Seek(0, io.SeekCurrent)
	if err != nil {
		return 0, err
	}
	if uint64(pos) >= bp.OffsetTableOffset {
		return 0, nil
	}
	return bp.OffsetTableOffset - uint64(pos), nil
}

// checkCount rejects a declared element count that cannot fit in the remaining
// object-region bytes (each element occupies at least unit bytes on the wire).
func (bp *binaryParser) checkCount(count, unit uint64) error {
	rem, err := bp.remainingInObjects()
	if err != nil {
		return err
	}
	if unit == 0 || count > rem/unit {
		return fmt.Errorf("plist: declared object size (%d) exceeds remaining input", count)
	}
	return nil
}
