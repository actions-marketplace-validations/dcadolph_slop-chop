package sanitize

import (
	"fmt"
	"testing"
)

// TestTemplateStem checks that opening sentences stamped from one template are flagged and
// that a caveat restated word for word above each thing it applies to is not.
func TestTemplateStem(t *testing.T) {
	t.Parallel()
	s, err := New(DefaultProfile())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	tests := []struct {
		Name    string
		In      string
		WantHit bool
	}{{ // Test 0: Three paragraphs opening alike with varying tails.
		Name: "stamped outline", WantHit: true,
		In: "Our team cares about retries today.\n\nOur team cares about timeouts today.\n\n" +
			"Our team cares about backoff today.",
	}, { // Test 1: The same sentence restated verbatim is a notice.
		Name: "verbatim caveat", WantHit: false,
		In: "OpenBSD has no user-settable keepalive options.\n\n" +
			"OpenBSD has no user-settable keepalive options.\n\n" +
			"OpenBSD has no user-settable keepalive options.",
	}, { // Test 2: A verbatim repeat differing only in case and closing punctuation.
		Name: "verbatim caveat with punctuation drift", WantHit: false,
		In: "OpenBSD has no user-settable keepalive options.\n\n" +
			"openbsd has no user-settable keepalive options!\n\n" +
			"OpenBSD has no user-settable keepalive options.",
	}, { // Test 3: One verbatim repeat among varying tails is still a template.
		Name: "one odd tail", WantHit: true,
		In: "Our team cares about retries today.\n\nOur team cares about retries today.\n\n" +
			"Our team cares about backoff today.",
	}, { // Test 4: Colon lead-ins sharing a stem introduce lists and are not a template.
		Name: "colon lead-ins", WantHit: false,
		In: "To run the app locally:\n\nTo run the app in Docker:\n\n" +
			"To run the app with Make:",
	}, { // Test 5: List items sharing a stem after the marker are not a template.
		Name: "list items", WantHit: false,
		In: "- Can parse the header and the body.\n\n- Can parse the footer and the index.\n\n" +
			"- Can parse the trailer and the map.",
	}, { // Test 6: Plain paragraphs opening alike still fire.
		Name: "plain paragraph openers", WantHit: true,
		In: "The cache layer stores retries safely.\n\nThe cache layer stores timeouts safely.\n\n" +
			"The cache layer stores backoff safely.",
	}}
	for testNum, test := range tests {
		t.Run(fmt.Sprintf("test %d %s", testNum, test.Name), func(t *testing.T) {
			t.Parallel()
			hit := false
			for _, f := range s.Check(test.In) {
				if f.Rule == "structural:template-stem" {
					hit = true
				}
			}
			if hit != test.WantHit {
				t.Errorf("template-stem hit = %v, want %v", hit, test.WantHit)
			}
		})
	}
}
