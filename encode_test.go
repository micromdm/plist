package plist

import (
	"bytes"
	"testing"
	"time"
)

var fooRef = `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><string>foo</string></plist>
`

var utf8Ref = `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><string>UTF-8 ☼</string></plist>
`

var zeroRef = `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><integer>0</integer></plist>
`

var oneRef = `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><integer>1</integer></plist>
`

var minOneRef = `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><integer>-1</integer></plist>
`

var realRef = `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><real>1.2</real></plist>
`

var falseRef = `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><false/></plist>
`

var trueRef = `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><true/></plist>
`

var arrRef = `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><array><string>a</string><string>b</string><string>c</string><integer>4</integer><true/></array></plist>
`

var byteArrRef = `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><array><data>/////////////////////w==</data></array></plist>
`

var time1900Ref = `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><date>1900-01-01T12:00:00Z</date></plist>
`

var dataRef = `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><data>PD94bWwgdmVyc2lvbj0iMS4wIiBlbmNvZGluZz0iVVRGLTgiPz4KPCFET0NUWVBFIHBsaXN0IFBVQkxJQyAiLS8vQXBwbGUvL0RURCBQTElTVCAxLjAvL0VOIiAiaHR0cDovL3d3dy5hcHBsZS5jb20vRFREcy9Qcm9wZXJ0eUxpc3QtMS4wLmR0ZCI+CjxwbGlzdCB2ZXJzaW9uPSIxLjAiPjxzdHJpbmc+Zm9vPC9zdHJpbmc+PC9wbGlzdD4K</data></plist>
`

var emptyDataRef = `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><data></data></plist>
`

var dictRef = `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><dict><key>bool</key><true/><key>foo</key><string>bar</string></dict></plist>
`

var indentRef = `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
   <key>Boolean</key>
   <true/>
   <key>BooleanList</key>
   <array>
      <true/>
      <false/>
   </array>
   <key>CFBundleInfoDictionaryVersion</key>
   <string>6.0</string>
   <key>Strings</key>
   <array>
      <string>a</string>
      <string>b</string>
   </array>
   <key>band-size</key>
   <integer>8388608</integer>
   <key>bundle-backingstore-version</key>
   <integer>1</integer>
   <key>diskimage-bundle-type</key>
   <string>com.apple.diskimage.sparsebundle</string>
   <key>size</key>
   <integer>4398046511104</integer>
   <key>useless</key>
   <dict>
      <key>unused-string</key>
      <string>unused</string>
   </dict>
</dict>
</plist>
`

var indentRefOmit = `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
   <key>Boolean</key>
   <true/>
   <key>BooleanList</key>
   <array>
      <true/>
      <false/>
   </array>
   <key>CFBundleInfoDictionaryVersion</key>
   <string>6.0</string>
   <key>Strings</key>
   <array>
      <string>a</string>
      <string>b</string>
   </array>
   <key>bundle-backingstore-version</key>
   <integer>1</integer>
   <key>diskimage-bundle-type</key>
   <string>com.apple.diskimage.sparsebundle</string>
   <key>size</key>
   <integer>4398046511104</integer>
</dict>
</plist>
`

type testStruct struct {
	UnusedString string `plist:"unused-string"`
	UnusedByte   []byte `plist:"unused-byte,omitempty"`
}

var encodeTests = []struct {
	in  interface{}
	out string
}{
	{"foo", fooRef},
	{"UTF-8 ☼", utf8Ref},
	{0, zeroRef},
	{1, oneRef},
	{uint64(1), oneRef},
	{-1, minOneRef},
	{1.2, realRef},
	{false, falseRef},
	{true, trueRef},
	{[]interface{}{"a", "b", "c", 4, true}, arrRef},
	{time.Date(1900, 01, 01, 12, 00, 00, 0, time.UTC), time1900Ref},
	{[]byte(fooRef), dataRef},
	{map[string]interface{}{
		"foo":  "bar",
		"bool": true},
		dictRef},
	{struct {
		Foo  string `plist:"foo"`
		Bool bool   `plist:"bool"`
	}{"bar", true},
		dictRef},
	{[][16]byte{
		{0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF},
	}, byteArrRef},
}

func TestEncodeValues(t *testing.T) {
	t.Parallel()
	for _, tt := range encodeTests {
		b, err := Marshal(tt.in)
		if err != nil {
			t.Error(err)
			continue
		}
		out := string(b)
		if out != tt.out {
			t.Errorf("Marshal(%v) = \n%v, \nwant\n %v", tt.in, out, tt.out)
		}
	}
}

func TestNewLineString(t *testing.T) {
	t.Parallel()
	multiline := struct {
		Content string
	}{
		Content: "foo\nbar",
	}

	b, err := MarshalIndent(multiline, "   ")
	if err != nil {
		t.Fatal(err)
	}
	var ok = `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
   <key>Content</key>
   <string>foo
bar</string>
</dict>
</plist>
`
	out := string(b)
	if out != ok {
		t.Errorf("Marshal(%v) = \n%v, \nwant\n %v", multiline, out, ok)
	}

}

func TestIndent(t *testing.T) {
	t.Parallel()
	sparseBundleHeader := struct {
		InfoDictionaryVersion string     `plist:"CFBundleInfoDictionaryVersion"`
		BandSize              uint64     `plist:"band-size"`
		BackingStoreVersion   int        `plist:"bundle-backingstore-version"`
		DiskImageBundleType   string     `plist:"diskimage-bundle-type"`
		Size                  uint64     `plist:"size"`
		Unused                testStruct `plist:"useless"`
		Boolean               bool
		BooleanList           []bool
		Strings               []string
	}{
		InfoDictionaryVersion: "6.0",
		BandSize:              8388608,
		Size:                  4 * 1048576 * 1024 * 1024,
		DiskImageBundleType:   "com.apple.diskimage.sparsebundle",
		BackingStoreVersion:   1,
		Unused:                testStruct{UnusedString: "unused"},
		Boolean:               true,
		BooleanList:           []bool{true, false},
		Strings:               []string{"a", "b"},
	}
	b, err := MarshalIndent(sparseBundleHeader, "   ")
	if err != nil {
		t.Fatal(err)
	}
	out := string(b)
	if out != indentRef {
		t.Errorf("MarshalIndent(%v) = \n%v, \nwant\n%v", sparseBundleHeader, out, indentRef)
	}
}

func TestOmitNotEmpty(t *testing.T) {
	t.Parallel()
	sparseBundleHeader := struct {
		InfoDictionaryVersion string     `plist:"CFBundleInfoDictionaryVersion"`
		BandSize              uint64     `plist:"band-size,omitempty"`
		BackingStoreVersion   int        `plist:"bundle-backingstore-version"`
		DiskImageBundleType   string     `plist:"diskimage-bundle-type"`
		Size                  uint64     `plist:"size"`
		Unused                testStruct `plist:"useless"`
		Boolean               bool
		BooleanList           []bool
		Strings               []string
	}{
		InfoDictionaryVersion: "6.0",
		BandSize:              8388608,
		Size:                  4 * 1048576 * 1024 * 1024,
		DiskImageBundleType:   "com.apple.diskimage.sparsebundle",
		BackingStoreVersion:   1,
		Unused:                testStruct{UnusedString: "unused"},
		Boolean:               true,
		BooleanList:           []bool{true, false},
		Strings:               []string{"a", "b"},
	}
	b, err := MarshalIndent(sparseBundleHeader, "   ")
	if err != nil {
		t.Fatal(err)
	}
	out := string(b)
	if out != indentRef {
		t.Errorf("MarshalIndent(%v) = \n%v, \nwant\n %v", sparseBundleHeader, out, indentRefOmit)
	}
}

func TestOmitIsEmpty(t *testing.T) {
	t.Parallel()
	sparseBundleHeader := struct {
		InfoDictionaryVersion string     `plist:"CFBundleInfoDictionaryVersion"`
		BandSize              uint64     `plist:"band-size,omitempty"`
		BackingStoreVersion   int        `plist:"bundle-backingstore-version"`
		DiskImageBundleType   string     `plist:"diskimage-bundle-type"`
		Size                  uint64     `plist:"size"`
		Unused                testStruct `plist:"useless,omitempty"`
		Boolean               bool
		BooleanList           []bool
		Strings               []string
	}{
		InfoDictionaryVersion: "6.0",
		Size:                  4 * 1048576 * 1024 * 1024,
		DiskImageBundleType:   "com.apple.diskimage.sparsebundle",
		BackingStoreVersion:   1,
		Boolean:               true,
		BooleanList:           []bool{true, false},
		Strings:               []string{"a", "b"},
	}
	b, err := MarshalIndent(sparseBundleHeader, "   ")
	if err != nil {
		t.Fatal(err)
	}
	out := string(b)
	if out != indentRefOmit {
		t.Errorf("MarshalIndent(%v) = \n%v, \nwant\n %v", sparseBundleHeader, out, indentRefOmit)
	}
}

type marshalerTest struct {
	marshalFuncInvoked bool
	MustMarshal        string
}

func (m *marshalerTest) MarshalPlist() (interface{}, error) {
	m.marshalFuncInvoked = true
	return &m.MustMarshal, nil
}

func TestMarshaler(t *testing.T) {
	t.Parallel()
	want := []byte(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><string>pants</string></plist>
`)
	m := marshalerTest{MustMarshal: "pants"}
	have, err := Marshal(&m)
	if err != nil {
		t.Fatal(err)
	}

	if !bytes.Equal(have, want) {
		t.Errorf("expected \n%s got \n%s\n", have, want)
	}
}

func TestSelfClosing(t *testing.T) {
	t.Parallel()
	selfClosing := struct {
		True   bool
		False  bool
		Absent bool
	}{
		True:  true,
		False: false,
	}

	want := []byte(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><dict><key>Absent</key><false/><key>False</key><false/><key>True</key><true/></dict></plist>
`)

	have, err := Marshal(selfClosing)
	if err != nil {
		t.Fatal(err)
	}

	if !bytes.Equal(have, want) {
		t.Errorf("expected \n%s got \n%s\n", have, want)
	}

}

func TestEncodeTagSkip(t *testing.T) {
	// Test struct
	testStruct := struct {
		NoTag   string
		Tag     string `plist:"OtherTag"`
		SkipTag string `plist:"-"`
	}{
		NoTag:   "NoTag",
		Tag:     "Tag",
		SkipTag: "SkipTag",
	}

	have, err := Marshal(&testStruct)
	if err != nil {
		t.Fatal(err)
	}

	if bytes.Contains([]byte(have), []byte(testStruct.SkipTag)) {
		t.Error("field encoded when it was tagged as -")
	}
}
