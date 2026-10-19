package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestCheckScansComments checks that check on a source file reads the comments and only
// the comments: a tell in a comment is flagged at its real position, while the same word
// as an identifier, a tell inside a string literal, and gofmt alignment spacing draw
// nothing.
func TestCheckScansComments(t *testing.T) {
	dir := t.TempDir()
	src := "package main\n" +
		"\n" +
		"// This delivers a comprehensive solution.\n" +
		"func main() {\n" +
		"\tcomprehensive := 1     // twelve\n" +
		"\ts := \"a robust plan\"\n" +
		"\t_, _ = comprehensive, s\n" +
		"}\n"
	goFile := writeTemp(t, dir, "main.go", src)
	_, stderr, err := runCLI(t, []string{"check", goFile}, "")
	if !errors.Is(err, errFindings) {
		t.Fatalf("err = %v, want errFindings for a tell in a comment", err)
	}
	if !strings.Contains(stderr, goFile+":3:") || !strings.Contains(stderr, "word:comprehensive") {
		t.Errorf("stderr = %q, want word:comprehensive reported on line 3", stderr)
	}
	if strings.Contains(stderr, "word:robust") {
		t.Errorf("stderr = %q, want no finding for a tell inside a string literal", stderr)
	}
	if strings.Contains(stderr, "double-space") {
		t.Errorf("stderr = %q, want no cleanup findings on a source file", stderr)
	}
	if strings.Contains(stderr, goFile+":5:2") {
		t.Errorf("stderr = %q, want no finding for the identifier on line 5", stderr)
	}
}

// TestCheckScansIndentedComments checks that a comment nested inside a function is read
// like one at the top of the file. The mask once left such a comment indented four spaces,
// which the prose engine skipped as an indented code block, so only top-level comments
// were ever scanned.
func TestCheckScansIndentedComments(t *testing.T) {
	dir := t.TempDir()
	src := "package main\n" +
		"\n" +
		"func main() {\n" +
		"\t// In summary, this delivers a comprehensive solution.\n" +
		"\tx := 1\n" +
		"\t_ = x\n" +
		"}\n"
	goFile := writeTemp(t, dir, "main.go", src)
	_, stderr, err := runCLI(t, []string{"check", goFile}, "")
	if !errors.Is(err, errFindings) {
		t.Fatalf("err = %v, want errFindings for a tell in an indented comment", err)
	}
	if !strings.Contains(stderr, goFile+":4:") || !strings.Contains(stderr, "word:comprehensive") {
		t.Errorf("stderr = %q, want word:comprehensive reported on line 4", stderr)
	}
}

// TestCheckAsIndentedCommentKeepsSentenceStart checks that the first word of an indented
// comment is still read as the start of a sentence. Dropping an opener like "It is
// important to note that" leaves the next word to open the sentence, so its replacement
// must be capitalized, as it is for a comment at the top of the file.
func TestCheckAsIndentedCommentKeepsSentenceStart(t *testing.T) {
	src := "package main\n\nfunc f() {\n\t// It is important to note that we delve into the map here.\n}\n"
	stdout, _, err := runCLI(t, []string{"check", "--json", "--as", ".go"}, src)
	if !errors.Is(err, errFindings) {
		t.Fatalf("err = %v, want errFindings", err)
	}
	if !strings.Contains(stdout, `"match":"It is important to note that w","replacement":"W"`) {
		t.Errorf("stdout = %s, want the opener replaced by a capital W", stdout)
	}
}

// TestCheckSourceColumns checks that a finding in a comment is placed by rune column in the
// original file, whether the file comes by path or through stdin with --as. The mask blanks
// each byte of code to a space, so a multibyte rune of code before the comment once pushed
// the reported column right by its extra bytes.
func TestCheckSourceColumns(t *testing.T) {
	src := "package x\n\nvar s = \"ééé\" // We delve here.\n"
	tests := []struct {
		WantText string
		WantJSON string
		Args     []string
		In       string
	}{{ // Test 0: A source file read by path.
		Args: []string{"check"}, WantText: ":3:21 word:delve",
	}, { // Test 1: The same source read from stdin with --as.
		Args: []string{"check", "--json", "--as", ".go"}, In: src, WantJSON: `"line":3,"col":21`,
	}}
	for testNum, test := range tests {
		t.Run(fmt.Sprintf("test %d", testNum), func(t *testing.T) {
			args := test.Args
			if test.In == "" {
				args = append(args, writeTemp(t, t.TempDir(), "x.go", src))
			}
			stdout, stderr, err := runCLI(t, args, test.In)
			if !errors.Is(err, errFindings) {
				t.Fatalf("err = %v, want errFindings", err)
			}
			if test.WantText != "" && !strings.Contains(stderr, test.WantText) {
				t.Errorf("stderr = %q, want %q", stderr, test.WantText)
			}
			if test.WantJSON != "" && !strings.Contains(stdout, test.WantJSON) {
				t.Errorf("stdout = %s, want %s", stdout, test.WantJSON)
			}
		})
	}
}

// TestAttackWriteSkipsSource checks that attack -w leaves a source file byte for byte
// intact, while attack without -w still reports on its comments. The attack reads a source
// file as its comment mask, so writing the result once blanked every line of code.
func TestAttackWriteSkipsSource(t *testing.T) {
	src := "package main\n\n// We delve into a robust plan.\nfunc F() int { return 42 }\n"
	goFile := writeTemp(t, t.TempDir(), "main.go", src)

	_, stderr, err := runCLI(t, []string{"attack", "-w", goFile}, "")
	if err != nil {
		t.Fatalf("attack -w err = %v, want nil with the file skipped", err)
	}
	if !strings.Contains(stderr, "skipping "+goFile) {
		t.Errorf("stderr = %q, want a skip warning for %q", stderr, goFile)
	}
	got, err := os.ReadFile(filepath.Clean(goFile))
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	if string(got) != src {
		t.Errorf("attack -w changed a source file:\n got %q\nwant %q", got, src)
	}

	stdout, _, err := runCLI(t, []string{"attack", goFile}, "")
	if err != nil {
		t.Fatalf("attack err = %v, want nil", err)
	}
	if !strings.Contains(stdout, "word:delve") {
		t.Errorf("stdout = %q, want the comment's tells in the report", stdout)
	}
}

// TestCheckSkipsDataFile checks that a file type with no comments still skips with a
// warning.
func TestCheckSkipsDataFile(t *testing.T) {
	dir := t.TempDir()
	jsonFile := writeTemp(t, dir, "cfg.json", "{\"note\": \"a robust plan\"}\n")
	_, stderr, err := runCLI(t, []string{"check", jsonFile}, "")
	if err != nil {
		t.Fatalf("err = %v, want nil for a skipped data file", err)
	}
	if !strings.Contains(stderr, "skipping "+jsonFile) {
		t.Errorf("stderr = %q, want a skip warning for %q", stderr, jsonFile)
	}
}

// TestFixRefusesSource checks that fix to stdout refuses a source file outright, and that
// fix --write skips it and leaves the file byte for byte intact.
func TestFixRefusesSource(t *testing.T) {
	dir := t.TempDir()
	src := "package main\n\nfunc main() {\n\tif n := len(os.Args); n > 0 {\n\t\treturn\n\t}\n}\n"
	goFile := writeTemp(t, dir, "main.go", src)

	if _, _, err := runCLI(t, []string{"fix", goFile}, ""); err == nil {
		t.Fatal("fix on a source file returned nil, want an error")
	}

	_, stderr, err := runCLI(t, []string{"fix", "--write", goFile}, "")
	if err != nil {
		t.Fatalf("fix --write err = %v, want nil with the file skipped", err)
	}
	if !strings.Contains(stderr, "skipping "+goFile) {
		t.Errorf("stderr = %q, want a skip warning for %q", stderr, goFile)
	}
	got, err := os.ReadFile(filepath.Clean(goFile))
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	if string(got) != src {
		t.Errorf("fix --write changed a source file:\n got %q\nwant %q", got, src)
	}
}

// TestScoreScansComments checks that score on a source file rates its comments, so a
// slop-commented file scores above a tersely commented one.
func TestScoreScansComments(t *testing.T) {
	dir := t.TempDir()
	sloppy := writeTemp(t, dir, "sloppy.go",
		"package gen\n\n// In summary, this comprehensive solution leverages robust synergy.\nvar x = 1\n")
	terse := writeTemp(t, dir, "terse.go",
		"package gen\n\n// x counts retries.\nvar x = 1\n")
	stdout, _, err := runCLI(t, []string{"score", sloppy, terse}, "")
	if err != nil {
		t.Fatalf("err = %v, want nil without a gate", err)
	}
	if !strings.Contains(stdout, sloppy+": ") || !strings.Contains(stdout, terse+": 0") {
		t.Errorf("stdout = %q, want a high score for the sloppy comments and 0 for the terse ones", stdout)
	}
}

// TestCheckAsReadsStdinComments checks that --as reads stdin as a source file of the named
// type: findings come from the comments alone and carry the offsets of the original text,
// which is how an editor draws them on an unsaved buffer.
func TestCheckAsReadsStdinComments(t *testing.T) {
	src := "package main\n" +
		"\n" +
		"// This delivers a comprehensive solution.\n" +
		"func main() {\n" +
		"\tcomprehensive := 1     // twelve\n" +
		"\ts := \"a robust plan\"\n" +
		"\t_, _ = comprehensive, s\n" +
		"}\n"
	for _, as := range []string{".go", "go", "main.go"} {
		t.Run(as, func(t *testing.T) {
			stdout, _, err := runCLI(t, []string{"check", "--json", "--as", as}, src)
			if !errors.Is(err, errFindings) {
				t.Fatalf("err = %v, want errFindings for a tell in a comment", err)
			}
			var report struct {
				Findings []struct {
					Rule   string `json:"rule"`
					Match  string `json:"match"`
					Offset int    `json:"offset"`
					Line   int    `json:"line"`
				} `json:"findings"`
			}
			if err := json.Unmarshal([]byte(stdout), &report); err != nil {
				t.Fatalf("stdout is not JSON: %v\n%s", err, stdout)
			}
			if len(report.Findings) != 1 {
				t.Fatalf("findings = %+v, want exactly the comment's tell", report.Findings)
			}
			got := report.Findings[0]
			if got.Rule != "word:comprehensive" || got.Line != 3 {
				t.Errorf("finding = %+v, want word:comprehensive on line 3", got)
			}
			if src[got.Offset:got.Offset+len(got.Match)] != got.Match {
				t.Errorf("offset %d does not point at %q in the original text", got.Offset, got.Match)
			}
		})
	}
}

// TestCheckAsRejectsUnknownType checks that a type with no comment syntax fails loudly
// instead of quietly scanning the code as prose.
func TestCheckAsRejectsUnknownType(t *testing.T) {
	_, _, err := runCLI(t, []string{"check", "--as", ".txt"}, "a comprehensive plan\n")
	if err == nil || !strings.Contains(err.Error(), "no comment syntax") {
		t.Errorf("err = %v, want a no-comment-syntax error", err)
	}
}

// TestScoreAsRatesComments checks that score --as rates the comments of piped source and
// nothing else, so the same code with and without a slop comment scores differently.
func TestScoreAsRatesComments(t *testing.T) {
	sloppy := "x = 1  # In summary, this comprehensive solution leverages robust synergy.\n"
	stdout, _, err := runCLI(t, []string{"score", "--as", ".py"}, sloppy)
	if err != nil {
		t.Fatalf("err = %v, want nil without a gate", err)
	}
	if strings.HasPrefix(stdout, "0 ") {
		t.Errorf("stdout = %q, want a nonzero score for the sloppy comment", stdout)
	}
	stdout, _, err = runCLI(t, []string{"score", "--as", ".py"}, "comprehensive = 1\n")
	if err != nil {
		t.Fatalf("err = %v, want nil without a gate", err)
	}
	if !strings.HasPrefix(stdout, "0 ") {
		t.Errorf("stdout = %q, want 0 for code with no comment", stdout)
	}
}
