package v42

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

// UnmarshalYAML reads the mapping form of a dependency. Keys are applied one at
// a time in set, which keeps the field logic independent of how the key/value
// pairs are produced.
func (d *Dep) UnmarshalYAML(node *yaml.Node) error {
	if node.Kind != yaml.MappingNode {
		return errors.NewTaskfileDecodeError(nil, node).WithTypeMessage("dependency")
	}
	for i := 0; i+1 < len(node.Content); i += 2 {
		if err := d.set(node.Content[i].Value, node.Content[i+1]); err != nil {
			return err
		}
	}
	return nil
}

func (d *Dep) set(key string, val *yaml.Node) error {
	switch key {
	case "task":
		d.Task = val.Value
	case "for":
		if err := val.Decode(&d.For); err != nil {
			return errors.NewTaskfileDecodeError(err, val)
		}
	case "vars":
		if err := val.Decode(&d.Vars); err != nil {
			return errors.NewTaskfileDecodeError(err, val)
		}
	}
	return nil
}
