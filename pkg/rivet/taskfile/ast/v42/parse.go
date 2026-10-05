package v42

import (
	"fmt"

	"go.yaml.in/yaml/v3"

	"github.com/go-rivet/rivet/pkg/rivet/taskfile/ast"
)

// Parse decodes a version 42 Taskfile document with the v42 schema types and
// returns the runtime model consumed by the compiler.
func Parse(node *yaml.Node) (*ast.Taskfile, error) {
	var tf Taskfile
	if err := node.Decode(&tf); err != nil {
		return nil, err
	}
	if tf.Version == nil || !tf.Version.Equal(V42) {
		return nil, fmt.Errorf("task: schema version must be 42")
	}
	return tf.toRuntime(), nil
}
