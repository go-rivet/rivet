package main

import (
	"os"
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

// TestScript runs the end-to-end scripts in testdata/script against the rivet command.
func TestScript(t *testing.T) {
	testscript.Run(t, testscript.Params{
		Dir: "testdata/script",
		Setup: func(env *testscript.Env) error {
			env.Setenv("NO_COLOR", "1")
			return nil
		},
	})
}
