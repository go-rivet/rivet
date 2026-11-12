package luascript

import (
	"errors"
	"io"
)

// Options configures one Lua script invocation.
type Options struct {
	Script string
	Vars   map[string]any
	Stdout io.Writer
}

// ScriptError reports a Lua syntax or runtime error.
type ScriptError struct {
	Message string
}

func (err *ScriptError) Error() string {
	return "lua: " + err.Message
}

var ErrCGODisabled = errors.New("lua: support requires a cgo-enabled build")
