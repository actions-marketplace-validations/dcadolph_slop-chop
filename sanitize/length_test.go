package sanitize

import (
	"fmt"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
)

// testWords returns n distinct lower-case words joined by spaces, with no punctuation.
func testWords(n int) string {
	words := make([]string, n)
	for i := range words {
		words[i] = fmt.Sprintf("w%d", i)
	}
	return strings.Join(words, " ")
}

// procedureSanitizer returns a sanitizer for the default profile with the procedure preset.
func procedureSanitizer(t *testing.T) *Sanitizer {
	t.Helper()
	p, err := ApplyPresets(DefaultProfile(), "procedure")
	if err != nil {
		t.Fatalf("ApplyPresets: %v", err)
	}
	s, err := New(p)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return s
}

// lengthRules returns the rule name of every sentence length finding in findings, in order.
func lengthRules(findings []Finding) []string {
	var out []string
	for _, f := range findings {
		if strings.HasPrefix(f.Rule, "length:") {
			out = append(out, f.Rule)
		}
	}
	return out
}

// TestSentenceLength checks the procedure preset's limits: twenty words for a sentence in a
// list item, twenty-five for any other sentence, and nothing flagged at the limit itself.
//
//nolint:funlen // Test function.
func TestSentenceLength(t *testing.T) {
	t.Parallel()
	s := procedureSanitizer(t)
	tests := []struct {
		In        string
		WantRules []string
	}{{ // Test 0: A numbered step one word over the limit.
		In: "1. " + testWords(21) + ".", WantRules: []string{"length:step"},
	}, { // Test 1: A numbered step at the limit.
		In: "1. " + testWords(20) + ".",
	}, { // Test 2: A prose sentence one word over the description limit.
		In: testWords(26) + ".", WantRules: []string{"length:description"},
	}, { // Test 3: A prose sentence at the description limit.
		In: testWords(25) + ".",
	}, { // Test 4: A prose sentence over the step limit but under the description limit.
		In: testWords(21) + ".",
	}, { // Test 5: A bullet step with no closing period.
		In: "- " + testWords(21), WantRules: []string{"length:step"},
	}, { // Test 6: A step hard-wrapped across two lines is one sentence.
		In: "2. " + testWords(11) + "\n   " + testWords(10) + ".", WantRules: []string{"length:step"},
	}, { // Test 7: Two short sentences in one step are judged apart.
		In: "1. " + testWords(12) + ". " + testWords(12) + ".",
	}, { // Test 8: Fenced code is not prose.
		In: "```\n" + testWords(40) + ".\n```\n",
	}, { // Test 9: An inline code span counts once, and its dots do not end the sentence.
		In: "1. Run `go test ./... -run X.Y` " + testWords(19) + ".", WantRules: []string{"length:step"},
	}, { // Test 10: A file name's dot does not end the sentence.
		In: "Open config.yaml " + testWords(25) + ".", WantRules: []string{"length:description"},
	}, { // Test 11: A bare URL counts as one word and its dots do not split the sentence.
		In:        "See https://example.com/a.b.c " + testWords(24) + ".",
		WantRules: []string{"length:description"},
	}, { // Test 12: An ignore directive silences the line.
		In: testWords(30) + ". <!-- slop-chop-ignore -->",
	}, { // Test 13: A heading is not a sentence.
		In: "# " + testWords(30) + "\n",
	}, { // Test 14: A table row is not a sentence.
		In: "| " + testWords(30) + " |\n",
	}, { // Test 15: A link counts its visible words and not its destination.
		In:        "[" + testWords(25) + "](https://example.com/a.b) now.",
		WantRules: []string{"length:description"},
	}, { // Test 16: A paragraph under a list item after a blank line is a description.
		In: "1. Close the valve.\n\n   " + testWords(26) + ".", WantRules: []string{"length:description"},
	}, { // Test 17: A step and a description in one document are each flagged by their own limit.
		In:        "Read this first. " + testWords(26) + ".\n\n- " + testWords(21) + ".\n",
		WantRules: []string{"length:description", "length:step"},
	}, { // Test 18: Punctuation-only fields are not words.
		In: "1. " + testWords(20) + " - ( ) .",
	}, { // Test 19: An abbreviation does not end the sentence.
		In:        "Use a wrench, e.g. a torque one, " + testWords(20) + ".",
		WantRules: []string{"length:description"},
	}, { // Test 20: A task-list checkbox is not a word of the step.
		In: "- [x] " + testWords(20) + ".",
	}, { // Test 21: A task-list step over the limit.
		In: "- [ ] " + testWords(21) + ".", WantRules: []string{"length:step"},
	}}
	for testNum, test := range tests {
		t.Run(fmt.Sprintf("test %d", testNum), func(t *testing.T) {
			t.Parallel()
			got := lengthRules(s.Check(test.In))
			if diff := cmp.Diff(test.WantRules, got, cmpopts.EquateEmpty()); diff != "" {
				t.Errorf("length findings mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

// TestSentenceLengthFinding checks the finding a long sentence produces: flag only, the
// whole sentence as the match, and a position that points at its first word.
func TestSentenceLengthFinding(t *testing.T) {
	t.Parallel()
	s := procedureSanitizer(t)
	sentence := testWords(21) + "."
	in := "Intro.\n\n1. " + sentence + "\n"
	var got []Finding
	for _, f := range s.Check(in) {
		if strings.HasPrefix(f.Rule, "length:") {
			got = append(got, f)
		}
	}
	want := []Finding{{
		Rule: "length:step", Match: sentence, Offset: strings.Index(in, sentence), Line: 3, Col: 4,
	}}
	if diff := cmp.Diff(want, got, cmpopts.IgnoreUnexported(Finding{})); diff != "" {
		t.Errorf("finding mismatch (-want +got):\n%s", diff)
	}
}

// TestSentenceLengthOff checks that the default profile has no sentence length limit.
func TestSentenceLengthOff(t *testing.T) {
	t.Parallel()
	s, err := New(DefaultProfile())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	in := testWords(60) + ".\n\n1. " + testWords(60) + ".\n"
	if got := lengthRules(s.Check(in)); len(got) != 0 {
		t.Errorf("default profile length findings = %q, want none", got)
	}
}

// TestSentenceLengthWeight checks that a length finding carries no score weight unless a
// profile asks for it, so the procedure preset never moves a score on its own.
func TestSentenceLengthWeight(t *testing.T) {
	t.Parallel()
	in := testWords(40) + ". " + testWords(40) + ".\n\n- " + testWords(30) + ".\n"
	base, err := New(DefaultProfile())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	s := procedureSanitizer(t)
	for _, rule := range []string{"length:step", "length:description"} {
		if w := s.Weight(rule); w != 0 {
			t.Errorf("Weight(%q) = %v, want 0", rule, w)
		}
	}
	if got, want := s.Score(in).Value, base.Score(in).Value; got != want {
		t.Errorf("procedure score = %d, default score = %d, want equal", got, want)
	}
	p, err := ApplyPresets(DefaultProfile(), "procedure")
	if err != nil {
		t.Fatalf("ApplyPresets: %v", err)
	}
	p.ScoreWeights = map[string]float64{"length": 1}
	weighted, err := New(p)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if got, floor := weighted.Score(in).Value, base.Score(in).Value; got <= floor {
		t.Errorf("score with length weighted = %d, want above %d", got, floor)
	}
}

// TestProcedureScoresMatchDefault checks every benchmark passage scores the same under the
// procedure preset as under the default profile, the promise that it stays out of the score.
func TestProcedureScoresMatchDefault(t *testing.T) {
	t.Parallel()
	base, err := New(DefaultProfile())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	s := procedureSanitizer(t)
	flagged := 0
	for _, p := range loadCorpus(t) {
		if len(lengthRules(s.Check(p.Text))) > 0 {
			flagged++
		}
		if got, want := s.Score(p.Text).Value, base.Score(p.Text).Value; got != want {
			t.Errorf("%s: procedure score %d, default %d", passageKey(p), got, want)
		}
	}
	if flagged == 0 {
		t.Error("no benchmark passage carries a long sentence, so this test proves nothing")
	}
}

// TestSentenceLengthLayers checks how the limit resolves through the layers: a preset fills
// an unset limit, a project file beats the preset, and a voice leaves the limit alone.
func TestSentenceLengthLayers(t *testing.T) {
	t.Parallel()
	voice := Voice{Keep: []string{"robust"}, Avoid: []string{"synergy"}}
	tests := []struct {
		Layers     Layers
		WantLength SentenceLength
	}{{ // Test 0: The default has no limit.
		Layers: Layers{}, WantLength: SentenceLength{},
	}, { // Test 1: The preset sets both limits.
		Layers:     Layers{Presets: []string{"procedure"}},
		WantLength: SentenceLength{Step: 20, Description: 25},
	}, { // Test 2: A voice on top of the preset keeps its limits.
		Layers:     Layers{Presets: []string{"procedure"}, Voice: voice},
		WantLength: SentenceLength{Step: 20, Description: 25},
	}, { // Test 3: A project limit beats the preset and the other limit comes from the preset.
		Layers: Layers{
			Project: &Profile{SentenceLength: SentenceLength{Step: 15}},
			Presets: []string{"procedure"}, Voice: voice,
		},
		WantLength: SentenceLength{Step: 15, Description: 25},
	}, { // Test 4: A project limit works with no preset at all.
		Layers:     Layers{Project: &Profile{SentenceLength: SentenceLength{Description: 30}}},
		WantLength: SentenceLength{Description: 30},
	}, { // Test 5: A standalone project keeps its own limit under the preset.
		Layers: Layers{
			Project: &Profile{Standalone: true, SentenceLength: SentenceLength{Step: 12, Description: 18}},
			Presets: []string{"procedure"},
		},
		WantLength: SentenceLength{Step: 12, Description: 18},
	}}
	for testNum, test := range tests {
		t.Run(fmt.Sprintf("test %d", testNum), func(t *testing.T) {
			t.Parallel()
			p, err := test.Layers.Profile()
			if err != nil {
				t.Fatalf("Profile: %v", err)
			}
			if diff := cmp.Diff(test.WantLength, p.SentenceLength); diff != "" {
				t.Errorf("sentence length mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

// TestSentenceLengthSurvivesDedupe checks that a length finding and another rule's finding
// on the same sentence are both kept, so the length rule can never hide a weighted tell.
func TestSentenceLengthSurvivesDedupe(t *testing.T) {
	t.Parallel()
	p, err := ApplyPresets(DefaultProfile(), "procedure")
	if err != nil {
		t.Fatalf("ApplyPresets: %v", err)
	}
	p.FlagPatterns = map[string]string{"whole-line": `(?m)^w0 [^.\n]*\.`}
	s, err := New(p)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	var got []string
	for _, f := range s.Check(testWords(26) + ".") {
		got = append(got, f.Rule)
	}
	want := []string{"length:description", "structural:whole-line"}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("rules mismatch (-want +got):\n%s", diff)
	}
}

// TestSentenceLengthReason checks that a length finding explains itself instead of reading
// as house-style cleanup.
func TestSentenceLengthReason(t *testing.T) {
	t.Parallel()
	got := Why(Finding{Rule: "length:step"})
	want := "A sentence longer than the profile's sentence length limit. It carries no score " +
		"weight. It only flags; the rewording is your call."
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("Why mismatch (-want +got):\n%s", diff)
	}
}

// TestSentenceLengthStepOnly checks that a profile limiting only steps leaves every other
// sentence alone, however long.
func TestSentenceLengthStepOnly(t *testing.T) {
	t.Parallel()
	s, err := New(Profile{SentenceLength: SentenceLength{Step: 5}})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	got := lengthRules(s.Check(testWords(40) + ".\n\n1. " + testWords(6) + ".\n"))
	if diff := cmp.Diff([]string{"length:step"}, got); diff != "" {
		t.Errorf("length findings mismatch (-want +got):\n%s", diff)
	}
}
