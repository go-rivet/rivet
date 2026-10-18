package taskfile

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestReadNodeSelectsSchemaParser(t *testing.T) {
	t.Parallel()

	for _, version := range []string{"3", "42"} {
		t.Run(version, func(t *testing.T) {
			t.Parallel()

			dir := t.TempDir()
			taskfile := "version: '" + version + "'\ntasks:\n  hello:\n    cmds:\n      - echo hello\n"
			require.NoError(t, os.WriteFile(filepath.Join(dir, "Taskfile.yml"), []byte(taskfile), 0o644))

			node := &FileNode{
				baseNode:   NewBaseNode(dir),
				entrypoint: filepath.Join(dir, "Taskfile.yml"),
			}

			got, err := NewReader().readNode(context.Background(), node)
			require.NoError(t, err)
			require.Equal(t, version+".0.0", got.Version.String())

			task, ok := got.Tasks.Get("hello")
			require.True(t, ok)
			require.Equal(t, "echo hello", task.Cmds[0].Cmd)
		})
	}

}

func TestReadRootDoesNotReadIncludes(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	taskfile := "version: '42'\nnamespace: one:two\nincludes:\n  missing: missing.yml\n"
	require.NoError(t, os.WriteFile(filepath.Join(dir, "Taskfile.yml"), []byte(taskfile), 0o644))

	node := &FileNode{
		baseNode:   NewBaseNode(dir),
		entrypoint: filepath.Join(dir, "Taskfile.yml"),
	}
	got, err := NewReader().ReadRoot(context.Background(), node)
	require.NoError(t, err)
	require.Equal(t, "one:two", got.Namespace)
}
