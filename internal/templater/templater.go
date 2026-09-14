//go:generate go run ../../cmd/docgen templater
package templater

import (
	"bytes"
	"encoding/json"
	"fmt"
	"maps"
	"regexp"
	"strings"
	"sync"

	"text/template"

	"github.com/go-rivet/rivet/internal/deepcopy"
	"github.com/go-rivet/rivet/internal/templater/sprig"
	"github.com/go-rivet/rivet/internal/templater/task"
	"github.com/go-rivet/rivet/pkg/rivet/taskfile/ast"
)

var funcs = template.FuncMap{}

// lookupRe matches a lone '{{ ... }}' expression, used by resolveDirectLookup.
// Compiled once since regexp.MustCompile is expensive to run per call.
var lookupRe = regexp.MustCompile(`^\{\{(.+)\}\}$`)

// templateCache holds parsed templates keyed by their source text, avoiding
// re-parsing (which allocates a lexer/AST) for strings seen more than once,
// e.g. the same command template rendered for each item in a loop.
var templateCache sync.Map // map[string]*template.Template

// bufPool reuses bytes.Buffers across template executions to cut down on
// per-call allocations.
var bufPool = sync.Pool{
	New: func() any { return new(bytes.Buffer) },
}

// parseTemplate returns a cached *template.Template for s, parsing and
// caching it on first use.
func parseTemplate(s string) (*template.Template, error) {
	if t, ok := templateCache.Load(s); ok {
		return t.(*template.Template), nil
	}
	tpl, err := template.New("").Funcs(funcs).Parse(s)
	if err != nil {
		return nil, err
	}
	// If another goroutine raced us, keep whichever was stored first.
	actual, _ := templateCache.LoadOrStore(s, tpl)
	return actual.(*template.Template), nil
}

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

	// The "resolve" function captures the argument and returns an empty string
	// so it doesn't mess up standard template execution outputs.
	resolve := func(v any) string {
		resolvedValue = v
		return ""
	}

	// Wrap the user's reference inside our interceptor function: {{resolve (ref)}}
	tmplString := fmt.Sprintf("{{resolve (%s)}}", ref)

	// Register the global funcs first, then layer the single-entry closure map
	// on top; this avoids copying the whole global funcs map into a throwaway
	// map on every call.
	t, err := template.New("resolver").Funcs(funcs).Funcs(template.FuncMap{"resolve": resolve}).Parse(tmplString)
	if err != nil {
		cache.err = err
		return nil
	}

	// Execute the template into a discard buffer.
	// This forces the template to evaluate, triggers our function, and populates resolvedValue.
	buf := bufPool.Get().(*bytes.Buffer)
	buf.Reset()
	defer bufPool.Put(buf)
	err = t.Execute(buf, cache.cacheMap)
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
	match := lookupRe.FindStringSubmatch(text)
	if len(match) != 2 {
		return text
	}
	innerRef := match[1]

	// Variable to intercept and store the actual typed value
	var resolvedValue any

	resolve := func(v any) string {
		resolvedValue = v
		return ""
	}

	// Wrap the inner expression: {{resolve (.LOOKUP)}}
	tmplString := fmt.Sprintf("{{resolve (%s)}}", innerRef)

	// Register the global funcs first, then layer the single-entry closure map
	// on top; this avoids copying the whole global funcs map into a throwaway
	// map on every call.
	t, err := template.New("resolver").Funcs(funcs).Funcs(template.FuncMap{"resolve": resolve}).Parse(tmplString)
	if err != nil {
		cache.err = err
		return nil
	}

	// Execute the template to trigger the function evaluation
	buf := bufPool.Get().(*bytes.Buffer)
	buf.Reset()
	defer bufPool.Put(buf)
	err = t.Execute(buf, cache.cacheMap)
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

	// Optimization: If it's a plain string with no templates, exit immediately.
	// No map initialization, no deepcopy traversal overhead.
	if s, ok := any(v).(string); ok {
		if !strings.Contains(s, "{{") {
			return v
		}
	}

	// For all structural or complex types (like slices, structs, pointers),
	// let deepcopy traverse them. We defer map creation until we find a string
	// that actually needs rendering.
	var data map[string]any

	copy, err := deepcopy.TraverseStringsFunc(v, func(str string) (string, error) {
		// If an individual string inside the structure isn't a template, skip it.
		if !strings.Contains(str, "{{") {
			return str, nil
		}

		// LAZY INITIALIZATION: We found a template! Only initialize the map now.
		if cache.cacheMap == nil {
			cache.cacheMap = cache.Vars.ToCacheMap()
		}

		// Prepare the data map context only on the first template found in this call
		if data == nil {
			if len(extra) > 0 {
				data = make(map[string]any, len(cache.cacheMap)+len(extra))
				maps.Copy(data, cache.cacheMap)
				maps.Copy(data, extra)
			} else {
				data = cache.cacheMap
			}
		}

		tpl, err := parseTemplate(str)
		if err != nil {
			return str, err
		}

		buf := bufPool.Get().(*bytes.Buffer)
		buf.Reset()
		defer bufPool.Put(buf)

		if err := tpl.Execute(buf, data); err != nil {
			return str, err
		}
		return strings.ReplaceAll(buf.String(), "<no value>", ""), nil
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
