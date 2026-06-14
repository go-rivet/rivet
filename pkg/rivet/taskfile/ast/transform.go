package ast

import (
	"github.com/go-rivet/rivet/internal/deepcopy"
	"github.com/go-rivet/rivet/pkg/rivet/errors"
	"go.yaml.in/yaml/v3"
)

type Transform struct {
	Matches []*Glob
	Yields  []*Glob
}

func (t *Transform) DeepCopy() *Transform {
	if t == nil {
		return nil
	} else {
		return &Transform{
			Matches: deepcopy.Slice(t.Matches),
			Yields:  deepcopy.Slice(t.Yields),
		}
	}
}

func (t *Transform) UnmarshalYAML(node *yaml.Node) error {
	switch node.Kind {
	case yaml.MappingNode:
		var transform struct {
			Matches []*Glob
			Yields  []*Glob
		}
		if err := node.Decode(&transform); err != nil {
			return errors.NewTaskfileDecodeError(err, node)

		}
		t.Matches = transform.Matches
		t.Yields = transform.Yields
		return nil
	}
	return errors.NewTaskfileDecodeError(nil, node).WithTypeMessage("transform")
}
