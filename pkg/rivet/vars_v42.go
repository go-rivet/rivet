package rivet

import (
	"maps"

	"github.com/go-rivet/rivet/internal/filepathext"
	"github.com/go-rivet/rivet/internal/templater"
	"github.com/go-rivet/rivet/pkg/rivet/namespace"
	"github.com/go-rivet/rivet/pkg/rivet/taskfile/ast"
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
func (e *Executor) v42Scope(t *ast.Task, call *Call) (*namespace.Scope, error) {
	base, err := e.v42Env()
	if err != nil {
		return nil, err
	}
	if len(t.Dotenv) == 0 {
		return e.scope(t, call, base), nil
	}
	env := maps.Clone(base)
	scope := e.scope(t, call, env)
	dotenv, err := namespace.LoadDotenv(filepathext.SmartJoin(e.Dir, t.Dir), t.Dotenv, scope.Render)
	if err != nil {
		return nil, err
	}
	maps.Copy(env, dotenv)
	return scope, nil
}

// namespacedVars returns the templater resolver for a task, or nil for
// Taskfiles that are not version 42.
func (e *Executor) namespacedVars(t *ast.Task, call *Call, vars *ast.Vars) templater.NamespacedResolver {
	if e.TaskfileDAG == nil {
		return nil
	}
	scope, err := e.v42TaskScope(t, call, vars)
	if err != nil {
		return func([]string) map[string]any { return nil }
	}
	return scope.Resolve
}

func (e *Executor) v42TaskScope(t *ast.Task, call *Call, vars *ast.Vars) (*namespace.Scope, error) {
	scope, err := e.v42Scope(t, call)
	if err != nil {
		return nil, err
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
	if call != nil {
		addLocalVars(call.Vars)
	}
	addLocalVars(taskVars)
	return scope, nil
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

func (e *Executor) v42LuaVars(t *ast.Task, call *Call, vars *ast.Vars) (map[string]any, error) {
	scope, err := e.v42TaskScope(t, call, vars)
	if err != nil {
		return nil, err
	}
	taskVars := e.taskVarDeclarations(t)

	result := maps.Clone(scope.Env)
	maps.Copy(result, scope.Special)
	paths := []string{""}
	if t != nil && t.Namespace != "" {
		paths = append(paths, t.Namespace)
	}
	for _, path := range paths {
		for name := range e.TaskfileDAG.Vars(path) {
			ref := ":" + name
			if path != "" {
				ref = path + ast.NamespaceSeparator + name
			}
			if value, ok := scope.Resolve([]string{ref})[ref]; ok {
				result[name] = value
			}
		}
	}

	overlay := func(declarations *ast.Vars) {
		for name := range declarations.Keys() {
			if variable, ok := vars.Get(name); ok {
				setLocalVar(result, name, variable)
			}
		}
	}
	if call != nil {
		overlay(call.Vars)
	}
	overlay(taskVars)
	return result, nil
}

// v42TaskVars builds the vars handed to the task: the environment and special
// vars, then the call and task vars. DAG vars are not copied here, they are
// resolved on demand by the templater.
func (e *Executor) v42TaskVars(t *ast.Task, call *Call) (*ast.Vars, error) {
	scope, err := e.v42Scope(t, call)
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
		resolved := templater.ReplaceVar(v, cache)
		vars.Set(name, resolved)
		setLocalVar(scope.Local, name, resolved)
		cache.ResetCache()
	}
	return vars, cache.Err()
}
