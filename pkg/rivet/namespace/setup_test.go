package namespace_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/go-rivet/rivet/pkg/rivet"
)

func TestSetupVersion42ReadsRootTaskfileIntoDAG(t *testing.T) {
	t.Parallel()

	for _, namespace := range []string{"", "one:two"} {
		t.Run(namespace, func(t *testing.T) {
			dir := t.TempDir()
			taskfile := "version: 42\n"
			if namespace != "" {
				taskfile += "namespace: " + namespace + "\n"
			}
			taskfile += "tasks:\n  hello:\n    cmds:\n      - echo hello\n"
			entrypoint := filepath.Join(dir, "Taskfile.yml")
			require.NoError(t, os.WriteFile(entrypoint, []byte(taskfile), 0o644))

			executor := rivet.NewExecutor(rivet.WithDir(dir), rivet.WithEntrypoint(entrypoint))
			require.NoError(t, executor.Setup(context.Background()))
			require.NotNil(t, executor.TaskfileDAG)

			taskfileAtNamespace, ok := executor.TaskfileDAG.Taskfile(namespace)
			require.True(t, ok)
			require.Equal(t, namespace, taskfileAtNamespace.Namespace)

			if namespace == "" {
				require.Same(t, taskfileAtNamespace, executor.Taskfile)
			} else {
				_, ok = executor.TaskfileDAG.Taskfile("")
				require.False(t, ok)
				task, ok := executor.Taskfile.Tasks.Get("one:two:hello")
				require.True(t, ok)
				require.Equal(t, namespace, task.Namespace)
			}
		})
	}
}
