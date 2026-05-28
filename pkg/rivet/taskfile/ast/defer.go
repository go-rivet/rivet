package ast

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
		var deferStruct struct {
			Defer string
			Task  string
			Vars  *Vars
		}
		if err := node.Decode(&deferStruct); err != nil {
			return errors.NewTaskfileDecodeError(err, node)
		}
		d.Cmd = deferStruct.Defer
		d.Task = deferStruct.Task
		d.Vars = deferStruct.Vars
		return nil
	}

	return errors.NewTaskfileDecodeError(nil, node).WithTypeMessage("defer")
}
