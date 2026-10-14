// Package source tells source code and machine-read data apart from prose, and reads the
// comments out of a source file so the prose rules can run on them without touching code.
package source

import (
	"path/filepath"
	"strings"
)

// exts lists file extensions whose content is source code or machine-read data rather than
// prose. The engine's cleanups are prose rules: a semicolon split or an orphan-comma strip
// applied to code breaks it, so these files are skipped instead of corrupted. Piping a file
// through stdin bypasses the gate for the rare deliberate run.
//
//nolint:gochecknoglobals // Immutable lookup.
var exts = map[string]bool{
	".asm": true, ".bash": true, ".c": true, ".cc": true, ".clj": true, ".cpp": true,
	".cs": true, ".css": true, ".csv": true, ".dart": true, ".erl": true, ".ex": true,
	".exs": true, ".fish": true, ".go": true, ".h": true, ".hcl": true, ".hpp": true,
	".hs": true, ".ini": true, ".java": true, ".jl": true, ".js": true, ".json": true,
	".jsonc": true, ".jsx": true, ".kt": true, ".lua": true, ".m": true, ".ml": true,
	".mm": true, ".nim": true, ".php": true, ".pl": true, ".proto": true, ".py": true,
	".r": true, ".rb": true, ".rs": true, ".scala": true, ".sh": true, ".sql": true,
	".swift": true, ".tf": true, ".toml": true, ".ts": true, ".tsx": true, ".tsv": true,
	".vim": true, ".yaml": true, ".yml": true, ".zig": true, ".zsh": true,
}

// names lists extensionless file names that are code or build configuration.
//
//nolint:gochecknoglobals // Immutable lookup.
var names = map[string]bool{
	"dockerfile": true, "makefile": true, "gnumakefile": true, "rakefile": true,
	"gemfile": true, "justfile": true,
}

// Is reports whether path names a source code or data file the prose rules must not touch.
func Is(path string) bool {
	if exts[strings.ToLower(filepath.Ext(path))] {
		return true
	}
	return names[strings.ToLower(filepath.Base(path))]
}
