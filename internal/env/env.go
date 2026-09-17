package env

import (
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/go-rivet/rivet/pkg/rivet/taskfile/ast"
)

const taskVarPrefix = "TASK_"

var (
	baselineEnv     *ast.Vars
	baselineEnvOnce sync.Once
)

// GetEnviron returns a fresh copy of the baseline environment variables.
func GetEnviron() *ast.Vars {
	isBenchmark := false
	if benchFlag := flag.Lookup("test.bench"); benchFlag != nil && benchFlag.Value.String() != "" {
		isBenchmark = true
	}

	isStandardTest := flag.Lookup("test.v") != nil && !isBenchmark

	if isStandardTest {
		return buildEnvironSnapshot()
	}

	baselineEnvOnce.Do(func() {
		baselineEnv = buildEnvironSnapshot()
	})
	return baselineEnv.DeepCopy()
}

func buildEnvironSnapshot() *ast.Vars {
	rawEnv := os.Environ()
	env := ast.NewVarsWithCapacity(len(rawEnv))
	for _, e := range rawEnv {
		key, val, found := strings.Cut(e, "=")
		if !found {
			continue
		}
		env.Set(key, ast.Var{Value: val})
	}
	return env
}

func GetFromVars(vars *ast.Vars) []string {
	environ := make([]string, 0, vars.Len())

	for k, v := range vars.All() {
		actualVal := v.Value
		if v.Live != nil {
			actualVal = v.Live
		}
		if !isTypeAllowed(actualVal) {
			continue
		}
		var strVal string
		if s, ok := actualVal.(string); ok {
			strVal = s
		} else {
			strVal = fmt.Sprint(actualVal)
		}
		environ = append(environ, k+"="+strVal)
	}

	return environ
}

func isTypeAllowed(v any) bool {
	switch v.(type) {
	case string, bool, int, float32, float64:
		return true
	default:
		return false
	}
}

func GetTaskEnv(key string) string {
	return os.Getenv(taskVarPrefix + key)
}

// GetTaskEnvBool returns the boolean value of a TASK_ prefixed env var.
// Returns the value and true if set and valid, or false and false if not set or invalid.
func GetTaskEnvBool(key string) (bool, bool) {
	v := GetTaskEnv(key)
	if v == "" {
		return false, false
	}
	b, err := strconv.ParseBool(v)
	return b, err == nil
}

// GetTaskEnvInt returns the integer value of a TASK_ prefixed env var.
// Returns the value and true if set and valid, or 0 and false if not set or invalid.
func GetTaskEnvInt(key string) (int, bool) {
	v := GetTaskEnv(key)
	if v == "" {
		return 0, false
	}
	i, err := strconv.Atoi(v)
	return i, err == nil
}

// GetTaskEnvDuration returns the duration value of a TASK_ prefixed env var.
// Returns the value and true if set and valid, or 0 and false if not set or invalid.
func GetTaskEnvDuration(key string) (time.Duration, bool) {
	v := GetTaskEnv(key)
	if v == "" {
		return 0, false
	}
	d, err := time.ParseDuration(v)
	return d, err == nil
}

// GetTaskEnvString returns the string value of a TASK_ prefixed env var.
// Returns the value and true if set (non-empty), or empty string and false if not set.
func GetTaskEnvString(key string) (string, bool) {
	v := GetTaskEnv(key)
	return v, v != ""
}

// GetTaskEnvStringSlice returns a comma-separated list from a TASK_ prefixed env var.
// Returns the slice and true if set (non-empty), or nil and false if not set.
func GetTaskEnvStringSlice(key string) ([]string, bool) {
	v := GetTaskEnv(key)
	if v == "" {
		return nil, false
	}
	parts := strings.Split(v, ",")
	result := make([]string, 0, len(parts))
	for _, p := range parts {
		if trimmed := strings.TrimSpace(p); trimmed != "" {
			result = append(result, trimmed)
		}
	}
	if len(result) == 0 {
		return nil, false
	}
	return result, true
}
