package sanitize

import "errors"

// ErrCompile means a profile could not be turned into rules.
var ErrCompile = errors.New("profile compile")

// ErrDialect means a profile named a spelling dialect the engine does not know.
var ErrDialect = errors.New("unknown dialect")

// ErrPreset means a run named a preset the engine does not ship.
var ErrPreset = errors.New("unknown preset")

// ErrLocate means findings could not be placed in a text: an offset falls outside it, or
// the findings are out of offset order.
var ErrLocate = errors.New("locate")
