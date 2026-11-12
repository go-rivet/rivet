package lua

import "errors"

// ErrCGODisabled is returned when Lua is requested from a build without cgo.
var ErrCGODisabled = errors.New("lua: support requires a cgo-enabled build")
