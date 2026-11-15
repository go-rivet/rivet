package v42

import (
	"go.yaml.in/yaml/v3"

	"github.com/go-rivet/rivet/pkg/rivet/errors"
)

type Defer struct {
	Cmd  string
	Task string
	Vars *Vars
}

func (d *Defer) UnmarshalYAML(node *yaml.Node) error {
	switch node.Kind {

	case yaml.ScalarNode:
		var cmd string
		if err := node.Decode(&cmd); err != nil {
			return errors.NewTaskfileDecodeError(err, node)
		}
		d.Cmd = cmd
		return nil

	case yaml.MappingNode:
		var lua string
		for i := 0; i < len(node.Content); i += 2 {
			keyNode := node.Content[i]
			valNode := node.Content[i+1]

			switch keyNode.Value {
			case "for", "if":
				return unsupportedControlField(keyNode, keyNode.Value)
			case "cmd":
				d.Cmd = valNode.Value
			case "task":
				d.Task = valNode.Value
			case "lua":
				lua = valNode.Value
			case "vars":
				_ = valNode.Decode(&d.Vars)
			}
		}
		if hasMultipleCommandTypes(d.Cmd, d.Task, lua) {
			return errors.NewTaskfileDecodeError(nil, node).WithMessage(multipleCommandTypesMessage)
		}
		if lua != "" {
			return errors.NewTaskfileDecodeError(nil, node).WithMessage("Lua commands cannot be deferred")
		}
		return nil
	}

	return errors.NewTaskfileDecodeError(nil, node).WithTypeMessage("defer")
}
