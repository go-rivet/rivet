// SPDX-FileCopyrightText: 2026 Timothy Rule
// SPDX-License-Identifier: MIT

package namespace

import (
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"strings"

	"github.com/go-rivet/rivet/internal/filepathext"
	"github.com/go-rivet/rivet/internal/templater"
	"github.com/go-rivet/rivet/internal/version"
	"github.com/go-rivet/rivet/pkg/rivet/taskfile/ast"
)

type shellVariableError struct {
	Err error
}

func (e *shellVariableError) Error() string { return e.Err.Error() }
func (e *shellVariableError) Unwrap() error { return e.Err }

var errVariableCycle = errors.New("cyclic variable reference")

// ErrShellEvaluationDisabled indicates that a shell variable cannot be evaluated in a fast scope.
var ErrShellEvaluationDisabled = errors.New("shell variable evaluation disabled")

// Environment returns the process environment as a map.
func Environment() map[string]any {
	raw := os.Environ()
	env := make(map[string]any, len(raw))
	for _, entry := range raw {
		if name, value, ok := strings.Cut(entry, "="); ok && name != "" {
			env[name] = value
		}
	}
	return env
}

// SpecialParams describes the invocation the special vars are derived from.
type SpecialParams struct {
	RootDir        string
	Entrypoint     string
	Dir            string
	UserWorkingDir string
	// Alias is the task name as it was called.
	Alias string
}

// SpecialVars returns the special vars for a task.
func SpecialVars(p SpecialParams, task *ast.Task) map[string]any {
	vars := map[string]any{
		"RIVET_EXE":        filepath.ToSlash(os.Args[0]),
		"RIVET_VERSION":    version.GetVersion(),
		"ROOT_DIR":         p.RootDir,
		"ROOT_TASKFILE":    filepathext.SmartJoin(p.RootDir, p.Entrypoint),
		"TASK_EXE":         filepath.ToSlash(os.Args[0]),
		"TASK_VERSION":     version.GetVersion(),
		"TASKFILE":         "",
		"TASKFILE_DIR":     "",
		"USER_WORKING_DIR": p.UserWorkingDir,
		// Task related.
		"ALIAS":     p.Alias,
		"NAMESPACE": "",
		"TASK":      "",
		"TASKNAME":  "",
		"TASK_DIR":  "",
	}
	if task == nil {
		return vars
	}
	vars["TASK"] = task.Task
	vars["NAMESPACE"] = task.Namespace
	vars["TASKNAME"] = localName(task)
	vars["TASK_DIR"] = filepath.ToSlash(filepathext.SmartJoin(p.Dir, task.Dir))
	if task.Location != nil {
		vars["TASKFILE"] = filepath.ToSlash(task.Location.Taskfile)
		vars["TASKFILE_DIR"] = filepath.ToSlash(filepath.Dir(task.Location.Taskfile))
	}
	return vars
}

// Scope resolves var references for one task. Lookups go to the DAG node the
// reference names. The special and environment vars sit below the root node,
// so the vars on a node override them.
type Scope struct {
	DAG *DAG
	// Namespace is the node that relative references resolve against.
	Namespace      string
	LocalNamespace string
	Special        map[string]any
	Env            map[string]any
	// RootEnv excludes the task's dotenv so rooted lookups bypass task values.
	RootEnv    map[string]any
	Local      map[string]any
	LocalVars  map[string]ast.Var
	EvaluateSh func(*Scope, ast.Var) (string, error)

	// visiting holds the vars being rendered, to detect reference cycles.
	visiting map[string]bool
}

// Resolve returns the value of each reference that can be resolved. A
// reference is relative to the scope's namespace ("name"), qualified
// ("ns:name") or rooted (":name").
func (s *Scope) Resolve(refs []string) (map[string]any, error) {
	resolved := make(map[string]any, len(refs))
	for _, ref := range refs {
		value, ok, err := s.ResolveReference(ref)
		if err != nil {
			return nil, err
		}
		if ok {
			resolved[ref] = value
		}
	}
	return resolved, nil
}

// ResolveVariable resolves a variable declared at an exact DAG path.
func (s *Scope) ResolveVariable(path, name string) (any, bool, error) {
	if variable, ok := s.DAG.Vars(path)[name]; ok {
		return s.render(path, name, variable, false)
	}
	return nil, false, nil
}

// ResolveReference resolves one reference using the same rules as Resolve.
func (s *Scope) ResolveReference(ref string) (any, bool, error) {
	path, name, belowRoot := s.Namespace, ref, true
	relative := true
	if i := strings.LastIndex(ref, ast.NamespaceSeparator); i >= 0 {
		path, name = ref[:i], ref[i+1:]
		belowRoot = path == ""
		relative = false
	}
	if relative {
		if value, ok := s.Local[name]; ok {
			return value, true, nil
		}
		if variable, ok := s.LocalVars[name]; ok {
			return s.render(s.LocalNamespace, name, variable, true)
		}
	}
	if _, ok := s.DAG.Vars(path)[name]; ok {
		return s.ResolveVariable(path, name)
	}
	if !belowRoot {
		return nil, false, nil
	}
	if value, ok := s.Special[name]; ok {
		return value, true, nil
	}
	env := s.Env
	if !relative && s.RootEnv != nil {
		env = s.RootEnv
	}
	value, ok := env[name]
	return value, ok, nil
}

// ResolveLocal resolves a task or call variable independently of its order.
func (s *Scope) ResolveLocal(name string) (any, bool, error) {
	if value, ok := s.Local[name]; ok {
		return value, true, nil
	}
	variable, ok := s.LocalVars[name]
	if !ok {
		return nil, false, nil
	}
	return s.render(s.LocalNamespace, name, variable, true)
}

// Render expands templates in text, relative to the scope's namespace.
func (s *Scope) Render(text string) (string, error) {
	if !strings.Contains(text, "{{") {
		return text, nil
	}
	cache := &templater.Cache{Vars: ast.NewVars(), Namespaced: s.Resolve}
	rendered := templater.Replace(text, cache)
	return rendered, cache.Err()
}

// render expands templates in a var value. The value is evaluated on demand,
// relative to the node that declares it.
func (s *Scope) render(path, name string, variable ast.Var, local bool) (any, bool, error) {
	if variable.Sh != nil && s.EvaluateSh == nil {
		return nil, false, ErrShellEvaluationDisabled
	}
	if variable.Sh == nil && variable.Ref == "" {
		if variable.Live != nil {
			return variable.Live, true, nil
		}
		text, isString := variable.Value.(string)
		if (isString && !strings.Contains(text, "{{")) || (!local && !isString) {
			return variable.Value, true, nil
		}
	}
	key := path + ast.NamespaceSeparator + name
	if local {
		key = "local:" + key
	}
	if s.visiting[key] {
		return nil, false, fmt.Errorf("task: %w: %q", errVariableCycle, key)
	}
	visiting := map[string]bool{key: true}
	maps.Copy(visiting, s.visiting)
	node := *s
	node.Namespace = path
	node.visiting = visiting
	base := ast.NewVars()
	for k, v := range s.Env {
		base.Set(k, ast.Var{Value: v})
	}
	for k, v := range s.Special {
		base.Set(k, ast.Var{Value: v})
	}
	for k, v := range s.Local {
		base.Set(k, ast.Var{Value: v})
	}
	cache := &templater.Cache{Vars: base, Namespaced: node.Resolve}
	resolved := templater.ReplaceVar(variable, cache)
	if err := cache.Err(); err != nil {
		var shellErr *shellVariableError
		if local || variable.Sh != nil || errors.As(err, &shellErr) || errors.Is(err, errVariableCycle) || errors.Is(err, ErrShellEvaluationDisabled) {
			return nil, false, err
		}
		return nil, false, nil
	}
	if resolved.Sh != nil {
		value, err := s.EvaluateSh(&node, resolved)
		if err != nil {
			return nil, false, &shellVariableError{Err: fmt.Errorf("task: shell variable %q: %w", key, err)}
		}
		return value, true, nil
	}
	if resolved.Live != nil {
		return resolved.Live, true, nil
	}
	return resolved.Value, true, nil
}

func localName(task *ast.Task) string {
	if name, ok := strings.CutPrefix(task.Task, task.Namespace+ast.NamespaceSeparator); ok && task.Namespace != "" {
		return name
	}
	return task.Task
}
