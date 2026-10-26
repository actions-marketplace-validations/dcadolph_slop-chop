package source

import (
	"slices"

	"github.com/dcadolph/slop-chop/sanitize"
)

// CheckComments runs san over the comments of text, read as a source file of the type as
// names: a path, an extension with or without its dot like .go or go, or a file name like
// Makefile. Cleanup findings are dropped, since a spacing or punctuation cleanup inside a
// comment is neither a tell nor something a fix rewrites there. ok is false when the type
// has no comment syntax. Findings keep the byte offsets and lines of text, since the mask
// keeps every byte in place. Columns count the mask's spaces, one per byte, so a caller that
// reports columns sets them on text with sanitize.Locate.
func CheckComments(san *sanitize.Sanitizer, as, text string) ([]sanitize.Finding, bool) {
	if san == nil {
		panic("source.CheckComments: sanitizer required")
	}
	for _, name := range []string{as, "." + as} {
		masked, ok := Mask(name, text)
		if !ok {
			continue
		}
		return slices.DeleteFunc(san.Check(masked), func(f sanitize.Finding) bool {
			return sanitize.TidyRule(f.Rule)
		}), true
	}
	return nil, false
}
