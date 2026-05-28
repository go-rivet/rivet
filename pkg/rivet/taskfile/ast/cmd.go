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
		var cmdStruct struct {
			Cmd         string
			Task        string
			For         *For
			If          string
			Set         []string
			Shopt       []string
			Vars        *Vars
			IgnoreError bool `yaml:"ignore_error"`
			Defer       *Defer
			Platforms   []*Platform
		}
		if err := node.Decode(&cmdStruct); err != nil {
			return errors.NewTaskfileDecodeError(err, node)
		}
		if cmdStruct.Defer != nil {

			// A deferred command
			if cmdStruct.Defer.Cmd != "" {
				c.Defer = true
				c.Cmd = cmdStruct.Defer.Cmd
				return nil
			}

			// A deferred task call
			if cmdStruct.Defer.Task != "" {
				c.Defer = true
				c.Task = cmdStruct.Defer.Task
				c.Vars = cmdStruct.Defer.Vars
				return nil
			}
			return nil
		}

		// A task call
		if cmdStruct.Task != "" {
			c.Task = cmdStruct.Task
			c.Vars = cmdStruct.Vars
			c.For = cmdStruct.For
			c.If = cmdStruct.If
			c.IgnoreError = cmdStruct.IgnoreError
			return nil
		}

		// A command with additional options
		if cmdStruct.Cmd != "" {
			c.Cmd = cmdStruct.Cmd
			c.For = cmdStruct.For
			c.If = cmdStruct.If
			c.Set = cmdStruct.Set
			c.Shopt = cmdStruct.Shopt
			c.IgnoreError = cmdStruct.IgnoreError
			c.Platforms = cmdStruct.Platforms
			return nil
		}

		return errors.NewTaskfileDecodeError(nil, node).WithMessage("invalid keys in command")
	}

	return errors.NewTaskfileDecodeError(nil, node).WithTypeMessage("command")
}
