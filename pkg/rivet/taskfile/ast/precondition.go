package ast

import (
	"fmt"

	"go.yaml.in/yaml/v3"

	"github.com/go-rivet/rivet/pkg/rivet/errors"
)

// Precondition represents a precondition necessary for a task to run
type Precondition struct {
	Sh  string
	Msg string
}

func (p *Precondition) DeepCopy() *Precondition {
	if p == nil {
		return nil
	}
	return &Precondition{
		Sh:  p.Sh,
		Msg: p.Msg,
	}
}

func (p *Precondition) UnmarshalYAML(node *yaml.Node) error {
	switch node.Kind {

	case yaml.ScalarNode:
		var cmd string
		if err := node.Decode(&cmd); err != nil {
			return errors.NewTaskfileDecodeError(err, node)
		}
		p.Sh = cmd
		p.Msg = fmt.Sprintf("`%s` failed", cmd)
		return nil

	case yaml.MappingNode:
		for i := 0; i < len(node.Content); i += 2 {
			keyNode := node.Content[i]
			valNode := node.Content[i+1]

			switch keyNode.Value {
			case "sh":
				p.Sh = valNode.Value
			case "msg":
				p.Msg = valNode.Value
			}
		}

		if p.Msg == "" {
			p.Msg = fmt.Sprintf("%s failed", p.Sh)
		}
		return nil
	}

	return errors.NewTaskfileDecodeError(nil, node).WithTypeMessage("precondition")
}
