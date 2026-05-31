package task

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"maps"
	mrand "math/rand/v2"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"text/template"

	"go.yaml.in/yaml/v3"
	"mvdan.cc/sh/v3/shell"
	"mvdan.cc/sh/v3/syntax"
)

var TaskFuncs = template.FuncMap{
	// --- System ---
	"OS":     func() string { return runtime.GOOS },
	"ARCH":   func() string { return runtime.GOARCH },
	"numCPU": runtime.NumCPU,
	"exeExt": func() string {
		if runtime.GOOS == "windows" {
			return ".exe"
		}
		return ""
	},
	"IsSH":     func() bool { return true }, // Deprecated
	"uuid":     newUUID,
	"randIntN": mrand.IntN,

	// --- String Manipulation ---
	"catLines":   catLines,
	"splitLines": splitLines,
	"toString":   func(v interface{}) string { return fmt.Sprintf("%v", v) },

	// --- File Path ---
	"fromSlash": filepath.FromSlash,
	"toSlash":   filepath.ToSlash,
	"joinPath":  filepath.Join,
	"relPath":   filepath.Rel,
	"absPath":   filepath.Abs,

	// --- Shell Tokenization ---
	"shellQuote": shellQuote,
	"splitArgs":  splitArgs,

	// --- Utility ---
	"joinEnv": joinEnv,
	"joinUrl": joinUrl,
	"merge":   merge,
	"spew":    nativeSdump,

	// --- Serialization ---
	"fromYaml":     func(v string) any { var out any; _ = yaml.Unmarshal([]byte(v), &out); return out },
	"mustFromYaml": mustFromYaml,
	"toYaml":       func(v any) string { out, _ := yaml.Marshal(v); return string(out) },
	"mustToYaml":   mustToYaml,
}

func nativeSdump(v interface{}) string {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return fmt.Sprintf("%#v", v)
	}
	return string(b)
}

func newUUID() string {
	uuid := make([]byte, 16)
	if _, err := rand.Read(uuid); err != nil {
		panic(err)
	}
	uuid[6] = (uuid[6] & 0x0f) | 0x40
	uuid[8] = (uuid[8] & 0x3f) | 0x80

	return fmt.Sprintf("%x-%x-%x-%x-%x",
		uuid[0:4], uuid[4:6], uuid[6:8], uuid[8:10], uuid[10:16])
}

func catLines(s string) string {
	s = strings.ReplaceAll(s, "\r\n", " ")
	return strings.ReplaceAll(s, "\n", " ")
}

func splitLines(s string) []string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	return strings.Split(s, "\n")
}

func shellQuote(str string) (string, error) {
	return syntax.Quote(str, syntax.LangBash)
}

func splitArgs(s string) ([]string, error) {
	return shell.Fields(s, nil)
}

func joinEnv(elem ...string) string {
	return strings.Join(elem, string(os.PathListSeparator))
}

// Fixed joinUrl to safely preserve web protocol scheme identifiers (e.g., https://)
func joinUrl(elem ...string) string {
	if len(elem) == 0 {
		return ""
	}
	var paths []string
	for i, e := range elem {
		trimmed := e
		if i > 0 {
			trimmed = strings.TrimLeft(trimmed, "/")
		}
		if i < len(elem)-1 {
			trimmed = strings.TrimRight(trimmed, "/")
		}
		if trimmed != "" {
			paths = append(paths, trimmed)
		}
	}
	return strings.Join(paths, "/")
}

// Fixed map key capacity estimation algorithm
func merge(base map[string]any, v ...map[string]any) map[string]any {
	capacity := len(base)
	for _, m := range v {
		capacity += len(m)
	}
	result := make(map[string]any, capacity)
	maps.Copy(result, base)
	for _, m := range v {
		maps.Copy(result, m)
	}
	return result
}

func mustFromYaml(v string) (any, error) {
	var output any
	err := yaml.Unmarshal([]byte(v), &output)
	return output, err
}

func mustToYaml(v any) (string, error) {
	output, err := yaml.Marshal(v)
	if err != nil {
		return "", err
	}
	return string(output), nil
}
