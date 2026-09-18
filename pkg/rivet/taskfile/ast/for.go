package ast

import (
	"go.yaml.in/yaml/v3"

	"github.com/go-rivet/rivet/internal/deepcopy"
	"github.com/go-rivet/rivet/pkg/rivet/errors"
)

type For struct {
	From   string
	List   []any
	Matrix *Matrix
	Var    string
	Split  string
	As     string
}

func (f *For) UnmarshalYAML(node *yaml.Node) error {
	switch node.Kind {

	case yaml.ScalarNode:
		var from string
		if err := node.Decode(&from); err != nil {
			return errors.NewTaskfileDecodeError(err, node)
		}
		f.From = from
		return nil

	case yaml.SequenceNode:
		var list []any
		if err := node.Decode(&list); err != nil {
			return errors.NewTaskfileDecodeError(err, node)
		}
		f.List = list
		return nil

	case yaml.MappingNode:
		for i := 0; i < len(node.Content); i += 2 {
			keyNode := node.Content[i]
			valNode := node.Content[i+1]

			switch keyNode.Value {
			case "matrix":
				_ = valNode.Decode(&f.Matrix)
			case "var":
				f.Var = valNode.Value
			case "split":
				f.Split = valNode.Value
			case "as":
				f.As = valNode.Value
			}
		}

		if f.Var == "" && f.Matrix.Len() == 0 {
			return errors.NewTaskfileDecodeError(nil, node).WithMessage("invalid keys in for")
		}
		if f.Var != "" && f.Matrix.Len() != 0 {
			return errors.NewTaskfileDecodeError(nil, node).WithMessage("cannot use both var and matrix in for")
		}
		return nil
	}

	return errors.NewTaskfileDecodeError(nil, node).WithTypeMessage("for")
}

func (f *For) DeepCopy() *For {
	if f == nil {
		return nil
	}
	return &For{
		From:   f.From,
		List:   deepcopy.Slice(f.List),
		Matrix: f.Matrix.DeepCopy(),
		Var:    f.Var,
		Split:  f.Split,
		As:     f.As,
	}
}
