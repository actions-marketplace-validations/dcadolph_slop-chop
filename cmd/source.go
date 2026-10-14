package cmd

import (
	"fmt"
	"io"

	"github.com/dcadolph/slop-chop/internal/source"
	"github.com/dcadolph/slop-chop/sanitize"
)

// warnSourceSkip tells the user a file was skipped as source code and how to run it
// anyway.
func warnSourceSkip(stderr io.Writer, path string) {
	_, _ = fmt.Fprintf(stderr,
		"slop-chop: skipping %s: source code, not prose; pipe it through stdin to run anyway\n", path)
}

// readProse returns the text of path ready for the prose rules: a prose file as it is,
// and a source file as its comment mask, so check and score read the comments without
// exposing the code. ok is false when the file was skipped because its type has no
// comments to scan. An empty path reads stdin unmasked.
func readProse(path string, stdin io.Reader, stderr io.Writer) (text string, ok bool, err error) {
	text, err = readInput(path, stdin)
	if err != nil {
		return "", false, err
	}
	if path == "" || !source.Is(path) {
		return text, true, nil
	}
	masked, scannable := source.Mask(path, text)
	if !scannable {
		warnSourceSkip(stderr, path)
		return "", false, nil
	}
	return masked, true, nil
}

// maskStdin returns text read as a source file of the type --as names, so check and score
// scan its comments and never its code. The type may be an extension with or without its
// dot, like .go or go, or a file name like Makefile. Findings keep the offsets of the
// original text, which is what lets an editor draw them on the right characters.
func maskStdin(text, as string) (string, error) {
	for _, name := range []string{as, "." + as} {
		if masked, ok := source.Mask(name, text); ok {
			return masked, nil
		}
	}
	return "", fmt.Errorf("--as %q: no comment syntax is known for that file type", as)
}

// dropTidyFindings removes cleanup findings from a source file's report. The comments
// are scanned for tells, and a spacing or punctuation cleanup inside one is neither a
// tell nor something fix will ever rewrite there.
func dropTidyFindings(findings []sanitize.Finding) []sanitize.Finding {
	out := findings[:0]
	for _, f := range findings {
		if !sanitize.TidyRule(f.Rule) {
			out = append(out, f)
		}
	}
	return out
}

// countFixable counts the findings fix would rewrite, the ones carrying a replacement.
func countFixable(findings []sanitize.Finding) int {
	n := 0
	for _, f := range findings {
		if f.Replacement != nil {
			n++
		}
	}
	return n
}
