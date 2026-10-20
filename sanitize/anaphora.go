package sanitize

import "strings"

// Anaphora detection covers the repeated-opener drumbeat, "We needed retries. We needed
// observability. We needed ownership.", a signature cadence of model prose. A regular
// expression cannot express "the same two words again", so this walks sentences directly.

// anaphoraMaxWords is the longest sentence that still counts toward an anaphora run. The
// tell is a run of short, punchy sentences; long parallel sentences are the ordinary
// register of legal and technical writing.
const anaphoraMaxWords = 12

// anaphoraOrder places anaphora findings after every compiled rule in the dedupe order.
// The finding only flags, so the position never affects which rewrite is reported.
const anaphoraOrder = 1 << 30

// identicalRun reports whether every sentence in the run says the same thing word for
// word, ignoring case and closing punctuation. "It's not fair. It's not fair. It's not
// fair." is epizeuxis, deliberate emphasis and an old human device, while the anaphora
// tell is parallel openers with varying tails. Verbatim repetition is the author leaning
// on a line, so it is exempt.
func identicalRun(text string, spans []sentSpan) bool {
	first := sentenceWords(text[spans[0].start:spans[0].end])
	for _, sp := range spans[1:] {
		if sentenceWords(text[sp.start:sp.end]) != first {
			return false
		}
	}
	return true
}

// sentenceWords normalizes a sentence to its lower-cased words with sentence punctuation
// stripped, so "No way!" and "no way." compare equal.
func sentenceWords(s string) string {
	return strings.ToLower(strings.Join(strings.Fields(strings.TrimRight(strings.TrimSpace(s), ".!?")), " "))
}

// sentSpan is one sentence located in the text, with the opener key anaphora runs are
// grouped by.
type sentSpan struct {
	// start and end are the byte range of the sentence.
	start, end int
	// key is the sentence's first two words, lower-cased, or empty when the sentence is
	// too short or too long to count toward a run.
	key string
	// adjacent reports that no paragraph break separates this sentence from the one
	// before it, so a run never reaches across paragraphs.
	adjacent bool
}

// anaphoraFindings reports every run of three or more consecutive short sentences in one
// paragraph that open with the same two words. The finding flags the whole run and is
// never rewritten: collapsing a drumbeat is a judgment call the author makes.
func anaphoraFindings(text string, protected [][2]int) []Finding {
	spans := sentenceSpans(text)
	// A markdown table is not prose, and neither is a list or a run of HTML rows. Their
	// lines open alike by construction, which is alignment rather than a drumbeat, so a
	// sentence starting on one neither seeds a run nor extends one. The line decides, not
	// the sentence, since the dot in "mtproto.py" or "epoch_19.jpg" ends a sentence partway
	// through an item.
	for i := range spans {
		if inTableRow(text, spans[i].start) || inListOrMarkupLine(text, spans[i].start) {
			spans[i].key = ""
		}
	}
	var out []Finding
	i := 0
	for i < len(spans) {
		j := i + 1
		for j < len(spans) && spans[j].adjacent && spans[j].key != "" && spans[j].key == spans[i].key {
			j++
		}
		if spans[i].key != "" && j-i >= 3 && !identicalRun(text, spans[i:j]) {
			start, end := spans[i].start, spans[j-1].end
			if !overlapsAny(protected, start, end) && !holdsDirective(text[start:end]) {
				out = append(out, Finding{
					Rule:   "structural:anaphora-run",
					Match:  text[start:end],
					Offset: start,
					order:  anaphoraOrder,
				})
			}
		}
		if j > i+1 {
			i = j
		} else {
			i++
		}
	}
	return out
}

// sentenceSpans splits text into located sentences. A sentence ends at sentence
// punctuation that is not an abbreviation, or at a paragraph break. A heading or a list
// item without closing punctuation runs to the next blank line, which keeps it from
// seeding a run of prose sentences.
func sentenceSpans(text string) []sentSpan {
	var spans []sentSpan
	i := 0
	adjacent := false
	for i < len(text) {
		newlines := 0
		for i < len(text) && (text[i] == ' ' || text[i] == '\t' || text[i] == '\n' || text[i] == '\r') {
			if text[i] == '\n' {
				newlines++
			}
			i++
		}
		if newlines > 1 {
			adjacent = false
		}
		if i >= len(text) {
			break
		}
		start := i
		end := -1
		for i < len(text) {
			c := text[i]
			if c == '.' || c == '!' || c == '?' {
				if c == '.' && abbreviationEnd(text, i) {
					i++
					continue
				}
				for i < len(text) && (text[i] == '.' || text[i] == '!' || text[i] == '?') {
					i++
				}
				end = i
				break
			}
			if c == '\n' && blankFrom(text, i+1) {
				end = i
				break
			}
			i++
		}
		if end < 0 {
			end = len(text)
		}
		spans = append(spans, sentSpan{start: start, end: end, key: anaphoraKey(text[start:end]), adjacent: adjacent})
		adjacent = true
	}
	return spans
}

// anaphoraKey returns the lower-cased first two words of sentence, or empty when the
// sentence is too short or too long to count toward an anaphora run. Punctuation comes off
// both words. Trimming only the second was an asymmetry rather than a decision, and it
// meant "We, needed retries." and "We needed observability." opened differently and never
// joined one run, which is the drumbeat the rule exists to catch.
func anaphoraKey(sentence string) string {
	fields := strings.Fields(sentence)
	if len(fields) < 2 || len(fields) > anaphoraMaxWords {
		return ""
	}
	first := strings.TrimRight(fields[0], ".!?,;:")
	second := strings.TrimRight(fields[1], ".!?,;:")
	if first == "" || second == "" {
		return ""
	}
	return strings.ToLower(first + " " + second)
}

// inListOrMarkupLine reports whether offset sits on a line that opens with a list marker
// or an HTML tag, after any indentation.
func inListOrMarkupLine(text string, offset int) bool {
	i := lineStart(text, offset)
	for i < len(text) && (text[i] == ' ' || text[i] == '\t') {
		i++
	}
	j := i
	for j < len(text) && text[j] != ' ' && text[j] != '\t' && text[j] != '\n' && text[j] != '\r' {
		j++
	}
	return isListMarker(text[i:j]) || strings.HasPrefix(text[i:j], "<")
}

// isListMarker reports whether field is a markdown list marker: a dash, star, plus, the
// "#." auto-number, or ASCII digits followed by one period or closing parenthesis.
func isListMarker(field string) bool {
	switch field {
	case "-", "*", "+", "#.":
		return true
	}
	if len(field) < 2 {
		return false
	}
	last := field[len(field)-1]
	if last != '.' && last != ')' {
		return false
	}
	for _, c := range field[:len(field)-1] {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

// holdsDirective reports whether run holds a C preprocessor line. A stack of #include
// lines pasted into a comment is code, and its sentence splits at every dot in a header
// name, so it never reads as a drumbeat.
func holdsDirective(run string) bool {
	for _, line := range strings.Split(run, "\n") {
		word, _, _ := strings.Cut(strings.TrimSpace(line), " ")
		if preprocessorDirectives[word] {
			return true
		}
	}
	return false
}

// preprocessorDirectives are the C preprocessor words that follow a hash with no space.
var preprocessorDirectives = map[string]bool{
	"#define": true, "#elif": true, "#else": true, "#endif": true, "#error": true,
	"#if": true, "#ifdef": true, "#ifndef": true, "#include": true, "#pragma": true,
	"#undef": true,
}
