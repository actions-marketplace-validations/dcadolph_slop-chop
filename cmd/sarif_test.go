package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"

	"github.com/dcadolph/slop-chop/sanitize"
)

// TestArtifactURI checks how a path as given becomes the URI a result points at.
func TestArtifactURI(t *testing.T) {
	t.Parallel()
	tests := []struct {
		WantResult string
		In         string
		WD         string
	}{{ // Test 0: A relative path stays relative.
		In: "docs/a.md", WD: "/w", WantResult: "docs/a.md",
	}, { // Test 1: A leading dot segment is cleaned away.
		In: "./docs/a.md", WD: "/w", WantResult: "docs/a.md",
	}, { // Test 2: A space is percent-encoded.
		In: "docs/my file.md", WD: "/w", WantResult: "docs/my%20file.md",
	}, { // Test 3: An absolute path under the working directory is made relative.
		In: "/w/docs/a.md", WD: "/w", WantResult: "docs/a.md",
	}, { // Test 4: An absolute path outside the working directory becomes a file URI.
		In: "/x/a.md", WD: "/w", WantResult: "file:///x/a.md",
	}, { // Test 5: A sibling directory sharing a prefix is outside the working directory.
		In: "/wide/a.md", WD: "/w", WantResult: "file:///wide/a.md",
	}, { // Test 6: Without a working directory an absolute path becomes a file URI.
		In: "/w/a.md", WantResult: "file:///w/a.md",
	}, { // Test 7: A colon in the first segment is not read as a scheme.
		In: "a:b.md", WD: "/w", WantResult: "./a:b.md",
	}}
	for testNum, test := range tests {
		t.Run(fmt.Sprintf("test %d", testNum), func(t *testing.T) {
			t.Parallel()
			if diff := cmp.Diff(test.WantResult, artifactURI(test.In, test.WD)); diff != "" {
				t.Errorf("mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

// TestSARIFRegion checks that a region is placed by code point column in the original text,
// with an exclusive end and no trailing line break.
func TestSARIFRegion(t *testing.T) {
	t.Parallel()
	tests := []struct {
		WantResult sarifRegion
		Raw        string
		Match      string
		Offset     int
	}{{ // Test 0: A word on the first line.
		Raw: "a robust plan", Offset: 2, Match: "robust",
		WantResult: sarifRegion{1, 3, 1, 9, sarifText{Text: "robust"}},
	}, { // Test 1: Multibyte runes before the match count one column each.
		Raw: "ééé delve", Offset: 7, Match: "delve",
		WantResult: sarifRegion{1, 5, 1, 10, sarifText{Text: "delve"}},
	}, { // Test 2: A match spanning lines ends on its last line.
		Raw: "one\ntwo three\nfour", Offset: 8, Match: "three\nfo",
		WantResult: sarifRegion{2, 5, 3, 3, sarifText{Text: "three\nfo"}},
	}, { // Test 3: A trailing line break stays out of the region.
		Raw: "one\ntwo\n", Offset: 4, Match: "two\n",
		WantResult: sarifRegion{2, 1, 2, 4, sarifText{Text: "two"}},
	}, { // Test 4: The snippet is the original text, not a mask the engine read.
		Raw: "x := 1 // a robust\n// plan", Offset: 12, Match: "robust\n   plan",
		WantResult: sarifRegion{1, 13, 2, 8, sarifText{Text: "robust\n// plan"}},
	}}
	for testNum, test := range tests {
		t.Run(fmt.Sprintf("test %d", testNum), func(t *testing.T) {
			t.Parallel()
			f := sanitize.Finding{Offset: test.Offset, Match: test.Match}
			got := sarifRegionFor(test.Raw, lineStarts(test.Raw), f)
			if diff := cmp.Diff(test.WantResult, got); diff != "" {
				t.Errorf("mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

// TestSARIFTitle checks the readable rule titles, including characters that do not print.
func TestSARIFTitle(t *testing.T) {
	t.Parallel()
	tests := []struct {
		WantResult string
		In         string
	}{{ // Test 0: A buzzword names its word.
		In: "word:delve", WantResult: "Buzzword: delve",
	}, { // Test 1: A printing character spells its code point and shows itself.
		In: "char:—", WantResult: "Character: U+2014 —",
	}, { // Test 2: An invisible character spells only its code point.
		In: "char:\u200b", WantResult: "Character: U+200B",
	}, { // Test 3: A space character spells only its code point.
		In: "char:\u00a0", WantResult: "Character: U+00A0",
	}, { // Test 4: A colonless cleanup rule is a cleanup.
		In: "double-space", WantResult: "Cleanup: double-space",
	}, { // Test 5: An unknown class still gets a title.
		In: "future:thing", WantResult: "Rule: thing",
	}}
	for testNum, test := range tests {
		t.Run(fmt.Sprintf("test %d", testNum), func(t *testing.T) {
			t.Parallel()
			class, name, ok := strings.Cut(test.In, ":")
			if !ok {
				class, name = "tidy", test.In
			}
			if diff := cmp.Diff(test.WantResult, sarifTitle(class, name)); diff != "" {
				t.Errorf("mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

// TestSARIFName checks the Pascal-case name each rule gets.
func TestSARIFName(t *testing.T) {
	t.Parallel()
	tests := []struct {
		WantResult string
		In         string
	}{{ // Test 0: A buzzword names its word.
		In: "word:delve", WantResult: "BuzzwordDelve",
	}, { // Test 1: A structural rule splits on hyphens.
		In: "structural:copula-landing", WantResult: "StructuralTellCopulaLanding",
	}, { // Test 2: A character rule spells its code point.
		In: "char:—", WantResult: "CharacterU2014",
	}, { // Test 3: An invisible character keeps lowercase hex.
		In: "char:\u200b", WantResult: "CharacterU200b",
	}, { // Test 4: A colonless cleanup rule is a cleanup.
		In: "double-space", WantResult: "CleanupDoubleSpace",
	}, { // Test 5: A phrase with an apostrophe drops it without splitting.
		In: "phrase:in today's fast-paced world", WantResult: "StockPhraseInTodaysFastPacedWorld",
	}, { // Test 6: A curly apostrophe is dropped too.
		In: "phrase:it\u2019s worth noting", WantResult: "StockPhraseItsWorthNoting",
	}, { // Test 7: A one-letter word extends the previous segment.
		In: "phrase:a lot", WantResult: "StockPhraseaLot",
	}, { // Test 8: A word starting with a digit extends the previous segment.
		In: "word:2fa", WantResult: "Buzzword2fa",
	}, { // Test 9: An unknown class still gets a name.
		In: "future:thing", WantResult: "RuleThing",
	}, { // Test 10: A cased non-ASCII letter stays a letter.
		In: "word:naïve", WantResult: "BuzzwordNaïve",
	}, { // Test 11: Uncased letters are spelled.
		In: "word:中文", WantResult: "BuzzwordU4e2dU6587",
	}, { // Test 12: Capitals lowercase before the first is raised.
		In: "word:API", WantResult: "BuzzwordApi",
	}, { // Test 13: Underscores and hyphens split words.
		In: "regex:my_rule-2", WantResult: "RegexRuleMyRule2",
	}, { // Test 14: A name with no words keeps only the title.
		In: "phrase:!!!", WantResult: "StockPhrase",
	}, { // Test 15: A long name clips on a rune boundary and drops a lone capital.
		In: "phrase:" + strings.Repeat("abc ", 100), WantResult: "StockPhrase" + strings.Repeat("Abc", 81),
	}}
	for testNum, test := range tests {
		t.Run(fmt.Sprintf("test %d", testNum), func(t *testing.T) {
			t.Parallel()
			class, name, ok := strings.Cut(test.In, ":")
			if !ok {
				class, name = "tidy", test.In
			}
			if diff := cmp.Diff(test.WantResult, sarifName(class, name)); diff != "" {
				t.Errorf("mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

// FuzzSARIFName checks that every name is one valid Pascal-case identifier within the limit.
func FuzzSARIFName(f *testing.F) {
	seeds := []string{
		"delve", "a lot", "中文", "ß", "ǅx", "ⅫA", "e\u0301", strings.Repeat("é", 300),
		strings.Repeat("abc ", 100), "", "U", "u",
	}
	for _, seed := range seeds {
		f.Add("phrase", seed)
		f.Add("char", seed)
	}
	pascal := regexp.MustCompile(`^(\p{Lu}[\p{Ll}\p{Nd}]+)+$`)
	f.Fuzz(func(t *testing.T, class, name string) {
		got := sarifName(class, name)
		if !pascal.MatchString(got) {
			t.Errorf("sarifName(%q, %q) = %q, not Pascal case", class, name, got)
		}
		if len(got) > sarifNameMax {
			t.Errorf("sarifName(%q, %q) is %d bytes, over %d", class, name, len(got), sarifNameMax)
		}
		if !utf8.ValidString(got) {
			t.Errorf("sarifName(%q, %q) = %q, invalid UTF-8", class, name, got)
		}
	})
}

// TestCheckSARIF drives check --sarif over files and decodes the log: one run, rules sorted
// and deduplicated with every result pointing at its own rule, levels from score weight,
// relative URIs, and the usual exit codes.
func TestCheckSARIF(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	writeTemp(t, dir, "dirty.md", "We delve into a robust plan.  Then we delve again.\n")
	writeTemp(t, dir, "clean.md", "A plain sentence that reads fine.\n")
	writeTemp(t, dir, "main.go", "package x\n\nvar s = \"ééé\" // We delve here.\n")
	writeTemp(t, dir, "weights.json", `{"scoreWeights": {"word": 0, "tidy": 1}}`)
	type result struct {
		URI   string
		Rule  string
		Level string
		Line  int
		Col   int
	}
	tests := []struct {
		Want        error
		WantRules   []string
		WantLevels  map[string]string
		WantResults []result
		Args        []string
	}{{ // Test 0: Findings across two files share one rule list and fail the run.
		Args: []string{"check", "--sarif", "dirty.md", "clean.md", "main.go"},
		Want: errFindings, WantRules: []string{"double-space", "word:delve", "word:robust"},
		WantLevels: map[string]string{"double-space": "note", "word:delve": "warning", "word:robust": "warning"},
		WantResults: []result{
			{"dirty.md", "word:delve", "warning", 1, 4},
			{"dirty.md", "word:robust", "warning", 1, 17},
			{"dirty.md", "double-space", "note", 1, 29},
			{"dirty.md", "word:delve", "warning", 1, 39},
			{"main.go", "word:delve", "warning", 3, 21},
		},
	}, { // Test 1: A clean file writes an empty log and passes.
		Args:       []string{"check", "--sarif", "clean.md"},
		WantRules:  []string{},
		WantLevels: map[string]string{},
	}, { // Test 2: Profile weights move rules between warning and note.
		Args: []string{"check", "--sarif", "--profile", "weights.json", "dirty.md"},
		Want: errFindings, WantRules: []string{"double-space", "word:delve", "word:robust"},
		WantLevels: map[string]string{"double-space": "warning", "word:delve": "note", "word:robust": "note"},
		WantResults: []result{
			{"dirty.md", "word:delve", "note", 1, 4},
			{"dirty.md", "word:robust", "note", 1, 17},
			{"dirty.md", "double-space", "warning", 1, 29},
			{"dirty.md", "word:delve", "note", 1, 39},
		},
	}}
	for testNum, test := range tests {
		t.Run(fmt.Sprintf("test %d", testNum), func(t *testing.T) {
			stdout, _, err := runCLI(t, test.Args, "")
			if !errors.Is(err, test.Want) {
				t.Fatalf("err = %v, want %v", err, test.Want)
			}
			var log sarifLog
			if err := json.Unmarshal([]byte(stdout), &log); err != nil {
				t.Fatalf("decode %s: %v", stdout, err)
			}
			if log.Version != "2.1.0" || len(log.Runs) != 1 {
				t.Fatalf("version %q with %d runs, want 2.1.0 with one run", log.Version, len(log.Runs))
			}
			run := log.Runs[0]
			if !strings.Contains(stdout, `"results":[]`) && len(test.WantResults) == 0 {
				t.Errorf("stdout = %s, want an empty results array, not null", stdout)
			}
			var rules []string
			levels := make(map[string]string)
			for _, r := range run.Tool.Driver.Rules {
				rules = append(rules, r.ID)
				class, name, ok := strings.Cut(r.ID, ":")
				if !ok {
					class, name = "tidy", r.ID
				}
				if want := sarifName(class, name); r.Name != want || r.Name == "" {
					t.Errorf("rule %s has name %q, want %q", r.ID, r.Name, want)
				}
				levels[r.ID] = r.DefaultConfiguration.Level
			}
			if diff := cmp.Diff(test.WantRules, rules, cmpopts.EquateEmpty()); diff != "" {
				t.Errorf("rules mismatch (-want +got):\n%s", diff)
			}
			if diff := cmp.Diff(test.WantLevels, levels, cmpopts.EquateEmpty()); diff != "" {
				t.Errorf("levels mismatch (-want +got):\n%s", diff)
			}
			var got []result
			for _, r := range run.Results {
				if r.RuleIndex < 0 || r.RuleIndex >= len(rules) || rules[r.RuleIndex] != r.RuleID {
					t.Errorf("result %s has ruleIndex %d, which does not name it", r.RuleID, r.RuleIndex)
				}
				loc := r.Locations[0].PhysicalLocation
				got = append(got, result{loc.ArtifactLocation.URI, r.RuleID, r.Level, loc.Region.StartLine, loc.Region.StartColumn})
			}
			if diff := cmp.Diff(test.WantResults, got, cmpopts.EquateEmpty()); diff != "" {
				t.Errorf("results mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

// TestCheckSARIFRefuses checks the flag combinations --sarif cannot serve.
func TestCheckSARIFRefuses(t *testing.T) {
	tests := []struct {
		WantText string
		Args     []string
	}{{ // Test 0: Stdin has no file for a result to point at.
		Args: []string{"check", "--sarif"}, WantText: "needs a file argument",
	}, { // Test 1: The two JSON shapes cannot share stdout.
		Args: []string{"check", "--sarif", "--json", "x.md"}, WantText: "cannot use --sarif with --json",
	}}
	for testNum, test := range tests {
		t.Run(fmt.Sprintf("test %d", testNum), func(t *testing.T) {
			_, _, err := runCLI(t, test.Args, "a robust plan")
			if err == nil || errors.Is(err, errFindings) || !strings.Contains(err.Error(), test.WantText) {
				t.Errorf("err = %v, want a usage error containing %q", err, test.WantText)
			}
		})
	}
}
