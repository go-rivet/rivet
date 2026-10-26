package rivet

import (
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

func (e *Executor) v42Scope(t *ast.Task, call *Call) *namespace.Scope {
	e.envOnce.Do(func() { e.env = namespace.Environment() })
	alias := ""
	if call != nil {
		alias = call.Task
	}
	return &namespace.Scope{
		DAG:       e.TaskfileDAG,
		Namespace: t.Namespace,
		Env:       e.env,
		Special: namespace.SpecialVars(namespace.SpecialParams{
			RootDir:        e.RootDir,
			Entrypoint:     e.Entrypoint,
			Dir:            e.Dir,
			UserWorkingDir: e.UserWorkingDir,
			Alias:          alias,
		}, t),
	}
}

// namespacedVars returns the templater resolver for a task, or nil for
// Taskfiles that are not version 42.
func (e *Executor) namespacedVars(t *ast.Task, call *Call) templater.NamespacedResolver {
	if e.TaskfileDAG == nil {
		return nil
	}
	return e.v42Scope(t, call).Resolve
}

// v42TaskVars builds the vars handed to the task: the environment and special
// vars, then the call and task vars. DAG vars are not copied here, they are
// resolved on demand by the templater.
func (e *Executor) v42TaskVars(t *ast.Task, call *Call) (*ast.Vars, error) {
	scope := e.v42Scope(t, call)
	vars := ast.NewVarsWithCapacity(len(scope.Env) + len(scope.Special) + t.Vars.Len())
	for name, value := range scope.Env {
		vars.Set(name, ast.Var{Value: value})
	}
	for name, value := range scope.Special {
		vars.Set(name, ast.Var{Value: value})
	}
	cache := &templater.Cache{Vars: vars, Namespaced: scope.Resolve}
	if call != nil {
		for name, v := range call.Vars.All() {
			vars.Set(name, v)
		}
	}
	for name, v := range t.Vars.All() {
		resolved := templater.ReplaceVar(v, cache)
		vars.Set(name, resolved)
		cache.ResetCache()
	}
	return vars, cache.Err()
}
