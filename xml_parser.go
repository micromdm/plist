package plist

import (
	"bytes"
	"encoding/base64"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"
)

// maxXMLNodes bounds the TOTAL number of XML nodes allowed in a plist document.
var maxXMLNodes = 1 << 21 // roughly 2.1 million nodes

// maxXMLDepth bounds the maximum depth of the XML element tree to prevent stack exhaustion.
const maxXMLDepth = 129 // 128 but we include <plist>

// replacer is used to remove all whitespace characters from a string in a <data> element.
// package level as we don't need to re-instantiate it for every <data> element.
var replacer = strings.NewReplacer("\t", "", "\n", "", " ", "", "\r", "")

// xmlParser uses xml.Decoder to parse an xml plist into the corresponding plistValues
type xmlParser struct {
	depth int // current depth of the XML element tree
	nodes int // total number of XML nodes encountered so far
	*xml.Decoder
}

func (p *xmlParser) Token() (xml.Token, error) {
	for {
		token, err := p.Decoder.Token()
		if err != nil {
			return nil, err
		}
		if _, ok := token.(xml.Comment); ok {
			continue
		}
		return token, nil
	}
}

// newXMLParser returns a new xmlParser
func newXMLParser(r io.Reader) *xmlParser {
	return &xmlParser{Decoder: xml.NewDecoder(r)}
}

func (p *xmlParser) parseDocument(start *xml.StartElement) (*plistValue, error) {
	if start != nil {
		return p.parseXMLElement(start)
	}

	for {
		tok, err := p.Token()
		if err != nil {
			return nil, err
		}
		switch el := tok.(type) {
		case xml.StartElement:
			return p.parseXMLElement(&el)
		case xml.ProcInst, xml.Directive:
			continue
		case xml.CharData:
			if len(bytes.TrimSpace(el)) != 0 {
				return nil, errors.New("plist: unexpected non-empty xml.CharData")
			}
		default:
			return nil, fmt.Errorf("unexpected element: %T", el)
		}
	}
}

// Shared values for leaves that carry no data. The decoder only reads the
// parsed tree, so these never need their own allocation.
// To avoid allocating a new empty object, but rather re-use
var (
	xmlTrue        = &plistValue{Boolean, true}
	xmlFalse       = &plistValue{Boolean, false}
	xmlEmptyString = &plistValue{String, ""}
	xmlEmptyArray  = &plistValue{Array, []*plistValue(nil)}
	xmlEmptyDict   = &plistValue{Dictionary, &dictionary{}}
)

func (p *xmlParser) parseXMLElement(element *xml.StartElement) (*plistValue, error) {
	// verify total object nodes and depth before parsing this element
	p.nodes++
	if p.nodes > maxXMLNodes {
		return nil, fmt.Errorf("plist: object graph exceeds maximum size (%d nodes)", maxXMLNodes)
	}

	if p.depth >= maxXMLDepth {
		return nil, fmt.Errorf("plist: object nesting exceeds maximum depth %d", maxXMLDepth)
	}
	p.depth++
	defer func() {
		p.depth--
	}()

	switch element.Name.Local {
	case "plist":
		return p.parsePlist(element)
	case "dict":
		return p.parseDict(element)
	case "string":
		return p.parseString(element)
	case "true", "false":
		return p.parseBoolean(element)
	case "array":
		return p.parseArray(element)
	case "real":
		return p.parseReal(element)
	case "integer":
		return p.parseInteger(element)
	case "data":
		return p.parseData(element)
	case "date":
		return p.parseDate(element)
	default:
		return nil, fmt.Errorf("plist: Unknown plist element %s", element.Name.Local)
	}
}

func (p *xmlParser) parsePlist(element *xml.StartElement) (*plistValue, error) {
	var val *plistValue
	for {
		token, err := p.Token()
		if err != nil {
			return nil, err
		}
		switch el := token.(type) {
		case xml.EndElement:
			if val == nil {
				return nil, errors.New("plist: empty plist tag")
			}
			return val, nil
		case xml.StartElement:
			v, err := p.parseXMLElement(&el)
			if err != nil {
				return v, err
			}
			val = v
		case xml.CharData:
			if len(bytes.TrimSpace(el)) != 0 {
				return nil, errors.New("plist: unexpected non-empty xml.CharData")
			}
		default:
			return nil, fmt.Errorf("unexpected element: %T", el)
		}
	}
}

func (p *xmlParser) parseDict(element *xml.StartElement) (*plistValue, error) {
	var key *string
	// Parallel slices rather than a map: a map costs ~250 bytes even with one
	// entry, which a small dict in the input costs ~30 bytes to produce.
	var keys []string
	var values []*plistValue
	for {
		token, err := p.Token()
		if err != nil {
			return nil, err
		}
		switch el := token.(type) {
		case xml.EndElement:
			if len(keys) == 0 {
				return xmlEmptyDict, nil
			}
			dict := &dictionary{keys: keys, values: values}
			// A UID is a dict whose only key is CF$UID, so it must come first;
			// skip building the map for every other dict.
			if keys[0] == "CF$UID" {
				if m := dict.lookup(); len(m) == 1 {
					if uidVal, ok := m["CF$UID"]; ok && uidVal.kind == Integer {
						return &plistValue{CFUID, UID(uidVal.value.(signedInt).value)}, nil
					}
				}
			}
			return &plistValue{Dictionary, dict}, nil
		case xml.StartElement:
			if el.Name.Local == "key" {
				var k string
				if err := p.DecodeElement(&k, &el); err != nil {
					return nil, err
				}
				key = &k
				continue
			}
			if key == nil {
				return nil, errors.New("plist: missing key in dict")
			}
			v, err := p.parseXMLElement(&el)
			if err != nil {
				return nil, err
			}
			keys = append(keys, *key)
			values = append(values, v)
			key = nil
		case xml.CharData:
			if len(bytes.TrimSpace(el)) != 0 {
				return nil, errors.New("plist: unexpected non-empty xml.CharData")
			}
		default:
			return nil, fmt.Errorf("unexpected element: %T", el)
		}
	}
}

func (p *xmlParser) parseString(element *xml.StartElement) (*plistValue, error) {
	var value string
	if err := p.DecodeElement(&value, element); err != nil {
		return nil, err
	}
	if value == "" {
		return xmlEmptyString, nil
	}
	return &plistValue{String, value}, nil
}

func (p *xmlParser) parseBoolean(element *xml.StartElement) (*plistValue, error) {
	if err := p.Skip(); err != nil {
		return nil, err
	}
	if element.Name.Local == "true" {
		return xmlTrue, nil
	}
	return xmlFalse, nil
}

func (p *xmlParser) parseArray(element *xml.StartElement) (*plistValue, error) {
	var subvalues []*plistValue
	for {
		token, err := p.Token()
		if err != nil {
			return nil, err
		}
		switch el := token.(type) {
		case xml.EndElement:
			if subvalues == nil {
				return xmlEmptyArray, nil
			}
			return &plistValue{Array, subvalues}, nil
		case xml.StartElement:
			subv, err := p.parseXMLElement(&el)
			if err != nil {
				return nil, err
			}
			subvalues = append(subvalues, subv)
		case xml.CharData:
			if len(bytes.TrimSpace(el)) != 0 {
				return nil, errors.New("plist: unexpected non-empty xml.CharData")
			}
		default:
			return nil, fmt.Errorf("unexpected element: %T", el)
		}
	}
}

func (p *xmlParser) parseReal(element *xml.StartElement) (*plistValue, error) {
	var n float64
	if err := p.DecodeElement(&n, element); err != nil {
		return nil, err
	}
	return &plistValue{Real, sizedFloat{n, 64}}, nil
}

func (p *xmlParser) parseInteger(element *xml.StartElement) (*plistValue, error) {
	// Based on testing with plutil -lint, the largest positive integer
	// that you can store in an XML plist is 2^64 - 1 (in a uint64)
	// and the largest negative integer you can store is -2^63 (in an int64)
	// Since we need to know the sign before we can know what integer type
	// to decode into, first decode into a string to check for "-".
	var s string
	if err := p.DecodeElement(&s, element); err != nil {
		return nil, err
	}
	// Determine if this is a negative number by checking for minus sign.
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, "-") {
		i, err := strconv.ParseInt(s, 10, 64)
		if err != nil {
			return nil, err
		}
		return &plistValue{Integer, signedInt{uint64(i), true}}, nil
	}
	// Otherwise assume positive number and put into uint64.
	u, err := strconv.ParseUint(s, 10, 64)
	if err != nil {
		return nil, err
	}
	return &plistValue{Integer, signedInt{u, false}}, nil
}

func (p *xmlParser) parseData(element *xml.StartElement) (*plistValue, error) {
	var data []byte
	if err := p.DecodeElement(&data, element); err != nil {
		return nil, err
	}
	if len(data) == 0 {
		return &plistValue{Data, []byte(nil)}, nil
	}
	str := replacer.Replace(string(data))
	decoded, err := base64.StdEncoding.DecodeString(str)
	if err != nil {
		return nil, err
	}
	data = []byte(decoded)
	return &plistValue{Data, data}, nil
}

func (p *xmlParser) parseDate(element *xml.StartElement) (*plistValue, error) {
	var date time.Time
	if err := p.DecodeElement(&date, element); err != nil {
		return nil, err
	}
	return &plistValue{Date, date}, nil
}
