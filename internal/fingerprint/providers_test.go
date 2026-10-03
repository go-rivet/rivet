package fingerprint

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/go-rivet/rivet/pkg/rivet/taskfile/ast"
)

func TestWalkDirWithZglobMatchesMVdan(t *testing.T) {
	dir := t.TempDir()
	files := []string{
		"src/root.go",
		"src/nested/child.go",
		"src/nested/deep/grandchild.go",
		"src/readme.txt",
	}
	for _, name := range files {
		path := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}

	patterns := []*ast.Glob{
		{Glob: "src/**/*.go"},
		{Glob: "src/**/*.txt", Negate: true},
	}
	got, handled, err := walkDirWithZglob(dir, patterns)
	if err != nil {
		t.Fatal(err)
	}
	if !handled {
		t.Fatal("zglob backend did not handle simple glob patterns")
	}
	want, err := walkDirWithMVdan(context.Background(), dir, patterns)
	if err != nil {
		t.Fatal(err)
	}
	gotByGlob := make(map[string][]string, len(got))
	for _, result := range got {
		gotByGlob[result.glob] = entryNames(result.entries)
	}
	wantByGlob := make(map[string][]string, len(want))
	for _, result := range want {
		wantByGlob[result.glob] = entryNames(result.entries)
	}
	for _, pattern := range patterns {
		if !slices.Equal(gotByGlob[pattern.Glob], wantByGlob[pattern.Glob]) {
			t.Errorf("pattern %q: got entries=%v, want entries=%v", pattern.Glob, gotByGlob[pattern.Glob], wantByGlob[pattern.Glob])
		}
	}
}

func TestWalkDirWithZglobExpandsShellPath(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "src", "nested", "file.go")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("RIVET_TEST_GLOB_ROOT", "src")
	patterns := []*ast.Glob{{Glob: "$RIVET_TEST_GLOB_ROOT/**/*.go"}}

	got, handled, err := walkDirWithZglob(dir, patterns)
	if err != nil {
		t.Fatal(err)
	}
	if !handled {
		t.Fatal("zglob backend did not handle expanded glob pattern")
	}
	if names := entryNames(got[0].entries); !slices.Equal(names, []string{"file.go"}) {
		t.Fatalf("expanded glob entries = %v, want [file.go]", names)
	}
}

func TestWalkDirWithZglobFallsBackForUnsupportedShellSyntax(t *testing.T) {
	_, handled, err := walkDirWithZglob(t.TempDir(), []*ast.Glob{{Glob: "$(printf src)/**/*.go"}})
	if err != nil {
		t.Fatal(err)
	}
	if handled {
		t.Fatal("zglob backend should defer unsupported shell expansion to mvdan")
	}
}

func entryNames(entries []os.DirEntry) []string {
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		names = append(names, entry.Name())
	}
	slices.Sort(names)
	return names
}
