package sanitize

import (
	"errors"
	"fmt"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
)

// TestLocate checks that Locate places findings by rune column in the text it is given and
// refuses offsets it cannot place.
func TestLocate(t *testing.T) {
	t.Parallel()
	tests := []struct {
		In      string
		Offsets []int
		WantPos [][2]int
		Want    error
	}{{ // Test 0: Multibyte runes before the match count one column each.
		In: `var s = "ééé" // We delve.`, Offsets: []int{23}, WantPos: [][2]int{{1, 21}},
	}, { // Test 1: A match on a later line counts from that line's start.
		In: "a\nbé delve", Offsets: []int{6}, WantPos: [][2]int{{2, 4}},
	}, { // Test 2: Several findings on one line are placed in one pass.
		In: "é a é b", Offsets: []int{3, 8}, WantPos: [][2]int{{1, 3}, {1, 7}},
	}, { // Test 3: An offset at the end of the text is placed after the last rune.
		In: "ab", Offsets: []int{2}, WantPos: [][2]int{{1, 3}},
	}, { // Test 4: No findings is not an error.
		In: "ab",
	}, { // Test 5: Findings out of offset order are refused.
		In: "abcd", Offsets: []int{2, 1}, Want: ErrLocate,
	}, { // Test 6: An offset past the end of the text is refused.
		In: "ab", Offsets: []int{3}, Want: ErrLocate,
	}, { // Test 7: A negative offset is refused.
		In: "ab", Offsets: []int{-1}, Want: ErrLocate,
	}}
	for testNum, test := range tests {
		t.Run(fmt.Sprintf("test %d", testNum), func(t *testing.T) {
			t.Parallel()
			findings := make([]Finding, len(test.Offsets))
			for i, off := range test.Offsets {
				findings[i] = Finding{Offset: off}
			}
			err := Locate(test.In, findings)
			if !errors.Is(err, test.Want) {
				t.Fatalf("err = %v, want %v", err, test.Want)
			}
			if err != nil {
				return
			}
			var got [][2]int
			for _, f := range findings {
				got = append(got, [2]int{f.Line, f.Col})
			}
			if diff := cmp.Diff(test.WantPos, got, cmpopts.EquateEmpty()); diff != "" {
				t.Errorf("mismatch (-want +got):\n%s", diff)
			}
		})
	}
}
