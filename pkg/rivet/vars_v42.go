// SPDX-FileCopyrightText: 2026 Timothy Rule
// SPDX-License-Identifier: MIT

package rivet

import (
	"context"
	"maps"
	"strings"

	"github.com/go-rivet/rivet/internal/filepathext"
	"github.com/go-rivet/rivet/internal/templater"
	"github.com/go-rivet/rivet/pkg/rivet/namespace"
	"github.com/go-rivet/rivet/pkg/rivet/taskfile/ast"
	"github.com/go-rivet/rivet/pkg/rlog"
)

// taskVars returns the vars for a task. Version 42 Taskfiles do not use the
// Compiler: their vars come from the Taskfile DAG.
func (e *Executor) taskVars(t *ast.Task, call *Call, evaluateShVars bool) (*ast.Vars, error) {
	if e.TaskfileDAG == nil {
		if evaluateShVars {
			return e.Compiler.GetVariables(e.ctx, t, call)
		}
		return e.Compiler.FastGetVariables(e.ctx, t, call)
	}
	return e.v42TaskVars(t, call)
}

func (e *Executor) taskVarDeclarations(t *ast.Task) *ast.Vars {
	if t == nil || e.TaskfileDAG == nil {
		if t == nil {
			return nil
		}
		return t.Vars
	}
	location := taskfileLocation(t)
	for _, entry := range e.TaskfileDAG.Resolve(t.Task, location) {
		if entry.Taskfile != nil && entry.Taskfile.Location == location {
			return entry.Task.Vars
		}
	}
	return t.Vars
}

// v42Env returns the environment layer: the process environment with the
// Taskfile dotenv files applied on top.
func (e *Executor) v42Env() (map[string]any, error) {
	e.envOnce.Do(func() {
		e.env = namespace.Environment()
		scope := e.scope(nil, nil, e.env)
		var dotenv map[string]any
		dotenv, e.envErr = namespace.LoadDotenv(e.Dir, e.Taskfile.Dotenv, scope.Render)
		maps.Copy(e.env, dotenv)
	})
	return e.env, e.envErr
}

func (e *Executor) scope(t *ast.Task, call *Call, env map[string]any) *namespace.Scope {
	alias, ns := "", ""
	if call != nil {
		alias = call.Task
	}
	if t != nil {
		ns = t.Namespace
	}
	return &namespace.Scope{
		DAG:       e.TaskfileDAG,
		Namespace: ns,
		Env:       env,
		RootEnv:   env,
		Special: namespace.SpecialVars(namespace.SpecialParams{
			RootDir:        e.RootDir,
			Entrypoint:     e.Entrypoint,
			Dir:            e.Dir,
			UserWorkingDir: e.UserWorkingDir,
			Alias:          alias,
		}, t),
	}
}

// v42Scope builds the resolution scope for a task. The task's dotenv files are
// applied over the environment layer.
func (e *Executor) v42Scope(t *ast.Task, call *Call) (*namespace.Scope, map[string]any, error) {
	base, err := e.v42Env()
	if err != nil {
		return nil, nil, err
	}
	if len(t.Dotenv) == 0 {
		return e.scope(t, call, base), nil, nil
	}
	env := maps.Clone(base)
	scope := e.scope(t, call, env)
	scope.RootEnv = base
	dotenv, err := namespace.LoadDotenv(filepathext.SmartJoin(e.Dir, t.Dir), t.Dotenv, scope.Render)
	if err != nil {
		return nil, nil, err
	}
	maps.Copy(env, dotenv)
	return scope, dotenv, nil
}

// namespacedVars returns the templater resolver for a task, or nil for
// Taskfiles that are not version 42.
func (e *Executor) namespacedVars(t *ast.Task, call *Call, vars *ast.Vars) templater.NamespacedResolver {
	if e.TaskfileDAG == nil {
		return nil
	}
	scope, _, err := e.v42TaskScope(t, call, vars)
	if err != nil {
		return func([]string) map[string]any { return nil }
	}
	return scope.Resolve
}

func (e *Executor) v42TaskScope(t *ast.Task, call *Call, vars *ast.Vars) (*namespace.Scope, map[string]any, error) {
	scope, dotenv, err := e.v42Scope(t, call)
	if err != nil {
		return nil, nil, err
	}
	scope.Local = make(map[string]any)
	taskVars := e.taskVarDeclarations(t)
	addLocalVars := func(declarations *ast.Vars) {
		for name := range declarations.Keys() {
			if value, ok := vars.Get(name); ok {
				setLocalVar(scope.Local, name, value)
			}
		}
	}
	addLocalVars(taskVars)
	if call != nil {
		addLocalVars(call.Vars)
	}
	return scope, dotenv, nil
}

func setLocalVar(local map[string]any, name string, value ast.Var) {
	if value.Sh != nil && *value.Sh != "" {
		return
	}
	if value.Live != nil {
		local[name] = value.Live
	} else {
		local[name] = value.Value
	}
}

func shellNamespaceName(ref string) string {
	var name strings.Builder
	for _, c := range ref {
		switch {
		case c == ':':
			name.WriteString("__")
		case c >= 'a' && c <= 'z':
			name.WriteByte(byte(c - 'a' + 'A'))
		case c >= 'A' && c <= 'Z', c == '_':
			name.WriteByte(byte(c))
		case c >= '0' && c <= '9' && name.Len() > 0:
			name.WriteByte(byte(c))
		default:
			name.WriteByte('_')
		}
	}
	return name.String()
}

// shellVars materializes DAG variables for shell commands without changing
// the task's template variables.
func (e *Executor) shellVars(ctx context.Context, t *ast.Task, call *Call) (*ast.Vars, error) {
	if e.TaskfileDAG == nil {
		return t.Vars, nil
	}
	scope, _, err := e.v42TaskScope(t, call, t.Vars)
	if err != nil {
		return nil, err
	}
	// Taskfiles and their vars are traversed in declaration order, so the
	// last namespace variable wins when normalized shell names collide.
	type variableRef struct {
		path string
		name string
	}
	names := make(map[string]variableRef)
	for taskfile := range e.TaskfileDAG.TaskfilesInOrder() {
		if taskfile.Namespace == "" {
			continue
		}
		for name := range taskfile.Vars.Keys() {
			if _, ok := e.TaskfileDAG.Vars(taskfile.Namespace)[name]; !ok {
				continue
			}
			ref := taskfile.Namespace + ast.NamespaceSeparator + name
			names[shellNamespaceName(ref)] = variableRef{path: taskfile.Namespace, name: name}
		}
	}

	var vars *ast.Vars
	if t.Vars != nil {
		vars = ast.NewVarsWithCapacity(t.Vars.Len() + len(e.TaskfileDAG.Vars("")) + len(names))
	}
	for name, variable := range t.Vars.All() {
		vars.Set(name, variable)
		if variable.Sh != nil && *variable.Sh != "" {
			rlog.Warnf(ctx, "task: shell variable %q omitted: sh variables are not supported in the DAG\n", name)
			vars.Set(name, ast.Var{})
		}
	}
	setDAGVar := func(exportName, path, name string) {
		ref := path + ast.NamespaceSeparator + name
		variable := e.TaskfileDAG.Vars(path)[name]
		if variable.Sh != nil && *variable.Sh != "" {
			rlog.Warnf(ctx, "task: shell variable %q omitted: sh variables are not supported in the DAG\n", ref)
			vars.Set(exportName, ast.Var{})
			return
		}
		value, ok := scope.ResolveVariable(path, name)
		if !ok {
			rlog.Warnf(ctx, "task: shell variable %q omitted: unable to resolve value\n", ref)
			vars.Set(exportName, ast.Var{})
			return
		}
		vars.Set(exportName, ast.Var{Value: value})
	}
	taskVars := e.taskVarDeclarations(t)
	for name := range e.TaskfileDAG.Vars("") {
		if _, ok := taskVars.Get(name); ok {
			continue
		}
		if call != nil {
			if _, ok := call.Vars.Get(name); ok {
				continue
			}
		}
		setDAGVar(name, "", name)
	}

	for name, ref := range names {
		setDAGVar(name, ref.path, ref.name)
	}
	return vars, nil
}

// v42LuaVars returns Lua variables and their structured scope tables.
func (e *Executor) v42LuaVars(t *ast.Task, call *Call, vars *ast.Vars) (map[string]any, map[string]any, error) {
	scope, dotenv, err := e.v42TaskScope(t, call, vars)
	if err != nil {
		return nil, nil, err
	}
	taskVars := e.taskVarDeclarations(t)

	result := maps.Clone(scope.Env)
	maps.Copy(result, scope.Special)

	// Collect all namespace paths from the DAG
	var paths []string
	if e.TaskfileDAG != nil {
		adjMap, err := e.TaskfileDAG.AdjacencyMap()
		if err == nil {
			for path := range adjMap {
				paths = append(paths, path)
			}
		}
	}
	// Ensure we have at least the global namespace
	if len(paths) == 0 {
		paths = []string{""}
	}

	// Build structured namespace tables for Lua.
	rootVars := maps.Clone(scope.RootEnv)
	globalVars := maps.Clone(scope.RootEnv)
	for name, value := range scope.RootEnv {
		result[":"+name] = value
	}
	for name, value := range scope.Special {
		result[":"+name] = value
	}
	nsMap := make(map[string]map[string]any)
	taskScope := make(map[string]any, len(dotenv))
	maps.Copy(taskScope, dotenv)

	for _, path := range paths {
		for name := range e.TaskfileDAG.Vars(path) {
			ref := ":" + name
			if path != "" {
				ref = path + ast.NamespaceSeparator + name
			}
			if value, ok := scope.ResolveReference(ref); ok {
				if path == scope.Namespace {
					result[name] = value
				}
				result[ref] = value

				// Also populate structured tables
				if path == "" {
					rootVars[name] = value
				} else {
					if nsMap[path] == nil {
						nsMap[path] = make(map[string]any)
					}
					nsMap[path][name] = value
				}
				if path == scope.Namespace {
					globalVars[name] = value
				}
			}
		}
	}

	overlay := func(declarations *ast.Vars, callVars bool) {
		for name := range declarations.Keys() {
			if variable, ok := vars.Get(name); ok {
				setLocalVar(result, name, variable)
				// MATCH is inserted into call.Vars by task lookup, not supplied by the caller.
				if !callVars || name != "MATCH" {
					setLocalVar(taskScope, name, variable)
				}
			}
		}
	}
	overlay(taskVars, false)
	if call != nil {
		overlay(call.Vars, true)
	}
	return result, map[string]any{"global": globalVars, "root": rootVars, "ns": nsMap, "task": taskScope}, nil
}

// v42TaskVars builds the vars handed to the task: the environment and special
// vars, then task vars and call vars. DAG vars are not copied here, they are
// resolved on demand by the templater. Call vars are available while task
// vars are evaluated and override task vars with the same name.
func (e *Executor) v42TaskVars(t *ast.Task, call *Call) (*ast.Vars, error) {
	scope, _, err := e.v42Scope(t, call)
	if err != nil {
		return nil, err
	}
	taskVars := e.taskVarDeclarations(t)
	vars := ast.NewVarsWithCapacity(len(scope.Env) + len(scope.Special) + taskVars.Len())
	for name, value := range scope.Env {
		vars.Set(name, ast.Var{Value: value})
	}
	for name, value := range scope.Special {
		vars.Set(name, ast.Var{Value: value})
	}
	scope.Local = make(map[string]any)
	cache := &templater.Cache{Vars: vars, Namespaced: scope.Resolve}
	if call != nil {
		for name, v := range call.Vars.All() {
			vars.Set(name, v)
			setLocalVar(scope.Local, name, v)
		}
	}
	for name, v := range taskVars.All() {
		if call != nil {
			if _, ok := call.Vars.Get(name); ok {
				continue
			}
		}
		resolved := templater.ReplaceVar(v, cache)
		vars.Set(name, resolved)
		setLocalVar(scope.Local, name, resolved)
		cache.ResetCache()
	}
	return vars, cache.Err()
}
