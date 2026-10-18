package v42

import (
	"fmt"
	"strings"

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
	if err := validateNamespace(tf.Namespace); err != nil {
		return nil, err
	}
	return tf.toRuntime(), nil
}

func validateNamespace(namespace string) error {
	for _, component := range strings.Split(namespace, NamespaceSeparator) {
		if component == "" && namespace != "" {
			return fmt.Errorf("task: namespace %q contains an empty path component", namespace)
		}
	}
	return nil
}
