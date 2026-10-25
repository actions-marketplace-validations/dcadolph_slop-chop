package sanitize

import (
	"errors"
	"fmt"
	"testing"

	"github.com/google/go-cmp/cmp"
)

// TestLayersProfile checks the precedence Layers resolves: default, project, dialect,
// presets, voice, and the project again over a voice.
func TestLayersProfile(t *testing.T) {
	t.Parallel()
	tests := []struct {
		In          Layers
		WantSwap    string
		WantDialect Dialect
		WantDefault bool
		Want        error
	}{{ // Test 0: No layers is the built-in default.
		In: Layers{}, WantSwap: "", WantDefault: true,
	}, { // Test 1: A project file extends the default.
		In:       Layers{Project: &Profile{WordReplace: map[string]string{"boast": "claim"}}},
		WantSwap: "claim", WantDefault: true,
	}, { // Test 2: A standalone project file replaces the default.
		In:       Layers{Project: &Profile{Standalone: true, WordReplace: map[string]string{"boast": "claim"}}},
		WantSwap: "claim", WantDefault: false,
	}, { // Test 3: The dialect overrides the project's own.
		In:       Layers{Project: &Profile{Dialect: DialectBritish}, Dialect: string(DialectAmerican)},
		WantSwap: "", WantDialect: DialectAmerican, WantDefault: true,
	}, { // Test 4: A preset adds its swaps.
		In: Layers{Presets: []string{"cleaver"}}, WantSwap: "have", WantDefault: true,
	}, { // Test 5: The project wins over a preset.
		In: Layers{
			Project: &Profile{WordReplace: map[string]string{"boast": "claim"}},
			Presets: []string{"cleaver"},
		},
		WantSwap: "claim", WantDefault: true,
	}, { // Test 6: A voice wins over a preset.
		In: Layers{
			Presets: []string{"cleaver"},
			Voice:   Voice{Prefer: map[string]string{"boast": "own"}},
		},
		WantSwap: "own", WantDefault: true,
	}, { // Test 7: The project still outranks a voice.
		In: Layers{
			Project: &Profile{WordReplace: map[string]string{"boast": "claim"}},
			Voice:   Voice{Prefer: map[string]string{"boast": "own"}},
		},
		WantSwap: "claim", WantDefault: true,
	}, { // Test 8: An unknown preset fails.
		In: Layers{Presets: []string{"no-such-preset"}}, Want: ErrPreset,
	}}

	defaults := len(DefaultProfile().BlockWords)
	for testNum, test := range tests {
		t.Run(fmt.Sprintf("test %d", testNum), func(t *testing.T) {
			t.Parallel()
			got, err := test.In.Profile()
			if !errors.Is(err, test.Want) {
				t.Fatalf("error = %v, want %v", err, test.Want)
			}
			if test.Want != nil {
				return
			}
			if diff := cmp.Diff(test.WantSwap, got.WordReplace["boast"]); diff != "" {
				t.Errorf("swap mismatch (-want +got):\n%s", diff)
			}
			if diff := cmp.Diff(test.WantDialect, got.Dialect); diff != "" {
				t.Errorf("dialect mismatch (-want +got):\n%s", diff)
			}
			if gotDefault := len(got.BlockWords) >= defaults && defaults > 0; gotDefault != test.WantDefault {
				t.Errorf("default block words kept = %v, want %v", gotDefault, test.WantDefault)
			}
		})
	}
}
