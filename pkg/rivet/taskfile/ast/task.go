package ast

import (
	"fmt"
	"regexp"
	"strings"

	"go.yaml.in/yaml/v3"

	"github.com/go-rivet/rivet/internal/deepcopy"
	"github.com/go-rivet/rivet/pkg/rivet/errors"
)

// Task represents a task
type Task struct {
	Task          string `json:"-"`
	Cmds          []*Cmd
	Deps          []*Dep
	Label         string
	Desc          string
	Prompt        Prompt
	Summary       string
	Requires      *Requires
	Aliases       []string
	Transform     *Transform
	Status        []string
	Preconditions []*Precondition
	Dir           string
	Set           []string
	Shopt         []string
	Vars          *Vars
	Dotenv        []string
	Interactive   bool
	Internal      bool
	Prefix        string `json:"-"`
	IgnoreError   bool
	Run           string
	Platforms     []*Platform
	If            string
	Watch         bool
	Location      *Location
	Failfast      bool
	// Populated during merging
	Namespace            string `json:"-"`
	IncludeVars          *Vars
	IncludedTaskfileVars *Vars

	FullName string `json:"-"`
}

func (t *Task) Name() string {
	if t.Label != "" {
		return t.Label
	}
	if t.FullName != "" {
		return t.FullName
	}
	return t.Task
}

func (t *Task) LocalName() string {
	name := t.FullName
	name = strings.TrimPrefix(name, t.Namespace)
	name = strings.TrimPrefix(name, ":")
	return name
}

// WildcardMatch will check if the given string matches the name of the Task and returns any wildcard values.
func (t *Task) WildcardMatch(name string) (bool, []string) {
	names := append([]string{t.Task}, t.Aliases...)

	for _, taskName := range names {
		regexStr := fmt.Sprintf("^%s$", strings.ReplaceAll(taskName, "*", "(.*)"))
		regex := regexp.MustCompile(regexStr)
		wildcards := regex.FindStringSubmatch(name)

		if len(wildcards) == 0 {
			continue
		}

		// Remove the first match, which is the full string
		wildcards = wildcards[1:]
		wildcardCount := strings.Count(taskName, "*")

		if len(wildcards) != wildcardCount {
			continue
		}

		return true, wildcards
	}

	return false, nil
}

func (t *Task) UnmarshalYAML(node *yaml.Node) error {
	switch node.Kind {

	// Shortcut syntax for a task with a single command
	case yaml.ScalarNode:
		var cmd Cmd
		if err := node.Decode(&cmd); err != nil {
			return errors.NewTaskfileDecodeError(err, node)
		}
		t.Cmds = append(t.Cmds, &cmd)
		return nil

	// Shortcut syntax for a simple task with a list of commands
	case yaml.SequenceNode:
		var cmds []*Cmd
		if err := node.Decode(&cmds); err != nil {
			return errors.NewTaskfileDecodeError(err, node)
		}
		t.Cmds = cmds
		return nil

	case yaml.MappingNode:
		var hasCmd, hasCmds bool
		var singleCmd *Cmd
		var sources, generates []*Glob

		for i := 0; i < len(node.Content); i += 2 {
			keyNode := node.Content[i]
			valNode := node.Content[i+1]

			switch keyNode.Value {
			case "label":
				t.Label = valNode.Value
			case "desc":
				t.Desc = valNode.Value
			case "summary":
				t.Summary = valNode.Value
			case "dir":
				t.Dir = valNode.Value
			case "if":
				t.If = valNode.Value
			case "run":
				t.Run = valNode.Value
			case "prefix":
				t.Prefix = valNode.Value
			case "interactive":
				_ = valNode.Decode(&t.Interactive)
			case "internal":
				_ = valNode.Decode(&t.Internal)
			case "watch":
				_ = valNode.Decode(&t.Watch)
			case "failfast":
				_ = valNode.Decode(&t.Failfast)
			case "ignore_error":
				_ = valNode.Decode(&t.IgnoreError)
			case "vars":
				_ = valNode.Decode(&t.Vars)
			case "prompt":
				_ = valNode.Decode(&t.Prompt)
			case "requires":
				_ = valNode.Decode(&t.Requires)
			case "transform":
				_ = valNode.Decode(&t.Transform)
			case "status":
				_ = valNode.Decode(&t.Status)
			case "preconditions":
				_ = valNode.Decode(&t.Preconditions)
			case "platforms":
				_ = valNode.Decode(&t.Platforms)
			case "shopt":
				_ = valNode.Decode(&t.Shopt)
			case "set":
				_ = valNode.Decode(&t.Set)
			case "dotenv":
				_ = valNode.Decode(&t.Dotenv)
			case "aliases":
				_ = valNode.Decode(&t.Aliases)
			case "deps":
				_ = valNode.Decode(&t.Deps)
			case "sources":
				_ = valNode.Decode(&sources)
			case "generates":
				_ = valNode.Decode(&generates)
			case "cmds":
				if err := valNode.Decode(&t.Cmds); err == nil {
					hasCmds = true
				}
			case "cmd":
				var cmd Cmd
				if err := valNode.Decode(&cmd); err == nil {
					singleCmd = &cmd
					hasCmd = true
				}
			}
		}

		if hasCmd && hasCmds {
			return errors.NewTaskfileDecodeError(nil, node).WithMessage("task cannot have both cmd and cmds")
		}
		if hasCmd {
			t.Cmds = []*Cmd{singleCmd}
		}

		if t.Transform == nil && (len(sources) > 0 || len(generates) > 0) {
			t.Transform = &Transform{
				Matches: sources,
				Yields:  generates,
			}
		}
		return nil
	}

	return errors.NewTaskfileDecodeError(nil, node).WithTypeMessage("task")
}

// DeepCopy creates a new instance of Task and copies
// data by value from the source struct.
func (t *Task) DeepCopy() *Task {
	if t == nil {
		return nil
	}
	c := &Task{
		Task:                 t.Task,
		Cmds:                 deepcopy.Slice(t.Cmds),
		Deps:                 deepcopy.Slice(t.Deps),
		Label:                t.Label,
		Desc:                 t.Desc,
		Prompt:               t.Prompt,
		Summary:              t.Summary,
		Aliases:              deepcopy.Slice(t.Aliases),
		Transform:            t.Transform.DeepCopy(),
		Status:               deepcopy.Slice(t.Status),
		Preconditions:        deepcopy.Slice(t.Preconditions),
		Dir:                  t.Dir,
		Set:                  deepcopy.Slice(t.Set),
		Shopt:                deepcopy.Slice(t.Shopt),
		Vars:                 t.Vars.DeepCopy(),
		Dotenv:               deepcopy.Slice(t.Dotenv),
		Interactive:          t.Interactive,
		Internal:             t.Internal,
		Prefix:               t.Prefix,
		IgnoreError:          t.IgnoreError,
		Run:                  t.Run,
		IncludeVars:          t.IncludeVars.DeepCopy(),
		IncludedTaskfileVars: t.IncludedTaskfileVars.DeepCopy(),
		Platforms:            deepcopy.Slice(t.Platforms),
		If:                   t.If,
		Location:             t.Location.DeepCopy(),
		Requires:             t.Requires.DeepCopy(),
		Namespace:            t.Namespace,
		FullName:             t.FullName,
		Watch:                t.Watch,
		Failfast:             t.Failfast,
	}
	return c
}
