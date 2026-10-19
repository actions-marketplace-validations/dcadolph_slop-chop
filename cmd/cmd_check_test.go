package cmd

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
)

// TestCheckFindings checks that check returns errFindings on slop and nil on clean
// text.
func TestCheckFindings(t *testing.T) {
	tests := []struct {
		In   string
		Want error
	}{{ // Test 0: Slop returns the sentinel.
		In: "a robust plan", Want: errFindings,
	}, { // Test 1: Clean text returns nil.
		In: "a plain sentence", Want: nil,
	}}

	for testNum, test := range tests {
		t.Run(fmt.Sprintf("test %d", testNum), func(t *testing.T) {
			_, _, err := runCLI(t, []string{"check"}, test.In)
			if !errors.Is(err, test.Want) {
				t.Errorf("err = %v, want %v", err, test.Want)
			}
		})
	}
}

// TestCheckFilePrefix checks that findings on a file carry the path, so terminals can
// make them clickable.
func TestCheckFilePrefix(t *testing.T) {
	path := writeTemp(t, t.TempDir(), "notes.md", "a robust plan")
	_, stderr, err := runCLI(t, []string{"check", path}, "")
	if !errors.Is(err, errFindings) {
		t.Fatalf("err = %v, want errFindings", err)
	}
	if !strings.Contains(stderr, path+":1:3") {
		t.Errorf("stderr = %q, want prefix %q", stderr, path+":1:3")
	}
}

// TestCheckMultiFile checks that every file is scanned, each finding carries its own
// path, and one dirty file fails the whole run.
func TestCheckMultiFile(t *testing.T) {
	dir := t.TempDir()
	clean := writeTemp(t, dir, "clean.md", "a plain sentence")
	dirty := writeTemp(t, dir, "dirty.md", "a robust plan")
	_, stderr, err := runCLI(t, []string{"check", clean, dirty}, "")
	if !errors.Is(err, errFindings) {
		t.Fatalf("err = %v, want errFindings", err)
	}
	if !strings.Contains(stderr, dirty+":1:3") {
		t.Errorf("stderr = %q, want a finding for %q", stderr, dirty)
	}
	if strings.Contains(stderr, clean+":") {
		t.Errorf("stderr = %q, want no findings for %q", stderr, clean)
	}
}

// TestCheckMissingFile checks that a missing input file is a read error, not a crash.
func TestCheckMissingFile(t *testing.T) {
	_, _, err := runCLI(t, []string{"check", "/no/such/file.md"}, "")
	if err == nil || !strings.Contains(err.Error(), "read file") {
		t.Errorf("err = %v, want a read-file error", err)
	}
}

// TestCheckMalformedProfile checks that a profile that is not valid JSON is an error.
func TestCheckMalformedProfile(t *testing.T) {
	bad := writeTemp(t, t.TempDir(), "bad.json", "{not json")
	_, _, err := runCLI(t, []string{"check", "--profile", bad}, "some text")
	if err == nil || !strings.Contains(err.Error(), "profile decode") {
		t.Errorf("err = %v, want a profile-decode error", err)
	}
}

// TestCheckMissingProfile checks that a profile path that does not exist is an error.
func TestCheckMissingProfile(t *testing.T) {
	_, _, err := runCLI(t, []string{"check", "--profile", "/no/such/profile.json"}, "some text")
	if err == nil || !strings.Contains(err.Error(), "profile open") {
		t.Errorf("err = %v, want a profile-open error", err)
	}
}

// TestCheckJSONFindings checks that check --json still exits through errFindings.
func TestCheckJSONFindings(t *testing.T) {
	out, _, err := runCLI(t, []string{"check", "--json"}, "a robust plan")
	if !errors.Is(err, errFindings) {
		t.Fatalf("err = %v, want errFindings", err)
	}
	if !strings.Contains(out, `"findings"`) {
		t.Errorf("stdout = %q, want findings JSON", out)
	}
}

// TestCheckWhy checks that --why explains each finding in plain words, and that the
// explanation matches what the finding is: a flag, a swap, or a cut.
func TestCheckWhy(t *testing.T) {
	_, stderr, err := runCLI(t, []string{"check", "--why"},
		"In summary, we leverage robust synergy.")
	if !errors.Is(err, errFindings) {
		t.Fatalf("err = %v, want errFindings", err)
	}
	for _, want := range []string{
		"A stock phrase of the machine register. The profile swaps it.",
		"block list of terms models lean on. It only flags",
	} {
		if !strings.Contains(stderr, want) {
			t.Errorf("stderr missing %q:\n%s", want, stderr)
		}
	}
	// Without the flag, no explanation lines appear.
	_, plain, _ := runCLI(t, []string{"check"}, "In summary, we leverage robust synergy.")
	if strings.Contains(plain, "machine register") {
		t.Errorf("explanations printed without --why:\n%s", plain)
	}
}

// TestOutputStartsNoWorkflowCommand checks that no line check or score prints starts with
// "::", even when lines of the input do. The action pipes pull request text through both,
// and the GitHub runner reads an output line that starts with "::" as a workflow command.
func TestOutputStartsNoWorkflowCommand(t *testing.T) {
	// The span rule matches across lines, so a finding carries a line that starts with "::".
	profile := writeTemp(t, t.TempDir(), "profile.json",
		`{"flagPatterns": {"span": "(?s)open.*?close"}}`)
	in := "We delve into it.\n::error::injected\nopen\n::warning close\n::add-mask::secret\n"
	tests := []struct {
		Args         []string
		WantContains string
	}{{ // Test 0: Check prints the finding that spans the "::" line.
		Args: []string{"check", "--profile", profile}, WantContains: "structural:span",
	}, { // Test 1: Score with the gate on prints its verdict.
		Args: []string{"score", "--max", "0", "--profile", profile}, WantContains: "tells in",
	}}

	for testNum, test := range tests {
		t.Run(fmt.Sprintf("test %d", testNum), func(t *testing.T) {
			stdout, stderr, err := runCLI(t, test.Args, in)
			if !errors.Is(err, errFindings) {
				t.Fatalf("err = %v, want errFindings", err)
			}
			out := stdout + stderr
			if !strings.Contains(out, test.WantContains) {
				t.Fatalf("output = %q, want it to contain %q", out, test.WantContains)
			}
			for line := range strings.Lines(out) {
				if strings.HasPrefix(strings.TrimSpace(line), "::") {
					t.Errorf("output line %q starts a workflow command", line)
				}
			}
		})
	}
}

// TestCheckVerdict checks that check fails on a finding that carries score weight, lists a
// cleanup without failing on it, and fails on any finding under --strict. Weight resolves
// through the profile, so a profile can move a rule either way.
func TestCheckVerdict(t *testing.T) {
	dir := t.TempDir()
	tidyText := "A plain sentence  with a double space.\n"
	srcText := "package x\n\n// It’s fine  here.\nvar a = 1\n"
	tidy := writeTemp(t, dir, "tidy.md", tidyText)
	dirty := writeTemp(t, dir, "dirty.md", "a robust plan\n")
	clean := writeTemp(t, dir, "clean.md", "a plain sentence\n")
	src := writeTemp(t, dir, "main.go", srcText)
	tidyWeighted := writeTemp(t, dir, "tidy-weighted.json", `{"scoreWeights": {"double-space": 1}}`)
	robustFree := writeTemp(t, dir, "robust-free.json", `{"scoreWeights": {"word:robust": 0}}`)
	tests := []struct {
		Want         error
		WantContains string
		Args         []string
		In           string
	}{{ // Test 0: A cleanup on stdin lists and passes.
		Args: []string{"check"}, In: tidyText, Want: nil, WantContains: "1:17 double-space",
	}, { // Test 1: A cleanup on stdin fails under --strict.
		Args: []string{"check", "--strict"}, In: tidyText, Want: errFindings, WantContains: "1:17 double-space",
	}, { // Test 2: A cleanup in a file lists and passes.
		Args: []string{"check", tidy}, Want: nil, WantContains: tidy + ":1:17 double-space",
	}, { // Test 3: A cleanup in a file fails under --strict.
		Args: []string{"check", "--strict", tidy}, Want: errFindings, WantContains: tidy + ":1:17 double-space",
	}, { // Test 4: A typography swap in a source file comment lists and passes.
		Args: []string{"check", src}, Want: nil, WantContains: src + ":3:6 char:’",
	}, { // Test 5: A typography swap in a source file comment fails under --strict.
		Args: []string{"check", "--strict", src}, Want: errFindings, WantContains: src + ":3:6 char:’",
	}, { // Test 6: A typography swap in source on stdin with --as lists and passes.
		Args: []string{"check", "--as", "go"}, In: srcText, Want: nil, WantContains: "3:6 char:’",
	}, { // Test 7: A typography swap in source on stdin with --as fails under --strict.
		Args: []string{"check", "--as", "go", "--strict"}, In: srcText, Want: errFindings, WantContains: "3:6 char:’",
	}, { // Test 8: A weighted finding fails.
		Args: []string{"check", dirty}, Want: errFindings, WantContains: dirty + ":1:3 word:robust",
	}, { // Test 9: A weighted finding fails under --strict.
		Args: []string{"check", "--strict", dirty}, Want: errFindings, WantContains: dirty + ":1:3 word:robust",
	}, { // Test 10: A profile that weights the cleanup makes it fail.
		Args: []string{"check", "--profile", tidyWeighted, tidy}, Want: errFindings, WantContains: "double-space",
	}, { // Test 11: A profile that sets a word to weight 0 lists it and passes.
		Args: []string{"check", "--profile", robustFree, dirty}, Want: nil, WantContains: dirty + ":1:3 word:robust",
	}, { // Test 12: Cleanups in one file and a clean file pass the run.
		Args: []string{"check", clean, tidy}, Want: nil, WantContains: clean + " is clean",
	}, { // Test 13: One weighted finding fails a run that also holds cleanups.
		Args: []string{"check", tidy, dirty}, Want: errFindings, WantContains: dirty + ":1:3 word:robust",
	}, { // Test 14: JSON lists a cleanup and passes.
		Args: []string{"check", "--json", tidy}, Want: nil, WantContains: `"rule":"double-space"`,
	}, { // Test 15: JSON fails on a cleanup under --strict.
		Args: []string{"check", "--json", "--strict", tidy}, Want: errFindings, WantContains: `"rule":"double-space"`,
	}, { // Test 16: SARIF logs a cleanup as a note and passes.
		Args: []string{"check", "--sarif", tidy}, Want: nil, WantContains: `"level":"note"`,
	}, { // Test 17: SARIF fails on a cleanup under --strict.
		Args: []string{"check", "--sarif", "--strict", tidy}, Want: errFindings, WantContains: `"level":"note"`,
	}}

	for testNum, test := range tests {
		t.Run(fmt.Sprintf("test %d", testNum), func(t *testing.T) {
			stdout, stderr, err := runCLI(t, test.Args, test.In)
			if !errors.Is(err, test.Want) {
				t.Fatalf("err = %v, want %v", err, test.Want)
			}
			if out := stdout + stderr; !strings.Contains(out, test.WantContains) {
				t.Errorf("output = %q, want it to contain %q", out, test.WantContains)
			}
		})
	}
}

// TestCheckStrictKeepsOutput checks that --strict changes the exit and nothing else a
// program reads: the JSON and SARIF output and the per-finding lines are the same with and
// without it.
func TestCheckStrictKeepsOutput(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	writeTemp(t, dir, "mixed.md", "We delve into a robust plan.  Then we go.\n")
	writeTemp(t, dir, "tidy.md", "A plain sentence  with a double space.\n")
	tests := []struct {
		Want error
		Args []string
	}{{ // Test 0: Text on a mixed report.
		Args: []string{"check", "mixed.md"}, Want: errFindings,
	}, { // Test 1: Text on cleanups only.
		Args: []string{"check", "tidy.md"}, Want: nil,
	}, { // Test 2: JSON on a mixed report.
		Args: []string{"check", "--json", "mixed.md"}, Want: errFindings,
	}, { // Test 3: JSON on cleanups only.
		Args: []string{"check", "--json", "tidy.md"}, Want: nil,
	}, { // Test 4: SARIF over a mixed report and cleanups.
		Args: []string{"check", "--sarif", "mixed.md", "tidy.md"}, Want: errFindings,
	}, { // Test 5: SARIF on cleanups only.
		Args: []string{"check", "--sarif", "tidy.md"}, Want: nil,
	}}

	for testNum, test := range tests {
		t.Run(fmt.Sprintf("test %d", testNum), func(t *testing.T) {
			stdout, stderr, err := runCLI(t, test.Args, "")
			if !errors.Is(err, test.Want) {
				t.Fatalf("err = %v, want %v", err, test.Want)
			}
			strictOut, strictErr, err := runCLI(t, append(test.Args, "--strict"), "")
			if !errors.Is(err, errFindings) {
				t.Fatalf("strict err = %v, want errFindings", err)
			}
			if diff := cmp.Diff(stdout, strictOut); diff != "" {
				t.Errorf("stdout mismatch (-plain +strict):\n%s", diff)
			}
			if diff := cmp.Diff(findingLines(stderr), findingLines(strictErr)); diff != "" {
				t.Errorf("finding lines mismatch (-plain +strict):\n%s", diff)
			}
		})
	}
}

// TestCheckMixedReport pins one report that holds weighted findings and a cleanup: the
// per-finding lines and the JSON are what they were when any finding failed check, and the
// summary line names the cleanup that does not fail.
func TestCheckMixedReport(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	writeTemp(t, dir, "mixed.md", "We delve into a robust plan.  Then we go.\n")
	wantLines := "mixed.md:1:4 word:delve: \"delve\"\n" +
		"mixed.md:1:17 word:robust: \"robust\"\n" +
		"mixed.md:1:29 double-space: \"  \" -> \" \"\n"
	wantJSON := `{"findings":[` +
		`{"rule":"word:delve","match":"delve","offset":3,"line":1,"col":4},` +
		`{"rule":"word:robust","match":"robust","offset":16,"line":1,"col":17},` +
		`{"rule":"double-space","match":"  ","replacement":" ","offset":28,"line":1,"col":29}]}` + "\n"
	tests := []struct {
		WantSummary string
		Args        []string
	}{{ // Test 0: The summary names the one cleanup that does not fail.
		Args: []string{"check", "mixed.md"},
		WantSummary: "slop-chop: 3 finding(s), 1 fixable with 'slop-chop fix', 1 with no score weight that check does not fail on; " +
			"silence a one-off with an allow entry or <!-- slop-chop-ignore -->\n",
	}, { // Test 1: Under --strict every finding fails and the summary names no cleanup.
		Args: []string{"check", "--strict", "mixed.md"},
		WantSummary: "slop-chop: 3 finding(s), 1 fixable with 'slop-chop fix'; " +
			"silence a one-off with an allow entry or <!-- slop-chop-ignore -->\n",
	}}

	for testNum, test := range tests {
		t.Run(fmt.Sprintf("test %d", testNum), func(t *testing.T) {
			_, stderr, err := runCLI(t, test.Args, "")
			if !errors.Is(err, errFindings) {
				t.Fatalf("err = %v, want errFindings", err)
			}
			if diff := cmp.Diff(wantLines+test.WantSummary, stderr); diff != "" {
				t.Errorf("stderr mismatch (-want +got):\n%s", diff)
			}
			stdout, _, err := runCLI(t, append([]string{"check", "--json"}, test.Args[1:]...), "")
			if !errors.Is(err, errFindings) {
				t.Fatalf("json err = %v, want errFindings", err)
			}
			if diff := cmp.Diff(wantJSON, stdout); diff != "" {
				t.Errorf("json mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

// TestCheckCleanupSummary checks the summary line of a report where nothing fails, for a
// fixable cleanup and for a flag-only finding a profile set to weight 0.
func TestCheckCleanupSummary(t *testing.T) {
	profile := writeTemp(t, t.TempDir(), "profile.json", `{"scoreWeights": {"word:robust": 0}}`)
	tests := []struct {
		WantSummary string
		Args        []string
		In          string
	}{{ // Test 0: A fixable cleanup.
		Args: []string{"check"}, In: "A plain sentence  with a double space.\n",
		WantSummary: "slop-chop: 1 finding(s), none with score weight, so none fail check; 1 fixable with 'slop-chop fix'\n",
	}, { // Test 1: A flag-only finding with weight 0.
		Args: []string{"check", "--profile", profile}, In: "a robust plan\n",
		WantSummary: "slop-chop: 1 finding(s), none with score weight, so none fail check; all flag-only\n",
	}}

	for testNum, test := range tests {
		t.Run(fmt.Sprintf("test %d", testNum), func(t *testing.T) {
			_, stderr, err := runCLI(t, test.Args, test.In)
			if err != nil {
				t.Fatalf("err = %v, want nil", err)
			}
			lines := strings.SplitAfter(stderr, "\n")
			if len(lines) < 2 {
				t.Fatalf("stderr = %q, want a finding line and a summary line", stderr)
			}
			if diff := cmp.Diff(test.WantSummary, lines[len(lines)-2]); diff != "" {
				t.Errorf("summary mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

// findingLines returns the lines of check's text output that are not its summary line.
func findingLines(stderr string) []string {
	var out []string
	for line := range strings.Lines(stderr) {
		if !strings.HasPrefix(line, "slop-chop: ") {
			out = append(out, line)
		}
	}
	return out
}
