package rivet

import "github.com/go-rivet/rivet/pkg/rivet/taskfile/ast"

// Call is the parameters to a task call
type Call struct {
	Task     string
	Vars     *ast.Vars
	Indirect bool // True if the task was called by another task
}
