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
		base  *Vars
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
		om: orderedmap.NewOrderedMapWithCapacity[string, Var](capacity),
	}
	return vars
}

// NewVarsWithBase creates a Vars overlay with a read-only base. The base must
// not itself be an overlay.
func NewVarsWithBase(base *Vars, capacity int) *Vars {
	return &Vars{
		om:   orderedmap.NewOrderedMapWithCapacity[string, Var](capacity),
		base: base,
	}
}

// Len returns the number of variables in the Vars map.
func (vars *Vars) Len() int {
	if vars == nil {
		return 0
	}
	vars.mutex.RLock()
	defer vars.mutex.RUnlock()
	localLen := 0
	if vars.om != nil {
		localLen = vars.om.Len()
	}
	if vars.base == nil {
		return localLen
	}
	if localLen == 0 {
		return vars.base.Len()
	}
	n := vars.base.Len()
	for k := range vars.om.Keys() {
		if !vars.base.om.Has(k) {
			n++
		}
	}
	return n
}

// Get returns the value of the variable with the provided key and a boolean
// that indicates if the value was found or not.
func (vars *Vars) Get(key string) (Var, bool) {
	if vars == nil {
		return Var{}, false
	}
	vars.mutex.RLock()
	if vars.om != nil {
		if v, ok := vars.om.Get(key); ok {
			vars.mutex.RUnlock()
			return v, true
		}
	}
	base := vars.base
	vars.mutex.RUnlock()
	if base == nil {
		return Var{}, false
	}
	return base.Get(key)
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
	if vars == nil {
		return func(yield func(string, Var) bool) {}
	}
	return func(yield func(string, Var) bool) {
		if vars.base != nil {
			for k, baseValue := range vars.base.om.All() {
				if value, ok := vars.om.Get(k); ok {
					baseValue = value
				}
				if !yield(k, baseValue) {
					return
				}
			}
		}
		if vars.om == nil {
			return
		}
		for k, value := range vars.om.All() {
			if vars.base != nil && vars.base.om.Has(k) {
				continue
			}
			if !yield(k, value) {
				return
			}
		}
	}
}

// Keys returns an iterator that loops over all task keys.
func (vars *Vars) Keys() iter.Seq[string] {
	if vars == nil {
		return func(yield func(string) bool) {}
	}
	return func(yield func(string) bool) {
		for k := range vars.All() {
			if !yield(k) {
				return
			}
		}
	}
}

// Values returns an iterator that loops over all task values.
func (vars *Vars) Values() iter.Seq[Var] {
	if vars == nil {
		return func(yield func(Var) bool) {}
	}
	return func(yield func(Var) bool) {
		for _, v := range vars.All() {
			if !yield(v) {
				return
			}
		}
	}
}

// ToCacheMap converts Vars to an unordered map containing only the static variables.
func (vars *Vars) ToCacheMap() map[string]any {
	if vars == nil {
		return map[string]any{}
	}
	vars.mutex.RLock()
	defer vars.mutex.RUnlock()
	if vars.om == nil && vars.base == nil {
		return map[string]any{}
	}

	// OPTIMIZATION: Pre-allocate the exact map capacity up-front!
	// This directly targets the 843MB maps.clone / bucket resize pressure.
	capacity := 0
	if vars.om != nil {
		capacity = vars.om.Len()
	}
	if vars.base != nil {
		capacity += vars.base.Len()
	}
	m := make(map[string]any, capacity)

	set := func(k string, v Var) {
		if v.Sh != nil && *v.Sh != "" {
			return
		}
		if v.Live != nil {
			m[k] = v.Live
		} else {
			m[k] = v.Value
		}
	}

	// Preserve base order while allowing local values to shadow it.
	if vars.base != nil {
		for k, v := range vars.base.om.All() {
			set(k, v)
		}
	}
	if vars.om != nil {
		for k, v := range vars.om.All() {
			set(k, v)
		}
	}
	return m
}

// Merge loops over other and merges its values with the variables in vars.
func (vars *Vars) Merge(other *Vars, include *Include) {
	if vars == nil || vars.om == nil || other == nil {
		return
	}

	for k, v := range other.All() {
		if include != nil && include.AdvancedImport {
			v.Dir = include.Dir
		}
		vars.Set(k, v)
	}
}

// ReverseMerge merges other variables with the existing variables in vars, but
// keeps the other variables first in order.
func (vars *Vars) ReverseMerge(other *Vars, include *Include) {
	if vars == nil || vars.om == nil || other == nil {
		return
	}

	newOM := orderedmap.NewOrderedMapWithCapacity[string, Var](other.Len() + vars.Len())
	for k, v := range other.All() {
		if include != nil && include.AdvancedImport {
			v.Dir = include.Dir
		}
		newOM.Set(k, v)
	}
	for k, v := range vars.All() {
		// Only append if it wasn't already set by the incoming updates map path
		if !newOM.Has(k) {
			newOM.Set(k, v)
		}
	}
	vars.mutex.Lock()
	vars.om = newOM
	vars.base = nil
	vars.mutex.Unlock()
}

func (vs *Vars) DeepCopy() *Vars {
	if vs == nil {
		return nil
	}
	vs.mutex.RLock()
	defer vs.mutex.RUnlock()
	if vs.base == nil {
		return &Vars{
			om: deepcopy.OrderedMap(vs.om),
		}
	}
	// Flatten the base and local entries into an independent copy.
	localLen := 0
	if vs.om != nil {
		localLen = vs.om.Len()
	}
	merged := orderedmap.NewOrderedMapWithCapacity[string, Var](vs.base.Len() + localLen)
	for k, v := range vs.base.om.All() {
		merged.Set(k, v)
	}
	if vs.om != nil {
		for k, v := range vs.om.All() {
			merged.Set(k, v)
		}
	}
	return &Vars{
		om: deepcopy.OrderedMap(merged),
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
		vs.base = nil
		vs.mutex.Unlock()
		return nil
	}

	return errors.NewTaskfileDecodeError(nil, node).WithTypeMessage("vars")
}
