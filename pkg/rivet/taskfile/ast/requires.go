package ast

import (
	"go.yaml.in/yaml/v3"

	"github.com/go-rivet/rivet/internal/deepcopy"
	"github.com/go-rivet/rivet/pkg/rivet/errors"
)

// Requires represents a set of required variables necessary for a task to run
type Requires struct {
	Vars []*VarsWithValidation
}

func (r *Requires) DeepCopy() *Requires {
	if r == nil {
		return nil
	}

	return &Requires{
		Vars: deepcopy.Slice(r.Vars),
	}
}

// Enum represents an enum constraint for a required variable.
// It can either be a static list of values or a reference to another variable.
type Enum struct {
	Ref   string
	Value []string
}

func (e *Enum) DeepCopy() *Enum {
	if e == nil {
		return nil
	}
	return &Enum{
		Ref:   e.Ref,
		Value: deepcopy.Slice(e.Value),
	}
}

// UnmarshalYAML implements yaml.Unmarshaler interface.
func (e *Enum) UnmarshalYAML(node *yaml.Node) error {
	switch node.Kind {
	case yaml.SequenceNode:
		// Static list of values: enum: ["a", "b"]
		var values []string
		if err := node.Decode(&values); err != nil {
			return errors.NewTaskfileDecodeError(err, node)
		}
		e.Value = values
		return nil

	case yaml.MappingNode:
		for i := 0; i < len(node.Content); i += 2 {
			keyNode := node.Content[i]
			valNode := node.Content[i+1]

			if keyNode.Value == "ref" {
				e.Ref = valNode.Value
			}
		}

		if e.Ref == "" {
			return errors.NewTaskfileDecodeError(nil, node).WithTypeMessage("enum")
		}
		return nil
	}

	return errors.NewTaskfileDecodeError(nil, node).WithTypeMessage("enum")
}

type VarsWithValidation struct {
	Name string
	Enum *Enum
}

func (v *VarsWithValidation) DeepCopy() *VarsWithValidation {
	if v == nil {
		return nil
	}
	return &VarsWithValidation{
		Name: v.Name,
		Enum: v.Enum.DeepCopy(),
	}
}

// UnmarshalYAML implements yaml.Unmarshaler interface.
func (v *VarsWithValidation) UnmarshalYAML(node *yaml.Node) error {
	switch node.Kind {

	case yaml.ScalarNode:
		var cmd string
		if err := node.Decode(&cmd); err != nil {
			return errors.NewTaskfileDecodeError(err, node)
		}
		v.Name = cmd
		v.Enum = nil
		return nil

	case yaml.MappingNode:
		for i := 0; i < len(node.Content); i += 2 {
			keyNode := node.Content[i]
			valNode := node.Content[i+1]

			switch keyNode.Value {
			case "name":
				v.Name = valNode.Value
			case "enum":
				_ = valNode.Decode(&v.Enum)
			}
		}
		return nil
	}

	return errors.NewTaskfileDecodeError(nil, node).WithTypeMessage("requires")
}
