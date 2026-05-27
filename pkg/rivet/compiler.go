package rivet

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/go-rivet/rivet/internal/env"
	"github.com/go-rivet/rivet/internal/execext"
	"github.com/go-rivet/rivet/internal/filepathext"
	"github.com/go-rivet/rivet/internal/templater"
	"github.com/go-rivet/rivet/internal/version"
	"github.com/go-rivet/rivet/pkg/rivet/taskfile/ast"
	"github.com/go-rivet/rivet/pkg/rlog"
)

type Compiler struct {
	Dir            string
	Entrypoint     string
	UserWorkingDir string
	RootDir        string

	TaskfileEnv  *ast.Vars
	TaskfileVars *ast.Vars

	dynamicCache   map[string]string
	muDynamicCache sync.Mutex
}

func (c *Compiler) GetTaskfileVariables(ctx context.Context) (*ast.Vars, error) {
	return c.getVariables(ctx, nil, nil, true)
}

func (c *Compiler) GetVariables(ctx context.Context, t *ast.Task, call *Call) (*ast.Vars, error) {
	return c.getVariables(ctx, t, call, true)
}

func (c *Compiler) FastGetVariables(ctx context.Context, t *ast.Task, call *Call) (*ast.Vars, error) {
	return c.getVariables(ctx, t, call, false)
}

func (c *Compiler) getVariables(ctx context.Context, t *ast.Task, call *Call, evaluateShVars bool) (*ast.Vars, error) {
	result := env.GetEnviron()
	specialVars, err := c.getSpecialVars(t, call)
	if err != nil {
		return nil, err
	}
	for k, v := range specialVars {
		result.Set(k, ast.Var{Value: v})
	}

	getRangeFunc := func(dir string) func(k string, v ast.Var) error {
		return func(k string, v ast.Var) error {
			cache := &templater.Cache{Vars: result}
			// Replace values
			newVar := templater.ReplaceVar(v, cache)
			// If the variable should not be evaluated, but is nil, set it to an empty string
			// This stops empty interface errors when using the templater to replace values later
			// Preserve the Sh field so it can be displayed in summary
			if !evaluateShVars && newVar.Value == nil {
				result.Set(k, ast.Var{Value: "", Sh: newVar.Sh})
				return nil
			}
			// If the variable should not be evaluated and it is set, we can set it and return
			if !evaluateShVars {
				result.Set(k, ast.Var{Value: newVar.Value, Sh: newVar.Sh})
				return nil
			}
			// Now we can check for errors since we've handled all the cases when we don't want to evaluate
			if err := cache.Err(); err != nil {
				return err
			}
			// If the variable is already set, we can set it and return
			if newVar.Value != nil || newVar.Sh == nil {
				result.Set(k, ast.Var{Value: newVar.Value})
				return nil
			}
			// If the variable is dynamic, we need to resolve it first
			static, err := c.HandleDynamicVar(ctx, newVar, dir, env.GetFromVars(result))
			if err != nil {
				return err
			}
			result.Set(k, ast.Var{Value: static})
			return nil
		}
	}
	rangeFunc := getRangeFunc(c.Dir)

	var taskRangeFunc func(k string, v ast.Var) error
	if t != nil {
		// NOTE(@andreynering): We're manually joining these paths here because
		// this is the raw task, not the compiled one.
		cache := &templater.Cache{Vars: result}
		dir := templater.Replace(t.Dir, cache)
		if err := cache.Err(); err != nil {
			return nil, err
		}
		dir = filepathext.SmartJoin(c.Dir, dir)
		taskRangeFunc = getRangeFunc(dir)
	}

	for k, v := range c.TaskfileEnv.All() {
		if err := rangeFunc(k, v); err != nil {
			return nil, err
		}
	}
	for k, v := range c.TaskfileVars.All() {
		if err := rangeFunc(k, v); err != nil {
			return nil, err
		}
	}
	if t != nil {
		for k, v := range t.IncludeVars.All() {
			if err := rangeFunc(k, v); err != nil {
				return nil, err
			}
		}
		for k, v := range t.IncludedTaskfileVars.All() {
			if err := taskRangeFunc(k, v); err != nil {
				return nil, err
			}
		}
	}

	if t == nil || call == nil {
		return result, nil
	}

	for k, v := range call.Vars.All() {
		if err := rangeFunc(k, v); err != nil {
			return nil, err
		}
	}
	for k, v := range t.Vars.All() {
		if err := taskRangeFunc(k, v); err != nil {
			return nil, err
		}
	}

	return result, nil
}

func (c *Compiler) HandleDynamicVar(ctx context.Context, v ast.Var, dir string, e []string) (string, error) {
	c.muDynamicCache.Lock()
	defer c.muDynamicCache.Unlock()

	// If the variable is not dynamic or it is empty, return an empty string
	if v.Sh == nil || *v.Sh == "" {
		return "", nil
	}

	if c.dynamicCache == nil {
		c.dynamicCache = make(map[string]string, 30)
	}
	if result, ok := c.dynamicCache[*v.Sh]; ok {
		return result, nil
	}

	// NOTE(@andreynering): If a var have a specific dir, use this instead
	if v.Dir != "" {
		dir = v.Dir
	}

	var stderr io.Writer = os.Stderr
	if ext, ok := slog.Default().Handler().(*rlog.RlogHandler); ok {
		stderr = ext.Stderr
	}

	var stdout bytes.Buffer
	opts := &execext.RunCommandOptions{
		Command: *v.Sh,
		Dir:     dir,
		Stdout:  &stdout,
		Stderr:  stderr,
		Env:     e,
	}
	if err := execext.RunCommand(ctx, opts); err != nil {
		return "", fmt.Errorf(`task: Command "%s" failed: %s`, opts.Command, err)
	}

	// Trim a single trailing newline from the result to make most command
	// output easier to use in shell commands.
	result := strings.TrimSuffix(stdout.String(), "\r\n")
	result = strings.TrimSuffix(result, "\n")

	c.dynamicCache[*v.Sh] = result
	rlog.Debugf(ctx, "task: dynamic variable: %q result: %q\n", *v.Sh, result)

	return result, nil
}

// ResetCache clear the dynamic variables cache
func (c *Compiler) ResetCache() {
	c.muDynamicCache.Lock()
	defer c.muDynamicCache.Unlock()

	c.dynamicCache = nil
}

func (c *Compiler) getSpecialVars(t *ast.Task, call *Call) (map[string]string, error) {
	// Use filepath.ToSlash for all paths to ensure consistent forward slashes
	// across platforms. This prevents issues with backslashes being interpreted
	// as escape sequences when paths are used in shell commands on Windows.
	allVars := map[string]string{
		"TASK_EXE":         filepath.ToSlash(os.Args[0]),
		"ROOT_TASKFILE":    filepathext.SmartJoin(c.RootDir, c.Entrypoint),
		"ROOT_DIR":         c.RootDir,
		"USER_WORKING_DIR": c.UserWorkingDir,
		"TASK_VERSION":     version.GetVersion(),
	}
	if t != nil {
		allVars["TASK"] = t.Task
		allVars["TASK_DIR"] = filepath.ToSlash(filepathext.SmartJoin(c.Dir, t.Dir))
		allVars["TASKFILE"] = filepath.ToSlash(t.Location.Taskfile)
		allVars["TASKFILE_DIR"] = filepath.ToSlash(filepath.Dir(t.Location.Taskfile))
	} else {
		allVars["TASK"] = ""
		allVars["TASK_DIR"] = ""
		allVars["TASKFILE"] = ""
		allVars["TASKFILE_DIR"] = ""
	}
	if call != nil {
		allVars["ALIAS"] = call.Task
	} else {
		allVars["ALIAS"] = ""
	}

	return allVars, nil
}
