package v42

import (
	"go.yaml.in/yaml/v3"

	"github.com/go-rivet/rivet/internal/deepcopy"
	"github.com/go-rivet/rivet/pkg/rivet/errors"
)

const multipleCommandTypesMessage = "command cannot have more than one of lua, cmd, or task"

func hasMultipleCommandTypes(cmd, task, lua string) bool {
	count := 0
	for _, value := range []string{cmd, task, lua} {
		if value != "" {
			count++
		}
	}
	return count > 1
}

// Cmd is a task command
type Cmd struct {
	Cmd         string
	Lua         string
	Task        string
	Set         []string
	Shopt       []string
	Vars        *Vars
	IgnoreError bool
	Defer       bool
	Platforms   []*Platform
}

func (c *Cmd) DeepCopy() *Cmd {
	if c == nil {
		return nil
	}
	return &Cmd{
		Cmd:         c.Cmd,
		Lua:         c.Lua,
		Task:        c.Task,
		Set:         deepcopy.Slice(c.Set),
		Shopt:       deepcopy.Slice(c.Shopt),
		Vars:        c.Vars.DeepCopy(),
		IgnoreError: c.IgnoreError,
		Defer:       c.Defer,
		Platforms:   deepcopy.Slice(c.Platforms),
	}
}

func (c *Cmd) UnmarshalYAML(node *yaml.Node) error {
	switch node.Kind {

	case yaml.ScalarNode:
		var cmd string
		if err := node.Decode(&cmd); err != nil {
			return errors.NewTaskfileDecodeError(err, node)
		}
		c.Cmd = cmd
		return nil

	case yaml.MappingNode:
		var hasDefer bool
		var deferBlock *Defer

		for i := 0; i < len(node.Content); i += 2 {
			keyNode := node.Content[i]
			valNode := node.Content[i+1]

			switch keyNode.Value {
			case "cmd":
				c.Cmd = valNode.Value
			case "lua":
				c.Lua = valNode.Value
			case "task":
				c.Task = valNode.Value
			case "for", "if":
				return unsupportedControlField(keyNode, keyNode.Value)
			case "ignore_error":
				_ = valNode.Decode(&c.IgnoreError)
			case "set":
				_ = valNode.Decode(&c.Set)
			case "shopt":
				_ = valNode.Decode(&c.Shopt)
			case "vars":
				_ = valNode.Decode(&c.Vars)
			case "platforms":
				_ = valNode.Decode(&c.Platforms)
			case "defer":
				if err := valNode.Decode(&deferBlock); err != nil {
					return err
				}
				hasDefer = true
			}
		}

		if hasMultipleCommandTypes(c.Cmd, c.Task, c.Lua) {
			return errors.NewTaskfileDecodeError(nil, node).WithMessage(multipleCommandTypesMessage)
		}

		if c.Lua != "" && hasDefer {
			return errors.NewTaskfileDecodeError(nil, node).WithMessage("Lua commands cannot be deferred")
		}

		if hasDefer && deferBlock != nil {
			if deferBlock.Cmd != "" {
				c.Defer = true
				c.Cmd = deferBlock.Cmd
				return nil
			}
			if deferBlock.Task != "" {
				c.Defer = true
				c.Task = deferBlock.Task
				c.Vars = deferBlock.Vars
				return nil
			}
			return nil
		}

		if c.Task != "" || c.Lua != "" || c.Cmd != "" {
			return nil
		}

		// Fail open if structural metadata parameters are unpopulated
		return errors.NewTaskfileDecodeError(nil, node).WithMessage("invalid keys in command")
	}

	return errors.NewTaskfileDecodeError(nil, node).WithTypeMessage("command")
}
