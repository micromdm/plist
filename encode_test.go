package plist

import (
	"bytes"
	"errors"
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

func TestEncodeUID(t *testing.T) {
	data := struct {
		MyUID UID `plist:"uid"`
	}{
		MyUID: 42,
	}

	b, err := Marshal(data)
	if err != nil {
		t.Fatal(err)
	}

	if !bytes.Contains(b, []byte("CF$UID")) {
		t.Error("expected CF$UID in output")
	}
	if !bytes.Contains(b, []byte("<integer>42</integer>")) {
		t.Error("expected <integer>42</integer> in output")
	}
}

func TestEncodeDecodeUIDRoundtrip(t *testing.T) {
	type Data struct {
		MyUID UID `plist:"uid"`
	}

	original := Data{MyUID: 42}

	b, err := Marshal(original)
	if err != nil {
		t.Fatal(err)
	}

	var decoded Data
	if err := Unmarshal(b, &decoded); err != nil {
		t.Fatal(err)
	}

	if decoded.MyUID != original.MyUID {
		t.Error("Expected", original.MyUID, "got", decoded.MyUID)
	}
}

func TestEncodeDecodeUIDRoundtripIncompatibleType(t *testing.T) {
	type Data struct {
		MyUID UID `plist:"uid"`
	}

	original := Data{MyUID: 42}

	b, err := Marshal(original)
	if err != nil {
		t.Fatal(err)
	}

	var decoded struct {
		MyUID string `plist:"uid"`
	}
	err = Unmarshal(b, &decoded)
	if err == nil {
		t.Error("Expected error when decoding UID to string")
	}
}

// xmlDoc wraps a plist body in the header the encoder emits, so that the tests
// below can state just the part they care about.
func xmlDoc(body string) string {
	return `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">` + body + `</plist>
`
}

// valueMarshaler implements Marshaler with a value receiver, so both it and
// *valueMarshaler satisfy the interface even when stored in an interface{}.
type valueMarshaler struct {
	Field string
}

func (v valueMarshaler) MarshalPlist() (interface{}, error) {
	return map[string]string{"marshaled": v.Field}, nil
}

// marshaledRef is what valueMarshaler must encode to. If MarshalPlist is
// skipped the encoder falls back to reflecting over the struct and emits the
// exported field name instead, which is what these tests guard against.
const marshaledRef = `<dict><key>marshaled</key><string>x</string></dict>`

// TestMarshalerThroughInterface checks that a Marshaler is honored when it is
// reached through an interface{}. The interface's static type implements
// nothing, so the marshaler is only found by inspecting the dynamic type.
func TestMarshalerThroughInterface(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name string
		in   interface{}
		out  string
	}{
		{
			name: "struct field",
			in:   struct{ X interface{} }{X: valueMarshaler{Field: "x"}},
			out:  xmlDoc(`<dict><key>X</key>` + marshaledRef + `</dict>`),
		},
		{
			name: "slice element",
			in:   []interface{}{valueMarshaler{Field: "x"}},
			out:  xmlDoc(`<array>` + marshaledRef + `</array>`),
		},
		{
			name: "map value",
			in:   map[string]interface{}{"k": valueMarshaler{Field: "x"}},
			out:  xmlDoc(`<dict><key>k</key>` + marshaledRef + `</dict>`),
		},
		{
			name: "pointer in struct field",
			in:   struct{ X interface{} }{X: &valueMarshaler{Field: "x"}},
			out:  xmlDoc(`<dict><key>X</key>` + marshaledRef + `</dict>`),
		},
		{
			name: "pointer in map value",
			in:   map[string]interface{}{"k": &valueMarshaler{Field: "x"}},
			out:  xmlDoc(`<dict><key>k</key>` + marshaledRef + `</dict>`),
		},
	} {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			b, err := Marshal(tt.in)
			if err != nil {
				t.Fatal(err)
			}
			if string(b) != tt.out {
				t.Errorf("Marshal(%v) =\n%s\nwant\n%s", tt.in, b, tt.out)
			}
		})
	}
}

// ptrMarshaler implements Marshaler with a pointer receiver that dereferences
// it, so calling the method on a nil *ptrMarshaler panics.
type ptrMarshaler struct {
	Field string
}

func (p *ptrMarshaler) MarshalPlist() (interface{}, error) {
	return map[string]string{"marshaled": p.Field}, nil
}

// TestMarshalerThroughIndirection checks that a Marshaler is found however many
// layers of pointer and interface sit above it. Only looking once, before
// unwrapping, misses any type that becomes visible further down.
func TestMarshalerThroughNestedIndirection(t *testing.T) {
	t.Parallel()
	iface := interface{}(valueMarshaler{Field: "x"})
	ptr := &valueMarshaler{Field: "x"}

	for _, tt := range []struct {
		name string
		in   interface{}
		out  string
	}{
		{
			name: "pointer to interface",
			in:   struct{ P *interface{} }{P: &iface},
			out:  xmlDoc(`<dict><key>P</key>` + marshaledRef + `</dict>`),
		},
		{
			name: "pointer to interface in map",
			in:   map[string]interface{}{"k": &iface},
			out:  xmlDoc(`<dict><key>k</key>` + marshaledRef + `</dict>`),
		},
		{
			name: "pointer to pointer",
			in:   struct{ P **valueMarshaler }{P: &ptr},
			out:  xmlDoc(`<dict><key>P</key>` + marshaledRef + `</dict>`),
		},
		{
			name: "pointer to pointer in slice",
			in:   []interface{}{&ptr},
			out:  xmlDoc(`<array>` + marshaledRef + `</array>`),
		},
	} {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			b, err := Marshal(tt.in)
			if err != nil {
				t.Fatal(err)
			}
			if string(b) != tt.out {
				t.Errorf("Marshal(%v) =\n%s\nwant\n%s", tt.in, b, tt.out)
			}
		})
	}
}

// TestMarshalNilMarshaler checks that a nil pointer whose type implements
// Marshaler is omitted rather than having its method invoked. A nil *T still
// satisfies the interface, so calling through it would run the method against
// a nil receiver.
func TestMarshalNilMarshaler(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name string
		in   interface{}
		out  string
	}{
		{
			name: "value receiver, struct field",
			in:   struct{ P *valueMarshaler }{},
			out:  xmlDoc(`<dict></dict>`),
		},
		{
			name: "pointer receiver, struct field",
			in:   struct{ P *ptrMarshaler }{},
			out:  xmlDoc(`<dict></dict>`),
		},
		{
			name: "value receiver, map value",
			in:   map[string]interface{}{"k": (*valueMarshaler)(nil)},
			out:  xmlDoc(`<dict></dict>`),
		},
		{
			name: "pointer receiver, map value",
			in:   map[string]interface{}{"k": (*ptrMarshaler)(nil)},
			out:  xmlDoc(`<dict></dict>`),
		},
	} {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			b, err := Marshal(tt.in)
			if err != nil {
				t.Fatal(err)
			}
			if string(b) != tt.out {
				t.Errorf("Marshal(%v) =\n%s\nwant\n%s", tt.in, b, tt.out)
			}
		})
	}
}

// TestMarshalNilValue checks that nil pointers and nil interfaces are omitted
// from dictionaries rather than crashing the encoder. A property list has no
// null, so an absent key is the closest representation.
func TestMarshalNilValue(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name string
		in   interface{}
		out  string
	}{
		{
			name: "nil pointer struct field",
			in:   struct{ P *string }{},
			out:  xmlDoc(`<dict></dict>`),
		},
		{
			name: "nil interface struct field",
			in:   struct{ X interface{} }{},
			out:  xmlDoc(`<dict></dict>`),
		},
		{
			name: "nil alongside populated field",
			in: struct {
				P *string
				S string
			}{S: "here"},
			out: xmlDoc(`<dict><key>S</key><string>here</string></dict>`),
		},
		{
			name: "nil map value",
			in:   map[string]interface{}{"a": nil, "b": "x"},
			out:  xmlDoc(`<dict><key>b</key><string>x</string></dict>`),
		},
		{
			name: "typed nil pointer map value",
			in:   map[string]interface{}{"a": (*string)(nil), "b": "x"},
			out:  xmlDoc(`<dict><key>b</key><string>x</string></dict>`),
		},
	} {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			b, err := Marshal(tt.in)
			if err != nil {
				t.Fatal(err)
			}
			if string(b) != tt.out {
				t.Errorf("Marshal(%v) =\n%s\nwant\n%s", tt.in, b, tt.out)
			}
		})
	}
}

// TestMarshalNilArrayElement checks that a nil inside an array reports an
// error. A property list has no null, and unlike a dictionary key an array
// element cannot be left out without shifting every later index, so the
// encoder refuses rather than quietly returning a shorter array.
func TestMarshalNilArrayElement(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name string
		in   interface{}
	}{
		{"nil interface", []interface{}{nil}},
		{"typed nil pointer", []interface{}{(*string)(nil)}},
		{"nil after a value", []interface{}{"a", nil}},
		{"nil between values", []interface{}{"a", nil, "c"}},
		{"nil in a nested array", []interface{}{[]interface{}{nil}}},
		{"nil in an array under a key", map[string]interface{}{"k": []interface{}{nil}}},
	} {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if _, err := Marshal(tt.in); err == nil {
				t.Errorf("Marshal(%v) succeeded, want error", tt.in)
			}
		})
	}
}

// TestMarshalThroughIndirection checks that the encoder reaches the concrete
// value behind more than one layer of pointer and interface. Stopping after a
// single layer leaves a pointer the type switch cannot encode.
func TestMarshalThroughIndirection(t *testing.T) {
	t.Parallel()
	s := "x"
	ps := &s
	tm := time.Date(1900, 01, 01, 12, 00, 00, 0, time.UTC)

	for _, tt := range []struct {
		name string
		in   interface{}
		out  string
	}{
		{
			name: "pointer in interface",
			in:   map[string]interface{}{"k": &s},
			out:  xmlDoc(`<dict><key>k</key><string>x</string></dict>`),
		},
		{
			name: "pointer to pointer",
			in:   map[string]interface{}{"k": &ps},
			out:  xmlDoc(`<dict><key>k</key><string>x</string></dict>`),
		},
		{
			name: "time pointer in interface",
			in:   map[string]interface{}{"k": &tm},
			out:  xmlDoc(`<dict><key>k</key><date>1900-01-01T12:00:00Z</date></dict>`),
		},
		{
			name: "pointer in slice element",
			in:   []interface{}{&s},
			out:  xmlDoc(`<array><string>x</string></array>`),
		},
	} {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			b, err := Marshal(tt.in)
			if err != nil {
				t.Fatal(err)
			}
			if string(b) != tt.out {
				t.Errorf("Marshal(%v) =\n%s\nwant\n%s", tt.in, b, tt.out)
			}
		})
	}
}

// TestMarshalNilRoot checks that a nil with nothing around it reports an
// error. There is no key or element to drop, and an empty <plist> is not a
// document this package can read back.
func TestMarshalNilRoot(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name string
		in   interface{}
	}{
		{"untyped nil", nil},
		{"typed nil pointer", (*string)(nil)},
	} {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if _, err := Marshal(tt.in); err == nil {
				t.Errorf("Marshal(%v) succeeded, want error", tt.in)
			}
		})
	}
}

// marshalNoHang runs Marshal off the test goroutine so a regression that
// hangs (rather than erroring) fails the test instead of the whole suite.
func marshalNoHang(t *testing.T, in interface{}) error {
	t.Helper()
	errc := make(chan error, 1)
	go func() {
		_, err := Marshal(in)
		errc <- err
	}()
	select {
	case err := <-errc:
		return err
	case <-time.After(5 * time.Second):
		t.Fatal("Marshal hung on cyclic value, want error")
		return nil
	}
}

// TestMarshalCyclicValue checks that a self-referential pointer reports an
// error instead of hanging the encoder. A property list is a tree, so a
// value that never resolves has no representation.
//
// Note: this guards the pointer/interface descent only. Cycles back through
// containers (a map or slice containing itself) still recurse; that predates
// the descent loop and is left for a follow-up.
func TestMarshalCyclicValue(t *testing.T) {
	t.Parallel()

	var v interface{}
	v = &v
	err := marshalNoHang(t, v)
	var uve *UnsupportedValueError
	if !errors.As(err, &uve) {
		t.Errorf("Marshal(self-referential interface) err = %v (%T), want *UnsupportedValueError", err, err)
	}
}
