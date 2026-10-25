package sanitize

import "strings"

// ignoreToken silences the line it sits on. ignoreNextToken silences the line after it.
// Both usually sit inside an HTML comment so they read as a directive, not prose.
const (
	ignoreToken     = "slop-chop-ignore"
	ignoreNextToken = "slop-chop-ignore-next-line"
)

// skipRanges returns every byte range the rules must leave alone: protected code spans,
// Markdown structure and front matter, and lines silenced by an inline ignore directive. The
// sources are merged so the result is sorted and disjoint, which overlapsAny relies on to
// decide whether a match is protected.
func skipRanges(text string) [][2]int {
	code := codeRanges(text)
	ranges := append(append([][2]int{}, code...), ignoredRanges(text, code)...)
	ranges = append(ranges, structuralRanges(text)...)
	return mergeRanges(ranges)
}

// ignoredRanges returns the byte ranges of lines silenced by an ignore directive. A line
// holding the next-line directive silences the line after it, and a line holding the plain
// directive silences itself. code holds the protected code ranges, sorted and disjoint. A
// directive inside one is text about the directive rather than a directive, so it silences
// nothing.
func ignoredRanges(text string, code [][2]int) [][2]int {
	if !strings.Contains(text, ignoreToken) {
		return nil
	}
	lines := lineRanges(text)
	var ranges [][2]int
	for i, lr := range lines {
		next, self := directivesOn(text, lr, code)
		switch {
		case next:
			if i+1 < len(lines) {
				ranges = append(ranges, lines[i+1])
			}
		case self:
			ranges = append(ranges, lr)
		}
	}
	return ranges
}

// directivesOn reports which directives the line at lr carries outside code: next for the
// next-line form and self for the plain form. Each occurrence of the token is judged on its
// own, so a line quoting one form in a code span and using the other in prose acts on the one
// in prose. The next-line token starts with the plain token, so every occurrence is found by
// the plain token and then told apart by prefix.
func directivesOn(text string, lr [2]int, code [][2]int) (next, self bool) {
	for at := lr[0]; at < lr[1]; {
		idx := strings.Index(text[at:lr[1]], ignoreToken)
		if idx < 0 {
			break
		}
		start := at + idx
		isNext := strings.HasPrefix(text[start:lr[1]], ignoreNextToken)
		end := start + len(ignoreToken)
		if isNext {
			end = start + len(ignoreNextToken)
		}
		if !overlapsAny(code, start, end) {
			next = next || isNext
			self = self || !isNext
		}
		at = end
	}
	return next, self
}

// lineRanges returns the byte range of each line in text, the newline excluded. A trailing
// newline still yields a final empty range, which no match can overlap.
func lineRanges(text string) [][2]int {
	var ranges [][2]int
	start := 0
	for i := range len(text) {
		if text[i] == '\n' {
			ranges = append(ranges, [2]int{start, i})
			start = i + 1
		}
	}
	return append(ranges, [2]int{start, len(text)})
}
