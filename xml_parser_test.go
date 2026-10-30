package plist

import (
	"bytes"
	"os"
	"os/exec"
	"strings"
	"testing"
)

func xmlPlist(rootValue string) []byte {
	return []byte(`<?xml version="1.0" encoding="UTF-8"?><plist version="1.0"><dict><key>Root</key>` + rootValue + `</dict></plist>`)
}

func nested(open, close, leaf string, depth int) string {
	return strings.Repeat(open, depth) + leaf + strings.Repeat(close, depth)
}

func wantErrContaining(t *testing.T, err error, substr string) {
	t.Helper()
	if err == nil || !strings.Contains(err.Error(), substr) {
		t.Fatalf("want error containing %q, got %v", substr, err)
	}
}

func TestXMLDepthLimit(t *testing.T) {
	const over = maxXMLDepth + 5
	for _, tc := range []struct {
		name string
		data []byte
	}{
		{"array", xmlPlist(nested("<array>", "</array>", "<true/>", over))},
		{"dict", xmlPlist(nested("<dict><key>a</key>", "</dict>", "<true/>", over))},
		{"array and dict", xmlPlist(nested("<array><dict><key>k</key>", "</dict></array>", "<true/>", over))},
		{"plist in plist", []byte(nested("<plist>", "</plist>", "<true/>", over))},
		// Tags never closed: the parser must stop on depth, not on EOF.
		{"unclosed array", []byte(`<?xml version="1.0"?><plist version="1.0">` + strings.Repeat("<array>", over))},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var v interface{}
			wantErrContaining(t, Unmarshal(tc.data, &v), "maximum depth")
		})
	}

	t.Run("struct target via NewXMLDecoder", func(t *testing.T) {
		var v struct {
			MessageType string
			Payload     interface{}
		}
		data := []byte(`<plist><dict><key>MessageType</key><string>Authenticate</string><key>Payload</key>` +
			nested("<array>", "</array>", "<true/>", over) + `</dict></plist>`)
		wantErrContaining(t, NewXMLDecoder(bytes.NewReader(data)).Decode(&v), "maximum depth")
	})

	// <plist>, the root <dict>, and the leaf take three levels.
	t.Run("at the limit decodes", func(t *testing.T) {
		var v interface{}
		if err := Unmarshal(xmlPlist(nested("<array>", "</array>", "<true/>", maxXMLDepth-3)), &v); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("one past the limit fails", func(t *testing.T) {
		var v interface{}
		wantErrContaining(t, Unmarshal(xmlPlist(nested("<array>", "</array>", "<true/>", maxXMLDepth-2)), &v), "maximum depth")
	})
}

// <plist>, the root <dict>, and the container take three nodes; <key> isn't counted.
func TestXMLNodeLimit(t *testing.T) {
	for _, tc := range []struct{ name, open, value, close string }{
		{"array", "<array>", "<true/>", "</array>"},
		{"dict", "<dict>", "<key>k</key><true/>", "</dict>"},
	} {
		t.Run(tc.name+" at the limit decodes", func(t *testing.T) {
			var v interface{}
			if err := Unmarshal(xmlPlist(tc.open+strings.Repeat(tc.value, maxXMLNodes-3)+tc.close), &v); err != nil {
				t.Fatal(err)
			}
		})
		t.Run(tc.name+" one past the limit fails", func(t *testing.T) {
			var v interface{}
			wantErrContaining(t, Unmarshal(xmlPlist(tc.open+strings.Repeat(tc.value, maxXMLNodes-2)+tc.close), &v), "maximum size")
		})
	}
}

// TestXMLDeepNesting feeds ~20 MiB of unclosed nested arrays. Without a depth limit this overflows
// the goroutine stack, which kills the process and can't be recovered, so the decode runs in a
// child test process and the parent reports how it exited.
func TestXMLDeepNesting(t *testing.T) {
	if os.Getenv("PLIST_DEEP_NESTING_CHILD") == "1" {
		data := []byte(`<?xml version="1.0"?><plist version="1.0">` + strings.Repeat("<array>", 3000000))
		var v interface{}
		wantErrContaining(t, Unmarshal(data, &v), "maximum depth")
		return
	}
	cmd := exec.Command(os.Args[0], "-test.run=^TestXMLDeepNesting$")
	cmd.Env = append(os.Environ(), "PLIST_DEEP_NESTING_CHILD=1")
	out, err := cmd.CombinedOutput()
	if err != nil {
		if len(out) > 2048 {
			out = out[:2048]
		}
		t.Fatalf("child process failed: %v\n%s", err, out)
	}
}
