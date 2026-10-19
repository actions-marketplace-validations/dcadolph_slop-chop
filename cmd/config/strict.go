package config

import (
	"strconv"

	"github.com/spf13/pflag"
)

// KeyStrict is the strict flag name.
const KeyStrict = "strict"

// DefaultStrict is false, meaning cleanups that carry no score weight do not fail a check.
//
//nolint:gochecknoglobals // Flag definition.
var DefaultStrict = false

// FlagStrict makes check fail on any finding, including cleanups that carry no score weight.
//
//nolint:gochecknoglobals // Flag definition.
var FlagStrict = pflag.Flag{
	Name:        KeyStrict,
	Usage:       "Fail on any finding, including cleanups that carry no score weight.",
	Value:       &FlagValue{Val: strconv.FormatBool(DefaultStrict), ValType: "bool"},
	DefValue:    strconv.FormatBool(DefaultStrict),
	NoOptDefVal: "true",
}

// Strict reports whether check should fail on any finding.
func Strict() bool { return loadBool(KeyStrict, DefaultStrict) }
