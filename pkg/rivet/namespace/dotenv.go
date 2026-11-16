// SPDX-FileCopyrightText: 2026 Timothy Rule
// SPDX-License-Identifier: MIT

package namespace

import (
	"fmt"
	"os"

	"github.com/go-rivet/rivet/internal/env"
	"github.com/go-rivet/rivet/internal/filepathext"
)

// LoadDotenv reads dotenv files relative to dir into a map. When a name is
// defined by several files the first file wins. Missing files are skipped. The
// render function, if set, expands templates in file names.
func LoadDotenv(dir string, files []string, render func(string) (string, error)) (map[string]any, error) {
	vars := map[string]any{}
	for _, file := range files {
		if render != nil {
			var err error
			if file, err = render(file); err != nil {
				return nil, err
			}
		}
		if file == "" {
			continue
		}
		path := filepathext.SmartJoin(dir, file)
		if _, err := os.Stat(path); os.IsNotExist(err) {
			continue
		}
		values, err := env.LoadDotenv(path)
		if err != nil {
			return nil, fmt.Errorf("error reading env file %s: %w", path, err)
		}
		for name, value := range values {
			if _, exists := vars[name]; !exists {
				vars[name] = value
			}
		}
	}
	return vars, nil
}
