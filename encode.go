package plist

import (
	"bytes"
	"fmt"
	"io"
	"reflect"
	"time"
)

type Marshaler interface {
	MarshalPlist() (interface{}, error)
}

// nilReceiver reports whether invoking a method on m would run it against a
// nil pointer receiver. Addr-derived receivers are never nil, so this only
// matters for values unboxed from an interface.
func nilReceiver(m Marshaler) bool {
	rv := reflect.ValueOf(m)
	return rv.Kind() == reflect.Ptr && rv.IsNil()
}

// isIndirect reports whether v wraps another value that can be reached with
// Elem: a pointer, or an empty interface. Non-empty interfaces are excluded
// because a value satisfying one may implement Marshaler itself.
func isIndirect(v reflect.Value) bool {
	return v.Kind() == reflect.Ptr || (v.Kind() == reflect.Interface && v.NumMethod() == 0)
}

// asMarshaler reports whether v, or a pointer to it, implements Marshaler.
//
// The check is made against the value's dynamic type rather than its static
// one, so that a Marshaler held in an interface{} — as a struct field, a slice
// element or a map value — is still found. Testing the static type would see
// only the empty interface, which implements nothing, and the value would be
// silently encoded by reflection instead.
func asMarshaler(v reflect.Value) (Marshaler, bool) {
	if v.CanInterface() {
		// Unboxing an interface can hand back a typed nil pointer, which
		// satisfies Marshaler but would run the method against a nil receiver.
		// Report no marshaler and let the caller's nil handling take over once
		// it has descended far enough to see the pointer.
		if m, ok := v.Interface().(Marshaler); ok && !nilReceiver(m) {
			return m, true
		}
	}
	if v.CanAddr() {
		if pv := v.Addr(); pv.CanInterface() {
			if m, ok := pv.Interface().(Marshaler); ok {
				return m, true
			}
		}
	}
	return nil, false
}

// Encoder ...
type Encoder struct {
	w io.Writer

	indent string
}

// Marshal ...
func Marshal(v interface{}) ([]byte, error) {
	var buf bytes.Buffer
	if err := NewEncoder(&buf).Encode(v); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// MarshalIndent ...
func MarshalIndent(v interface{}, indent string) ([]byte, error) {
	var buf bytes.Buffer
	enc := NewEncoder(&buf)
	enc.Indent(indent)
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// NewEncoder returns a new encoder that writes to w.
func NewEncoder(w io.Writer) *Encoder {
	return &Encoder{w: w}
}

// Encode ...
func (e *Encoder) Encode(v interface{}) error {
	pval, err := e.marshal(reflect.ValueOf(v))
	if err != nil {
		return err
	}
	// Nested nils are dropped by their container, but a nil root has no
	// container to drop it, and an empty <plist> is not a document this
	// package can read back.
	if pval == nil {
		return &UnsupportedValueError{reflect.ValueOf(v), "nil"}
	}

	enc := newXMLEncoder(e.w)
	enc.indent = e.indent
	enc.Indent("", e.indent)
	return enc.generateDocument(pval)
}

// Indent ...
func (e *Encoder) Indent(indent string) {
	e.indent = indent
}

// marshal converts v into a plistValue. A nil pointer or interface has no
// property list representation, so it yields a nil plistValue and no error;
// callers decide what to do with it.
func (e *Encoder) marshal(v reflect.Value) (*plistValue, error) {
	// A nil interface reaches us as the zero Value, which has no type to
	// inspect.
	if !v.IsValid() {
		return nil, nil
	}

	// Descend through empty interfaces and pointers to the concrete value they
	// hold, looking for a Marshaler at every level on the way down. One level
	// is not enough: a **T or an *interface{} only reveals the type that
	// implements Marshaler once it has been unwrapped, and a pointer left
	// unwrapped is something the switch below cannot encode.
	var seen map[uintptr]struct{}
	for {
		if isIndirect(v) && v.IsNil() {
			// Nothing to encode. Returning before the Marshaler check is
			// deliberate: a nil *T still satisfies the interface, so calling
			// MarshalPlist here would run the method against a nil receiver.
			// encoding/json short-circuits nil pointers the same way.
			return nil, nil
		}

		if m, ok := asMarshaler(v); ok {
			val, err := m.MarshalPlist()
			if err != nil {
				return nil, err
			}
			return e.marshal(reflect.ValueOf(val))
		}

		if !isIndirect(v) {
			break
		}
		if v.Kind() == reflect.Ptr {
			// A self-referential pointer (e.g. var v interface{}; v = &v)
			// never reaches a terminal value. A chain that revisits an
			// address is a cycle, so report it instead of looping forever.
			// An UnsupportedValueError matches encoding/json, which
			// reports "encountered a cycle via %s" the same way: the type
			// itself is encodable, the value is not.
			//
			// Note: this guards the indirection chain only. Cycles back
			// through containers (a map or slice containing itself) still
			// recurse; that predates this change and is left for a
			// follow-up.
			if ptr := v.Pointer(); ptr != 0 {
				if _, dup := seen[ptr]; dup {
					return nil, &UnsupportedValueError{v, fmt.Sprintf("encountered a cycle via %s", v.Type())}
				}
				if seen == nil {
					seen = make(map[uintptr]struct{})
				}
				seen[ptr] = struct{}{}
			}
		}
		v = v.Elem()
	}

	// check for time type
	if v.Type() == reflect.TypeOf((*time.Time)(nil)).Elem() {
		if date, ok := v.Interface().(time.Time); ok {
			return &plistValue{Date, date}, nil
		}
		return nil, &UnsupportedValueError{v, v.String()}
	}

	// check for UID type
	if v.Type() == reflect.TypeOf(UID(0)) {
		return &plistValue{CFUID, UID(v.Uint())}, nil
	}

	switch v.Kind() {
	case reflect.String:
		return &plistValue{String, v.String()}, nil
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return &plistValue{Integer, signedInt{uint64(v.Int()), true}}, nil
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return &plistValue{Integer, signedInt{uint64(v.Uint()), false}}, nil
	case reflect.Float32, reflect.Float64:
		return &plistValue{Real, sizedFloat{v.Float(), v.Type().Bits()}}, nil
	case reflect.Bool:
		return &plistValue{Boolean, v.Bool()}, nil
	case reflect.Slice, reflect.Array:
		return e.marshalArray(v)
	case reflect.Map:
		return e.marshalMap(v)
	case reflect.Struct:
		return e.marshalStruct(v)
	default:
		return nil, &UnsupportedTypeError{v.Type()}
	}
}

func (e *Encoder) marshalStruct(v reflect.Value) (*plistValue, error) {
	fields := cachedTypeFields(v.Type())
	dict := &dictionary{
		m: make(map[string]*plistValue, len(fields)),
	}
	for _, field := range fields {
		val := field.value(v)
		if field.omitEmpty && isEmptyValue(val) {
			continue
		}
		value, err := e.marshal(val)
		if err != nil {
			return nil, err
		}
		if value == nil {
			// A nil field has no property list representation; an absent key
			// is the closest equivalent.
			continue
		}
		dict.m[field.name] = value
	}
	return &plistValue{Dictionary, dict}, nil
}

func (e *Encoder) marshalArray(v reflect.Value) (*plistValue, error) {
	if v.Type().Elem().Kind() == reflect.Uint8 {
		bytes := []byte(nil)
		if v.CanAddr() {
			bytes = v.Slice(0, v.Len()).Bytes()
		} else {
			bytes = make([]byte, v.Len())
			reflect.Copy(reflect.ValueOf(bytes), v)
		}
		return &plistValue{Data, bytes}, nil
	}
	subvalues := make([]*plistValue, v.Len())
	for idx, length := 0, v.Len(); idx < length; idx++ {
		subpval, err := e.marshal(v.Index(idx))
		if err != nil {
			return nil, err
		}
		if subpval == nil {
			// A property list has no null. Unlike a dictionary key, an array
			// element cannot be left out without shifting every later index,
			// so report it rather than quietly returning a shorter array.
			return nil, &UnsupportedValueError{
				v.Index(idx),
				fmt.Sprintf("nil at array index %d", idx),
			}
		}
		subvalues[idx] = subpval
	}
	return &plistValue{Array, subvalues}, nil
}

func (e *Encoder) marshalMap(v reflect.Value) (*plistValue, error) {
	if v.Type().Key().Kind() != reflect.String {
		return nil, &UnsupportedTypeError{v.Type()}
	}

	l := v.Len()
	dict := &dictionary{
		m: make(map[string]*plistValue, l),
	}
	for _, keyv := range v.MapKeys() {
		subpval, err := e.marshal(v.MapIndex(keyv))
		if err != nil {
			return nil, err
		}
		if subpval != nil {
			dict.m[keyv.String()] = subpval
		}
	}
	return &plistValue{Dictionary, dict}, nil
}

// An UnsupportedTypeError is returned by Marshal when attempting
// to encode an unsupported value type.
type UnsupportedTypeError struct {
	Type reflect.Type
}

func (e *UnsupportedTypeError) Error() string {
	return "plist: unsupported type: " + e.Type.String()
}

// UnsupportedValueError ...
type UnsupportedValueError struct {
	Value reflect.Value
	Str   string
}

func (e *UnsupportedValueError) Error() string {
	return "plist: unsupported value: " + e.Str
}

func isEmptyValue(v reflect.Value) bool {
	switch v.Kind() {
	case reflect.Array, reflect.Map, reflect.Slice, reflect.String:
		return v.Len() == 0
	case reflect.Bool:
		return !v.Bool()
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return v.Int() == 0
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return v.Uint() == 0
	case reflect.Float32, reflect.Float64:
		return v.Float() == 0
	case reflect.Interface, reflect.Ptr:
		return v.IsNil()
	case reflect.Struct:
		return reflect.DeepEqual(v.Interface(), reflect.Zero(v.Type()).Interface())
	}
	return false
}
