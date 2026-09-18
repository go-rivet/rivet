package ast

import (
	"go.yaml.in/yaml/v3"

	"github.com/go-rivet/rivet/internal/deepcopy"
	"github.com/go-rivet/rivet/pkg/rivet/errors"
)

// Cmd is a task command
type Cmd struct {
	Cmd         string
	Task        string
	For         *For
	If          string
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
		Task:        c.Task,
		For:         c.For.DeepCopy(),
		If:          c.If,
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
		var hasCmd, hasTask, hasDefer bool
		var deferBlock *Defer

		for i := 0; i < len(node.Content); i += 2 {
			keyNode := node.Content[i]
			valNode := node.Content[i+1]

			switch keyNode.Value {
			case "cmd":
				c.Cmd = valNode.Value
				hasCmd = true
			case "task":
				c.Task = valNode.Value
				hasTask = true
			case "if":
				c.If = valNode.Value
			case "ignore_error":
				_ = valNode.Decode(&c.IgnoreError)
			case "set":
				_ = valNode.Decode(&c.Set)
			case "shopt":
				_ = valNode.Decode(&c.Shopt)
			case "vars":
				_ = valNode.Decode(&c.Vars)
			case "for":
				_ = valNode.Decode(&c.For)
			case "platforms":
				_ = valNode.Decode(&c.Platforms)
			case "defer":
				if err := valNode.Decode(&deferBlock); err == nil {
					hasDefer = true
				}
			}
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

		// Handle explicit task calls
		if hasTask && c.Task != "" {
			return nil
		}

		// Handle raw commands with custom configuration options
		if hasCmd && c.Cmd != "" {
			return nil
		}

		// Fail open if structural metadata parameters are unpopulated
		return errors.NewTaskfileDecodeError(nil, node).WithMessage("invalid keys in command")
	}

	return errors.NewTaskfileDecodeError(nil, node).WithTypeMessage("command")
}
