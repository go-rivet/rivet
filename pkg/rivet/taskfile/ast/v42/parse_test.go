package v42_test

import (
	"reflect"
	"strings"
	"testing"

	"go.yaml.in/yaml/v3"

	"github.com/go-rivet/rivet/pkg/rivet/taskfile/ast"
	v42 "github.com/go-rivet/rivet/pkg/rivet/taskfile/ast/v42"
)

const doc = `version: 42
output: group
vars:
  A: a
  B: {sh: echo b}
tasks:
  build:
    desc: d
    deps: [other, {task: other, vars: {X: 1}}]
    vars: {Y: y}
    platforms: [linux/amd64]
    requires: {vars: [{name: Z, enum: [a, b]}]}
    preconditions: [{sh: "true", msg: m}]
    cmds:
      - echo hi
      - task: other
      - for: [a, b]
        cmd: echo {{.ITEM}}
      - defer: echo done
  other:
    cmds: [echo o]
`

// The runtime adapter must match what the v3 decoder produces for the same document.
func TestParseMatchesV3Decode(t *testing.T) {
	var node yaml.Node
	if err := yaml.Unmarshal([]byte(doc), &node); err != nil {
		t.Fatal(err)
	}
	got, err := v42.Parse(&node)
	if err != nil {
		t.Fatal(err)
	}
	var want ast.Taskfile
	if err := node.Decode(&want); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, &want) {
		t.Fatalf("mismatch\n got: %+v\nwant: %+v", got, &want)
	}
}

func TestParseRejectsOtherVersions(t *testing.T) {
	var node yaml.Node
	if err := yaml.Unmarshal([]byte(strings.Replace(doc, "42", "3", 1)), &node); err != nil {
		t.Fatal(err)
	}
	if _, err := v42.Parse(&node); err == nil {
		t.Fatal("expected error")
	}
}
