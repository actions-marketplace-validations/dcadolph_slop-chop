package source

import (
	"fmt"
	"strings"
	"testing"
)

// TestMask checks the comment mask across language families: comment content
// survives at its exact offsets, code and strings blank to spaces, and the mask never
// changes the text's length or line breaks.
func TestMask(t *testing.T) {
	t.Parallel()
	tests := []struct {
		Path     string
		In       string
		WantKeep []string
		WantDrop []string
	}{{ // Test 0: Go line comment kept, code and string dropped.
		Path:     "a.go",
		In:       "// a robust plan\nvar s = \"seamless code\"\n",
		WantKeep: []string{"a robust plan"},
		WantDrop: []string{"var", "seamless"},
	}, { // Test 1: A comment marker inside a string is code.
		Path:     "a.go",
		In:       "s := \"// not a comment\" // a real comment\n",
		WantKeep: []string{"a real comment"},
		WantDrop: []string{"not a comment"},
	}, { // Test 2: Block comment content spans lines.
		Path:     "a.c",
		In:       "/* first line\nsecond line */ int x;\n",
		WantKeep: []string{"first line", "second line"},
		WantDrop: []string{"int x", "/*", "*/"},
	}, { // Test 3: Python hash comment kept, triple-quoted docstring dropped.
		Path:     "a.py",
		In:       "\"\"\"a # docstring\"\"\"\nx = 1  # trailing note\n",
		WantKeep: []string{"trailing note"},
		WantDrop: []string{"docstring", "x = 1"},
	}, { // Test 4: A Rust lifetime apostrophe does not swallow the comment.
		Path:     "a.rs",
		In:       "fn f<'a>(x: &'a str) {} // lifetime note\n",
		WantKeep: []string{"lifetime note"},
		WantDrop: []string{"str"},
	}, { // Test 5: SQL double-dash comment kept, quoted marker dropped.
		Path:     "a.sql",
		In:       "SELECT '-- not this' AS x -- but this\n",
		WantKeep: []string{"but this"},
		WantDrop: []string{"not this", "SELECT"},
	}, { // Test 6: A Makefile scans with hash comments.
		Path:     "Makefile",
		In:       "all: # build everything\n\tgo build .\n",
		WantKeep: []string{"build everything"},
		WantDrop: []string{"go build"},
	}, { // Test 7: An unterminated string gives up at the line break.
		Path:     "a.go",
		In:       "s := \"broken\n// still a comment\n",
		WantKeep: []string{"still a comment"},
		WantDrop: []string{"broken"},
	}, { // Test 8: A JS template literal holding a comment marker is code.
		Path:     "a.ts",
		In:       "const u = `http://example.com`; // a real note\n",
		WantKeep: []string{"a real note"},
		WantDrop: []string{"example.com", "const"},
	}, { // Test 9: An escaped backtick does not close a JS template early.
		Path:     "a.js",
		In:       "const t = `a \\` // not a comment`; // kept\n",
		WantKeep: []string{"kept"},
		WantDrop: []string{"not a comment"},
	}, { // Test 10: An unterminated JS template runs to the end of the file.
		Path:     "a.js",
		In:       "const t = `never closed // hidden\nmore code\n",
		WantDrop: []string{"hidden", "more code"},
	}, { // Test 11: An unterminated block comment runs to the end of the text.
		Path:     "a.c",
		In:       "int x; /* never closed\nstill comment text\n",
		WantKeep: []string{"never closed", "still comment text"},
		WantDrop: []string{"int x"},
	}, { // Test 12: A backslash escape keeps a quote inside a string.
		Path:     "a.go",
		In:       "s := \"say \\\" // not a comment\" // a kept note\n",
		WantKeep: []string{"a kept note"},
		WantDrop: []string{"not a comment"},
	}, { // Test 13: A lifetime apostrophe at the end of a line does not swallow the next line.
		Path:     "a.rs",
		In:       "fn f<'a>\n// next line note\n",
		WantKeep: []string{"next line note"},
		WantDrop: []string{"fn f"},
	}, { // Test 14: A shell script keeps its hash comment and drops the command.
		Path:     "a.sh",
		In:       "echo \"# not this\" # but this\n",
		WantKeep: []string{"but this"},
		WantDrop: []string{"not this", "echo"},
	}, { // Test 15: PHP reads both comment styles.
		Path:     "a.php",
		In:       "$a = 1; // slash note\n$b = 2; # hash note\n",
		WantKeep: []string{"slash note", "hash note"},
		WantDrop: []string{"$a", "$b"},
	}, { // Test 16: A CRLF file keeps its line breaks and its comment.
		Path:     "a.go",
		In:       "x := 1 // crlf note\r\ny := 2\r\n",
		WantKeep: []string{"crlf note"},
		WantDrop: []string{"x := 1", "y := 2"},
	}, { // Test 17: A deeply indented comment on the last line, with no final newline.
		Path:     "a.go",
		In:       "func f() {\n\t\t\t// last note",
		WantKeep: []string{"last note"},
		WantDrop: []string{"func f"},
	}}
	for testNum, test := range tests {
		t.Run(fmt.Sprintf("test %d", testNum), func(t *testing.T) {
			t.Parallel()
			got, ok := Mask(test.Path, test.In)
			if !ok {
				t.Fatalf("Mask(%q) not scannable, want ok", test.Path)
			}
			if len(got) != len(test.In) {
				t.Fatalf("mask changed length: %d != %d", len(got), len(test.In))
			}
			if strings.Count(got, "\n") != strings.Count(test.In, "\n") {
				t.Fatalf("mask changed line count")
			}
			for _, want := range test.WantKeep {
				at := strings.Index(test.In, want)
				if at < 0 || !strings.Contains(got, want) {
					t.Errorf("mask lost %q (%q)", want, got)
					continue
				}
				if got[at:at+len(want)] != want {
					t.Errorf("mask moved %q off its original offset (%q)", want, got)
				}
			}
			for _, drop := range test.WantDrop {
				if strings.Contains(got, drop) {
					t.Errorf("mask kept %q, want it blanked (%q)", drop, got)
				}
			}
		})
	}

	// A type with no comments is not scannable.
	if _, ok := Mask("data.json", "{}"); ok {
		t.Error("Mask on .json = ok, want not scannable")
	}
}

// TestMaskKeepsIndentedCommentsReadable checks that a comment nested inside a block does
// not open its line with four spaces or a tab. The prose engine skips such a line as an
// indented code block, which once hid every comment inside a function. The blank lines that
// stand for code must stay all spaces, since the engine reads those as paragraph breaks,
// and the mask must stay exactly as long as the original.
func TestMaskKeepsIndentedCommentsReadable(t *testing.T) {
	t.Parallel()
	in := "func main() {\n\t// a robust plan\n\tx := 1\n\t\t// a deeper note\n}\n"
	got, ok := Mask("a.go", in)
	if !ok {
		t.Fatal("Mask(a.go) not scannable, want ok")
	}
	if len(got) != len(in) {
		t.Fatalf("mask changed length: %d != %d", len(got), len(in))
	}
	for i, line := range strings.Split(got, "\n") {
		if strings.TrimSpace(line) == "" {
			if strings.Contains(line, "\t") {
				t.Errorf("line %d is blank but holds a tab, so it is not a paragraph break: %q", i, line)
			}
			continue
		}
		if strings.HasPrefix(line, "    ") || strings.HasPrefix(line, "\t") {
			t.Errorf("line %d opens with an indent the engine reads as code: %q", i, line)
		}
	}
	for _, want := range []string{"a robust plan", "a deeper note"} {
		at := strings.Index(in, want)
		if at < 0 || got[at:at+len(want)] != want {
			t.Errorf("mask moved %q off its original offset (%q)", want, got)
		}
	}
}
