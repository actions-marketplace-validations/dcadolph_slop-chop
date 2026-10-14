package sanitize

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"slices"
	"strings"
	"testing"
)

// updateScoring rewrites the scoring golden file from the current engine.
//
//nolint:gochecknoglobals // Test flag, read once.
var updateScoring = flag.Bool("update-scoring", false,
	"rewrite testdata/scoring.golden.json from the current engine")

const (
	// scoringGoldenPath is the file that pins the scores the benchmark corpus receives.
	scoringGoldenPath = "testdata/scoring.golden.json"
	// defaultProfileName names the unmodified default profile in the golden file.
	defaultProfileName = "default"
	// maxMovedListed caps how many moved scores a failure message spells out.
	maxMovedListed = 8
)

// scoringGolden pins the score of every benchmark passage under the default profile and
// each built-in preset, for one scoring version.
type scoringGolden struct {
	// ScoringVersion is the revision of the scoring rules these scores belong to.
	ScoringVersion int `json:"scoringVersion"`
	// Scores maps a passage key to its score under each profile name.
	Scores map[string]map[string]int `json:"scores"`
}

// passageKey names a corpus passage by its label and a short hash of its text, so the key
// survives a reorder of the corpus file and a changed passage reads as a new one.
func passageKey(p benchPassage) string {
	sum := sha256.Sum256([]byte(p.Text))
	return p.Label + "-" + hex.EncodeToString(sum[:])[:12]
}

// currentScoring scores the whole benchmark corpus with the engine as it stands.
func currentScoring(t *testing.T) scoringGolden {
	t.Helper()
	profiles := map[string]Profile{defaultProfileName: DefaultProfile()}
	for _, name := range PresetNames() {
		p, err := ApplyPresets(DefaultProfile(), name)
		if err != nil {
			t.Fatalf("ApplyPresets(%q): %v", name, err)
		}
		profiles[name] = p
	}
	engines := make(map[string]*Sanitizer, len(profiles))
	for name, p := range profiles {
		s, err := New(p)
		if err != nil {
			t.Fatalf("New(%q): %v", name, err)
		}
		engines[name] = s
	}
	got := scoringGolden{ScoringVersion: ScoringVersion, Scores: map[string]map[string]int{}}
	for _, p := range loadCorpus(t) {
		row := make(map[string]int, len(engines))
		for name, s := range engines {
			row[name] = s.Score(p.Text).Value
		}
		got.Scores[passageKey(p)] = row
	}
	return got
}

// loadScoringGolden reads the pinned scores from disk.
func loadScoringGolden(t *testing.T) scoringGolden {
	t.Helper()
	data, err := os.ReadFile(scoringGoldenPath)
	if err != nil {
		t.Fatalf("read %s: %v (create it with -update-scoring)", scoringGoldenPath, err)
	}
	var g scoringGolden
	if err := json.Unmarshal(data, &g); err != nil {
		t.Fatalf("parse %s: %v", scoringGoldenPath, err)
	}
	return g
}

// writeScoringGolden saves g with one line per passage, so a moved score shows up in a code
// review as one changed line naming the passage and every profile it moved under.
func writeScoringGolden(g scoringGolden) error {
	keys := make([]string, 0, len(g.Scores))
	for k := range g.Scores {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	var b strings.Builder
	fmt.Fprintf(&b, "{\n  \"scoringVersion\": %d,\n  \"scores\": {\n", g.ScoringVersion)
	for i, k := range keys {
		row, err := json.Marshal(g.Scores[k])
		if err != nil {
			return err
		}
		sep := ","
		if i == len(keys)-1 {
			sep = ""
		}
		fmt.Fprintf(&b, "    %q: %s%s\n", k, row, sep)
	}
	b.WriteString("  }\n}\n")
	return os.WriteFile(scoringGoldenPath, []byte(b.String()), 0o644)
}

// scoringChange lists how two sets of scores differ, in a stable order.
type scoringChange struct {
	// Moved names each passage and profile whose score differs, with both values.
	Moved []string
	// Added counts passage and profile entries present now and absent from the pinned file.
	Added int
	// Removed counts entries in the pinned file that the engine no longer produces.
	Removed int
}

// diffScoring compares pinned scores with current ones.
func diffScoring(pinned, current scoringGolden) scoringChange {
	var c scoringChange
	keys := make([]string, 0, len(current.Scores))
	for k := range current.Scores {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	for _, k := range keys {
		for name, now := range current.Scores[k] {
			was, ok := pinned.Scores[k][name]
			switch {
			case !ok:
				c.Added++
			case was != now:
				c.Moved = append(c.Moved, fmt.Sprintf("%s under %s: %d to %d", k, name, was, now))
			}
		}
	}
	for k, row := range pinned.Scores {
		for name := range row {
			if _, ok := current.Scores[k][name]; !ok {
				c.Removed++
			}
		}
	}
	slices.Sort(c.Moved)
	return c
}

// scoringProblems returns what is wrong between the pinned scores and the engine, empty when
// the contract holds. The contract: a score moves only together with a ScoringVersion bump,
// and the pinned file always describes the version the engine reports.
func scoringProblems(pinned, current scoringGolden) []string {
	if pinned.ScoringVersion != current.ScoringVersion {
		return []string{fmt.Sprintf(
			"the pinned scores are for scoring version %d and the engine reports %d: "+
				"regenerate with go test ./sanitize -run TestScoringContract -update-scoring, "+
				"then describe the change in docs/SCORING.md",
			pinned.ScoringVersion, current.ScoringVersion)}
	}
	c := diffScoring(pinned, current)
	var problems []string
	if len(c.Moved) > 0 {
		listed := c.Moved
		if len(listed) > maxMovedListed {
			listed = listed[:maxMovedListed]
		}
		problems = append(problems, fmt.Sprintf(
			"%d scores moved without a ScoringVersion bump (still %d): bump ScoringVersion in "+
				"sanitize/score.go, regenerate with -update-scoring, and describe the change in "+
				"docs/SCORING.md. Moved: %s",
			len(c.Moved), current.ScoringVersion, strings.Join(listed, "; ")))
	}
	if c.Added > 0 || c.Removed > 0 {
		problems = append(problems, fmt.Sprintf(
			"the corpus or the preset list changed (%d entries added, %d removed): regenerate with "+
				"go test ./sanitize -run TestScoringContract -update-scoring",
			c.Added, c.Removed))
	}
	return problems
}

// TestScoringContract fails when the benchmark corpus scores differently from the pinned
// file without a ScoringVersion bump. It is the guard behind the promise that a score gate
// set on one scoring version keeps meaning the same thing until the version changes.
func TestScoringContract(t *testing.T) {
	t.Parallel()
	current := currentScoring(t)
	if *updateScoring {
		if pinned, err := os.ReadFile(scoringGoldenPath); err == nil {
			var old scoringGolden
			if err := json.Unmarshal(pinned, &old); err != nil {
				t.Fatalf("parse %s: %v", scoringGoldenPath, err)
			}
			if old.ScoringVersion == current.ScoringVersion {
				if moved := diffScoring(old, current).Moved; len(moved) > 0 {
					t.Fatalf("refusing to rewrite %s: %d scores moved under scoring version %d. "+
						"Bump ScoringVersion first. Moved: %s",
						scoringGoldenPath, len(moved), old.ScoringVersion,
						strings.Join(moved[:min(len(moved), maxMovedListed)], "; "))
				}
			}
		} else if !errors.Is(err, fs.ErrNotExist) {
			t.Fatalf("read %s: %v", scoringGoldenPath, err)
		}
		if err := writeScoringGolden(current); err != nil {
			t.Fatalf("write %s: %v", scoringGoldenPath, err)
		}
		return
	}
	for _, problem := range scoringProblems(loadScoringGolden(t), current) {
		t.Error(problem)
	}
}

// TestScoringProblems checks the contract's own logic against synthetic score sets, so the
// guard is shown to fail on the changes it exists to catch.
func TestScoringProblems(t *testing.T) {
	t.Parallel()
	base := func(version, a, b int) scoringGolden {
		return scoringGolden{ScoringVersion: version, Scores: map[string]map[string]int{
			"ai-aaa":  {"default": a, "cleaver": a + 1},
			"human-b": {"default": b, "cleaver": b},
		}}
	}
	tests := []struct {
		Pinned    scoringGolden
		Current   scoringGolden
		WantCount int
		WantText  string
	}{{ // Test 0: Identical scores under one version hold the contract.
		Pinned: base(1, 80, 10), Current: base(1, 80, 10), WantCount: 0,
	}, { // Test 1: A moved score under an unchanged version breaks it.
		Pinned: base(1, 80, 10), Current: base(1, 83, 10), WantCount: 1,
		WantText: "moved without a ScoringVersion bump",
	}, { // Test 2: A bumped version against a stale pinned file asks for a regeneration.
		Pinned: base(1, 80, 10), Current: base(2, 83, 10), WantCount: 1,
		WantText: "regenerate",
	}, { // Test 3: A bump with no moved scores still needs the pinned file refreshed.
		Pinned: base(1, 80, 10), Current: base(2, 80, 10), WantCount: 1,
		WantText: "pinned scores are for scoring version 1",
	}, { // Test 4: A new corpus passage asks for a regeneration.
		Pinned: base(1, 80, 10),
		Current: scoringGolden{ScoringVersion: 1, Scores: map[string]map[string]int{
			"ai-aaa":  {"default": 80, "cleaver": 81},
			"human-b": {"default": 10, "cleaver": 10},
			"human-c": {"default": 5, "cleaver": 5},
		}},
		WantCount: 1, WantText: "2 entries added, 0 removed",
	}, { // Test 5: A dropped passage asks for a regeneration.
		Pinned: base(1, 80, 10),
		Current: scoringGolden{ScoringVersion: 1, Scores: map[string]map[string]int{
			"ai-aaa": {"default": 80, "cleaver": 81},
		}},
		WantCount: 1, WantText: "0 entries added, 2 removed",
	}, { // Test 6: A moved score and a new passage are reported separately.
		Pinned: base(1, 80, 10),
		Current: scoringGolden{ScoringVersion: 1, Scores: map[string]map[string]int{
			"ai-aaa":  {"default": 90, "cleaver": 81},
			"human-b": {"default": 10, "cleaver": 10},
			"human-c": {"default": 5, "cleaver": 5},
		}},
		WantCount: 2, WantText: "moved without a ScoringVersion bump",
	}}
	for testNum, test := range tests {
		t.Run(fmt.Sprintf("test %d", testNum), func(t *testing.T) {
			t.Parallel()
			got := scoringProblems(test.Pinned, test.Current)
			if len(got) != test.WantCount {
				t.Fatalf("problems = %d %q, want %d", len(got), got, test.WantCount)
			}
			if test.WantText != "" && !strings.Contains(strings.Join(got, "\n"), test.WantText) {
				t.Errorf("problems %q lack %q", got, test.WantText)
			}
		})
	}
}

// TestScoreCarriesVersion checks that a score reports the revision that produced it.
func TestScoreCarriesVersion(t *testing.T) {
	t.Parallel()
	s, err := New(DefaultProfile())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	for _, text := range []string{"", "plain words", "In summary, a comprehensive result."} {
		if got := s.Score(text).ScoringVersion; got != ScoringVersion {
			t.Errorf("Score(%q).ScoringVersion = %d, want %d", text, got, ScoringVersion)
		}
	}
}
