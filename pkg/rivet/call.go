package rivet

import "github.com/go-rivet/rivet/pkg/rivet/taskfile/ast"

// CallKind describes the relationship between a task call and its caller.
type CallKind string

const (
	CallKindDirect CallKind = "direct" // requested directly (CLI arg or Run())
	CallKindDep    CallKind = "dep"    // invoked as a task's dependency
	CallKindCmd    CallKind = "cmd"    // invoked via a cmd: task: reference
)

// Call is the parameters to a task call
type Call struct {
	Task     string
	Vars     *ast.Vars
	Indirect bool // True if the task was called by another task
	Kind     CallKind

	TaskDotenv DotEnv
}
