package fingerprint

import (
	"context"

	"github.com/go-rivet/rivet/internal/env"
	"github.com/go-rivet/rivet/internal/execext"
	"github.com/go-rivet/rivet/pkg/rivet/taskfile/ast"
	"github.com/go-rivet/rivet/pkg/rlog"
)

type StatusChecker struct {
}

func NewStatusChecker() StatusCheckable {
	return &StatusChecker{}
}

func (checker *StatusChecker) IsUpToDate(ctx context.Context, t *ast.Task) (bool, error) {
	for _, s := range t.Status {
		err := execext.RunCommand(ctx, &execext.RunCommandOptions{
			Command: s,
			Dir:     t.Dir,
			Env:     env.GetFromVars(t.Vars),
		})
		if err != nil {
			rlog.Debugf(ctx, "task: status command %s exited non-zero: %s\n", s, err)
			return false, nil
		}
		rlog.Debugf(ctx, "task: status command %s exited zero\n", s)
	}
	return true, nil
}
