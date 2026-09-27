package ast

import (
	"go.yaml.in/yaml/v3"

	"github.com/go-rivet/rivet/pkg/rivet/errors"
)

// Var represents either a static or dynamic variable.
type Var struct {
	Value any
	Live  any
	Sh    *string
	Ref   string
	Dir   string
}

func (v *Var) UnmarshalYAML(node *yaml.Node) error {
	switch node.Kind {
	case yaml.MappingNode:
		key := "<none>"
		if len(node.Content) > 0 {
			key = node.Content[0].Value
		}

		switch key {
		case "sh", "ref", "map", "live", "dir":
			for i := 0; i < len(node.Content); i += 2 {
				keyNode := node.Content[i]
				valNode := node.Content[i+1]

				switch keyNode.Value {
				case "sh":
					_ = valNode.Decode(&v.Sh)
				case "ref":
					v.Ref = valNode.Value
				case "map":
					_ = valNode.Decode(&v.Value)
				case "live":
					if err := valNode.Decode(&v.Live); err != nil {
						return errors.NewTaskfileDecodeError(err, node)
					}
				case "dir":
					if err := valNode.Decode(&v.Dir); err != nil {
						return errors.NewTaskfileDecodeError(err, node)
					}
				}
			}
			return nil

		default:
			return errors.NewTaskfileDecodeError(nil, node).WithMessage(`%q is not a valid variable type. Try "sh", "ref", "map", "live", "dir" or using a scalar value`, key)
		}

	default:
		var value any
		if err := node.Decode(&value); err != nil {
			return errors.NewTaskfileDecodeError(err, node)
		}
		v.Value = value
		return nil
	}
}
