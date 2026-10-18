package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/rogpeppe/go-internal/testscript"
)

func TestMain(m *testing.M) {
	os.Exit(testscript.RunMain(m, map[string]func() int{
		"rivet": func() int {
			main()
			return 0
		},
	}))
}

// TestScript runs the end-to-end scripts in each subdirectory of testdata/script
// against the rivet command.
func TestScript(t *testing.T) {
	root := "testdata/script"
	var dirs []string
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.IsDir() {
			dirs = append(dirs, filepath.Join(root, entry.Name()))
		}
	}
	for _, dir := range dirs {
		t.Run(filepath.Base(dir), func(t *testing.T) {
			testscript.Run(t, testscript.Params{
				Dir: dir,
				Setup: func(env *testscript.Env) error {
					env.Setenv("NO_COLOR", "1")
					return nil
				},
			})
		})
	}
}
