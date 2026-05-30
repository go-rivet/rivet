package rivet

import (
	"context"

	"github.com/go-rivet/rivet/internal/env"
	"github.com/go-rivet/rivet/internal/execext"
	"github.com/go-rivet/rivet/pkg/rivet/errors"
	"github.com/go-rivet/rivet/pkg/rivet/taskfile/ast"
	"github.com/go-rivet/rivet/pkg/rlog"
)

// ErrPreconditionFailed is returned when a precondition fails
var ErrPreconditionFailed = errors.New("task: precondition not met")

func (e *Executor) areTaskPreconditionsMet(ctx context.Context, t *ast.Task) (bool, error) {
	for _, p := range t.Preconditions {
		err := execext.RunCommand(ctx, &execext.RunCommandOptions{
			Command: p.Sh,
			Dir:     t.Dir,
			Env:     env.GetFromVars(t.Vars),
		})
		if err != nil {
			if !errors.Is(err, context.Canceled) {
				rlog.Errf(ctx, rlog.Magenta, "task: %s\n", p.Msg)
			}
			return false, ErrPreconditionFailed
		}
	}

	return true, nil
}
