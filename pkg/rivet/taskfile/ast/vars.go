package ast

import (
	"iter"
	"sync"

	"go.yaml.in/yaml/v3"

	"github.com/go-rivet/rivet/internal/deepcopy"
	"github.com/go-rivet/rivet/internal/orderedmap"
	"github.com/go-rivet/rivet/pkg/rivet/errors"
)

type (
	// Vars is an ordered map of variable names to values.
	Vars struct {
		om    *orderedmap.OrderedMap[string, Var]
		mutex sync.RWMutex
	}
	// A VarElement is a key-value pair that is used for initializing a Vars
	// structure.
	VarElement orderedmap.Element[string, Var]
)

// NewVars creates a new instance of Vars and initializes it with the provided
// set of elements, if any. The elements are added in the order they are passed.
func NewVars(els ...*VarElement) *Vars {
	vars := &Vars{
		om: orderedmap.NewOrderedMap[string, Var](),
	}
	for _, el := range els {
		vars.Set(el.Key, el.Value)
	}
	return vars
}

// NewVars creates a new instance of Vars and initializes it with the provided
// set of elements, if any. The elements are added in the order they are passed.
func NewVarsWithCapacity(capacity int) *Vars {
	vars := &Vars{
		om: orderedmap.NewOrderedMapWithCapacity[string, Var](10),
	}
	return vars
}

// Len returns the number of variables in the Vars map.
func (vars *Vars) Len() int {
	if vars == nil || vars.om == nil {
		return 0
	}
	vars.mutex.RLock()
	defer vars.mutex.RUnlock()
	return vars.om.Len()
}

// Get returns the value of the variable with the provided key and a boolean
// that indicates if the value was found or not.
func (vars *Vars) Get(key string) (Var, bool) {
	if vars == nil || vars.om == nil {
		return Var{}, false
	}
	vars.mutex.RLock()
	defer vars.mutex.RUnlock()
	return vars.om.Get(key)
}

// Set sets the value of the variable with the provided key to the provided value.
func (vars *Vars) Set(key string, value Var) bool {
	if vars == nil {
		return false // Maintain safety bounds without breaking signatures
	}
	vars.mutex.Lock()
	defer vars.mutex.Unlock()
	if vars.om == nil {
		vars.om = orderedmap.NewOrderedMap[string, Var]()
	}
	return vars.om.Set(key, value)
}

// All returns an iterator that loops over all task key-value pairs.
func (vars *Vars) All() iter.Seq2[string, Var] {
	if vars == nil || vars.om == nil {
		return func(yield func(string, Var) bool) {}
	}
	return vars.om.All()
}

// Keys returns an iterator that loops over all task keys.
func (vars *Vars) Keys() iter.Seq[string] {
	if vars == nil || vars.om == nil {
		return func(yield func(string) bool) {}
	}
	return vars.om.Keys()
}

// Values returns an iterator that loops over all task values.
func (vars *Vars) Values() iter.Seq[Var] {
	if vars == nil || vars.om == nil {
		return func(yield func(Var) bool) {}
	}
	return vars.om.Values()
}

// ToCacheMap converts Vars to an unordered map containing only the static variables.
func (vars *Vars) ToCacheMap() map[string]any {
	if vars == nil || vars.om == nil {
		return map[string]any{}
	}
	vars.mutex.RLock()
	defer vars.mutex.RUnlock()

	// OPTIMIZATION: Pre-allocate the exact map capacity up-front!
	// This directly targets the 843MB maps.clone / bucket resize pressure.
	m := make(map[string]any, vars.om.Len())

	// Clean, native range loop sequence reading elements smoothly
	for k, v := range vars.om.All() {
		if v.Sh != nil && *v.Sh != "" {
			continue
		}
		if v.Live != nil {
			m[k] = v.Live
		} else {
			m[k] = v.Value
		}
	}
	return m
}

// Merge loops over other and merges its values with the variables in vars.
func (vars *Vars) Merge(other *Vars, include *Include) {
	if vars == nil || vars.om == nil || other == nil {
		return
	}
	other.mutex.RLock()
	defer other.mutex.RUnlock()

	vars.mutex.Lock()
	defer vars.mutex.Unlock()

	for k, v := range other.om.All() {
		if include != nil && include.AdvancedImport {
			v.Dir = include.Dir
		}
		vars.om.Set(k, v)
	}
}

// ReverseMerge merges other variables with the existing variables in vars, but
// keeps the other variables first in order.
func (vars *Vars) ReverseMerge(other *Vars, include *Include) {
	if vars == nil || vars.om == nil || other == nil || other.om == nil {
		return
	}

	other.mutex.RLock()
	newOM := orderedmap.NewOrderedMapWithCapacity[string, Var](other.om.Len() + vars.om.Len())
	for k, v := range other.om.All() {
		if include != nil && include.AdvancedImport {
			v.Dir = include.Dir
		}
		newOM.Set(k, v)
	}
	other.mutex.RUnlock()

	vars.mutex.Lock()
	for k, v := range vars.om.All() {
		// Only append if it wasn't already set by the incoming updates map path
		if !newOM.Has(k) {
			newOM.Set(k, v)
		}
	}
	vars.om = newOM
	vars.mutex.Unlock()
}

func (vs *Vars) DeepCopy() *Vars {
	if vs == nil {
		return nil
	}
	vs.mutex.RLock()
	defer vs.mutex.RUnlock()
	return &Vars{
		om: deepcopy.OrderedMap(vs.om),
	}
}

func (vs *Vars) UnmarshalYAML(node *yaml.Node) error {
	if vs == nil {
		return errors.NewTaskfileDecodeError(nil, node).WithTypeMessage("vars")
	}

	switch node.Kind {
	case yaml.MappingNode:
		capacity := len(node.Content) / 2
		localOM := orderedmap.NewOrderedMapWithCapacity[string, Var](capacity)

		for i := 0; i < len(node.Content); i += 2 {
			keyNode := node.Content[i]
			valueNode := node.Content[i+1]

			if valueNode.Kind == yaml.AliasNode && valueNode.Alias.Kind == yaml.MappingNode {
				targetNode := valueNode.Alias
				for j := 0; j < len(targetNode.Content); j += 2 {
					var v Var
					if err := targetNode.Content[j+1].Decode(&v); err != nil {
						return errors.NewTaskfileDecodeError(err, node)
					}
					localOM.Set(targetNode.Content[j].Value, v)
				}
				continue
			}

			var v Var
			if err := valueNode.Decode(&v); err != nil {
				return errors.NewTaskfileDecodeError(err, node)
			}
			localOM.Set(keyNode.Value, v)
		}

		vs.mutex.Lock()
		vs.om = localOM
		vs.mutex.Unlock()
		return nil
	}

	return errors.NewTaskfileDecodeError(nil, node).WithTypeMessage("vars")
}
