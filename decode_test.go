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
				Uint64   uint64    `plist:"uint64"`
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

			expectedUint64 := ^uint64(0) // all bits set
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
