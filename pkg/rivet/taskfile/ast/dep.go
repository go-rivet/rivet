package ast

import (
	"go.yaml.in/yaml/v3"

	"github.com/go-rivet/rivet/pkg/rivet/errors"
)

// Dep is a task dependency
type Dep struct {
	Task string
	For  *For
	Vars *Vars
}

func (d *Dep) DeepCopy() *Dep {
	if d == nil {
		return nil
	}
	return &Dep{
		Task: d.Task,
		For:  d.For.DeepCopy(),
		Vars: d.Vars.DeepCopy(),
	}
}

func (d *Dep) UnmarshalYAML(node *yaml.Node) error {
	switch node.Kind {

	case yaml.ScalarNode:
		var task string
		if err := node.Decode(&task); err != nil {
			return errors.NewTaskfileDecodeError(err, node)
		}
		d.Task = task
		return nil

	case yaml.MappingNode:
		for i := 0; i < len(node.Content); i += 2 {
			keyNode := node.Content[i]
			valNode := node.Content[i+1]

			switch keyNode.Value {
			case "task":
				d.Task = valNode.Value
			case "for":
				_ = valNode.Decode(&d.For)
			case "vars":
				_ = valNode.Decode(&d.Vars)
			}
		}
		return nil
	}

	return errors.NewTaskfileDecodeError(nil, node).WithTypeMessage("dependency")
}
