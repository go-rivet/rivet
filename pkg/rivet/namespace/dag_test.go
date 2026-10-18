package namespace

import (
	"testing"

	"github.com/Masterminds/semver/v3"
	"github.com/stretchr/testify/require"

	"github.com/go-rivet/rivet/pkg/rivet/taskfile/ast"
)

func TestDAGAddsNamespacePaths(t *testing.T) {
	t.Parallel()

	taskfile := &ast.Taskfile{Namespace: "one:two", Version: semver.MustParse("42")}
	dag, err := NewDAG(taskfile)
	require.NoError(t, err)

	root, err := dag.Vertex("")
	require.NoError(t, err)
	require.Empty(t, root.Taskfiles)
	require.NoError(t, func() error {
		_, err := dag.Edge("", "one")
		return err
	}())
	require.NoError(t, func() error {
		_, err := dag.Edge("one", "one:two")
		return err
	}())

	got, ok := dag.Taskfile("one:two")
	require.True(t, ok)
	require.Same(t, taskfile, got)
}

func TestDAGAddsRootTaskfile(t *testing.T) {
	t.Parallel()

	taskfile := &ast.Taskfile{Version: semver.MustParse("42")}
	dag, err := NewDAG(taskfile)
	require.NoError(t, err)

	got, ok := dag.Taskfile("")
	require.True(t, ok)
	require.Same(t, taskfile, got)
}

func TestDAGRejectsInvalidNamespace(t *testing.T) {
	t.Parallel()

	_, err := NewDAG(&ast.Taskfile{Namespace: "one::two"})
	require.Error(t, err)
}

func TestQualifyTaskNames(t *testing.T) {
	t.Parallel()

	taskfile := &ast.Taskfile{
		Namespace: "one:two",
		Tasks: ast.NewTasks(
			&ast.TaskElement{
				Key: "hello",
				Value: &ast.Task{
					Cmds: []*ast.Cmd{{Task: "world"}},
					Deps: []*ast.Dep{{Task: "world"}},
				},
			},
			&ast.TaskElement{Key: "world", Value: &ast.Task{}},
		),
	}

	QualifyTaskNames(taskfile)

	hello, ok := taskfile.Tasks.Get("one:two:hello")
	require.True(t, ok)
	require.Equal(t, "one:two", hello.Namespace)
	require.Equal(t, "one:two:world", hello.Cmds[0].Task)
	require.Equal(t, "one:two:world", hello.Deps[0].Task)
}

func TestDAGResolvesDuplicateTasks(t *testing.T) {
	t.Parallel()

	newTaskfile := func(location string) *ast.Taskfile {
		return &ast.Taskfile{Location: location, Namespace: "a:b", Version: semver.MustParse("42"), Tasks: ast.NewTasks(
			&ast.TaskElement{Key: "a:b:build", Value: &ast.Task{Task: "a:b:build"}},
		)}
	}
	first, second := newTaskfile("one.yml"), newTaskfile("two.yml")
	dag, err := NewDAG(first)
	require.NoError(t, err)
	require.NoError(t, dag.AddTaskfile(second))

	require.Equal(t, []*ast.Taskfile{first, second}, dag.Taskfiles("a:b"))
	require.Len(t, dag.Entries("a:b:build"), 2)
	require.Len(t, dag.Resolve("a:b:build", ""), 2)

	got := dag.Resolve("a:b:build", "two.yml")
	require.Len(t, got, 1)
	require.Same(t, second, got[0].Taskfile)

	// A caller from a Taskfile that does not define the task gets all of them.
	require.Len(t, dag.Resolve("a:b:build", "other.yml"), 2)
	require.Empty(t, dag.Resolve("a:b:missing", "one.yml"))
}
