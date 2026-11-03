package plist

import (
	"bytes"
	"runtime"
	"strings"
	"testing"
)

// BenchmarkXMLShapes decodes ~1 MiB XML plists made of one repeated shape.
// Besides -benchmem's B/op and allocs/op it reports tree-x: bytes the parsed
// *plistValue tree keeps live, as a multiple of the input size.
func BenchmarkXMLShapes(b *testing.B) {
	shapes := []struct{ name, leaf string }{
		{"true", "<true/>"},
		{"dict", "<dict/>"},
		{"array", "<array/>"},
		{"string", "<string>a</string>"},
		{"integer", "<integer>1</integer>"},
		{"real", "<real>1.5</real>"},
		{"data", "<data>AA==</data>"},
		{"date", "<date>2020-01-01T00:00:00Z</date>"},
		{"array-true", "<array><true/></array>"},
		{"dict-entry", "<dict><key>k</key><true/></dict>"},
	}
	const size = 1 << 20
	for _, s := range shapes {
		doc := []byte(xmlDoc("<array>" + strings.Repeat(s.leaf, size/len(s.leaf)) + "</array>"))
		b.Run(s.name, func(b *testing.B) {
			b.ReportAllocs()
			b.SetBytes(int64(len(doc)))
			for i := 0; i < b.N; i++ {
				var v interface{}
				if err := Unmarshal(doc, &v); err != nil {
					b.Fatal(err)
				}
			}
			b.ReportMetric(float64(treeBytes(b, doc))/float64(len(doc)), "tree-x")
		})
	}
}

// treeBytes returns the heap retained by the parsed tree for doc.
func treeBytes(b *testing.B, doc []byte) uint64 {
	var before, after runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&before)
	tree, err := newXMLParser(bytes.NewReader(doc)).parseDocument(nil)
	if err != nil {
		b.Fatal(err)
	}
	runtime.GC()
	runtime.ReadMemStats(&after)
	runtime.KeepAlive(tree)
	return after.HeapAlloc - before.HeapAlloc
}
