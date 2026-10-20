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
	}, { // Test 3: Python hash comment kept, assigned triple-quoted string dropped.
		Path:     "a.py",
		In:       "s = \"\"\"a # string\"\"\"\nx = 1  # trailing note\n",
		WantKeep: []string{"trailing note"},
		WantDrop: []string{"a # string", "x = 1"},
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
			checkMask(t, test.Path, test.In, test.WantKeep, test.WantDrop)
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

// TestMaskPythonDocstrings checks that a docstring reads as prose with its quotes blanked,
// while an assigned string, an argument string, and every other triple string stay code.
func TestMaskPythonDocstrings(t *testing.T) {
	t.Parallel()
	tests := []struct {
		In       string
		WantKeep []string
		WantDrop []string
	}{{ // Test 0: Module, class, and r-prefixed method docstrings read, the strings do not.
		In: "#!/usr/bin/env python3\n" +
			"\"\"\"Module docstring that will leverage the cache.\"\"\"\n\n" +
			"import os\n\n\n" +
			"class Store:\n" +
			"    \"\"\"Class docstring with a robust design.\"\"\"\n\n" +
			"    def load(self):\n" +
			"        r\"\"\"Method docstring that is seamlessly fast.\"\"\"\n" +
			"        x = \"\"\"assigned string holding powerful text\"\"\"\n" +
			"        foo(\n" +
			"            \"\"\"argument string holding crucial text\"\"\"\n" +
			"        )\n" +
			"        # a comment that will delve into it\n" +
			"        return x\n",
		WantKeep: []string{
			"Module docstring that will leverage the cache.", "Class docstring with a robust design.",
			"Method docstring that is seamlessly fast.", "a comment that will delve into it",
		},
		WantDrop: []string{`"""`, `r"""`, "assigned string", "argument string", "import os", "foo("},
	}, { // Test 1: A single-quoted docstring spans lines under a def with a trailing comment.
		In:       "def f():  # helper\n    '''First line.\n\n    Second line.\n    '''\n",
		WantKeep: []string{"First line.", "Second line.", "helper"},
		WantDrop: []string{"def f", "'''"},
	}, { // Test 2: A '#' inside a string on the header line is not a comment.
		In:       "def f(a=\"#\"):\n    \"\"\"Header docstring.\"\"\"\n",
		WantKeep: []string{"Header docstring."},
		WantDrop: []string{"def f"},
	}, { // Test 3: Comment lines between the header and the docstring are passed over.
		In:       "# -*- coding: utf-8 -*-\n# Copyright note\n\n\"\"\"Module text.\"\"\"\n",
		WantKeep: []string{"Module text.", "Copyright note"},
	}, { // Test 4: A bare string after a line of code stays a string.
		In:       "x = 1\n\"\"\"not a docstring\"\"\"\n",
		WantDrop: []string{"not a docstring"},
	}, { // Test 5: f, b, and two-letter prefixes are never docstrings.
		In:       "def f():\n    f\"\"\"formatted\"\"\"\ndef g():\n    rb\"\"\"raw bytes\"\"\"\n",
		WantDrop: []string{"formatted", "raw bytes"},
	}, { // Test 6: A CRLF header still ends with a colon.
		In:       "class C:\r\n    u\"\"\"Windows docstring.\"\"\"\r\n",
		WantKeep: []string{"Windows docstring."},
		WantDrop: []string{"class C"},
	}, { // Test 7: An unclosed docstring runs to the end of the text.
		In:       "def f():\n    \"\"\"never closed\nstill prose\n",
		WantKeep: []string{"never closed", "still prose"},
	}}
	for testNum, test := range tests {
		t.Run(fmt.Sprintf("test %d", testNum), func(t *testing.T) {
			t.Parallel()
			checkMask(t, "a.py", test.In, test.WantKeep, test.WantDrop)
		})
	}
}

// checkMask masks in as path and checks that the mask keeps its length and line breaks, that
// each of keep survives at its original offset, and that none of drop survives.
func checkMask(t *testing.T, path, in string, keep, drop []string) {
	t.Helper()
	got, ok := Mask(path, in)
	if !ok {
		t.Fatalf("Mask(%q) not scannable, want ok", path)
	}
	if len(got) != len(in) {
		t.Fatalf("mask changed length: %d != %d", len(got), len(in))
	}
	if strings.Count(got, "\n") != strings.Count(in, "\n") {
		t.Fatalf("mask changed line count")
	}
	for _, want := range keep {
		at := strings.Index(in, want)
		if at < 0 || !strings.Contains(got, want) {
			t.Errorf("mask lost %q (%q)", want, got)
			continue
		}
		if got[at:at+len(want)] != want {
			t.Errorf("mask moved %q off its original offset (%q)", want, got)
		}
	}
	for _, d := range drop {
		if strings.Contains(got, d) {
			t.Errorf("mask kept %q, want it blanked (%q)", d, got)
		}
	}
}
