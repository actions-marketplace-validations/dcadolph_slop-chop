package sanitize

import (
	"strings"
	"unicode"
)

// Sentence length covers the procedure register, where a reader follows the text with
// their hands busy and a long sentence is a step they have to read twice. It is opt-in and
// weighs nothing by default: a long sentence is a house-style choice, not a model tell.
// The limits split a step, a sentence inside a list item, from a description, any other
// prose sentence, since a writing standard for procedures allows a description more room.

// lengthFindings reports every sentence longer than the limit for its kind, one finding per
// sentence. It runs on its own segmentation rather than sentenceSpans, which ends a
// sentence at the dot of a "1." marker or a file name.
func lengthFindings(text string, limit SentenceLength) []Finding {
	if limit.Step <= 0 && limit.Description <= 0 {
		return nil
	}
	masked := lengthMask(text)
	ignored := ignoredRanges(text, codeRanges(text))
	var out []Finding
	for _, blk := range lengthBlocks(masked) {
		rule, max := "length:description", limit.Description
		if blk.step {
			rule, max = "length:step", limit.Step
		}
		if max <= 0 {
			continue
		}
		for _, sp := range blockSentences(masked, blk.start, blk.end) {
			if countWords(masked[sp[0]:sp[1]]) <= max || overlapsAny(ignored, sp[0], sp[1]) {
				continue
			}
			out = append(out, Finding{
				Rule:   rule,
				Match:  text[sp[0]:sp[1]],
				Offset: sp[0],
				order:  anaphoraOrder,
			})
		}
	}
	return out
}

// lengthMask returns text with code and Markdown structure blanked, keeping every byte
// offset and newline. A one-line span such as inline code or a bare URL keeps one letter, so
// it still counts as a single word, while a link destination is blanked outright, since
// the reader sees only the link text.
func lengthMask(text string) string {
	b := []byte(text)
	blank := func(start, end int, mark bool) {
		mark = mark && !strings.Contains(text[start:end], "\n")
		for i := start; i < end; i++ {
			if b[i] != '\n' {
				b[i] = ' '
			}
		}
		if mark && start < end {
			b[start] = 'x'
		}
	}
	code := codeRanges(text)
	for _, r := range code {
		blank(r[0], r[1], true)
	}
	for _, r := range structuralRanges(text) {
		if !overlapsAny(code, r[0], r[1]) {
			blank(r[0], r[1], true)
		}
	}
	for _, loc := range inlineDestRe.FindAllStringSubmatchIndex(text, -1) {
		blank(loc[2], loc[3], false)
	}
	return string(b)
}

// lengthBlock is one Markdown block of prose: a run of lines joined by soft wraps.
type lengthBlock struct {
	// start is the offset of the block's first content byte, past any list marker.
	start int
	// end is the offset just past the block's last line.
	end int
	// step reports whether the block is a list item.
	step bool
}

// lengthBlocks splits masked text into prose blocks and marks the list items among them.
// Headings, table rows, and HTML lines are not prose and are left out.
func lengthBlocks(masked string) []lengthBlock {
	var out []lengthBlock
	lines := lineRanges(masked)
	for i := 0; i < len(lines); i++ {
		first := lines[i]
		last := i
		for last+1 < len(lines) && softWrap(masked, lines[last][1]) {
			last++
		}
		i = last
		start := first[0]
		for start < first[1] && (masked[start] == ' ' || masked[start] == '\t') {
			start++
		}
		if start == first[1] {
			continue
		}
		switch masked[start] {
		case '#', '|', '<':
			if !isListMarker(firstField(masked[start:first[1]])) {
				continue
			}
		}
		blk := lengthBlock{start: start, end: lines[last][1]}
		if marker := firstField(masked[start:first[1]]); isListMarker(marker) {
			blk.step = true
			blk.start = skipTaskBox(masked, skipBlanks(masked, start+len(marker)))
		}
		out = append(out, blk)
	}
	return out
}

// firstField returns line up to its first space or tab.
func firstField(line string) string {
	if n := strings.IndexAny(line, " \t"); n >= 0 {
		return line[:n]
	}
	return line
}

// skipBlanks returns the offset of the first byte at or after i that is not a space or tab.
func skipBlanks(text string, i int) int {
	for i < len(text) && (text[i] == ' ' || text[i] == '\t') {
		i++
	}
	return i
}

// skipTaskBox returns the offset past a task-list checkbox at i, such as "[ ]" or "[x]",
// so the box is not counted as a word of the step.
func skipTaskBox(text string, i int) int {
	if i+3 <= len(text) && text[i] == '[' && text[i+2] == ']' &&
		strings.ContainsRune(" xX", rune(text[i+1])) {
		return skipBlanks(text, i+3)
	}
	return i
}

// blockSentences returns the byte range of each sentence in masked[start:end]. A sentence
// ends at sentence punctuation that is not an abbreviation and that is followed by a space,
// a closing mark, or the end of the block, so a dot inside a word never ends one.
func blockSentences(masked string, start, end int) [][2]int {
	var out [][2]int
	i := skipSpace(masked, start, end)
	from := i
	for i < end {
		c := masked[i]
		if c != '.' && c != '!' && c != '?' || c == '.' && abbreviationEnd(masked, i) {
			i++
			continue
		}
		j := i
		for j < end && strings.IndexByte(".!?", masked[j]) >= 0 {
			j++
		}
		for j < end && strings.IndexByte(`"')]*_`, masked[j]) >= 0 {
			j++
		}
		if j < end && !unicode.IsSpace(rune(masked[j])) {
			i = j
			continue
		}
		out = append(out, [2]int{from, j})
		i = skipSpace(masked, j, end)
		from = i
	}
	if tail := strings.TrimRightFunc(masked[from:end], unicode.IsSpace); tail != "" {
		out = append(out, [2]int{from, from + len(tail)})
	}
	return out
}

// skipSpace returns the offset of the first byte in text[i:end] that is not white space, or
// end when there is none.
func skipSpace(text string, i, end int) int {
	for i < end && unicode.IsSpace(rune(text[i])) {
		i++
	}
	return i
}

// lengthRule reports whether rule is a sentence length rule.
func lengthRule(rule string) bool {
	return strings.HasPrefix(rule, "length:")
}

// countWords counts the words in s, where a word is a run of non-space bytes holding at
// least one letter or digit, so a stray dash or parenthesis is not a word.
func countWords(s string) int {
	n := 0
	for _, field := range strings.Fields(s) {
		if strings.IndexFunc(field, isWordRune) >= 0 {
			n++
		}
	}
	return n
}

// isWordRune reports whether r is a letter or a digit.
func isWordRune(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r)
}
