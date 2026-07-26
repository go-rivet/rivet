package ast

import (
	"strings"

	"github.com/go-rivet/rivet/internal/deepcopy"
	"github.com/go-rivet/rivet/pkg/rivet/errors"
	"go.yaml.in/yaml/v3"
)

type Transform struct {
	Matches []*Glob
	Yields  []*Glob
	Subst   string `yaml:"subst"`
}

func (t *Transform) DeepCopy() *Transform {
	if t == nil {
		return nil
	} else {
		return &Transform{
			Matches: deepcopy.Slice(t.Matches),
			Yields:  deepcopy.Slice(t.Yields),
			Subst:   t.Subst,
		}
	}
}

func (t *Transform) UnmarshalYAML(node *yaml.Node) error {
	switch node.Kind {
	case yaml.MappingNode:
		var transform struct {
			Matches []*Glob
			Yields  []*Glob
			Subst   string
		}
		if err := node.Decode(&transform); err != nil {
			return errors.NewTaskfileDecodeError(err, node)

		}
		t.Matches = transform.Matches
		t.Yields = transform.Yields
		t.Subst = transform.Subst
		return nil
	}
	return errors.NewTaskfileDecodeError(nil, node).WithTypeMessage("transform")
}

// ToGlobPatterns converts the subst "from" rule to a glob for scanning.
// It also returns the "to" pattern so you can append targets if necessary.
func (t *Transform) SubstToGlob() (*Glob, *Glob) {
	if t.Subst == "" {
		return nil, nil
	}

	parts := strings.SplitN(t.Subst, ":", 2)
	if len(parts) != 2 {
		return nil, nil
	}
	fromPattern, toPattern := parts[0], parts[1]

	// Convert Make style "%" wildcard to a standard glob wildcard "*"
	// E.g., "src/%.old" becomes "src/*.old"
	globStr := strings.Replace(fromPattern, "%", "*", 1)

	// Note: If your walk engine needs the target yields explicitly tracked,
	// you can pass along the 'toPattern' mutated into a wildcard as well.
	yieldGlobStr := strings.Replace(toPattern, "%", "*", 1)

	return &Glob{Glob: globStr}, &Glob{Glob: yieldGlobStr}
}
