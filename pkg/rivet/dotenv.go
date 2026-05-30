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
	dot.Vars = ast.NewVars()
	if cache == nil {
		cache = &templater.Cache{Vars: vars}
	}

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
		for key, value := range envs {
			if _, ok := dot.Vars.Get(key); !ok {
				dot.Vars.Set(key, ast.Var{Value: value})
				changed = true
			}
		}
	}

	return changed, nil
}
