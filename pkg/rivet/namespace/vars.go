package namespace

import (
	"github.com/go-rivet/rivet/pkg/rivet/taskfile/ast"
)

// Vars is the set of variables declared on a DAG node. It is a plain map:
// when several Taskfiles share a node their vars are consolidated here, with
// later Taskfiles overriding earlier ones.
type Vars map[string]ast.Var

// Merge copies every var from src into v.
func (v Vars) Merge(src *ast.Vars) {
	if src == nil {
		return
	}
	for name, value := range src.All() {
		v[name] = value
	}
}

// Value returns the value of the var, if it can be evaluated without running
// anything.
func (v Vars) Value(name string) (any, bool) {
	variable, ok := v[name]
	if !ok || (variable.Sh != nil && *variable.Sh != "") {
		return nil, false
	}
	if variable.Live != nil {
		return variable.Live, true
	}
	return variable.Value, true
}

// Vars returns the consolidated vars stored on the node at path.
func (dag *DAG) Vars(path string) Vars {
	if dag == nil {
		return nil
	}
	vertex, ok := dag.vertices[path]
	if !ok {
		return nil
	}
	return vertex.Vars
}
