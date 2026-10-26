package namespace

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/go-rivet/rivet/internal/filepathext"
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
		"TASK_EXE":         filepath.ToSlash(os.Args[0]),
		"ROOT_TASKFILE":    filepathext.SmartJoin(p.RootDir, p.Entrypoint),
		"ROOT_DIR":         p.RootDir,
		"USER_WORKING_DIR": p.UserWorkingDir,
		"TASK_VERSION":     version.GetVersion(),
		"ALIAS":            p.Alias,
		"TASK":             "",
		"NAMESPACE":        "",
		"TASKNAME":         "",
		"TASK_DIR":         "",
		"TASKFILE":         "",
		"TASKFILE_DIR":     "",
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
	if i := strings.LastIndex(ref, ast.NamespaceSeparator); i >= 0 {
		path, name = ref[:i], ref[i+1:]
		belowRoot = path == ""
	}
	if value, ok := s.DAG.Vars(path).Value(name); ok {
		return value, true
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

func localName(task *ast.Task) string {
	if name, ok := strings.CutPrefix(task.Task, task.Namespace+ast.NamespaceSeparator); ok && task.Namespace != "" {
		return name
	}
	return task.Task
}
