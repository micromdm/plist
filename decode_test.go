package plist

import (
	"bytes"
	"encoding/base64"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

var decodeTests = []struct {
	out interface{}
	in  string
}{
	{"foo", fooRef},
	{"UTF-8 ☼", utf8Ref},
	{uint64(0), zeroRef},
	{uint64(1), oneRef},
	{1.2, realRef},
	{false, falseRef},
	{true, trueRef},
	{[]interface{}{"a", "b", "c", uint64(4), true}, arrRef},
	{time.Date(1900, 01, 01, 12, 00, 00, 0, time.UTC), time1900Ref},
	{map[string]interface{}{
		"foo":  "bar",
		"bool": true},
		dictRef},
}

func TestDecodeEmptyInterface(t *testing.T) {
	for _, tt := range decodeTests {
		var out interface{}
		if err := Unmarshal([]byte(tt.in), &out); err != nil {
			t.Error(err)
			continue
		}
		eq := reflect.DeepEqual(out, tt.out)
		if !eq {
			t.Errorf("Unmarshal(%v) = \n%v, want %v", tt.in, out, tt.out)
		}
	}
}

func TestDecodeDict(t *testing.T) {
	// Test struct
	expected := struct {
		InfoDictionaryVersion string `plist:"CFBundleInfoDictionaryVersion"`
		BandSize              uint64 `plist:"band-size"`
		BackingStoreVersion   int    `plist:"bundle-backingstore-version"`
		DiskImageBundleType   string `plist:"diskimage-bundle-type"`
		Size                  uint64 `plist:"size"`
	}{
		InfoDictionaryVersion: "6.0",
		BandSize:              8388608,
		Size:                  4 * 1048576 * 1024 * 1024,
		DiskImageBundleType:   "com.apple.diskimage.sparsebundle",
		BackingStoreVersion:   1,
	}

	var sparseBundleHeader struct {
		InfoDictionaryVersion string `plist:"CFBundleInfoDictionaryVersion"`
		BandSize              uint64 `plist:"band-size"`
		BackingStoreVersion   int    `plist:"bundle-backingstore-version"`
		DiskImageBundleType   string `plist:"diskimage-bundle-type"`
		Size                  uint64 `plist:"size"`
	}

	if err := Unmarshal([]byte(indentRef), &sparseBundleHeader); err != nil {
		t.Fatal(err)
	}
	if sparseBundleHeader != expected {
		t.Error("Expected", expected, "got", sparseBundleHeader)
	}

	// Test Map
	var mapHeader = map[string]interface{}{}
	// Output map[CFBundleInfoDictionaryVersion:6.0 band-size:8388608 bundle-backingstore-version:1 diskimage-bundle-type:com.apple.diskimage.sparsebundle size:4398046511104]
	if err := Unmarshal([]byte(indentRef), &mapHeader); err != nil {
		t.Fatal(err)
	}
	if mapHeader["CFBundleInfoDictionaryVersion"] != "6.0" {
		t.Fatal("Expected", "6.0", "got", mapHeader["CFBundleInfoDictionaryVersion"])
	}
}

func TestDecodeArray(t *testing.T) {
	const input = `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><array><string>foo</string><string>bar</string></array></plist>`
	var data []string
	expected := []string{"foo", "bar"}
	if err := Unmarshal([]byte(input), &data); err != nil {
		t.Fatal(err)
	}
	if eq := reflect.DeepEqual(data, expected); !eq {
		t.Error("Expected", expected, "got", data)
	}
}

func TestDecodeBoolean(t *testing.T) {
	const input = `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><true/></plist>`
	var data bool
	expected := true
	if err := Unmarshal([]byte(input), &data); err != nil {
		t.Fatal(err)
	}
	if data != expected {
		t.Error("Expected", expected, "got", data)
	}
}

func TestDecodeLargeInteger(t *testing.T) {
	const input = `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><integer>18446744073709551615</integer></plist>`
	var data uint64
	expected := uint64(18446744073709551615)
	if err := Unmarshal([]byte(input), &data); err != nil {
		t.Fatal(err)
	}
	if data != expected {
		t.Error("Expected", expected, "got", data)
	}
}

func TestDecodeNegativeInteger(t *testing.T) {
	// There is an intentional space before -42.
	const input = `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><integer> -42</integer></plist>`
	var data int
	expected := -42
	if err := Unmarshal([]byte(input), &data); err != nil {
		t.Fatal(err)
	}
	if data != expected {
		t.Error("Expected", expected, "got", data)
	}
}

func TestDecodeNegativeIntegerIntoUint(t *testing.T) {
	const input = `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><integer>-42</integer></plist>`
	var data uint
	if err := Unmarshal([]byte(input), &data); err == nil {
		t.Error("Expected error, but unmarshal gave", data)
	}
}

func TestDecodeLargeNegativeInteger(t *testing.T) {
	const input = `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><integer>-9223372036854775808</integer></plist>`
	var data int64
	expected := int64(-9223372036854775808)
	if err := Unmarshal([]byte(input), &data); err != nil {
		t.Fatal(err)
	}
	if data != expected {
		t.Error("Expected", expected, "got", data)
	}
}

func TestDecodeReal(t *testing.T) {
	const input = `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><real>1.2</real></plist>`
	var data float64
	expected := 1.2
	if err := Unmarshal([]byte(input), &data); err != nil {
		t.Fatal(err)
	}
	if data != expected {
		t.Error("Expected", expected, "got", data)
	}
}

func TestDecodeNegativeReal(t *testing.T) {
	const input = `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><real>-3.14159</real></plist>`
	var data float64
	expected := -3.14159
	if err := Unmarshal([]byte(input), &data); err != nil {
		t.Fatal(err)
	}
	if data != expected {
		t.Error("Expected", expected, "got", data)
	}
}

func TestDecodeDate(t *testing.T) {
	const input = `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
  <date>2011-05-12T01:00:00Z</date>
</plist>`
	var data time.Time
	expected, _ := time.Parse(time.RFC3339, "2011-05-12T01:00:00Z")
	if err := Unmarshal([]byte(input), &data); err != nil {
		t.Fatal(err)
	}
	if data != expected {
		t.Error("Expected", expected, "got", data)
	}
}

func TestDecodeData(t *testing.T) {
	expected := `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><string>foo</string></plist>
`
	type data []byte
	out := data{}
	if err := Unmarshal([]byte(dataRef), &out); err != nil {
		t.Fatal(err)
	}
	if string(out) != expected {
		t.Error("Want:\n", expected, "\ngot:\n", string(out))
	}
}

func TestDecodeData_emptyData(t *testing.T) {
	var before, after []byte
	if err := Unmarshal([]byte(emptyDataRef), &after); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(before, after) {
		t.Log("empty <data></data> should result in []byte(nil)")
		t.Errorf("before %#v, after %#v", before, after)
	}
}

func TestDecodeUnicodeString(t *testing.T) {
	const input = `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><string>こんにちは世界</string></plist>`
	var data string
	expected := "こんにちは世界"
	if err := Unmarshal([]byte(input), &data); err != nil {
		t.Fatal(err)
	}
	if data != expected {
		t.Error("Expected", expected, "got", data)
	}
}

// Unknown struct fields should return an error
func TestDecodeUnknownStructField(t *testing.T) {
	var sparseBundleHeader struct {
		InfoDictionaryVersion string `plist:"CFBundleInfoDictionaryVersion"`
		BandSize              uint64 `plist:"band-size"`
		BackingStoreVersion   int    `plist:"bundle-backingstore-version"`
		DiskImageBundleType   string `plist:"diskimage-bundle-type"`
		Size                  uint64 `plist:"unknownKey"`
	}
	if err := Unmarshal([]byte(indentRef), &sparseBundleHeader); err != nil {
		t.Error("Expected error `plist: unknown struct field unknownKey`, got nil")
	}
}

func TestHTTPDecoding(t *testing.T) {
	const raw = `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><string>bar</string></plist>`

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(raw))
	}))
	defer ts.Close()
	res, err := http.Get(ts.URL)
	if err != nil {
		log.Fatalf("GET failed: %v", err)
	}
	defer res.Body.Close()
	var foo string
	d := NewDecoder(res.Body)
	err = d.Decode(&foo)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if foo != "bar" {
		t.Errorf("decoded %q; want \"bar\"", foo)
	}
	err = d.Decode(&foo)
	if err != io.EOF {
		t.Errorf("err = %v; want io.EOF", err)
	}
}

func TestDecodePointer(t *testing.T) {
	var sparseBundleHeader struct {
		InfoDictionaryVersion *string `plist:"CFBundleInfoDictionaryVersion"`
		BandSize              *uint64 `plist:"band-size"`
		BackingStoreVersion   int     `plist:"bundle-backingstore-version"`
		DiskImageBundleType   string  `plist:"diskimage-bundle-type"`
		Size                  uint64  `plist:"unknownKey"`
	}
	if err := Unmarshal([]byte(indentRef), &sparseBundleHeader); err != nil {
		t.Fatal(err)
	}
	if *sparseBundleHeader.InfoDictionaryVersion != "6.0" {
		t.Error("Expected", "6.0", "got", *sparseBundleHeader.InfoDictionaryVersion)
	}
}

func TestDecodeBinaryPlist(t *testing.T) {
	tests := []struct {
		filename     string
		expectedInts []int64
	}{
		{
			filename:     "sample2.binary.plist",
			expectedInts: []int64{0, 42, -42, 255, -255, -123456, -9223372036854775807, 9223372036854775807},
		},
	}

	for _, tt := range tests {
		t.Run(tt.filename, func(t *testing.T) {
			var sample struct {
				Ints     []int64   `plist:"ints"`
				Signed   int64     `plist:"signed"`
				Unsigned uint64    `plist:"unsigned"`
				Uint64   int64     `plist:"uint64"` // 8-byte 0xffffffffffffffff; CF reads it as -1
				Reals    []float64 `plist:"reals"`
				Date     time.Time `plist:"date"`
				Strings  []string  `plist:"strings"`
				Data     [][]byte  `plist:"data"`
			}

			content, err := ioutil.ReadFile(filepath.Join("testdata", tt.filename))
			if err != nil {
				t.Fatal(err)
			}

			if err := Unmarshal(content, &sample); err != nil {
				t.Fatal(err)
			}

			if got, want := len(sample.Ints), len(tt.expectedInts); got != want {
				t.Errorf("decoded %d ints, want %d", got, want)
			}

			for i, x := range tt.expectedInts {
				if sample.Ints[i] != x {
					t.Error("expected", x, "got", sample.Ints[i])
				}
			}

			expectedUnsigned := uint64(1<<63 - 1)
			if sample.Unsigned != expectedUnsigned {
				t.Error("expected", expectedUnsigned, "got", sample.Unsigned)
			}

			expectedSigned := int64(-1)
			if sample.Signed != expectedSigned {
				t.Error("expected", expectedSigned, "got", sample.Signed)
			}

			expectedUint64 := int64(-1)
			if sample.Uint64 != expectedUint64 {
				t.Error("expected", expectedUint64, "got", sample.Uint64)
			}

			expectedReals := []float64{0.0, 3.14159, -1234.5678}
			if len(expectedReals) != len(sample.Reals) {
				t.Errorf("expected %d reals, but only decoded %d reals", len(expectedReals), len(sample.Reals))
			}

			for i, x := range expectedReals {
				if sample.Reals[i] != x {
					t.Error("expected", x, "got", sample.Reals[i])
				}
			}

			expectedDate, _ := time.Parse(time.RFC3339, "2038-01-19T03:14:08Z")
			if !sample.Date.Equal(expectedDate) {
				t.Error("expected", expectedDate, "got", sample.Date)
			}

			expectedStrings := []string{
				"short",
				"こんにちは世界",
				"this is a much longer string having more than 14 characters",
				"Lorem ipsum dolor sit amet, consectetur adipiscing elit, sed do eiusmod tempor incididunt ut labore et dolore magna aliqua. Ut enim ad minim veniam, quis nostrud exercitation ullamco laboris nisi ut aliquip ex ea commodo consequat. Duis aute irure dolor in reprehenderit in voluptate velit esse cillum dolore eu fugiat nulla pariatur. Excepteur sint occaecat cupidatat non proident, sunt in culpa qui officia deserunt mollit anim id est laborum.",
			}
			if len(expectedStrings) != len(sample.Strings) {
				t.Errorf("expected %d strings, but only decoded %d strings", len(expectedStrings), len(sample.Strings))
			}
			for i, x := range expectedStrings {
				if sample.Strings[i] != x {
					t.Error("expected", x, "got", sample.Strings[i])
				}
			}

			expectedData := [][]byte{
				MustDecodeBase64("PEKBpYGlmYFCPA=="),
				MustDecodeBase64("TG9yZW0gaXBzdW0gZG9sb3Igc2l0IGFtZXQsIGNvbnNlY3RldHVyIGFkaXBpc2NpbmcgZWxpdCwgc2VkIGRvIGVpdXNtb2QgdGVtcG9yIGluY2lkaWR1bnQgdXQgbGFib3JlIGV0IGRvbG9yZSBtYWduYSBhbGlxdWEuIFV0IGVuaW0gYWQgbWluaW0gdmVuaWFtLCBxdWlzIG5vc3RydWQgZXhlcmNpdGF0aW9uIHVsbGFtY28gbGFib3JpcyBuaXNpIHV0IGFsaXF1aXAgZXggZWEgY29tbW9kbyBjb25zZXF1YXQuIER1aXMgYXV0ZSBpcnVyZSBkb2xvciBpbiByZXByZWhlbmRlcml0IGluIHZvbHVwdGF0ZSB2ZWxpdCBlc3NlIGNpbGx1bSBkb2xvcmUgZXUgZnVnaWF0IG51bGxhIHBhcmlhdHVyLiBFeGNlcHRldXIgc2ludCBvY2NhZWNhdCBjdXBpZGF0YXQgbm9uIHByb2lkZW50LCBzdW50IGluIGN1bHBhIHF1aSBvZmZpY2lhIGRlc2VydW50IG1vbGxpdCBhbmltIGlkIGVzdCBsYWJvcnVtLg=="),
			}
			if len(expectedData) != len(sample.Data) {
				t.Errorf("expected %d data items, but only decoded %d", len(expectedData), len(sample.Data))
			}
			for i, x := range expectedData {
				if !bytes.Equal(sample.Data[i], x) {
					t.Error("expected", x, "got", sample.Data[i])
				}
			}
		})
	}
}

func MustDecodeBase64(b64 string) []byte {
	data, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		panic(err)
	}
	return data
}

type unmarshalerTest struct {
	unmarshalInvoked bool
	MustDecode       string
}

func (u *unmarshalerTest) UnmarshalPlist(f func(i interface{}) error) error {
	u.unmarshalInvoked = true
	return f(&u.MustDecode)
}

func TestUnmarshaler(t *testing.T) {
	const raw = `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><string>bar</string></plist>`

	var u unmarshalerTest
	if err := Unmarshal([]byte(raw), &u); err != nil {
		t.Fatal(err)
	}

	if !u.unmarshalInvoked {
		t.Errorf("expected the UnmarshalPlist method to be invoked for unmarshaler")
	}

	if have, want := u.MustDecode, "bar"; have != want {
		t.Errorf("have %s, want %s", have, want)
	}
}

func TestFuzzCrashers(t *testing.T) {
	dir := filepath.Join("testdata", "crashers")
	testDir, err := ioutil.ReadDir(dir)
	if err != nil {
		t.Fatalf("reading dir %q: %s", dir, err)
	}

	for _, tc := range testDir {
		tc := tc
		t.Run(tc.Name(), func(t *testing.T) {
			t.Parallel()

			crasher, err := ioutil.ReadFile(filepath.Join("testdata", "crashers", tc.Name()))
			if err != nil {
				t.Fatal(err)
			}

			var i interface{}
			Unmarshal(crasher, &i)
		})
	}
}

// arrayDoc wraps array elements in a complete XML plist document.
func arrayDoc(elements string) []byte {
	return []byte(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><array>` + elements + `</array></plist>`)
}

// TestDecodeArrayIntoPopulatedSlice checks that decoding into a slice that
// already has elements replaces its contents. The destination's prior length
// must not affect where elements land or how many survive.
func TestDecodeArrayIntoPopulatedSlice(t *testing.T) {
	for _, tt := range []struct {
		name     string
		elements string
		dst      []string
		expected []string
	}{
		{
			name:     "shorter than destination",
			elements: `<string>x</string>`,
			dst:      []string{"pre1", "pre2", "pre3"},
			expected: []string{"x"},
		},
		{
			name:     "same length as destination",
			elements: `<string>x</string><string>y</string>`,
			dst:      []string{"pre1", "pre2"},
			expected: []string{"x", "y"},
		},
		{
			name:     "longer than destination",
			elements: `<string>x</string><string>y</string><string>z</string>`,
			dst:      []string{"pre1"},
			expected: []string{"x", "y", "z"},
		},
		{
			name:     "empty array clears destination",
			elements: ``,
			dst:      []string{"pre1", "pre2"},
			expected: []string{},
		},
		{
			name:     "spare capacity is reused",
			elements: `<string>x</string><string>y</string>`,
			dst:      make([]string, 0, 10),
			expected: []string{"x", "y"},
		},
		{
			name:     "nil destination",
			elements: `<string>x</string>`,
			dst:      nil,
			expected: []string{"x"},
		},
	} {
		tc := tt
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			dst := tc.dst
			if err := Unmarshal(arrayDoc(tc.elements), &dst); err != nil {
				t.Fatal(err)
			}
			if len(dst) != len(tc.expected) {
				t.Fatalf("got len %d (%v), want len %d (%v)", len(dst), dst, len(tc.expected), tc.expected)
			}
			for i := range tc.expected {
				if dst[i] != tc.expected[i] {
					t.Errorf("index %d: got %q, want %q", i, dst[i], tc.expected[i])
				}
			}
		})
	}
}

// TestDecodeArrayReplacesStaleElements checks that an element left over in the
// destination slice cannot leak fields into the decoded result. Struct decoding
// only assigns keys that are present in the document, so a reused element has
// to be cleared first.
func TestDecodeArrayReplacesStaleElements(t *testing.T) {
	t.Parallel()
	type entry struct {
		A string
		B string
	}

	dst := []entry{{A: "stale-a", B: "stale-b"}}
	if err := Unmarshal(arrayDoc(`<dict><key>A</key><string>new-a</string></dict>`), &dst); err != nil {
		t.Fatal(err)
	}

	expected := []entry{{A: "new-a"}}
	if !reflect.DeepEqual(dst, expected) {
		t.Errorf("got %+v, want %+v", dst, expected)
	}
}

func TestSmallInput(t *testing.T) {
	type nop struct{}
	nopStruct := &nop{}
	for _, test := range []string{
		"",
		"!",
		"<?xml",
		"bplist0",
	} {
		err := Unmarshal([]byte(test), nopStruct)
		if err == nil {
			t.Errorf("should have produced error with input: %q", test)
		}
	}

}

func TestDecodeTagSkip(t *testing.T) {
	const input = `<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><dict><key>NoTag</key><string>NoTag</string><key>OtherTag</key><string>Tag</string><key>SkipTag</key><string>SkipTag</string></dict></plist>`
	// Test struct
	testStruct := struct {
		NoTag   string
		Tag     string `plist:"OtherTag"`
		SkipTag string `plist:"-"`
	}{}

	if err := Unmarshal([]byte(input), &testStruct); err != nil {
		t.Fatal(err)
	}

	if testStruct.SkipTag != "" {
		t.Error("field decoded when it was tagged as -")
	}
}

// TestXMLPlutilParity tests parity with plutil -lint on macOS
func TestXMLPlutilParity(t *testing.T) {
	type data struct {
		Key string `plist:"key"`
	}
	tests, err := ioutil.ReadDir("testdata/xml/")
	if err != nil {
		t.Fatalf("could not open testdata/xml: %v", err)
	}

	plutil, _ := exec.LookPath("plutil")

	for _, test := range tests {
		testPath := filepath.Join("testdata/xml/", test.Name())
		buf, err := ioutil.ReadFile(testPath)
		if err != nil {
			t.Errorf("could not read test %s: %v", test.Name(), err)
			continue
		}
		v := new(data)
		err = Unmarshal(buf, v)

		shouldFail := strings.HasSuffix(test.Name(), ".failure.plist")
		if plutil != "" {
			plutilFail := exec.Command(plutil, "-lint", testPath).Run() != nil
			if shouldFail != plutilFail {
				t.Errorf("expected plutil test failure: %v for %s, but got test failure: %v", shouldFail, test.Name(), plutilFail)
			}
		}

		if shouldFail && err == nil {
			t.Errorf("expected error for test %s but got: nil", test.Name())
		} else if !shouldFail && err != nil {
			t.Errorf("expected no error for test %s but got: %v", test.Name(), err)
		} else if !shouldFail && v.Key != "val" {
			t.Errorf("expected key=val for test %s but got: key=%s", test.Name(), v.Key)
		}
	}
}

type testVal struct {
	s string
	b bool
}

func (v *testVal) UnmarshalPlist(f func(interface{}) error) (err error) {
	var val interface{}
	err = f(&val)
	if err != nil {
		return err
	}
	switch value := val.(type) {
	case string:
		v.s = value
	case bool:
		v.b = value
	}
	return nil
}

type nestedType struct {
	Val  *testVal `plist:"val"`
	Val2 *testVal `plist:"val2"`
}

// TestDecodeCustomType tests decoding a type that decodes into multiple types
// based on the underlying plist type
func TestDecodeCustomType(t *testing.T) {
	p := `<?xml version="1.0" encoding="UTF-8"?>
		<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "https://www.apple.com/DTDs/PropertyList-1.0.dtd">
		<plist version="1.0">
		<dict>
			<key>val</key>
			<string>val</string>
			<key>val2</key>
			<true></true>
		</dict>
		</plist>`
	r := bytes.NewBuffer([]byte(p))
	decoder := NewXMLDecoder(r)
	typ := new(nestedType)
	err := decoder.Decode(typ)
	if err != nil {
		t.Fatalf("could not read profile: %v", err)
	}

	if typ.Val == nil {
		t.Fatal("unexpected nil for typ.Val")
	}
	if have, want := typ.Val.s, "val"; have != want {
		t.Errorf("typ.Val: have %v, want %v", have, want)
	}

	if typ.Val2 == nil {
		t.Fatal("unexpected nil for typ.Val2")
	}
	if have, want := typ.Val2.b, true; have != want {
		t.Errorf("typ.Val2: have %v, want %v", have, want)
	}
}

// inline bplist with dict containing uid key and UID(42) value
var uidBplist = []byte{
	0x62, 0x70, 0x6c, 0x69, 0x73, 0x74, 0x30, 0x30,
	0xd1, 0x01, 0x02,
	0x53, 0x75, 0x69, 0x64,
	0x80, 0x2a,
	0x08, 0x0b, 0x0f, 0x00, 0x00, 0x00, 0x00, 0x00,
	0x00, 0x01, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00,
	0x00, 0x00, 0x03, 0x00, 0x00, 0x00, 0x00, 0x00,
	0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	0x00, 0x00, 0x11,
}

func TestDecodeUID(t *testing.T) {
	var data struct {
		UID UID `plist:"uid"`
	}
	if err := Unmarshal(uidBplist, &data); err != nil {
		t.Fatal(err)
	}
	if data.UID != 42 {
		t.Error("Expected", UID(42), "got", data.UID)
	}
}

func TestDecodeUIDToUint64(t *testing.T) {
	var data struct {
		UID uint64 `plist:"uid"`
	}
	if err := Unmarshal(uidBplist, &data); err != nil {
		t.Fatal(err)
	}
	if data.UID != 42 {
		t.Error("Expected", 42, "got", data.UID)
	}
}

func TestDecodeUIDToInterface(t *testing.T) {
	var data interface{}
	if err := Unmarshal(uidBplist, &data); err != nil {
		t.Fatal(err)
	}
	m := data.(map[string]interface{})
	uid := m["uid"].(UID)
	if uid != 42 {
		t.Error("Expected", UID(42), "got", uid)
	}
}

func TestDecodeUIDToIncompatibleType(t *testing.T) {
	var data struct {
		UID string `plist:"uid"`
	}
	if err := Unmarshal(uidBplist, &data); err == nil {
		t.Error("Expected error unmarshaling UID into string")
	}
}

func TestDecodeUIDToFloat(t *testing.T) {
	var data struct {
		UID float64 `plist:"uid"`
	}
	if err := Unmarshal(uidBplist, &data); err == nil {
		t.Error("Expected error unmarshaling UID into float64")
	}
}

func TestDecodeUIDToBool(t *testing.T) {
	var data struct {
		UID bool `plist:"uid"`
	}
	if err := Unmarshal(uidBplist, &data); err == nil {
		t.Error("Expected error unmarshaling UID into bool")
	}
}

func TestDecodeUIDInvalidSize(t *testing.T) {
	// bplist with UID marker 0x89 (9 bytes, but max is 8)
	bplist := []byte{
		0x62, 0x70, 0x6c, 0x69, 0x73, 0x74, 0x30, 0x30,
		0x89, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x2a,
		0x08, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x01, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x12,
	}

	var data interface{}
	if err := Unmarshal(bplist, &data); err == nil {
		t.Error("Expected error for UID with invalid size")
	}
}

func TestDecodeUIDFromFile(t *testing.T) {
	content, err := ioutil.ReadFile(filepath.Join("testdata", "uid.binary.plist"))
	if err != nil {
		t.Fatal(err)
	}

	var data interface{}
	if err := Unmarshal(content, &data); err != nil {
		t.Fatal(err)
	}

	m := data.(map[string]interface{})
	objects := m["$objects"].([]interface{})

	obj1 := objects[1].(map[string]interface{})
	classUID, ok := obj1["$class"].(UID)
	if !ok {
		t.Fatal("Expected $class to be UID")
	}
	if classUID != 2 {
		t.Error("Expected", UID(2), "got", classUID)
	}

	top := m["$top"].(map[string]interface{})
	rootUID, ok := top["root"].(UID)
	if !ok {
		t.Fatal("Expected root to be UID")
	}
	if rootUID != 1 {
		t.Error("Expected", UID(1), "got", rootUID)
	}
}

// TestDecodeEmbeddedPointer checks that decoding a key promoted through a nil
// embedded struct pointer allocates it, and that a nil pointer to an
// unexported embedded struct, which reflection can't allocate, is an error
// rather than a panic.
func TestDecodeEmbeddedPointer(t *testing.T) {
	t.Parallel()
	doc := xmlDoc(`<dict><key>S</key><string>a</string><key>S2</key><string>b</string><key>S3</key><string>c</string></dict>`)

	t.Run("allocates", func(t *testing.T) {
		t.Parallel()
		var out EmbeddedOuter
		if err := Unmarshal([]byte(doc), &out); err != nil {
			t.Fatal(err)
		}
		if out.S != "a" || out.EmbeddedInner == nil || out.S2 != "b" {
			t.Errorf("Unmarshal = %+v (EmbeddedInner %+v), want S=a S2=b", out, out.EmbeddedInner)
		}
	})

	t.Run("allocates two levels down", func(t *testing.T) {
		t.Parallel()
		var out struct {
			*EmbeddedOuter
		}
		if err := Unmarshal([]byte(doc), &out); err != nil {
			t.Fatal(err)
		}
		if out.EmbeddedOuter == nil || out.EmbeddedInner == nil || out.S != "a" || out.S2 != "b" {
			t.Errorf("Unmarshal = %+v, want S=a S2=b", out)
		}
	})

	t.Run("leaves nil without a key", func(t *testing.T) {
		t.Parallel()
		var out EmbeddedOuter
		if err := Unmarshal([]byte(xmlDoc(`<dict><key>S</key><string>a</string></dict>`)), &out); err != nil {
			t.Fatal(err)
		}
		if out.EmbeddedInner != nil {
			t.Errorf("Unmarshal allocated EmbeddedInner with no key for it: %+v", out.EmbeddedInner)
		}
	})

	t.Run("unexported nil is an error", func(t *testing.T) {
		t.Parallel()
		var out embeddedOuterUnexported
		if err := Unmarshal([]byte(doc), &out); err == nil {
			t.Errorf("Unmarshal into nil unexported embedded pointer succeeded, want error")
		}
	})

	t.Run("unexported non-nil", func(t *testing.T) {
		t.Parallel()
		out := embeddedOuterUnexported{embeddedUnexported: &embeddedUnexported{}}
		if err := Unmarshal([]byte(doc), &out); err != nil {
			t.Fatal(err)
		}
		if out.S3 != "c" {
			t.Errorf("Unmarshal S3 = %q, want %q", out.S3, "c")
		}
	})
}

// TestDecodeDataIntoByteArray checks that <data> decodes into a fixed-size
// byte array, as it already encodes from one.
func TestDecodeDataIntoByteArray(t *testing.T) {
	t.Parallel()
	// "AAECAw==" is the four bytes 0x00 0x01 0x02 0x03.
	data4 := xmlDoc(`<data>AAECAw==</data>`)

	t.Run("exact size", func(t *testing.T) {
		t.Parallel()
		var out [4]byte
		if err := Unmarshal([]byte(data4), &out); err != nil {
			t.Fatal(err)
		}
		if want := [4]byte{0, 1, 2, 3}; out != want {
			t.Errorf("Unmarshal = %v, want %v", out, want)
		}
	})

	t.Run("shorter zeroes the rest", func(t *testing.T) {
		t.Parallel()
		out := [6]byte{9, 9, 9, 9, 9, 9}
		if err := Unmarshal([]byte(data4), &out); err != nil {
			t.Fatal(err)
		}
		if want := [6]byte{0, 1, 2, 3, 0, 0}; out != want {
			t.Errorf("Unmarshal = %v, want %v", out, want)
		}
	})

	t.Run("longer is a type error", func(t *testing.T) {
		t.Parallel()
		var out [3]byte
		err := Unmarshal([]byte(data4), &out)
		if _, ok := err.(UnmarshalTypeError); !ok {
			t.Errorf("Unmarshal of 4 bytes into %T: err = %v (%T), want UnmarshalTypeError", out, err, err)
		}
	})

	t.Run("named byte type", func(t *testing.T) {
		t.Parallel()
		type myByte byte
		var out [4]myByte
		if err := Unmarshal([]byte(data4), &out); err != nil {
			t.Fatal(err)
		}
		if want := [4]myByte{0, 1, 2, 3}; out != want {
			t.Errorf("Unmarshal = %v, want %v", out, want)
		}
	})

	t.Run("struct field and map value", func(t *testing.T) {
		t.Parallel()
		doc := xmlDoc(`<dict><key>ID</key><data>AAECAw==</data></dict>`)
		var s struct{ ID [4]byte }
		if err := Unmarshal([]byte(doc), &s); err != nil {
			t.Fatal(err)
		}
		var m map[string][4]byte
		if err := Unmarshal([]byte(doc), &m); err != nil {
			t.Fatal(err)
		}
		want := [4]byte{0, 1, 2, 3}
		if s.ID != want || m["ID"] != want {
			t.Errorf("Unmarshal = %v and %v, want %v", s.ID, m["ID"], want)
		}
	})

	t.Run("round trip", func(t *testing.T) {
		t.Parallel()
		in := [16]byte{0xde, 0xad, 0xbe, 0xef, 15: 0xff}
		b, err := Marshal(in)
		if err != nil {
			t.Fatal(err)
		}
		var out [16]byte
		if err := Unmarshal(b, &out); err != nil {
			t.Fatal(err)
		}
		if out != in {
			t.Errorf("round trip = %v, want %v", out, in)
		}
	})

	t.Run("array of non-bytes is a type error", func(t *testing.T) {
		t.Parallel()
		var out [4]int
		if err := Unmarshal([]byte(data4), &out); err == nil {
			t.Errorf("Unmarshal <data> into %T succeeded, want error", out)
		}
	})
}

// TestDecodeDictIntoMapKeyType checks that a dictionary decodes into a map
// with a string-kinded key, named or not, and that any other key type is a
// type error rather than a panic from reflect.
func TestDecodeDictIntoMapKeyType(t *testing.T) {
	t.Parallel()
	doc := []byte(xmlDoc(`<dict><key>1</key><string>a</string></dict>`))

	t.Run("named string key", func(t *testing.T) {
		t.Parallel()
		type key string
		var out map[key]string
		if err := Unmarshal(doc, &out); err != nil {
			t.Fatal(err)
		}
		if out["1"] != "a" {
			t.Errorf("Unmarshal = %v, want map[1:a]", out)
		}
	})

	for _, tt := range []struct {
		name string
		out  interface{}
	}{
		{"int key", &map[int]string{}},
		{"bool key", &map[bool]string{}},
		{"struct key", &map[struct{ S string }]string{}},
		// Plist dictionary keys are always strings, so an interface key is
		// a type error too, as in encoding/json, even though a string would
		// fit in it.
		{"interface key", &map[interface{}]string{}},
	} {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := Unmarshal(doc, tt.out)
			if _, ok := err.(UnmarshalTypeError); !ok {
				t.Errorf("Unmarshal into %T: err = %v (%T), want UnmarshalTypeError", tt.out, err, err)
			}
		})
	}

	t.Run("nil map stays nil", func(t *testing.T) {
		t.Parallel()
		var out map[int]string
		if err := Unmarshal(doc, &out); err == nil {
			t.Fatal("Unmarshal succeeded, want error")
		}
		if out != nil {
			t.Errorf("failed Unmarshal allocated %#v", out)
		}
	})
}

// TestDecodeIntoPopulatedMap checks that decoding into a map that already has
// entries replaces the value for each key in the document and keeps the rest,
// as in encoding/json. Map elements aren't addressable, so decoding into the
// existing element used to panic.
func TestDecodeIntoPopulatedMap(t *testing.T) {
	t.Parallel()
	type pair struct{ A, B int }

	t.Run("string values", func(t *testing.T) {
		t.Parallel()
		out := map[string]string{"k": "old", "keep": "x"}
		doc := xmlDoc(`<dict><key>k</key><string>new</string></dict>`)
		if err := Unmarshal([]byte(doc), &out); err != nil {
			t.Fatal(err)
		}
		if want := (map[string]string{"k": "new", "keep": "x"}); !reflect.DeepEqual(out, want) {
			t.Errorf("Unmarshal = %v, want %v", out, want)
		}
	})

	t.Run("struct values replace", func(t *testing.T) {
		t.Parallel()
		out := map[string]pair{"k": {A: 1, B: 2}}
		doc := xmlDoc(`<dict><key>k</key><dict><key>A</key><integer>9</integer></dict></dict>`)
		if err := Unmarshal([]byte(doc), &out); err != nil {
			t.Fatal(err)
		}
		if want := (pair{A: 9}); out["k"] != want {
			t.Errorf("Unmarshal k = %+v, want %+v", out["k"], want)
		}
	})

	t.Run("pointer values replace", func(t *testing.T) {
		t.Parallel()
		old := &pair{A: 1, B: 2}
		out := map[string]*pair{"k": old}
		doc := xmlDoc(`<dict><key>k</key><dict><key>A</key><integer>9</integer></dict></dict>`)
		if err := Unmarshal([]byte(doc), &out); err != nil {
			t.Fatal(err)
		}
		if want := (pair{A: 9}); out["k"] == nil || *out["k"] != want {
			t.Errorf("Unmarshal k = %+v, want %+v", out["k"], want)
		}
		if want := (pair{A: 1, B: 2}); *old != want {
			t.Errorf("Unmarshal wrote through the old pointer: %+v, want %+v", *old, want)
		}
	})
}
