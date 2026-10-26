package templater

import (
	"fmt"
	"maps"
	"strings"
)

// NamespacedResolver returns the values of the namespaced var references
// ("ns:name" or ":name") found in a template. References it cannot resolve are
// left out.
type NamespacedResolver func(refs []string) map[string]any

func isIdentStart(c byte) bool {
	return c == '_' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}

func isIdentChar(c byte) bool {
	return isIdentStart(c) || (c >= '0' && c <= '9')
}

// scanNamespacedRefs finds namespaced references inside the {{ }} actions of
// s. It returns s with each reference rewritten as a lookup into the template
// data, and the list of references found.
func scanNamespacedRefs(s string) (string, []string) {
	var out strings.Builder
	var refs []string
	seen := map[string]bool{}
	for {
		start := strings.Index(s, "{{")
		if start < 0 {
			break
		}
		end := strings.Index(s[start:], "}}")
		if end < 0 {
			break
		}
		end += start + 2
		out.WriteString(s[:start])
		out.WriteString(rewriteAction(s[start:end], func(ref string) {
			if !seen[ref] {
				seen[ref] = true
				refs = append(refs, ref)
			}
		}))
		s = s[end:]
	}
	out.WriteString(s)
	return out.String(), refs
}

// rewriteAction finds dotted var references in one {{ }} action:
//
//	.name       relative to the current namespace
//	.ns:name    qualified namespace path
//	.:name      root namespace
//
// Qualified references are rewritten into a lookup in the template data.
// Relative references are left as they are and found because their resolved
// value is overlaid on the data.
func rewriteAction(action string, found func(ref string)) string {
	var out strings.Builder
	for i := 0; i < len(action); {
		c := action[i]
		switch {
		case c == '"' || c == '`' || c == '\'':
			j := i + 1
			for j < len(action) && action[j] != c {
				if c == '"' && action[j] == '\\' {
					j++
				}
				j++
			}
			j = min(j+1, len(action))
			out.WriteString(action[i:j])
			i = j
		case c == '.' && i+1 < len(action) && (isIdentStart(action[i+1]) || (action[i+1] == ':' && i+2 < len(action) && isIdentStart(action[i+2]))):
			prev := byte(' ')
			if i > 0 {
				prev = action[i-1]
			}
			j := i + 1
			if action[j] == ':' {
				j++
			}
			for {
				for j < len(action) && isIdentChar(action[j]) {
					j++
				}
				if j+1 < len(action) && action[j] == ':' && isIdentStart(action[j+1]) {
					j++
					continue
				}
				break
			}
			ref := action[i+1 : j]
			if isIdentChar(prev) || prev == '.' || prev == '$' || prev == ')' {
				out.WriteString(action[i:j])
			} else {
				found(ref)
				if strings.Contains(ref, ":") {
					fmt.Fprintf(&out, "(index $ %q)", ref)
				} else {
					out.WriteString(action[i:j])
				}
			}
			i = j
		default:
			out.WriteByte(c)
			i++
		}
	}
	return out.String()
}

// withNamespaced rewrites namespaced references in str and returns the data
// extended with their resolved values. A reference that resolves nowhere is an
// error.
func withNamespaced(str string, data map[string]any, resolve NamespacedResolver) (string, map[string]any, error) {
	if resolve == nil {
		return str, data, nil
	}
	rewritten, refs := scanNamespacedRefs(str)
	if len(refs) == 0 {
		return str, data, nil
	}
	resolved := resolve(refs)
	extended := maps.Clone(data)
	if extended == nil {
		extended = map[string]any{}
	}
	maps.Copy(extended, resolved)
	// Inside range/with the dot is rebound, so relative names can't be checked.
	scoped := strings.Contains(str, "range") || strings.Contains(str, "with")
	for _, ref := range refs {
		if _, ok := resolved[ref]; ok {
			continue
		}
		if !strings.Contains(ref, ":") && (scoped || hasKey(data, ref)) {
			continue
		}
		return str, nil, fmt.Errorf("task: variable %q is not defined", "."+ref)
	}
	return rewritten, extended, nil
}

func hasKey(data map[string]any, key string) bool {
	_, ok := data[key]
	return ok
}
