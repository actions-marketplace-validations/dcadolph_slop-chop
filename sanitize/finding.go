package sanitize

import (
	"cmp"
	"fmt"
	"slices"
)

// Finding records one place where a rule matched the text.
type Finding struct {
	// Rule is the name of the rule that matched.
	Rule string `json:"rule"`
	// Match is the exact substring that matched.
	Match string `json:"match"`
	// Replacement is what the match was or would be replaced with. It is nil for a
	// rule that only flags, which keeps that case distinct from a rule that rewrites
	// the match to an empty string.
	Replacement *string `json:"replacement,omitempty"`
	// Offset is the byte offset of the match in the text it was found in.
	Offset int `json:"offset"`
	// Line is the one-based line number of the match.
	Line int `json:"line"`
	// Col is the one-based column (rune count) within the line.
	Col int `json:"col"`
	// order is the index of the producing rule in the compiled rule order, which is the
	// order Fix applies rewrites. It is unexported, so it never serializes; dedupeFindings
	// uses it to report the rewrite Fix actually performs when two rules match one span.
	order int
}

// String renders the finding as a single CI-friendly line.
func (f Finding) String() string {
	if f.Replacement == nil {
		return fmt.Sprintf("%d:%d %s: %q", f.Line, f.Col, f.Rule, f.Match)
	}
	return fmt.Sprintf("%d:%d %s: %q -> %q", f.Line, f.Col, f.Rule, f.Match, *f.Replacement)
}

// Locate refills the Line and Col of each finding from its Offset in text. It is for a
// caller that checks a stand-in for the text and reports positions in the original, such
// as a comment mask, which keeps every byte offset but blanks a multibyte rune of code to
// several spaces. The findings must be in offset order, as Check returns them.
func Locate(text string, findings []Finding) error {
	byOffset := func(a, b Finding) int { return cmp.Compare(a.Offset, b.Offset) }
	if !slices.IsSortedFunc(findings, byOffset) {
		return fmt.Errorf("%w: findings out of offset order", ErrLocate)
	}
	if n := len(findings); n > 0 && (findings[0].Offset < 0 || findings[n-1].Offset > len(text)) {
		return fmt.Errorf("%w: offset outside %d bytes of text", ErrLocate, len(text))
	}
	assignLineCol(text, findings)
	return nil
}
