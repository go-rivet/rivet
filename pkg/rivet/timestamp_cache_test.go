package rivet

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/go-rivet/rivet/internal/fingerprint"
	"github.com/go-rivet/rivet/pkg/rivet/taskfile/ast"
)

func TestTimestampValueCache(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"src/build.go", "lib/build.go"} {
		path := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("package build\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	checker := fingerprint.NewTimestampChecker(filepath.Join(dir, ".task"), false)
	cache := &timestampValueCache{}
	task := &ast.Task{
		Task: "build",
		Dir:  dir,
		Transform: &ast.Transform{
			Matches: []*ast.Glob{{Glob: "src/**/*.go"}},
		},
	}

	first, err := cache.get(task, checker)
	if err != nil || first == nil || cache.snapshot == nil {
		t.Fatalf("first get = %v, %v; expected timestamp and snapshot", first, err)
	}
	firstSnapshot := cache.snapshot
	second, err := cache.get(&ast.Task{
		Task:      task.Task,
		Dir:       task.Dir,
		Transform: &ast.Transform{Matches: []*ast.Glob{{Glob: "src/**/*.go"}}},
	}, checker)
	if err != nil || second != first || cache.snapshot != firstSnapshot {
		t.Fatalf("same-input get = %v, %v; timestamp snapshot was not reused", second, err)
	}

	changed := &ast.Task{
		Task:      task.Task,
		Dir:       task.Dir,
		Transform: &ast.Transform{Matches: []*ast.Glob{{Glob: "lib/**/*.go"}}},
	}
	third, err := cache.get(changed, checker)
	if err != nil || third == nil || cache.snapshot == firstSnapshot {
		t.Fatalf("changed-input get = %v, %v; expected a new timestamp snapshot", third, err)
	}

	changedSnapshot := cache.snapshot
	cache.clear()
	fourth, err := cache.get(changed, checker)
	if err != nil || fourth == nil || cache.snapshot == changedSnapshot {
		t.Fatalf("get after clear = %v, %v; expected a fresh timestamp snapshot", fourth, err)
	}
}
