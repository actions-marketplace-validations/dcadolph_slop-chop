package source

import (
	"fmt"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/dcadolph/slop-chop/sanitize"
)

// TestCheckComments checks that only comment text is read, cleanups are dropped, offsets
// point into the original text, and an unknown type reports not ok. Lines are checked after
// Locate, the way the command line reports them.
func TestCheckComments(t *testing.T) {
	t.Parallel()
	tests := []struct {
		As          string
		In          string
		WantMatches []string
		WantLine    int
		WantOK      bool
	}{{ // Test 0: A Go comment is read and its code is not.
		As: ".go", In: "x := robust\n// a robust plan\n", WantMatches: []string{"robust"},
		WantLine: 2, WantOK: true,
	}, { // Test 1: The extension works without its dot.
		As: "go", In: "// a robust plan\n", WantMatches: []string{"robust"}, WantLine: 1, WantOK: true,
	}, { // Test 2: A file name works for extensionless types.
		As: "Makefile", In: "# a robust plan\n", WantMatches: []string{"robust"}, WantLine: 1,
		WantOK: true,
	}, { // Test 3: A cleanup inside a comment is dropped.
		As: ".go", In: "// two  spaces here\n", WantMatches: nil, WantOK: true,
	}, { // Test 4: A type with no comment syntax is not ok.
		As: ".json", In: `{"a": "robust"}`, WantMatches: nil, WantOK: false,
	}, { // Test 5: An empty type is not ok.
		As: "", In: "// a robust plan\n", WantMatches: nil, WantOK: false,
	}}

	san, err := sanitize.New(sanitize.DefaultProfile())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	for testNum, test := range tests {
		t.Run(fmt.Sprintf("test %d", testNum), func(t *testing.T) {
			t.Parallel()
			findings, ok := CheckComments(san, test.As, test.In)
			if ok != test.WantOK {
				t.Fatalf("ok = %v, want %v", ok, test.WantOK)
			}
			if err := sanitize.Locate(test.In, findings); err != nil {
				t.Fatalf("Locate: %v", err)
			}
			var matches []string
			for _, f := range findings {
				matches = append(matches, f.Match)
				if got := test.In[f.Offset : f.Offset+len(f.Match)]; got != f.Match {
					t.Errorf("offset %d points at %q, want %q", f.Offset, got, f.Match)
				}
				if f.Line != test.WantLine {
					t.Errorf("line = %d, want %d", f.Line, test.WantLine)
				}
			}
			if diff := cmp.Diff(test.WantMatches, matches); diff != "" {
				t.Errorf("matches mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

// TestCheckCommentsNilPanics checks that a nil sanitizer is a developer error.
func TestCheckCommentsNilPanics(t *testing.T) {
	t.Parallel()
	defer func() {
		if recover() == nil {
			t.Error("CheckComments with a nil sanitizer did not panic")
		}
	}()
	_, _ = CheckComments(nil, ".go", "")
}
