package plist

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"reflect"
	"time"
)

// MarshalFunc is a function used to Unmarshal custom plist types.
type MarshalFunc func(interface{}) error

// Unmarshaler is the interface implemented by types that can unmarshal
// themselves from property list objects. The UnmarshalPlist method
// receives a function that may be called to unmarshal the original
// property list value into a field or variable.
//
// It is safe to call the unmarshal function more than once.
type Unmarshaler interface {
	UnmarshalPlist(f func(interface{}) error) error
}

// Unmarshal parses the plist-encoded data and stores the result in the value pointed to by v.
func Unmarshal(data []byte, v interface{}) error {
	// Check for binary plist here before setting up the decoder.
	if bytes.HasPrefix(data, []byte("bplist0")) {
		return NewBinaryDecoder(bytes.NewReader(data)).Decode(v)
	}
	return NewXMLDecoder(bytes.NewReader(data)).Decode(v)
}

// A Decoder reads and decodes Apple plist objects from an input stream.
// The plists can be in XML or binary format.
type Decoder struct {
	reader   io.Reader // binary decoders assert this to io.ReadSeeker
	isBinary bool      // true if this is a binary plist
}

// NewDecoder returns a new XML plist decoder.
// DEPRECATED: Please use NewXMLDecoder instead.
func NewDecoder(r io.Reader) *Decoder {
	return NewXMLDecoder(r)
}

// NewXMLDecoder returns a new decoder that reads an XML plist from r.
func NewXMLDecoder(r io.Reader) *Decoder {
	return &Decoder{reader: r, isBinary: false}
}

// NewBinaryDecoder returns a new decoder that reads a binary plist from r.
// No error checking is done to make sure that r is actually a binary plist.
func NewBinaryDecoder(r io.ReadSeeker) *Decoder {
	return &Decoder{reader: r, isBinary: true}
}

// Decode reads the next plist-encoded value from its input and stores it in
// the value pointed to by v.  Decode uses xml.Decoder to do the heavy lifting
// for XML plists, and uses binaryParser for binary plists.
func (d *Decoder) Decode(v interface{}) error {
	val := reflect.ValueOf(v)
	if val.Kind() != reflect.Ptr {
		return errors.New("plist: non-pointer passed to Unmarshal")
	}
	var pval *plistValue
	if d.isBinary {
		// For binary decoder, type assert the reader to an io.ReadSeeker
		var err error
		r, ok := d.reader.(io.ReadSeeker)
		if !ok {
			return fmt.Errorf("binary plist decoder requires an io.ReadSeeker")
		}
		parser, err := newBinaryParser(r)
		if err != nil {
			return err
		}
		pval, err = parser.parseDocument()
		if err != nil {
			return err
		}
	} else {
		var err error
		parser := newXMLParser(d.reader)
		pval, err = parser.parseDocument(nil)
		if err != nil {
			return err
		}
	}
	return d.unmarshal(pval, val.Elem())
}

func (d *Decoder) unmarshal(pval *plistValue, v reflect.Value) error {
	if v.Kind() == reflect.Ptr {
		if v.IsNil() {
			v.Set(reflect.New(v.Type().Elem()))
		}
		v = v.Elem()
	}

	// check for empty interface v type
	if v.Kind() == reflect.Interface && v.NumMethod() == 0 {
		val := reflect.ValueOf(d.valueInterface(pval))
		if !val.IsValid() {
			return fmt.Errorf("plist: invalid reflect.Value %v", v)
		}
		v.Set(val)
		return nil
	}

	unmarshalerType := reflect.TypeOf((*Unmarshaler)(nil)).Elem()

	if v.CanInterface() && v.Type().Implements(unmarshalerType) {
		u := v.Interface().(Unmarshaler)
		return u.UnmarshalPlist(func(i interface{}) error {
			return d.unmarshal(pval, reflect.ValueOf(i))
		})
	}

	if v.CanAddr() {
		pv := v.Addr()
		if pv.CanInterface() && pv.Type().Implements(unmarshalerType) {
			u := pv.Interface().(Unmarshaler)
			return u.UnmarshalPlist(func(i interface{}) error {
				return d.unmarshal(pval, reflect.ValueOf(i))
			})
		}

	}

	// change pointer values to the correct type
	// ex type foo struct {
	// 			  Foo *string `plist:"foo"
	//		   }
	if v.Kind() == reflect.Ptr {
		if v.IsNil() {
			v.Set(reflect.New(v.Type().Elem()))
		}
		v = v.Elem()

	}

	switch pval.kind {
	case String:
		return d.unmarshalString(pval, v)
	case Dictionary:
		return d.unmarshalDictionary(pval, v)
	case Array:
		return d.unmarshalArray(pval, v)
	case Boolean:
		return d.unmarshalBoolean(pval, v)
	case Real:
		return d.unmarshalReal(pval, v)
	case Integer:
		return d.unmarshalInteger(pval, v)
	case Data:
		return d.unmarshalData(pval, v)
	case Date:
		return d.unmarshalDate(pval, v)
	case CFUID:
		return d.unmarshalUID(pval, v)
	default:
		return fmt.Errorf("plist: %v is an unsuported plist element kind", pval.kind)
	}
}

func (d *Decoder) unmarshalDate(pval *plistValue, v reflect.Value) error {
	if v.Type() != reflect.TypeOf((*time.Time)(nil)).Elem() {
		return UnmarshalTypeError{fmt.Sprintf("%v", pval.value), v.Type()}
	}
	v.Set(reflect.ValueOf(pval.value.(time.Time)))
	return nil
}

func (d *Decoder) unmarshalUID(pval *plistValue, v reflect.Value) error {
	uidType := reflect.TypeOf(UID(0))
	if v.Type() == uidType {
		v.SetUint(uint64(pval.value.(UID)))
		return nil
	}
	switch v.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		v.SetInt(int64(pval.value.(UID)))
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		v.SetUint(uint64(pval.value.(UID)))
	default:
		return UnmarshalTypeError{fmt.Sprintf("UID(%d)", pval.value.(UID)), v.Type()}
	}
	return nil
}

func (d *Decoder) unmarshalData(pval *plistValue, v reflect.Value) error {
	if v.Kind() != reflect.Slice || v.Type().Elem().Kind() != reflect.Uint8 {
		return UnmarshalTypeError{fmt.Sprintf("%s", pval.value.([]byte)), v.Type()}
	}
	v.SetBytes(pval.value.([]byte))
	return nil
}

func (d *Decoder) unmarshalReal(pval *plistValue, v reflect.Value) error {
	if v.Kind() != reflect.Float32 && v.Kind() != reflect.Float64 {
		return UnmarshalTypeError{fmt.Sprintf("%v", pval.value.(sizedFloat).value), v.Type()}
	}
	v.SetFloat(pval.value.(sizedFloat).value)
	return nil
}

func (d *Decoder) unmarshalBoolean(pval *plistValue, v reflect.Value) error {
	if v.Kind() != reflect.Bool {
		return UnmarshalTypeError{fmt.Sprintf("%v", pval.value), v.Type()}
	}
	v.SetBool(pval.value.(bool))
	return nil
}

func (d *Decoder) unmarshalDictionary(pval *plistValue, v reflect.Value) error {
	subvalues := pval.value.(*dictionary).m
	switch v.Kind() {
	case reflect.Struct:
		fields := cachedTypeFields(v.Type())
		for _, field := range fields {
			if _, ok := subvalues[field.name]; !ok {
				continue
			}
			if err := d.unmarshal(subvalues[field.name], field.value(v)); err != nil {
				return err
			}
		}
	case reflect.Map:
		if v.IsNil() {
			v.Set(reflect.MakeMap(v.Type()))
		}
		for k, sval := range subvalues {
			keyv := reflect.ValueOf(k).Convert(v.Type().Key())
			mapElem := v.MapIndex(keyv)
			if !mapElem.IsValid() {
				mapElem = reflect.New(v.Type().Elem()).Elem()
			}
			if err := d.unmarshal(sval, mapElem); err != nil {
				return err
			}
			v.SetMapIndex(keyv, mapElem)
		}
	default:
		return UnmarshalTypeError{"dict", v.Type()}
	}
	return nil
}

func (d *Decoder) unmarshalString(pval *plistValue, v reflect.Value) error {
	if v.Kind() != reflect.String {
		return UnmarshalTypeError{fmt.Sprintf("%s", pval.value.(string)), v.Type()}
	}
	v.SetString(pval.value.(string))
	return nil
}

func (d *Decoder) unmarshalArray(pval *plistValue, v reflect.Value) error {
	subvalues := pval.value.([]*plistValue)
	if v.Kind() != reflect.Slice {
		return UnmarshalTypeError{"array", v.Type()}
	}

	// Decoding replaces the destination's contents, so the result holds
	// exactly the document's elements regardless of what was there before.
	// Reuse the existing backing array when it is already big enough.
	cnt := len(subvalues)
	reused := true
	if cnt > v.Cap() {
		ncap := v.Cap()
		for ncap < cnt {
			ncap = growSliceCap(ncap)
		}
		v.Set(reflect.MakeSlice(v.Type(), 0, ncap))
		reused = false
	}
	v.SetLen(cnt)

	zero := reflect.Zero(v.Type().Elem())
	for i, sval := range subvalues {
		if reused {
			// Clear whatever the caller left behind. Unmarshaling assigns only
			// the keys present in the document, so a reused element would
			// otherwise merge its stale fields into the result.
			v.Index(i).Set(zero)
		}
		if err := d.unmarshal(sval, v.Index(i)); err != nil {
			return err
		}
	}
	return nil
}

// growSliceCap returns the next capacity to try when growing a slice to hold a
// decoded array, following the same shape as the runtime's append: double while
// small, then taper off.
func growSliceCap(cap int) int {
	switch {
	case cap == 0:
		return 4
	case cap < 1024:
		return cap * 2
	default:
		return cap + cap/4
	}
}

func (d *Decoder) unmarshalInteger(pval *plistValue, v reflect.Value) error {
	switch v.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		v.SetInt(int64(pval.value.(signedInt).value))
	case reflect.Uint, reflect.Uint8, reflect.Uint16,
		reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		// Make sure plistValue isn't negative when decoding into uint.
		if pval.value.(signedInt).signed {
			return UnmarshalTypeError{
				fmt.Sprintf("%v", int64(pval.value.(signedInt).value)), v.Type()}
		}
		v.SetUint(pval.value.(signedInt).value)
	default:
		return UnmarshalTypeError{
			fmt.Sprintf("%v", pval.value.(signedInt).value), v.Type()}
	}
	return nil
}

// empty interface values
// borrowed from go-plist
func (d *Decoder) valueInterface(pval *plistValue) interface{} {
	switch pval.kind {
	case String:
		return pval.value.(string)
	case Integer:
		if pval.value.(signedInt).signed {
			return int64(pval.value.(signedInt).value)
		}
		return pval.value.(signedInt).value
	case Real:
		bits := pval.value.(sizedFloat).bits
		switch bits {
		case 32:
			return float32(pval.value.(sizedFloat).value)
		case 64:
			return pval.value.(sizedFloat).value
		default:
			return nil
		}
	case Boolean:
		return pval.value.(bool)
	case Array:
		return d.arrayInterface(pval.value.([]*plistValue))
	case Dictionary:
		return d.dictionaryInterface(pval.value.(*dictionary))
	case Data:
		return pval.value.([]byte)
	case Date:
		return pval.value.(time.Time)
	case CFUID:
		return pval.value.(UID)
	default:
		return nil
	}
}

func (d *Decoder) arrayInterface(subvalues []*plistValue) []interface{} {
	out := make([]interface{}, len(subvalues))
	for i, subv := range subvalues {
		out[i] = d.valueInterface(subv)
	}
	return out
}

func (d *Decoder) dictionaryInterface(dict *dictionary) map[string]interface{} {
	out := make(map[string]interface{})
	for k, subv := range dict.m {
		out[k] = d.valueInterface(subv)
	}
	return out
}

// An UnmarshalTypeError describes a plist value that was
// not appropriate for a value of a specific Go type.
type UnmarshalTypeError struct {
	Value string // description of plist value - "true", "string", "date"
	Type  reflect.Type
}

func (e UnmarshalTypeError) Error() string {
	return "plist: cannot unmarshal " + e.Value + " into Go value of type " + e.Type.String()
}
