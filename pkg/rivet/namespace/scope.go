package namespace

import (
	"maps"
	"os"
	"path/filepath"
	"strings"

	"github.com/go-rivet/rivet/internal/filepathext"
	"github.com/go-rivet/rivet/internal/templater"
	"github.com/go-rivet/rivet/internal/version"
	"github.com/go-rivet/rivet/pkg/rivet/taskfile/ast"
)

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
	Namespace string
	Special   map[string]any
	Env       map[string]any
	Local     map[string]any

	// visiting holds the vars being rendered, to detect reference cycles.
	visiting map[string]bool
}

// Resolve returns the value of each reference that can be resolved. A
// reference is relative to the scope's namespace ("name"), qualified
// ("ns:name") or rooted (":name").
func (s *Scope) Resolve(refs []string) map[string]any {
	resolved := make(map[string]any, len(refs))
	for _, ref := range refs {
		if value, ok := s.lookup(ref); ok {
			resolved[ref] = value
		}
	}
	return resolved
}

func (s *Scope) lookup(ref string) (any, bool) {
	path, name, belowRoot := s.Namespace, ref, true
	relative := true
	if i := strings.LastIndex(ref, ast.NamespaceSeparator); i >= 0 {
		path, name = ref[:i], ref[i+1:]
		belowRoot = path == ""
		relative = false
	}
	if relative {
		if value, ok := s.Local[name]; ok {
			return value, true
		}
	}
	if value, ok := s.DAG.Vars(path).Value(name); ok {
		return s.render(path, name, value)
	}
	if !belowRoot {
		return nil, false
	}
	if value, ok := s.Special[name]; ok {
		return value, true
	}
	value, ok := s.Env[name]
	return value, ok
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
func (s *Scope) render(path, name string, value any) (any, bool) {
	text, ok := value.(string)
	if !ok || !strings.Contains(text, "{{") {
		return value, true
	}
	key := path + ast.NamespaceSeparator + name
	if s.visiting[key] {
		return nil, false
	}
	visiting := map[string]bool{key: true}
	maps.Copy(visiting, s.visiting)
	node := &Scope{DAG: s.DAG, Namespace: path, Special: s.Special, Env: s.Env, Local: s.Local, visiting: visiting}
	cache := &templater.Cache{Vars: ast.NewVars(), Namespaced: node.Resolve}
	rendered := templater.Replace(text, cache)
	if cache.Err() != nil {
		return nil, false
	}
	return rendered, true
}

func localName(task *ast.Task) string {
	if name, ok := strings.CutPrefix(task.Task, task.Namespace+ast.NamespaceSeparator); ok && task.Namespace != "" {
		return name
	}
	return task.Task
}
