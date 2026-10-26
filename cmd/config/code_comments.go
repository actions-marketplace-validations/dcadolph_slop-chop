package config

import (
	"strconv"

	"github.com/spf13/pflag"
)

// KeyCodeComments is the code-comments flag name.
const KeyCodeComments = "code-comments"

// DefaultCodeComments is true, meaning the language server reads the comments in source
// files.
//
//nolint:gochecknoglobals // Flag definition.
var DefaultCodeComments = true

// FlagCodeComments turns the language server's comment checking in source files on or off.
// Off, the server leaves source files alone and checks prose only.
//
//nolint:gochecknoglobals // Flag definition.
var FlagCodeComments = pflag.Flag{
	Name:        KeyCodeComments,
	Usage:       "Check the comments in source files. Set false to check prose documents only.",
	Value:       &FlagValue{Val: strconv.FormatBool(DefaultCodeComments), ValType: "bool"},
	DefValue:    strconv.FormatBool(DefaultCodeComments),
	NoOptDefVal: "true",
}

// CodeComments reports whether the language server checks the comments in source files.
func CodeComments() bool { return loadBool(KeyCodeComments, DefaultCodeComments) }
