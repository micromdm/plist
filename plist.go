package plist

import "sort"

type plistKind uint

const (
	Invalid plistKind = iota
	Dictionary
	Array
	String
	Integer
	Real
	Boolean
	Data
	Date
	CFUID
)

type UID uint64

var plistKindNames = map[plistKind]string{
	Invalid:    "invalid",
	Dictionary: "dictionary",
	Array:      "array",
	String:     "string",
	Integer:    "integer",
	Real:       "real",
	Boolean:    "boolean",
	Data:       "data",
	Date:       "date",
	CFUID:      "uid",
}

type plistValue struct {
	kind  plistKind
	value interface{}
}

type signedInt struct {
	value  uint64
	signed bool
}

type sizedFloat struct {
	value float64
	bits  int
}

type dictionary struct {
	count  int
	m      map[string]*plistValue
	keys   sort.StringSlice
	values []*plistValue
}

// lookup returns the dict as a map. Parsed dicts hold only keys and values,
// so the map is built on demand and not retained.
func (d *dictionary) lookup() map[string]*plistValue {
	if d.m != nil || len(d.keys) == 0 {
		return d.m
	}
	m := make(map[string]*plistValue, len(d.keys))
	for i, k := range d.keys {
		m[k] = d.values[i] // later duplicates win, as with the old map
	}
	return m
}

func (d *dictionary) Len() int {
	return len(d.lookup())
}

func (d *dictionary) Less(i, j int) bool {
	return d.keys.Less(i, j)
}

func (d *dictionary) Swap(i, j int) {
	d.keys.Swap(i, j)
	d.values[i], d.values[j] = d.values[j], d.values[i]
}

func (d *dictionary) populateArrays() {
	d.keys = make([]string, len(d.m))
	d.values = make([]*plistValue, len(d.m))
	i := 0
	for k, v := range d.m {
		d.keys[i] = k
		d.values[i] = v
		i++
	}
	sort.Sort(d)
}
