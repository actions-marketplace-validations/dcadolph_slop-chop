package source

import (
	"fmt"
	"testing"
)

// TestIs checks the gate that keeps prose rules away from code and data files.
func TestIs(t *testing.T) {
	t.Parallel()
	tests := []struct {
		In         string
		WantResult bool
	}{{ // Test 0: Go source is code.
		In: "main.go", WantResult: true,
	}, { // Test 1: extension match is case-insensitive.
		In: "dir/APP.PY", WantResult: true,
	}, { // Test 2: a Makefile has no extension but is code.
		In: "sub/Makefile", WantResult: true,
	}, { // Test 3: YAML is machine-read data.
		In: "config.yaml", WantResult: true,
	}, { // Test 4: Markdown is prose.
		In: "README.md", WantResult: false,
	}, { // Test 5: plain text is prose.
		In: "notes.txt", WantResult: false,
	}, { // Test 6: an extensionless prose file passes.
		In: "README", WantResult: false,
	}}
	for testNum, test := range tests {
		t.Run(fmt.Sprintf("test %d", testNum), func(t *testing.T) {
			t.Parallel()
			if got := Is(test.In); got != test.WantResult {
				t.Errorf("Is(%q) = %v, want %v", test.In, got, test.WantResult)
			}
		})
	}
}
