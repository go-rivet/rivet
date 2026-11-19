// SPDX-FileCopyrightText: 2026 Timothy Rule
// SPDX-License-Identifier: MIT

package v42

import (
	"github.com/go-rivet/rivet/pkg/rivet/taskfile/ast"
)

// This file adapts the v42 schema types to the runtime model in package ast.
// Update it whenever the v42 types diverge from the runtime types.

func (tf *Taskfile) toRuntime() *ast.Taskfile {
	return &ast.Taskfile{
		Location:  tf.Location,
		Version:   tf.Version,
		Namespace: tf.Namespace,
		Output: ast.Output{
			Name: tf.Output.Name,
			Group: ast.OutputGroup{
				Begin:     tf.Output.Group.Begin,
				End:       tf.Output.Group.End,
				ErrorOnly: tf.Output.Group.ErrorOnly,
			},
		},
		Includes: includesToRuntime(tf.Includes),
		Set:      tf.Set,
		Shopt:    tf.Shopt,
		Vars:     varsToRuntime(tf.Vars),
		Tasks:    tasksToRuntime(tf.Tasks),
		Dotenv:   tf.Dotenv,
		Run:      tf.Run,
		Interval: tf.Interval,
	}
}

func includesToRuntime(in *Includes) *ast.Includes {
	out := ast.NewIncludes()
	for k, v := range in.All() {
		out.Set(k, &ast.Include{
			Namespace:      v.Namespace,
			Taskfile:       v.Taskfile,
			Dir:            v.Dir,
			Optional:       v.Optional,
			Internal:       v.Internal,
			Aliases:        v.Aliases,
			Excludes:       v.Excludes,
			AdvancedImport: v.AdvancedImport,
			Vars:           varsToRuntime(v.Vars),
			Flatten:        v.Flatten,
			Checksum:       v.Checksum,
		})
	}
	return out
}

func varsToRuntime(in *Vars) *ast.Vars {
	if in == nil {
		return nil
	}
	out := ast.NewVarsWithCapacity(in.Len())
	for k, v := range in.All() {
		out.Set(k, ast.Var{Value: v.Value, Live: v.Live, Sh: v.Sh, Ref: v.Ref, Dir: v.Dir})
	}
	return out
}

func tasksToRuntime(in *Tasks) *ast.Tasks {
	out := ast.NewTasks()
	for k, t := range in.All(nil) {
		out.Set(k, taskToRuntime(t))
	}
	return out
}

func taskToRuntime(t *Task) *ast.Task {
	if t == nil {
		return nil
	}
	return &ast.Task{
		Task:                 t.Task,
		Lua:                  t.Lua,
		Cmds:                 mapSlice(t.Cmds, cmdToRuntime),
		Deps:                 mapSlice(t.Deps, depToRuntime),
		Label:                t.Label,
		Desc:                 t.Desc,
		Prompt:               ast.Prompt(t.Prompt),
		Summary:              t.Summary,
		Requires:             requiresToRuntime(t.Requires),
		Aliases:              t.Aliases,
		Transform:            transformToRuntime(t.Transform),
		Dir:                  t.Dir,
		Set:                  t.Set,
		Shopt:                t.Shopt,
		Vars:                 varsToRuntime(t.Vars),
		Dotenv:               t.Dotenv,
		Interactive:          t.Interactive,
		Internal:             t.Internal,
		Prefix:               t.Prefix,
		IgnoreError:          t.IgnoreError,
		Run:                  t.Run,
		Platforms:            mapSlice(t.Platforms, platformToRuntime),
		Watch:                t.Watch,
		Location:             locationToRuntime(t.Location),
		Failfast:             t.Failfast,
		Namespace:            t.Namespace,
		IncludeVars:          varsToRuntime(t.IncludeVars),
		IncludedTaskfileVars: varsToRuntime(t.IncludedTaskfileVars),
		FullName:             t.FullName,
	}
}

func cmdToRuntime(c *Cmd) *ast.Cmd {
	if c == nil {
		return nil
	}
	return &ast.Cmd{
		Cmd:         c.Cmd,
		Lua:         c.Lua,
		Task:        c.Task,
		Set:         c.Set,
		Shopt:       c.Shopt,
		Vars:        varsToRuntime(c.Vars),
		IgnoreError: c.IgnoreError,
		Defer:       c.Defer,
		Platforms:   mapSlice(c.Platforms, platformToRuntime),
	}
}

func depToRuntime(d *Dep) *ast.Dep {
	if d == nil {
		return nil
	}
	return &ast.Dep{Task: d.Task, Cmd: d.Cmd, Vars: varsToRuntime(d.Vars)}
}

func requiresToRuntime(r *Requires) *ast.Requires {
	if r == nil {
		return nil
	}
	return &ast.Requires{Vars: mapSlice(r.Vars, func(v *VarsWithValidation) *ast.VarsWithValidation {
		out := &ast.VarsWithValidation{Name: v.Name}
		if v.Enum != nil {
			out.Enum = &ast.Enum{Ref: v.Enum.Ref, Value: v.Enum.Value}
		}
		return out
	})}
}

func transformToRuntime(t *Transform) *ast.Transform {
	if t == nil {
		return nil
	}
	glob := func(g *Glob) *ast.Glob { return &ast.Glob{Glob: g.Glob, Negate: g.Negate} }
	return &ast.Transform{Matches: mapSlice(t.Matches, glob), Yields: mapSlice(t.Yields, glob), Subst: t.Subst}
}

func platformToRuntime(p *Platform) *ast.Platform {
	return &ast.Platform{OS: p.OS, Arch: p.Arch}
}

func locationToRuntime(l *Location) *ast.Location {
	if l == nil {
		return nil
	}
	return &ast.Location{Line: l.Line, Column: l.Column, Taskfile: l.Taskfile}
}

func mapSlice[T, U any](in []T, f func(T) U) []U {
	if in == nil {
		return nil
	}
	out := make([]U, len(in))
	for i, v := range in {
		out[i] = f(v)
	}
	return out
}
