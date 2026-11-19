// SPDX-FileCopyrightText: 2026 Timothy Rule
// SPDX-License-Identifier: MIT

package v42_test

import (
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"go.yaml.in/yaml/v3"

	riveterrors "github.com/go-rivet/rivet/pkg/rivet/errors"
	"github.com/go-rivet/rivet/pkg/rivet/taskfile/ast"
	v42 "github.com/go-rivet/rivet/pkg/rivet/taskfile/ast/v42"
)

const doc = `version: 42
output: group
vars:
  A: a
  B: {sh: echo b}
tasks:
  build:
    desc: d
    deps: [{task: other}, {task: other, vars: {X: 1}}]
    vars: {Y: y}
    platforms: [linux/amd64]
    requires: {vars: [{name: Z, enum: [a, b]}]}
    cmds:
      - echo hi
      - task: other
      - defer: echo done
  other:
    cmds: [echo o]
`

// The runtime adapter must match what the v3 decoder produces for the same document.
func TestParseMatchesV3Decode(t *testing.T) {
	var node yaml.Node
	if err := yaml.Unmarshal([]byte(doc), &node); err != nil {
		t.Fatal(err)
	}
	got, err := v42.Parse(&node)
	if err != nil {
		t.Fatal(err)
	}
	var want ast.Taskfile
	if err := node.Decode(&want); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, &want) {
		t.Fatalf("mismatch\n got: %+v\nwant: %+v", got, &want)
	}
}

func TestParseRejectsOtherVersions(t *testing.T) {
	var node yaml.Node
	if err := yaml.Unmarshal([]byte(strings.Replace(doc, "42", "3", 1)), &node); err != nil {
		t.Fatal(err)
	}
	if _, err := v42.Parse(&node); err == nil {
		t.Fatal("expected error")
	}
}

func TestParseTaskLua(t *testing.T) {
	t.Parallel()
	var node yaml.Node
	require.NoError(t, yaml.Unmarshal([]byte("version: 42\ntasks:\n  example:\n    lua: 'task.skip()'\n    cmds: [echo ok]\n"), &node))
	taskfile, err := v42.Parse(&node)
	require.NoError(t, err)
	task, ok := taskfile.Tasks.Get("example")
	require.True(t, ok)
	require.Equal(t, "task.skip()", task.Lua)
	require.Equal(t, task.Lua, task.DeepCopy().Lua)
}

func TestParseRejectsForAndIf(t *testing.T) {
	t.Parallel()

	cases := map[string]string{
		"task for":           "for: [one, two]\n    cmds: [echo ok]",
		"task if":            "if: 'true'\n    cmds: [echo ok]",
		"task status":        "status: ['true']",
		"task preconditions": "preconditions: ['true']",
		"command for":        "cmds: [{cmd: echo ok, for: [one, two]}]",
		"command if":         "cmds: [{cmd: echo ok, if: 'false'}]",
		"single command if":  "cmd: {cmd: echo ok, if: ''}",
		"dependency for":     "deps: [{task: other, for: [one, two]}]",
		"dependency if":      "deps: [{task: other, if: 'true'}]",
		"deferred for":       "cmds: [{defer: {cmd: echo ok, for: [one, two]}}]",
		"deferred if":        "cmds: [{defer: {cmd: echo ok, if: 'true'}}]",
	}
	for name, fields := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			var node yaml.Node
			require.NoError(t, yaml.Unmarshal([]byte("version: 42\ntasks:\n  example:\n    "+fields+"\n"), &node))

			_, err := v42.Parse(&node)
			field := name[strings.LastIndex(name, " ")+1:]
			var decodeErr *riveterrors.TaskfileDecodeError
			require.ErrorAs(t, err, &decodeErr)
			require.Equal(t, strconv.Quote(field)+" is not supported in version 42; use Lua", decodeErr.Message)
		})
	}
}

func TestParseRejectsMultipleCommandTypes(t *testing.T) {
	t.Parallel()

	cases := map[string]string{
		"cmd and task":            "cmd: echo ok, task: other",
		"cmd and lua":             "cmd: echo ok, lua: print('ok')",
		"task and lua":            "task: other, lua: print('ok')",
		"all three":               "cmd: echo ok, task: other, lua: print('ok')",
		"cmd and task with defer": "cmd: echo ok, task: other, defer: echo later",
		"deferred cmd and task":   "defer: {cmd: echo ok, task: other}",
		"deferred cmd and lua":    "defer: {cmd: echo ok, lua: print('ok')}",
		"deferred task and lua":   "defer: {task: other, lua: print('ok')}",
	}
	for name, fields := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			var node yaml.Node
			require.NoError(t, yaml.Unmarshal([]byte("version: 42\ntasks:\n  example:\n    cmds: [{"+fields+"}]\n"), &node))

			_, err := v42.Parse(&node)
			var decodeErr *riveterrors.TaskfileDecodeError
			require.ErrorAs(t, err, &decodeErr)
			require.Equal(t, "command cannot have more than one of lua, cmd, or task", decodeErr.Message)
		})
	}
}

func TestParseIgnoresEmptyCommandTypes(t *testing.T) {
	t.Parallel()

	cases := map[string]struct {
		fields string
		cmd    ast.Cmd
	}{
		"empty cmd":           {"cmd: '', task: other", ast.Cmd{Task: "other"}},
		"empty task":          {"task: '', cmd: echo ok", ast.Cmd{Cmd: "echo ok"}},
		"empty lua with cmd":  {"lua: '', cmd: echo ok", ast.Cmd{Cmd: "echo ok"}},
		"empty lua with task": {"lua: '', task: other", ast.Cmd{Task: "other"}},
		"empty cmd and task":  {"cmd: '', task: '', lua: print('ok')", ast.Cmd{Lua: "print('ok')"}},
		"empty lua and defer": {"lua: '', defer: echo later", ast.Cmd{Cmd: "echo later", Defer: true}},
		"deferred empty cmd":  {"defer: {cmd: '', task: other}", ast.Cmd{Task: "other", Defer: true}},
		"deferred empty task": {"defer: {task: '', cmd: echo ok}", ast.Cmd{Cmd: "echo ok", Defer: true}},
		"deferred empty lua":  {"defer: {lua: '', cmd: echo ok}", ast.Cmd{Cmd: "echo ok", Defer: true}},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			var node yaml.Node
			require.NoError(t, yaml.Unmarshal([]byte("version: 42\ntasks:\n  example:\n    cmds: [{"+tc.fields+"}]\n"), &node))
			taskfile, err := v42.Parse(&node)
			require.NoError(t, err)
			task, ok := taskfile.Tasks.Get("example")
			require.True(t, ok)
			require.Len(t, task.Cmds, 1)
			require.Equal(t, tc.cmd, *task.Cmds[0])
		})
	}
}

func TestParseRejectsDeferredLua(t *testing.T) {
	t.Parallel()

	for _, fields := range []string{
		"lua: print('ok'), defer: echo later",
		"defer: {lua: print('ok')}",
	} {
		t.Run(fields, func(t *testing.T) {
			t.Parallel()
			var node yaml.Node
			require.NoError(t, yaml.Unmarshal([]byte("version: 42\ntasks:\n  example:\n    cmds: [{"+fields+"}]\n"), &node))
			_, err := v42.Parse(&node)
			var decodeErr *riveterrors.TaskfileDecodeError
			require.ErrorAs(t, err, &decodeErr)
			require.Equal(t, "Lua commands cannot be deferred", decodeErr.Message)
		})
	}
}

func TestParseNamespace(t *testing.T) {
	t.Parallel()

	var node yaml.Node
	require.NoError(t, yaml.Unmarshal([]byte("version: 42\nnamespace: one:two\n"), &node))

	taskfile, err := v42.Parse(&node)
	require.NoError(t, err)
	require.Equal(t, "one:two", taskfile.Namespace)
}

func TestParseRejectsNamespacesWithEmptyPathComponents(t *testing.T) {
	t.Parallel()

	for _, namespace := range []string{":one", "one:", "one::two"} {
		t.Run(namespace, func(t *testing.T) {
			var node yaml.Node
			require.NoError(t, yaml.Unmarshal([]byte("version: 42\nnamespace: "+strconv.Quote(namespace)+"\n"), &node))

			_, err := v42.Parse(&node)
			require.Error(t, err)
		})
	}
}
