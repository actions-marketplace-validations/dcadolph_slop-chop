package sanitize

import (
	"fmt"
	"testing"

	"github.com/google/go-cmp/cmp"
)

// TestIgnoreDirectives checks that an inline directive silences its own line, the next-line
// directive silences the line after it, and other lines still report.
func TestIgnoreDirectives(t *testing.T) {
	t.Parallel()
	tests := []struct {
		In        string
		WantLines []int
	}{{ // Test 0: A same-line directive silences that line.
		In:        "a robust plan <!-- slop-chop-ignore -->\nanother robust plan",
		WantLines: []int{2},
	}, { // Test 1: A next-line directive silences the line after it, not itself.
		In:        "<!-- slop-chop-ignore-next-line -->\nrobust skipped\nrobust flagged",
		WantLines: []int{3},
	}, { // Test 2: With no directive every match reports.
		In:        "robust one\nrobust two",
		WantLines: []int{1, 2},
	}, { // Test 3: A next-line directive with content on its own line still flags that line.
		In:        "robust here <!-- slop-chop-ignore-next-line -->\nrobust skipped",
		WantLines: []int{1},
	}, { // Test 4: A next-line directive inside inline code silences nothing.
		In:        "Write `<!-- slop-chop-ignore-next-line -->` above it.\nrobust flagged",
		WantLines: []int{2},
	}, { // Test 5: A same-line directive inside inline code silences nothing.
		In:        "a robust plan `slop-chop-ignore`",
		WantLines: []int{1},
	}, { // Test 6: A directive in a fenced block silences nothing after the fence.
		In:        "```\n<!-- slop-chop-ignore-next-line -->\n```\nrobust flagged",
		WantLines: []int{4},
	}, { // Test 7: A real directive beside an unrelated code span still works.
		In:        "a robust plan with `code` <!-- slop-chop-ignore -->\nrobust flagged",
		WantLines: []int{2},
	}, { // Test 8: The next-line form quoted in code and the plain form in prose act as plain.
		In:        "robust `slop-chop-ignore-next-line` <!-- slop-chop-ignore -->\nrobust flagged",
		WantLines: []int{2},
	}, { // Test 9: The plain form quoted in code and the next-line form in prose act as next-line.
		In:        "robust `slop-chop-ignore` <!-- slop-chop-ignore-next-line -->\nrobust skipped",
		WantLines: []int{1},
	}, { // Test 10: An unclosed backtick does not hide a directive that follows it.
		In:        "a robust ` plan <!-- slop-chop-ignore -->\nrobust flagged",
		WantLines: []int{2},
	}}

	s := mustSanitizer(t)
	for testNum, test := range tests {
		t.Run(fmt.Sprintf("test %d", testNum), func(t *testing.T) {
			t.Parallel()
			var got []int
			for _, f := range s.Check(test.In) {
				got = append(got, f.Line)
			}
			if diff := cmp.Diff(test.WantLines, got); diff != "" {
				t.Errorf("flagged lines mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

// TestLineRanges checks the byte ranges returned for each line.
func TestLineRanges(t *testing.T) {
	t.Parallel()
	tests := []struct {
		In   string
		Want [][2]int
	}{{ // Test 0: A single line spans the whole text.
		In: "abc", Want: [][2]int{{0, 3}},
	}, { // Test 1: Two lines split on the newline, which is excluded.
		In: "ab\ncd", Want: [][2]int{{0, 2}, {3, 5}},
	}, { // Test 2: A trailing newline yields a final empty range.
		In: "ab\n", Want: [][2]int{{0, 2}, {3, 3}},
	}}

	for testNum, test := range tests {
		t.Run(fmt.Sprintf("test %d", testNum), func(t *testing.T) {
			t.Parallel()
			if diff := cmp.Diff(test.Want, lineRanges(test.In)); diff != "" {
				t.Errorf("mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

// TestIgnoreDirectiveWithLaterCode checks that an ignore directive still protects its line
// when a code block sits later in the document. This once failed because the protected
// ranges were concatenated unsorted, so overlapsAny short-circuited before reaching the
// ignored line.
func TestIgnoreDirectiveWithLaterCode(t *testing.T) {
	t.Parallel()
	s := mustSanitizer(t)
	in := "a robust plan <!-- slop-chop-ignore -->\n\n```\nx := 1\n```\n"
	if got := s.Check(in); len(got) != 0 {
		t.Errorf("ignored line flagged despite a later code block: %v", got)
	}
	if got, _ := s.Fix(in); got != in {
		t.Errorf("Fix changed protected text:\n got %q\nwant %q", got, in)
	}
}
