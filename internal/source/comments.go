package source

import (
	"bytes"
	"path/filepath"
	"strings"
)

// Comment scanning lets check and score read the prose inside a source file, its
// comments, without exposing the code to prose rules. The file is masked rather than
// excerpted: every byte that is not comment text becomes a space and line breaks stay,
// so the masked copy is byte for byte as long as the original and every finding's
// offset, line, and column point at the real file. Code lines blank to all-space lines,
// which the engine reads as paragraph breaks, so a structural pattern never joins two
// unrelated comments.

// commentStyle describes how one language family writes comments and strings. String
// forms matter because a comment marker inside a string literal is code, not a comment.
type commentStyle struct {
	// line lists the markers that start a comment running to the end of the line.
	line []string
	// blockOpen and blockClose delimit a multi-line comment, or are empty when the
	// family has none.
	blockOpen, blockClose string
	// rawBacktick marks backtick-delimited raw strings, the Go form.
	rawBacktick bool
	// templateBacktick marks backtick-delimited template literals, the JS form:
	// multiline like a raw string but with backslash escapes. Without it a template
	// holding "//" starts a phantom comment that copies code into the prose scan.
	templateBacktick bool
	// triple marks Python-style triple-quoted strings.
	triple bool
	// shortSingle treats a single quote as a character literal only when it closes
	// within a few bytes, so a Rust lifetime like 'a does not swallow the rest of the
	// line as a string.
	shortSingle bool
}

// commentStyles maps an extension to its comment style. A source extension missing here,
// like .json or .csv, has no comments worth scanning and stays skipped.
//
//nolint:gochecknoglobals // Immutable lookup.
var commentStyles = map[string]commentStyle{
	".c": slashStyle, ".cc": slashStyle, ".cpp": slashStyle, ".cs": slashStyle,
	".dart": slashStyle, ".h": slashStyle, ".hpp": slashStyle, ".java": slashStyle,
	".js": jsStyle, ".jsx": jsStyle, ".kt": slashStyle, ".php": phpStyle,
	".proto": slashStyle, ".rs": slashStyle, ".scala": slashStyle, ".swift": slashStyle,
	".ts": jsStyle, ".tsx": jsStyle,
	".go":   {line: []string{"//"}, blockOpen: "/*", blockClose: "*/", rawBacktick: true, shortSingle: true},
	".py":   {line: []string{"#"}, triple: true},
	".bash": hashStyle, ".fish": hashStyle, ".pl": hashStyle, ".r": hashStyle,
	".rb": hashStyle, ".sh": hashStyle, ".tf": hashStyle, ".toml": hashStyle,
	".yaml": hashStyle, ".yml": hashStyle, ".zsh": hashStyle,
	".lua": {line: []string{"--"}},
	".sql": {line: []string{"--"}, blockOpen: "/*", blockClose: "*/"},
}

// The shared style values the table above reuses.
//
//nolint:gochecknoglobals // Immutable values.
var (
	slashStyle = commentStyle{line: []string{"//"}, blockOpen: "/*", blockClose: "*/", shortSingle: true}
	jsStyle    = commentStyle{line: []string{"//"}, blockOpen: "/*", blockClose: "*/", shortSingle: true, templateBacktick: true}
	hashStyle  = commentStyle{line: []string{"#"}}
	phpStyle   = commentStyle{line: []string{"//", "#"}, blockOpen: "/*", blockClose: "*/"}
)

// styleForPath returns the comment style for a file, or ok false when its type has no
// scannable comments.
func styleForPath(path string) (commentStyle, bool) {
	if s, ok := commentStyles[strings.ToLower(filepath.Ext(path))]; ok {
		return s, true
	}
	if names[strings.ToLower(filepath.Base(path))] {
		return hashStyle, true
	}
	return commentStyle{}, false
}

// Mask returns text with everything except comment content blanked to spaces.
// Line breaks survive, so the result is exactly as long as the input and every offset
// in it points at the same place in the original file. The second result is false when
// the file's type has no comments to scan.
func Mask(path, text string) (string, bool) {
	style, ok := styleForPath(path)
	if !ok {
		return "", false
	}
	out := make([]byte, len(text))
	for i := range out {
		switch text[i] {
		case '\n', '\r':
			out[i] = text[i]
		default:
			out[i] = ' '
		}
	}
	for i := 0; i < len(text); {
		i = nextToken(text, out, i, style)
	}
	unindent(out)
	return string(out), true
}

// unindent turns the fourth blank in front of comment text into a tab on any line that
// would otherwise open with four or more spaces. The prose engine reads four leading spaces
// after a blank line as a markdown indented code block and skips it, and every code line is
// blank in the mask, so that hid each comment nested inside a function or a block. The tab
// breaks the run of spaces, so the line is not an indent, yet it is still blank to the
// engine's sentence-start check, so the first word of a comment is capitalized correctly
// after a fix. One byte changes in place, so offsets still point at the real file. A line
// with no comment text on it stays all spaces, because the engine reads those as paragraph
// breaks.
func unindent(out []byte) {
	for start := 0; start < len(out); {
		n := bytes.IndexByte(out[start:], '\n')
		if n < 0 {
			n = len(out) - start
		}
		line := out[start : start+n]
		lead := 0
		for lead < len(line) && line[lead] == ' ' {
			lead++
		}
		if lead >= 4 && len(bytes.TrimRight(line[lead:], "\r")) > 0 {
			line[3] = '\t'
		}
		start += n + 1
	}
}

// nextToken consumes one lexical token starting at i, copying comment content into out,
// and returns the offset to continue from.
func nextToken(text string, out []byte, i int, style commentStyle) int {
	for _, m := range style.line {
		if strings.HasPrefix(text[i:], m) {
			return copyUntil(text, out, i+len(m), "\n")
		}
	}
	if style.blockOpen != "" && strings.HasPrefix(text[i:], style.blockOpen) {
		return copyUntil(text, out, i+len(style.blockOpen), style.blockClose)
	}
	switch c := text[i]; {
	case style.triple && (strings.HasPrefix(text[i:], `"""`) || strings.HasPrefix(text[i:], "'''")):
		if docstring(text, out, i) {
			return copyUntil(text, out, i+3, text[i:i+3])
		}
		return skipString(text, i+3, text[i:i+3], false)
	case c == '"':
		return skipString(text, i+1, `"`, true)
	case c == '\'':
		if style.shortSingle && !closesWithin(text, i+1, '\'', 8) {
			return i + 1
		}
		return skipString(text, i+1, "'", true)
	case c == '`' && style.rawBacktick:
		return skipString(text, i+1, "`", false)
	case c == '`' && style.templateBacktick:
		return skipTemplate(text, i+1)
	}
	return i + 1
}

// docstring reports whether the triple quote at i opens a Python docstring, which reads as
// prose rather than as a string. The quotes must open their line, after at most a one-letter
// r or u prefix, and the nearest earlier line of code must end with a colon, the header of a
// def, class, or other block, or there must be no earlier code at all, as above a module
// docstring under a shebang. Comment lines are passed over. Everything else, like an
// assigned string or one passed as an argument, stays a string.
func docstring(text string, out []byte, i int) bool {
	start := strings.LastIndexByte(text[:i], '\n') + 1
	lead := strings.TrimLeft(text[start:i], " \t")
	if len(lead) > 1 || (lead != "" && !strings.ContainsAny(lead, "rRuU")) {
		return false
	}
	for start > 0 {
		end := start - 1
		start = strings.LastIndexByte(text[:end], '\n') + 1
		if code := strings.TrimSpace(codeOf(text, out, start, end)); code != "" {
			return strings.HasSuffix(code, ":")
		}
	}
	return true
}

// codeOf returns the line text[start:end] without its trailing comment. out holds the mask
// so far, which tells a comment marker from a '#' inside a string: the mask copied
// everything after a marker, and blanked the rest of a string.
func codeOf(text string, out []byte, start, end int) string {
	for p := start; p < end; p++ {
		if text[p] == '#' && out[p] == ' ' && string(out[p+1:end]) == text[p+1:end] {
			return text[start:p]
		}
	}
	return text[start:end]
}

// copyUntil copies text into out from start until the closer, leaving the closer itself
// blanked, and returns the offset after it. A line comment's closer is the newline, which
// the mask already preserves. An unclosed block runs to the end of the text.
func copyUntil(text string, out []byte, start int, closer string) int {
	end := strings.Index(text[start:], closer)
	if end < 0 {
		copy(out[start:], text[start:])
		return len(text)
	}
	copy(out[start:], text[start:start+end])
	if closer == "\n" {
		return start + end
	}
	return start + end + len(closer)
}

// skipString consumes a string literal opened before start and closed by closer, honoring
// backslash escapes when escapes is true. An unterminated literal runs to the end of the
// line, matching how most languages recover, so one stray quote cannot hide the rest of
// the file's comments.
func skipString(text string, start int, closer string, escapes bool) int {
	multiline := len(closer) == 3 || closer == "`"
	for i := start; i < len(text); i++ {
		if escapes && text[i] == '\\' {
			i++
			continue
		}
		if !multiline && text[i] == '\n' {
			return i
		}
		if strings.HasPrefix(text[i:], closer) {
			return i + len(closer)
		}
	}
	return len(text)
}

// skipTemplate consumes a JS template literal opened before start: multiline, closed by
// a backtick, with backslash escapes honored.
func skipTemplate(text string, start int) int {
	for i := start; i < len(text); i++ {
		switch text[i] {
		case '\\':
			i++
		case '`':
			return i + 1
		}
	}
	return len(text)
}

// closesWithin reports whether c appears in text within n bytes after start, before any
// line break.
func closesWithin(text string, start int, c byte, n int) bool {
	for i := start; i < len(text) && i < start+n; i++ {
		if text[i] == c {
			return true
		}
		if text[i] == '\n' {
			return false
		}
	}
	return false
}
