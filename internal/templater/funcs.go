package templater

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"maps"
	mrand "math/rand/v2"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strings"

	"go.yaml.in/yaml/v3"
	"mvdan.cc/sh/v3/shell"
	"mvdan.cc/sh/v3/syntax"

	sprig "github.com/go-task/slim-sprig/v3"
	"github.com/go-task/template"
)

var templateFuncs template.FuncMap

func init() {
	taskFuncs := template.FuncMap{
		"OS":           goos,
		"ARCH":         goarch,
		"numCPU":       runtime.NumCPU,
		"catLines":     catLines,
		"splitLines":   splitLines,
		"fromSlash":    filepath.FromSlash,
		"toSlash":      filepath.ToSlash,
		"exeExt":       exeExt,
		"shellQuote":   shellQuote,
		"splitArgs":    splitArgs,
		"IsSH":         IsSH, // Deprecated
		"joinPath":     filepath.Join,
		"joinEnv":      joinEnv,
		"joinUrl":      joinUrl,
		"relPath":      filepath.Rel,
		"absPath":      filepath.Abs,
		"merge":        merge,
		"spew":         nativeSdump,
		"fromYaml":     fromYaml,
		"mustFromYaml": mustFromYaml,
		"toYaml":       toYaml,
		"mustToYaml":   mustToYaml,
		"uuid":         newUUID,
		"randIntN":     mrand.IntN,
	}

	// aliases
	taskFuncs["q"] = taskFuncs["shellQuote"]

	// Deprecated aliases for renamed functions.
	taskFuncs["FromSlash"] = taskFuncs["fromSlash"]
	taskFuncs["ToSlash"] = taskFuncs["toSlash"]
	taskFuncs["ExeExt"] = taskFuncs["exeExt"]

	templateFuncs = template.FuncMap(sprig.TxtFuncMap())
	maps.Copy(templateFuncs, taskFuncs)
}

func nativeSdump(v interface{}) string {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return fmt.Sprintf("%#v", v) // Fallback if JSON marshalling fails
	}
	return string(b)
}

func newUUID() string {
	uuid := make([]byte, 16)
	_, err := rand.Read(uuid)
	if err != nil {
		// crypto/rand.Read only fails if the system runtime lacks entropy
		panic(err)
	}

	// Set version 4 (bits 4-7 of byte 6 to 0100)
	uuid[6] = (uuid[6] & 0x0f) | 0x40
	// Set variant to RFC 4122 (bits 6-7 of byte 8 to 10)
	uuid[8] = (uuid[8] & 0x3f) | 0x80

	return fmt.Sprintf("%x-%x-%x-%x-%x",
		uuid[0:4], uuid[4:6], uuid[6:8], uuid[8:10], uuid[10:16])
}

func goos() string {
	return runtime.GOOS
}

func goarch() string {
	return runtime.GOARCH
}

func catLines(s string) string {
	s = strings.ReplaceAll(s, "\r\n", " ")
	return strings.ReplaceAll(s, "\n", " ")
}

func splitLines(s string) []string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	return strings.Split(s, "\n")
}

func exeExt() string {
	if runtime.GOOS == "windows" {
		return ".exe"
	}
	return ""
}

func shellQuote(str string) (string, error) {
	return syntax.Quote(str, syntax.LangBash)
}

func splitArgs(s string) ([]string, error) {
	return shell.Fields(s, nil)
}

// Deprecated: now always returns true
func IsSH() bool {
	return true
}

func joinEnv(elem ...string) string {
	return strings.Join(elem, string(os.PathListSeparator))
}

func joinUrl(elem ...string) string {
	return path.Join(elem...)
}

func merge(base map[string]any, v ...map[string]any) map[string]any {
	cap := len(v)
	for _, m := range v {
		cap += len(m)
	}
	result := make(map[string]any, cap)
	maps.Copy(result, base)
	for _, m := range v {
		maps.Copy(result, m)
	}
	return result
}

func fromYaml(v string) any {
	output, _ := mustFromYaml(v)
	return output
}

func mustFromYaml(v string) (any, error) {
	var output any
	err := yaml.Unmarshal([]byte(v), &output)
	return output, err
}

func toYaml(v any) string {
	output, _ := yaml.Marshal(v)
	return string(output)
}

func mustToYaml(v any) (string, error) {
	output, err := yaml.Marshal(v)
	if err != nil {
		return "", err
	}
	return string(output), nil
}
