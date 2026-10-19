package config

import (
	"strconv"

	"github.com/spf13/pflag"
)

// KeySARIF is the sarif flag name.
const KeySARIF = "sarif"

// DefaultSARIF is false, meaning the usual check output.
//
//nolint:gochecknoglobals // Flag definition.
var DefaultSARIF = false

// FlagSARIF switches check's stdout to one SARIF 2.1.0 log covering every file argument.
//
//nolint:gochecknoglobals // Flag definition.
var FlagSARIF = pflag.Flag{
	Name:        KeySARIF,
	Usage:       "Write one SARIF 2.1.0 log to stdout covering every file, for code scanning.",
	Value:       &FlagValue{Val: strconv.FormatBool(DefaultSARIF), ValType: "bool"},
	DefValue:    strconv.FormatBool(DefaultSARIF),
	NoOptDefVal: "true",
}

// SARIF reports whether check should write a SARIF log.
func SARIF() bool { return loadBool(KeySARIF, DefaultSARIF) }
