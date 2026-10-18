package namespace

import (
	"context"
	"fmt"
	"path/filepath"
	"slices"

	zglob "github.com/mattn/go-zglob"

	"github.com/go-rivet/rivet/pkg/rivet/taskfile"
	"github.com/go-rivet/rivet/pkg/rivet/taskfile/ast"
)

// Discover searches rootDir for Taskfiles with a known name. Version 42
// Taskfiles that declare a namespace are added to the DAG and their qualified
// first task of each name to root.
func (dag *DAG) Discover(ctx context.Context, reader *taskfile.Reader, rootDir, rootLocation string, root *ast.Taskfile) error {
	// Only one Taskfile is read per directory, chosen by DefaultTaskfiles
	// order. The root Taskfile's directory is never searched.
	rootDirOfTaskfile := filepath.Clean(filepath.Dir(rootLocation))
	selected := map[string]int{}
	for i, name := range taskfile.DefaultTaskfiles {
		matches, err := zglob.GlobFollowSymlinks(filepath.ToSlash(filepath.Join(rootDir, "**", name)))
		if err != nil {
			return err
		}
		for _, match := range matches {
			dir := filepath.Dir(filepath.Clean(match))
			if dir == rootDirOfTaskfile {
				continue
			}
			if _, ok := selected[dir]; !ok {
				selected[dir] = i
			}
		}
	}
	dirs := make([]string, 0, len(selected))
	for dir := range selected {
		dirs = append(dirs, dir)
	}
	slices.Sort(dirs)

	for _, dir := range dirs {
		path := filepath.Join(dir, taskfile.DefaultTaskfiles[selected[dir]])
		node, err := taskfile.NewFileNode(path, filepath.Dir(path))
		if err != nil {
			return err
		}
		tf, err := reader.ReadRoot(ctx, node)
		if err != nil {
			return err
		}
		if !tf.Version.Equal(ast.V42) || tf.Namespace == "" {
			continue
		}
		QualifyTaskNames(tf)
		if err := dag.AddTaskfile(tf); err != nil {
			return fmt.Errorf("%s: %w", path, err)
		}
		// Only the first task of a name is listed; the others are reachable
		// through the DAG.
		for name, task := range tf.Tasks.All(nil) {
			if _, exists := root.Tasks.Get(name); !exists {
				root.Tasks.Set(name, task)
			}
		}
	}
	return nil
}
