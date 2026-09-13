package config

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/go-rivet/rivet/internal/env"
	"github.com/go-rivet/rivet/internal/fsext"
	"github.com/go-rivet/rivet/pkg/rivet/taskfile"
)

// PeekTaskfilePath inspects CLI arguments before full flag parsing to locate
// the root Taskfile using -t/--taskfile, -d/--dir, or -g/--global.
func PeekTaskfilePath(cliArgs []string) string {
	var entrypoint string
	var dir string
	var global bool

	for i := 0; i < len(cliArgs); i++ {
		arg := cliArgs[i]
		if arg == "--" {
			break
		}

		if arg == "-g" || arg == "--global" {
			global = true
		} else if strings.HasPrefix(arg, "-g=") || strings.HasPrefix(arg, "--global=") {
			val := strings.SplitN(arg, "=", 2)[1]
			global, _ = strconv.ParseBool(val)
		} else if arg == "-t" || arg == "--taskfile" {
			if i+1 < len(cliArgs) {
				entrypoint = cliArgs[i+1]
				i++
			}
		} else if strings.HasPrefix(arg, "-t=") || strings.HasPrefix(arg, "--taskfile=") {
			entrypoint = strings.SplitN(arg, "=", 2)[1]
		} else if arg == "-d" || arg == "--dir" {
			if i+1 < len(cliArgs) {
				dir = cliArgs[i+1]
				i++
			}
		} else if strings.HasPrefix(arg, "-d=") || strings.HasPrefix(arg, "--dir=") {
			dir = strings.SplitN(arg, "=", 2)[1]
		}
	}

	if global {
		if home, err := os.UserHomeDir(); err == nil {
			dir = home
		}
	}

	taskfilePath, err := fsext.Search(entrypoint, dir, taskfile.DefaultTaskfiles)
	if err != nil {
		return ""
	}
	return taskfilePath
}

// FindEnvFile checks candidate paths in sequential priority order and returns the first matching file.
// If taskfilePath is non-empty, its directory is checked for local rivet.env / .rivet.env files.
func FindEnvFile(taskfilePath string) string {
	var taskfileVisibleEnv string
	var taskfileHiddenEnv string
	if taskfilePath != "" {
		taskfileDir := filepath.Dir(taskfilePath)
		taskfileVisibleEnv = filepath.Join(taskfileDir, "rivet.env")
		taskfileHiddenEnv = filepath.Join(taskfileDir, ".rivet.env")
	}

	var xdgConfigPath string
	if baseDir, err := os.UserConfigDir(); err == nil && baseDir != "" {
		xdgConfigPath = filepath.Join(baseDir, "rivet", "rivet.env")
	}

	var homeDotfilePath string
	if homeDir, err := os.UserHomeDir(); err == nil && homeDir != "" {
		homeDotfilePath = filepath.Join(homeDir, ".rivet.env")
	}

	searchTable := []struct {
		description string
		path        string
	}{
		{"Explicit Environment Override ($RIVET_ENV)", os.Getenv("RIVET_ENV")},
		{"Local Workspace Visible File (./rivet.env)", "./rivet.env"},
		{"Local Workspace Hidden File (./.rivet.env)", "./.rivet.env"},
		{"Taskfile Directory Visible File", taskfileVisibleEnv},
		{"Taskfile Directory Hidden File", taskfileHiddenEnv},
		{"Modern Linux XDG Configuration Path (~/.config/rivet/rivet.env)", xdgConfigPath},
		{"Legacy User Home Directory Dotfile (~/.rivet.env)", homeDotfilePath},
	}

	for _, target := range searchTable {
		if target.path == "" {
			continue
		}

		if info, err := os.Stat(target.path); err == nil && !info.IsDir() {
			return target.path
		}
	}

	return ""
}

// LoadEnv loads environment variables from a single env file
// without overwriting variables already present in the environment.
func LoadEnv(filename string) error {
	if _, err := os.Stat(filename); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	envMap, err := env.LoadDotenv(filename)
	if err != nil {
		return err
	}

	for key, val := range envMap {
		if _, exists := os.LookupEnv(key); !exists {
			if err := os.Setenv(key, val); err != nil {
				return err
			}
		}
	}

	return nil
}
