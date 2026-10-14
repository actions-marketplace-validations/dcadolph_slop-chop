package config

import "github.com/spf13/pflag"

// KeyAs is the as flag name.
const KeyAs = "as"

// DefaultAs is empty, which reads stdin as plain prose.
//
//nolint:gochecknoglobals // Flag definition.
var DefaultAs = ""

// FlagAs makes check and score read stdin as a source file of the given type, so an editor
// can send an unsaved buffer and get findings for its comments alone.
//
//nolint:gochecknoglobals // Flag definition.
var FlagAs = pflag.Flag{
	Name:     KeyAs,
	Usage:    "Read stdin as a source file of this type, like .go or py, and scan only its comments.",
	Value:    &FlagValue{Val: DefaultAs, ValType: "string"},
	DefValue: DefaultAs,
}

// As returns the file type stdin is read as, or empty when stdin is plain prose.
func As() string { return loadString(KeyAs, DefaultAs) }
