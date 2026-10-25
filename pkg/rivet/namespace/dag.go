package namespace

import (
	"fmt"
	"sort"
	"strings"

	"github.com/dominikbraun/graph"

	"github.com/go-rivet/rivet/pkg/rivet/taskfile/ast"
)

// DAG stores Taskfiles at colon-delimited namespace paths.
type DAG struct {
	graph.Graph[string, *Vertex]
	vertices map[string]*Vertex
	// tasks maps a qualified task name to every Taskfile task defined under it,
	// in the order the Taskfiles were added.
	tasks map[string][]*TaskEntry
}

// TaskEntry is a task together with the Taskfile that defines it.
type TaskEntry struct {
	Task     *ast.Task
	Taskfile *ast.Taskfile
}

// Vertex represents a namespace path in a DAG.
type Vertex struct {
	Path      string
	Taskfiles []*ast.Taskfile
}

func dagHash(vertex *Vertex) string {
	return vertex.Path
}

// NewDAG creates a DAG with an empty root namespace and adds taskfile at its
// declared namespace.
func NewDAG(taskfile *ast.Taskfile) (*DAG, error) {
	dag := &DAG{
		Graph:    graph.New(dagHash, graph.Directed(), graph.PreventCycles(), graph.Rooted()),
		vertices: map[string]*Vertex{},
		tasks:    map[string][]*TaskEntry{},
	}
	root := &Vertex{}
	if err := dag.AddVertex(root); err != nil {
		return nil, err
	}
	dag.vertices[""] = root
	if err := dag.AddTaskfile(taskfile); err != nil {
		return nil, err
	}
	return dag, nil
}

// AddTaskfile adds taskfile and any missing namespace path vertices to the DAG.
func (dag *DAG) AddTaskfile(taskfile *ast.Taskfile) error {
	if taskfile == nil {
		return fmt.Errorf("task: cannot add a nil Taskfile to the namespace DAG")
	}

	path := taskfile.Namespace
	if err := validatePath(path); err != nil {
		return err
	}

	parent := dag.vertices[""]
	currentPath := ""
	for _, component := range strings.Split(path, ast.NamespaceSeparator) {
		if component == "" {
			continue
		}
		if currentPath == "" {
			currentPath = component
		} else {
			currentPath += ast.NamespaceSeparator + component
		}
		vertex, ok := dag.vertices[currentPath]
		if !ok {
			vertex = &Vertex{Path: currentPath}
			if err := dag.AddVertex(vertex); err != nil {
				return err
			}
			if err := dag.AddEdge(parent.Path, vertex.Path); err != nil {
				return err
			}
			dag.vertices[currentPath] = vertex
		}
		parent = vertex
	}
	parent.Taskfiles = append(parent.Taskfiles, taskfile)
	if taskfile.Tasks != nil {
		for name, task := range taskfile.Tasks.All(nil) {
			dag.tasks[name] = append(dag.tasks[name], &TaskEntry{Task: task, Taskfile: taskfile})
		}
	}
	return nil
}

// Taskfiles returns the Taskfiles stored at path in the order they were added.
func (dag *DAG) Taskfiles(path string) []*ast.Taskfile {
	vertex, ok := dag.vertices[path]
	if !ok {
		return nil
	}
	return vertex.Taskfiles
}

// Taskfile returns the first Taskfile stored at path, if any.
func (dag *DAG) Taskfile(path string) (*ast.Taskfile, bool) {
	taskfiles := dag.Taskfiles(path)
	if len(taskfiles) == 0 {
		return nil, false
	}
	return taskfiles[0], true
}

// Entries returns every task registered under the qualified name.
func (dag *DAG) Entries(name string) []*TaskEntry {
	if dag == nil {
		return nil
	}
	return dag.tasks[name]
}

// Tails returns the tasks called name that are defined in a non-root namespace,
// ordered by qualified name. Tasks in the root namespace are excluded.
func (dag *DAG) Tails(name string) []*TaskEntry {
	if dag == nil {
		return nil
	}
	keys := make([]string, 0, len(dag.tasks))
	for key := range dag.tasks {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	var result []*TaskEntry
	for _, key := range keys {
		for _, entry := range dag.tasks[key] {
			if entry.Task == nil || entry.Task.Namespace == "" {
				continue
			}
			local := strings.TrimPrefix(entry.Task.Task, entry.Task.Namespace+ast.NamespaceSeparator)
			if local == name {
				result = append(result, entry)
			}
		}
	}
	return result
}

// Resolve returns the tasks to run for a call to name. When the call comes
// from a task in the Taskfile at callerLocation and that Taskfile defines name,
// only that Taskfile's task is returned, so namespace relative references stay
// within their own Taskfile. Otherwise every task registered under name is
// returned.
func (dag *DAG) Resolve(name string, callerLocation string) []*TaskEntry {
	entries := dag.Entries(name)
	if callerLocation == "" {
		return entries
	}
	for _, entry := range entries {
		if entry.Taskfile.Location == callerLocation {
			return []*TaskEntry{entry}
		}
	}
	return entries
}

// QualifyTaskNames applies a Taskfile's namespace to its task names and local
// task references so existing task execution can address the DAG Taskfile.
func QualifyTaskNames(taskfile *ast.Taskfile) {
	if taskfile == nil || taskfile.Namespace == "" {
		return
	}

	tasks := ast.NewTasks()
	for name, task := range taskfile.Tasks.All(nil) {
		if task == nil {
			continue
		}
		for _, dependency := range task.Deps {
			if dependency != nil && dependency.Task != "" {
				dependency.Task = qualifyTaskReference(dependency.Task, taskfile.Namespace)
			}
		}
		for _, command := range task.Cmds {
			if command != nil && command.Task != "" {
				command.Task = qualifyTaskReference(command.Task, taskfile.Namespace)
			}
		}
		for i, alias := range task.Aliases {
			task.Aliases[i] = qualifyTaskName(alias, taskfile.Namespace)
		}

		task.Namespace = taskfile.Namespace
		// Definitions are joined literally: only references may use a leading ":".
		task.Task = taskfile.Namespace + ast.NamespaceSeparator + name
		tasks.Set(task.Task, task)
	}
	taskfile.Tasks = tasks
}

func validatePath(path string) error {
	for _, component := range strings.Split(path, ast.NamespaceSeparator) {
		if component == "" && path != "" {
			return fmt.Errorf("task: namespace %q contains an empty path component", path)
		}
	}
	return nil
}

func qualifyTaskName(name string, namespace string) string {
	if localName, ok := strings.CutPrefix(name, ast.NamespaceSeparator); ok {
		return localName
	}
	return namespace + ast.NamespaceSeparator + name
}

func qualifyTaskReference(name string, namespace string) string {
	if rootName, ok := strings.CutPrefix(name, ast.NamespaceSeparator); ok {
		return ast.NamespaceSeparator + rootName
	}
	if strings.Contains(name, ast.NamespaceSeparator) {
		return ast.NamespaceSeparator + name
	}
	return qualifyTaskName(name, namespace)
}
