package hash

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"

	"github.com/go-rivet/rivet/pkg/rivet/taskfile/ast"
)

type HashFunc func(*ast.Task) (string, error)

func Empty(*ast.Task) (string, error) {
	return "", nil
}

func Name(t *ast.Task) (string, error) {
	return fmt.Sprintf("%s:%s", t.Location.Taskfile, t.LocalName()), nil
}

func Hash(t *ast.Task) (string, error) {
	// 1. Serialize the entire task configuration struct into a deterministic byte array
	b, err := json.Marshal(t)
	if err != nil {
		return "", fmt.Errorf("failed to marshal task for caching: %w", err)
	}

	// 2. Generate a standard SHA-256 signature from the serialized data
	hash := sha256.Sum256(b)

	// 3. Reconstruct your original colon-separated cache tracking layout
	// %x prints the SHA-256 digest string natively instead of the old uint64 integer
	return fmt.Sprintf("%s:%s:%x", t.Location.Taskfile, t.LocalName(), hash), nil
}
