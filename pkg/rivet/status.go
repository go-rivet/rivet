package rivet

import (
	"context"
	"fmt"

	"github.com/go-rivet/rivet/internal/env"
	"github.com/go-rivet/rivet/internal/fingerprint"
	"github.com/go-rivet/rivet/internal/luascript"
	"github.com/go-rivet/rivet/pkg/rivet/errors"
	"github.com/go-rivet/rivet/pkg/rivet/taskfile/ast"
)

// Status returns an error if any the of given tasks is not up-to-date
func (e *Executor) Status(ctx context.Context, calls ...*Call) (runErr error) {
	ctx, cancel := withFatalContext(ctx)
	defer cancel(nil)
	defer returnFatalCause(ctx, &runErr)
	e.ctx = ctx

	for _, call := range calls {

		// Compile the task
		t, err := e.CompiledTask(call)
		if err != nil {
			return err
		}

		if t.Lua != "" {
			if e.SuperDry {
				return fmt.Errorf(`task: status of %q is unknown: task-level Lua is disabled by --super-dry`, t.Name())
			}
			shellVars, err := e.shellVars(ctx, t, call)
			if err != nil {
				return err
			}
			release := e.acquireConcurrencyLimit()
			err = e.runTaskLua(ctx, t, call, env.GetFromVars(shellVars))
			release()
			var flow *luascript.FlowError
			if errors.As(err, &flow) {
				if flow.Kind == luascript.FlowStatus {
					if _, flowErr := handleLuaFlow(ctx, flow); flowErr != nil {
						return flowErr
					}
					continue
				}
				if flow.Kind == luascript.FlowSkip || flow.Kind == luascript.FlowDone {
					if _, flowErr := handleLuaFlow(ctx, flow); flowErr != nil {
						return flowErr
					}
					return fmt.Errorf(`task: status of %q is unknown: Lua finished without reporting status`, t.Name())
				}
			}
			if err != nil {
				return err
			}
		}

		// Check if the task is up-to-date
		if len(t.Status) > 0 {
			t.Vars, err = e.shellVars(ctx, t, call)
			if err != nil {
				return err
			}
		}
		isUpToDate, err := fingerprint.IsTaskUpToDate(ctx, t,
			fingerprint.WithTempDir(e.TempDir.Fingerprint),
			fingerprint.WithDry(e.Dry),
		)
		if err != nil {
			return err
		}
		if !isUpToDate {
			return fmt.Errorf(`task: Task "%s" is not up-to-date`, t.Name())
		}
	}
	return nil
}

func (e *Executor) statusOnError(t *ast.Task) error {
	checker := fingerprint.NewTimestampChecker(e.TempDir.Fingerprint, e.Dry)
	return checker.OnError(t)
}
