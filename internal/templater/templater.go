//go:generate go run ../../cmd/docgen/main.go
package templater

import (
	"bytes"
	"encoding/json"
	"fmt"
	"maps"
	"regexp"
	"strings"

	"text/template"

	"github.com/go-rivet/rivet/internal/deepcopy"
	"github.com/go-rivet/rivet/internal/templater/sprig"
	"github.com/go-rivet/rivet/internal/templater/task"
	"github.com/go-rivet/rivet/pkg/rivet/taskfile/ast"
)

var funcs = template.FuncMap{}

func init() {
	maps.Copy(funcs, sprig.SprigFuncs)
	maps.Copy(funcs, task.TaskFuncs)

	// aliases
	funcs["q"] = task.TaskFuncs["shellQuote"]
	funcs["FromSlash"] = task.TaskFuncs["fromSlash"]
	funcs["ToSlash"] = task.TaskFuncs["toSlash"]
	funcs["ExeExt"] = task.TaskFuncs["exeExt"]
}

// Cache is a help struct that allow us to call "replaceX" funcs multiple
// times, without having to check for error each time. The first error that
// happen will be assigned to r.err, and consecutive calls to funcs will just
// return the zero value.
type Cache struct {
	Vars *ast.Vars

	cacheMap map[string]any
	err      error
}

func (r *Cache) ResetCache() {
	r.cacheMap = r.Vars.ToCacheMap()
}

func (r *Cache) Err() error {
	return r.err
}

func ResolveRef(ref string, cache *Cache) any {
	if cache.err != nil {
		return nil
	}

	if cache.cacheMap == nil {
		cache.cacheMap = cache.Vars.ToCacheMap()
	}

	if ref == "." {
		return cache.cacheMap
	}

	// Variable to intercept and store the actual typed value
	var resolvedValue any

	// Create a local function map combining global funcs and our interceptor
	localFuncs := make(template.FuncMap)
	maps.Copy(localFuncs, funcs)

	// The "resolve" function captures the argument and returns an empty string
	// so it doesn't mess up standard template execution outputs.
	localFuncs["resolve"] = func(v any) string {
		resolvedValue = v
		return ""
	}

	// Wrap the user's reference inside our interceptor function: {{resolve (ref)}}
	tmplString := fmt.Sprintf("{{resolve (%s)}}", ref)

	t, err := template.New("resolver").Funcs(localFuncs).Parse(tmplString)
	if err != nil {
		cache.err = err
		return nil
	}

	// Execute the template into a discard buffer.
	// This forces the template to evaluate, triggers our function, and populates resolvedValue.
	var discard bytes.Buffer
	err = t.Execute(&discard, cache.cacheMap)
	if err != nil {
		cache.err = err
		return nil
	}

	return resolvedValue
}

func resolveDirectLookup(text string, cache *Cache) any {
	// If there is already an error, do nothing
	if cache.err != nil {
		return nil
	}

	// Initialize the cache map if it's not already initialized
	if cache.cacheMap == nil {
		cache.cacheMap = cache.Vars.ToCacheMap()
	}

	// Lookup should be in the form '{{.LOOKUP}}'.
	// Captures everything inside the brackets so we can rewrite it.
	re := regexp.MustCompile(`^\{\{(.+)\}\}$`)
	match := re.FindStringSubmatch(text)
	if len(match) != 2 {
		return text
	}
	innerRef := match[1]

	// Variable to intercept and store the actual typed value
	var resolvedValue any

	// Create a local function map combining global funcs and our interceptor
	localFuncs := make(template.FuncMap)
	maps.Copy(localFuncs, funcs)
	localFuncs["resolve"] = func(v any) string {
		resolvedValue = v
		return ""
	}

	// Wrap the inner expression: {{resolve (.LOOKUP)}}
	tmplString := fmt.Sprintf("{{resolve (%s)}}", innerRef)

	t, err := template.New("resolver").Funcs(localFuncs).Parse(tmplString)
	if err != nil {
		cache.err = err
		return nil
	}

	// Execute the template to trigger the function evaluation
	var discard bytes.Buffer
	err = t.Execute(&discard, cache.cacheMap)
	if err != nil {
		cache.err = err
		return nil
	}

	return resolvedValue
}

func Replace[T any](v T, cache *Cache) T {
	return ReplaceWithExtra(v, cache, nil)
}

func ReplaceWithExtra[T any](v T, cache *Cache, extra map[string]any) T {
	// If there is already an error, do nothing
	if cache.err != nil {
		return v
	}

	// Optimization: skip if string is not a template
	if s, ok := any(v).(string); ok {
		if !strings.Contains(s, "{{") {
			return v
		}
	}

	// Initialize the cache map if it's not already initialized
	if cache.cacheMap == nil {
		cache.cacheMap = cache.Vars.ToCacheMap()
	}

	// Create a copy of the cache map to avoid editing the original
	// If there is extra data, merge it with the cache map
	data := maps.Clone(cache.cacheMap)
	if extra != nil {
		maps.Copy(data, extra)
	}

	// Traverse the value and parse any template variables
	copy, err := deepcopy.TraverseStringsFunc(v, func(v string) (string, error) {
		// Optimization: skip if string is not a template
		if !strings.Contains(v, "{{") {
			return v, nil
		}
		tpl, err := template.New("").Funcs(funcs).Parse(v)
		if err != nil {
			return v, err
		}
		var b bytes.Buffer
		if err := tpl.Execute(&b, data); err != nil {
			return v, err
		}
		return strings.ReplaceAll(b.String(), "<no value>", ""), nil
	})
	if err != nil {
		cache.err = err
		return v
	}

	return copy
}

func ReplaceGlobs(globs []*ast.Glob, cache *Cache) []*ast.Glob {
	if cache.err != nil || len(globs) == 0 {
		return nil
	}

	new := []*ast.Glob{}
	for _, g := range globs {
		_glob := resolveDirectLookup(g.Glob, cache)
		switch glob := _glob.(type) {
		case []any:
			for _, v := range glob {
				new = append(new, &ast.Glob{
					Glob:   Replace(v.(string), cache),
					Negate: g.Negate,
				})
			}
		case string:
			glob = Replace(glob, cache)
			var jv any
			if err := json.Unmarshal([]byte(glob), &jv); err == nil {
				// JSON data (highly probable).
				switch val := jv.(type) {
				case []any:
					for _, v := range val {
						new = append(new, &ast.Glob{
							Glob:   v.(string),
							Negate: g.Negate,
						})
					}
				case string:
					new = append(new, &ast.Glob{
						Glob:   val,
						Negate: g.Negate,
					})
				}
			} else {
				// Otherwise take the glob as provided.
				new = append(new, &ast.Glob{
					Glob:   glob,
					Negate: g.Negate,
				})
			}
		default:
			new = append(new, &ast.Glob{
				Glob:   Replace(glob.(string), cache),
				Negate: g.Negate,
			})
		}
	}
	return new
}

func ReplaceVar(v ast.Var, cache *Cache) ast.Var {
	return ReplaceVarWithExtra(v, cache, nil)
}

func ReplaceVarWithExtra(v ast.Var, cache *Cache, extra map[string]any) ast.Var {
	if v.Ref != "" {
		return ast.Var{Value: ResolveRef(v.Ref, cache)}
	}
	return ast.Var{
		Value: ReplaceWithExtra(v.Value, cache, extra),
		Sh:    ReplaceWithExtra(v.Sh, cache, extra),
		Live:  v.Live,
		Ref:   v.Ref,
		Dir:   v.Dir,
	}
}

func ReplaceVars(vars *ast.Vars, cache *Cache) *ast.Vars {
	return ReplaceVarsWithExtra(vars, cache, nil)
}

func ReplaceVarsWithExtra(vars *ast.Vars, cache *Cache, extra map[string]any) *ast.Vars {
	if cache.err != nil || vars.Len() == 0 {
		return nil
	}

	newVars := ast.NewVars()
	for k, v := range vars.All() {
		newVars.Set(k, ReplaceVarWithExtra(v, cache, extra))
	}

	return newVars
}
