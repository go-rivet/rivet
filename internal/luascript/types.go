// SPDX-FileCopyrightText: 2026 Timothy Rule
// SPDX-License-Identifier: MIT

package luascript

import (
	"context"
	"errors"
	"io"
)

// TaskRunner is a callback function for running a task from Lua.
// taskName is the name of the task to run, and vars are the variables to pass to it.
type TaskRunner func(ctx context.Context, taskName string, vars map[string]any) error

// CmdRunner is a callback function for running a command from Lua.
// cmd is the shell command to run.
type CmdRunner func(ctx context.Context, cmd string) error

// Options configures one Lua script invocation.
type Options struct {
	Script     string
	Vars       map[string]any
	Ctx        map[string]any
	Stdout     io.Writer
	TaskRunner TaskRunner
	CmdRunner  CmdRunner
}

// ScriptError reports a Lua syntax or runtime error.
type ScriptError struct {
	Message string
}

func (err *ScriptError) Error() string {
	return "lua: " + err.Message
}

var ErrCGODisabled = errors.New("lua: support requires a cgo-enabled build")
