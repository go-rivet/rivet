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

	TaskfileDotenv DotEnv
	TaskfileVars   *ast.Vars

	dynamicCache   map[string]string
	muDynamicCache sync.Mutex
}

type mergeProc func() error

type mergeVars func() *ast.Vars

type mergeItem struct {
	name string    // Name of the merge item, for logging.
	cond bool      // Indicates if this mergeItem should be processed.
	vars mergeVars // Variables to be merged (overwrite existing).
	dir  *string   // Directory used when evaluating variables.
	proc mergeProc // Called to modify state between merge items.
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

func (c *Compiler) resolveAndSetVar(ctx context.Context, result *ast.Vars, k string, v ast.Var, dir string, evaluateSh bool) error {
	cache := &templater.Cache{Vars: result}
	newVar := templater.ReplaceVar(v, cache)

	set := func(key string, value ast.Var) {
		result.Set(key, value)
	}

	// Templating only (no shell evaluation).
	if !evaluateSh {
		if newVar.Value == nil {
			// If the variable should not be evaluated, but is nil, set it to an empty string.
			newVar.Value = ""
		}
		set(k, ast.Var{Value: newVar.Value, Sh: newVar.Sh})
		return nil
	}
	// Check cache error condition before continuing.
	if err := cache.Err(); err != nil {
		return err
	}
	// Variable already set, use use its value.
	if newVar.Value != nil || newVar.Sh == nil {
		set(k, ast.Var{Value: newVar.Value})
		return nil
	}
	// Resolve the variable.
	if static, err := c.HandleDynamicVar(ctx, newVar, dir, env.GetFromVars(result)); err == nil {
		set(k, ast.Var{Value: static})
	} else {
		return err
	}
	return nil
}

func (c *Compiler) mergeVars(ctx context.Context, dest *ast.Vars, source *ast.Vars, dir string, evaluateShVars bool) error {
	if source == nil || dest == nil {
		return nil
	}
	for k, v := range source.All() {
		if err := c.resolveAndSetVar(ctx, dest, k, v, dir, evaluateShVars); err != nil {
			return err
		}
	}
	return nil
}

func (c *Compiler) getVariables(ctx context.Context, t *ast.Task, call *Call, evaluateShVars bool) (*ast.Vars, error) {
	result := ast.NewVars()
	taskdir := ""
	taskOnly := (t != nil)
	taskCall := (t != nil && call != nil)

	processMergeItem := func(items []mergeItem) error {
		for _, m := range items {
			if m.proc != nil {
				if err := m.proc(); err != nil {
					return err
				}
			}
			if !m.cond {
				continue
			}
			if m.vars == nil {
				continue
			}
			dir := c.Dir
			if m.dir != nil {
				dir = *m.dir
			}
			evalSh := evaluateShVars
			if m.name == "SpecialVars" || m.name == "OS.Env" {
				evalSh = false
			}
			if err := c.mergeVars(ctx, result, m.vars(), dir, evalSh); err != nil {
				return err
			}
		}
		return nil
	}
	updateTaskdir := func() error {
		if t != nil {
			cache := &templater.Cache{Vars: result}
			dir := templater.Replace(t.Dir, cache)
			if err := cache.Err(); err != nil {
				return err
			}
			taskdir = filepathext.SmartJoin(c.Dir, dir)
		}
		return nil
	}
	resolveGlobalVarRefs := func() error {
		return nil
	}

	if err := processMergeItem([]mergeItem{
		{"OS.Env", true, func() *ast.Vars { return env.GetEnviron() }, nil, nil},
		{"SpecialVars", true, func() *ast.Vars { return c.getSpecialVars(t, call) }, nil, nil},
		{proc: updateTaskdir},
		{"TaskfileDotenv.Env", true, func() *ast.Vars { return c.TaskfileDotenv.Vars }, nil, nil},
		{"TaskDotenv.Env", taskOnly, func() *ast.Vars { return call.TaskDotenv.Vars }, nil, nil},
		{"Taskfile.Vars", true, func() *ast.Vars { return c.TaskfileVars }, nil, nil},
		{proc: resolveGlobalVarRefs},
		{"Inc.Vars", taskOnly, func() *ast.Vars { return t.IncludeVars }, nil, nil},
		{"IncTaskfile.Vars", taskOnly, func() *ast.Vars { return t.IncludedTaskfileVars }, &taskdir, nil},
		{"Call.Vars", taskCall, func() *ast.Vars { return call.Vars }, nil, nil},
		{"Task.Vars", taskCall, func() *ast.Vars { return t.Vars }, &taskdir, nil},
	}); err != nil {
		return nil, err
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

func (c *Compiler) getSpecialVars(t *ast.Task, call *Call) *ast.Vars {
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

	vars := ast.NewVars()
	for k, v := range allVars {
		vars.Set(k, ast.Var{Value: v})
	}
	return vars
}
