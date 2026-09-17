package rivet

import (
	"fmt"
	"os"

	ienv "github.com/go-rivet/rivet/internal/env"
	"github.com/go-rivet/rivet/internal/filepathext"
	"github.com/go-rivet/rivet/internal/templater"
	"github.com/go-rivet/rivet/pkg/rivet/taskfile/ast"
)

type DotEnv struct {
	Files []string
	Vars  *ast.Vars
}

func (dot *DotEnv) Load(dir string, vars *ast.Vars, cache *templater.Cache) (changed bool, err error) {
	changed = false
	if cache == nil {
		cache = &templater.Cache{Vars: vars}
	}

	// Staging arrays to calculate max capacity and preserve chronological order
	type stage struct {
		envs map[string]string
	}
	stagedFiles := make([]stage, 0, len(dot.Files))
	maxCapacity := 0

	// Step 1: Pre-read and validate files to calculate total max potential capacity
	for _, file := range dot.Files {
		path := templater.Replace(file, cache)
		if path == "" {
			continue
		}
		path = filepathext.SmartJoin(dir, path)
		if _, err := os.Stat(path); os.IsNotExist(err) {
			continue
		}

		envs, err := ienv.LoadDotenv(path)
		if err != nil {
			return changed, fmt.Errorf("error reading env file %s: %w", path, err)
		}

		if len(envs) > 0 {
			maxCapacity += len(envs)
			stagedFiles = append(stagedFiles, stage{envs: envs})
		}
	}

	// Step 2: Initialize the OrderedMap with the full capacity immediately
	if maxCapacity > 0 {
		dot.Vars = ast.NewVarsWithCapacity(maxCapacity)
	} else {
		dot.Vars = ast.NewVars()
		return changed, nil
	}

	// Step 3: Populate sequential keys. First-write-wins by guarding with !exists.
	for _, sf := range stagedFiles {
		for key, value := range sf.envs {
			if _, exists := dot.Vars.Get(key); !exists {
				dot.Vars.Set(key, ast.Var{Value: value})
				changed = true
			}
		}
	}

	return changed, nil
}
