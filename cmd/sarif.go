package cmd

import (
	"fmt"
	"net/url"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/dcadolph/slop-chop/sanitize"
)

// SARIF output turns check findings into a log GitHub code scanning and other SARIF readers
// can show as alerts. The score gate stays where it is: the log carries findings, and no
// result is ever an error, because code scanning fails a pull request check on errors by
// default and that would be a second gate disagreeing with the score.
const (
	sarifSchema   = "https://docs.oasis-open.org/sarif/sarif/v2.1.0/errata01/os/schemas/sarif-schema-2.1.0.json"
	sarifInfoURI  = "https://slop-chop.com/"
	sarifTellsURI = "https://slop-chop.com/TELLS.html"
	// sarifMatchRunes caps how much of a match a result message quotes, since a structural
	// walker can match whole paragraphs.
	sarifMatchRunes = 80
	// sarifNameMax is the longest rule name GitHub accepts, in bytes.
	sarifNameMax = 255
)

// sarifClassTitles names each rule class for the short description of its rules.
//
//nolint:gochecknoglobals // Immutable lookup.
var sarifClassTitles = map[string]string{
	"word":       "Buzzword",
	"phrase":     "Stock phrase",
	"char":       "Character",
	"structural": "Structural tell",
	"replace":    "Word swap",
	"drop":       "Word cut",
	"case":       "Capitalization",
	"regex":      "Regex rule",
	"spelling":   "Spelling",
	"tidy":       "Cleanup",
}

// sarifLog is the root of a SARIF 2.1.0 log.
type sarifLog struct {
	// Schema is the URI of the SARIF JSON schema the log follows.
	Schema string `json:"$schema"`
	// Version is the SARIF version.
	Version string `json:"version"`
	// Runs holds the single run a check makes.
	Runs []sarifRun `json:"runs"`
}

// sarifRun is one run of one tool over every checked file.
type sarifRun struct {
	// Tool describes slop-chop and the rules that fired.
	Tool sarifTool `json:"tool"`
	// ColumnKind says columns count Unicode code points, which is what the engine counts.
	ColumnKind string `json:"columnKind"`
	// Results holds one result per finding, in file then offset order.
	Results []sarifResult `json:"results"`
}

// sarifTool wraps the driver, the component that produced the results.
type sarifTool struct {
	// Driver is slop-chop itself.
	Driver sarifDriver `json:"driver"`
}

// sarifDriver names the tool, its version, and the rules its results point at.
type sarifDriver struct {
	// Name is the tool name.
	Name string `json:"name"`
	// Version is the build version when it is not a semantic version, such as dev.
	Version string `json:"version,omitempty"`
	// SemanticVersion is the release version without its leading v.
	SemanticVersion string `json:"semanticVersion,omitempty"`
	// InformationURI is the tool's home page.
	InformationURI string `json:"informationUri"`
	// Rules holds a descriptor for every rule that fired, sorted by ID.
	Rules []sarifRule `json:"rules"`
}

// sarifRule describes one rule.
type sarifRule struct {
	// ID is the engine's rule name, the same string --json prints and scoreWeights takes.
	ID string `json:"id"`
	// Name is the Pascal-case friendly name GitHub shows when filtering alerts by rule.
	Name string `json:"name"`
	// ShortDescription is a readable title.
	ShortDescription sarifText `json:"shortDescription"`
	// FullDescription says why the rule fires.
	FullDescription sarifText `json:"fullDescription"`
	// Help repeats the reason with what a structural walker reads, and links the catalog.
	Help sarifText `json:"help"`
	// HelpURI is the published catalog of tells.
	HelpURI string `json:"helpUri"`
	// DefaultConfiguration carries the rule's level.
	DefaultConfiguration sarifConfig `json:"defaultConfiguration"`
	// Properties carries the tags and severity code scanning filters on.
	Properties sarifRuleProps `json:"properties"`
}

// sarifText is a SARIF message: plain text with an optional Markdown form.
type sarifText struct {
	// Text is the plain text form.
	Text string `json:"text"`
	// Markdown is the rich form, shown instead of Text where a reader supports it.
	Markdown string `json:"markdown,omitempty"`
}

// sarifConfig is a rule's default configuration.
type sarifConfig struct {
	// Level is warning for a rule with score weight and note for one without.
	Level string `json:"level"`
}

// sarifRuleProps holds the rule properties code scanning reads.
type sarifRuleProps struct {
	// Tags are slop-chop and the rule's class.
	Tags []string `json:"tags"`
	// ProblemSeverity is the non-security severity code scanning shows on the alert.
	ProblemSeverity string `json:"problem.severity"`
}

// sarifResult is one finding.
type sarifResult struct {
	// RuleID is the rule that matched.
	RuleID string `json:"ruleId"`
	// RuleIndex is the rule's position in the driver's rules.
	RuleIndex int `json:"ruleIndex"`
	// Level matches the rule's default level.
	Level string `json:"level"`
	// Message quotes the match and says why it fired and what happens to it.
	Message sarifText `json:"message"`
	// Locations holds the one place the finding sits.
	Locations []sarifLocation `json:"locations"`
}

// sarifLocation wraps a physical location.
type sarifLocation struct {
	// PhysicalLocation is the file and region of the finding.
	PhysicalLocation sarifPhysical `json:"physicalLocation"`
}

// sarifPhysical is a file and a region in it.
type sarifPhysical struct {
	// ArtifactLocation names the file.
	ArtifactLocation sarifArtifact `json:"artifactLocation"`
	// Region is the span of the match.
	Region sarifRegion `json:"region"`
}

// sarifArtifact names a file by URI.
type sarifArtifact struct {
	// URI is relative to the working directory when the file sits under it.
	URI string `json:"uri"`
}

// sarifRegion is a span of lines and columns, one-based, with an exclusive end column.
type sarifRegion struct {
	// StartLine is the line the match starts on.
	StartLine int `json:"startLine"`
	// StartColumn is the code point column the match starts at.
	StartColumn int `json:"startColumn"`
	// EndLine is the line the match ends on, not counting a trailing line break.
	EndLine int `json:"endLine"`
	// EndColumn is the column just past the match's last code point.
	EndColumn int `json:"endColumn"`
	// Snippet is the file's text inside the region.
	Snippet sarifText `json:"snippet"`
}

// sarifFile is one checked file for the log.
type sarifFile struct {
	// URI is the file as the log names it.
	URI string
	// Raw is the file's original text, which every finding offset points into.
	Raw string
	// Findings are the file's findings in offset order.
	Findings []sanitize.Finding
}

// buildSARIF assembles one log covering every file. The rule set is whatever fired, since a
// profile, a preset, or a user regex can add rules, so the driver lists only those.
func buildSARIF(s *sanitize.Sanitizer, version string, files []sarifFile) sarifLog {
	var ids []string
	for _, file := range files {
		for _, f := range file.Findings {
			ids = append(ids, f.Rule)
		}
	}
	slices.Sort(ids)
	ids = slices.Compact(ids)

	walkers := make(map[string]string)
	for _, w := range sanitize.Walkers() {
		walkers[w.Name] = w.Reads
	}
	index := make(map[string]int, len(ids))
	rules := make([]sarifRule, 0, len(ids))
	for i, id := range ids {
		index[id] = i
		rules = append(rules, sarifRuleFor(s, id, walkers))
	}

	results := make([]sarifResult, 0)
	for _, file := range files {
		starts := lineStarts(file.Raw)
		for _, f := range file.Findings {
			i := index[f.Rule]
			results = append(results, sarifResult{
				RuleID:    f.Rule,
				RuleIndex: i,
				Level:     rules[i].DefaultConfiguration.Level,
				Message:   sarifText{Text: sarifMessage(f)},
				Locations: []sarifLocation{{PhysicalLocation: sarifPhysical{
					ArtifactLocation: sarifArtifact{URI: file.URI},
					Region:           sarifRegionFor(file.Raw, starts, f),
				}}},
			})
		}
	}

	driver := sarifDriver{Name: "slop-chop", InformationURI: sarifInfoURI, Rules: rules}
	if v := strings.TrimPrefix(version, "v"); v != "" && v != "dev" {
		driver.SemanticVersion = v
	} else {
		driver.Version = version
	}
	return sarifLog{
		Schema:  sarifSchema,
		Version: "2.1.0",
		Runs: []sarifRun{{
			Tool:       sarifTool{Driver: driver},
			ColumnKind: "unicodeCodePoints",
			Results:    results,
		}},
	}
}

// sarifRuleFor describes the rule named id. A colonless name is one of the cleanup passes.
func sarifRuleFor(s *sanitize.Sanitizer, id string, walkers map[string]string) sarifRule {
	class, name, ok := strings.Cut(id, ":")
	if !ok {
		class, name = "tidy", id
	}
	reason := sanitize.Reason(id)
	help, md := reason, reason
	if reads, ok := walkers[name]; ok && class == "structural" {
		help += " " + reads
		md += "\n\n" + reads
	}
	md += "\n\nEvery rule is listed in [the tells](" + sarifTellsURI + ")."
	level, severity := "note", "recommendation"
	if s.Weight(id) > 0 {
		level, severity = "warning", "warning"
	}
	return sarifRule{
		ID:                   id,
		Name:                 sarifName(class, name),
		ShortDescription:     sarifText{Text: sarifTitle(class, name)},
		FullDescription:      sarifText{Text: reason},
		Help:                 sarifText{Text: help, Markdown: md},
		HelpURI:              sarifTellsURI,
		DefaultConfiguration: sarifConfig{Level: level},
		Properties:           sarifRuleProps{Tags: []string{"slop-chop", class}, ProblemSeverity: severity},
	}
}

// sarifTitle names a rule for its short description. A character rule spells its code
// points, so an invisible character never renders as an empty title.
func sarifTitle(class, name string) string {
	title, ok := sarifClassTitles[class]
	if !ok {
		title = "Rule"
	}
	if class == "char" {
		name = codePoints(name)
	}
	return title + ": " + name
}

// sarifName builds the single Pascal-case identifier GitHub shows for a rule. It always matches
// ^(\p{Lu}[\p{Ll}\p{Nd}]+)+$ and never exceeds sarifNameMax bytes.
func sarifName(class, name string) string {
	title, ok := sarifClassTitles[class]
	if !ok {
		title = "Rule"
	}
	var b strings.Builder
	sarifPascal(&b, title, false)
	sarifPascal(&b, name, class == "char")
	return sarifClip(b.String())
}

// sarifPascal appends s to b as Pascal-case segments. With spellAll, every rune is spelled as
// U plus lowercase hex. Otherwise apostrophes are dropped, words split on every rune that is
// not a letter or digit, and runes that do not lowercase to a letter or digit are spelled.
func sarifPascal(b *strings.Builder, s string, spellAll bool) {
	if spellAll {
		for _, r := range s {
			fmt.Fprintf(b, "U%04x", r)
		}
		return
	}
	s = strings.NewReplacer("'", "", "\u2019", "").Replace(s)
	words := strings.FieldsFunc(s, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})
	for _, word := range words {
		var w strings.Builder
		for _, r := range word {
			l := unicode.ToLower(r)
			if unicode.IsLower(l) || unicode.IsDigit(l) {
				w.WriteRune(l)
			} else {
				fmt.Fprintf(&w, "U%04x", r)
			}
		}
		out := w.String()
		first, size := utf8.DecodeRuneInString(out)
		next, _ := utf8.DecodeRuneInString(out[size:])
		up := unicode.ToUpper(first)
		if first != 'U' && unicode.IsUpper(up) && (unicode.IsLower(next) || unicode.IsDigit(next)) {
			out = string(up) + out[size:]
		}
		b.WriteString(out)
	}
}

// sarifClip cuts s to sarifNameMax bytes on a rune boundary, then drops a trailing uppercase
// rune so no segment ends as a lone capital.
func sarifClip(s string) string {
	if len(s) <= sarifNameMax {
		return s
	}
	cut := sarifNameMax
	for cut > 0 && !utf8.RuneStart(s[cut]) {
		cut--
	}
	s = s[:cut]
	if r, size := utf8.DecodeLastRuneInString(s); unicode.IsUpper(r) {
		s = s[:len(s)-size]
	}
	return s
}

// codePoints spells each rune of s as U+XXXX, followed by the rune itself when it prints.
func codePoints(s string) string {
	parts := make([]string, 0, utf8.RuneCountInString(s))
	for _, r := range s {
		part := fmt.Sprintf("U+%04X", r)
		if unicode.IsGraphic(r) && !unicode.IsSpace(r) {
			part += " " + string(r)
		}
		parts = append(parts, part)
	}
	return strings.Join(parts, " ")
}

// sarifMessage quotes the match, and its rewrite when it has one, then says why it fired.
func sarifMessage(f sanitize.Finding) string {
	msg := strconv.Quote(clipRunes(f.Match, sarifMatchRunes))
	if f.Replacement != nil {
		msg += " -> " + strconv.Quote(clipRunes(*f.Replacement, sarifMatchRunes))
	}
	return msg + ": " + sanitize.Why(f)
}

// sarifRegionFor places a finding in the original text. Positions come from the byte
// offset rather than the finding's own line and column, so the region never depends on
// what text the engine read. A trailing line break stays out of the region, so a match
// ending a paragraph does not spill onto the next line.
func sarifRegionFor(raw string, starts []int, f sanitize.Finding) sarifRegion {
	end := f.Offset + len(strings.TrimRight(f.Match, "\r\n"))
	startLine, startCol := position(raw, starts, f.Offset)
	endLine, endCol := position(raw, starts, end)
	return sarifRegion{
		StartLine:   startLine,
		StartColumn: startCol,
		EndLine:     endLine,
		EndColumn:   endCol,
		Snippet:     sarifText{Text: raw[f.Offset:end]},
	}
}

// lineStarts returns the byte offset each line of text begins at.
func lineStarts(text string) []int {
	starts := []int{0}
	for i := range len(text) {
		if text[i] == '\n' {
			starts = append(starts, i+1)
		}
	}
	return starts
}

// position returns the one-based line and code point column of byte offset off in text.
func position(text string, starts []int, off int) (line, col int) {
	i := sort.SearchInts(starts, off+1) - 1
	return i + 1, utf8.RuneCountInString(text[starts[i]:off]) + 1
}

// artifactURI turns a path as given into the URI a result points at. A relative path stays
// relative, which code scanning resolves against the repository root, and an absolute path
// under wd is made relative to it. Any other absolute path becomes a file URI.
func artifactURI(path, wd string) string {
	p := filepath.Clean(path)
	if filepath.IsAbs(p) && wd != "" {
		if rel, err := filepath.Rel(wd, p); err == nil && filepath.IsLocal(rel) {
			p = rel
		}
	}
	slash := filepath.ToSlash(p)
	if !filepath.IsAbs(p) {
		return (&url.URL{Path: slash}).String()
	}
	if !strings.HasPrefix(slash, "/") {
		slash = "/" + slash
	}
	return (&url.URL{Scheme: "file", Path: slash}).String()
}
