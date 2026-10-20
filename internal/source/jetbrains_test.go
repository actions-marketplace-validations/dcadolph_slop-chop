package source

import (
	"encoding/xml"
	"fmt"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
)

// jetBrainsPluginPath is the plugin descriptor relative to this package.
const jetBrainsPluginPath = "../../jetbrains/src/main/resources/META-INF/plugin.xml"

// pluginDescriptor holds the parts of plugin.xml the drift test reads.
type pluginDescriptor struct {
	// Mappings lists every fileNamePatternMapping element.
	Mappings []patternMapping `xml:"extensions>fileNamePatternMapping"`
}

// patternMapping is one fileNamePatternMapping element.
type patternMapping struct {
	// Patterns is the semicolon-separated pattern list.
	Patterns string `xml:"patterns,attr"`
	// ServerID names the language server the patterns map to.
	ServerID string `xml:"serverId,attr"`
}

// normalizePatterns turns a semicolon-separated pattern list into a sorted, deduplicated,
// lowercased set where "*.ext" becomes ".ext" and anything else is a bare file name.
func normalizePatterns(list string) []string {
	var out []string
	for _, p := range strings.Split(list, ";") {
		p = strings.ToLower(strings.TrimSpace(p))
		if p == "" {
			continue
		}
		out = append(out, strings.TrimPrefix(p, "*"))
	}
	slices.Sort(out)
	return slices.Compact(out)
}

// wantedPatterns builds the set of extensions and file names the masker understands.
func wantedPatterns() []string {
	var out []string
	for ext := range commentStyles {
		out = append(out, ext)
	}
	for name := range names {
		out = append(out, name)
	}
	slices.Sort(out)
	return slices.Compact(out)
}

// shippedPatterns reads the slopChop pattern mappings from the shipped plugin.xml.
func shippedPatterns(t *testing.T) string {
	t.Helper()
	data, err := os.ReadFile(jetBrainsPluginPath)
	if err != nil {
		t.Fatalf("read plugin.xml: %v", err)
	}
	var d pluginDescriptor
	if err := xml.Unmarshal(data, &d); err != nil {
		t.Fatalf("parse plugin.xml: %v", err)
	}
	var lists []string
	for _, m := range d.Mappings {
		if m.ServerID == "slopChop" {
			lists = append(lists, m.Patterns)
		}
	}
	return strings.Join(lists, ";")
}

// TestJetBrainsPatterns keeps the JetBrains plugin's file patterns in step with the types the
// masker reads comments from.
func TestJetBrainsPatterns(t *testing.T) {
	t.Parallel()
	tests := []struct {
		Patterns  func(t *testing.T) string
		WantMatch bool
	}{{ // Test 0: The shipped plugin.xml matches the masker's tables.
		Patterns: shippedPatterns, WantMatch: true,
	}, { // Test 1: A list with *.go removed does not match.
		Patterns: func(t *testing.T) string {
			t.Helper()
			return strings.Replace(shippedPatterns(t), "*.go;", "", 1)
		},
		WantMatch: false,
	}}
	for testNum, test := range tests {
		t.Run(fmt.Sprintf("test %d", testNum), func(t *testing.T) {
			t.Parallel()
			got := normalizePatterns(test.Patterns(t))
			diff := cmp.Diff(wantedPatterns(), got, cmpopts.EquateEmpty())
			if (diff == "") != test.WantMatch {
				t.Errorf("match = %v, want %v (-want +got):\n%s", diff == "", test.WantMatch, diff)
			}
		})
	}
}
