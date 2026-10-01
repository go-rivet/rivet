package ast_test

import (
	"reflect"
	"testing"

	"github.com/go-rivet/rivet/pkg/rivet/taskfile/ast"
)

func TestVarsOverlay(t *testing.T) {
	base := ast.NewVars(
		&ast.VarElement{Key: "FIRST", Value: ast.Var{Value: "base-first"}},
		&ast.VarElement{Key: "SHADOWED", Value: ast.Var{Value: "base-value"}},
	)
	vars := ast.NewVarsWithBase(base, 2)
	vars.Set("SHADOWED", ast.Var{Value: "local-value"})
	vars.Set("LAST", ast.Var{Value: "local-last"})

	if got, want := vars.Len(), 3; got != want {
		t.Fatalf("Len() = %d, want %d", got, want)
	}
	if got, ok := vars.Get("SHADOWED"); !ok || got.Value != "local-value" {
		t.Fatalf("Get(SHADOWED) = %#v, %v; want local value", got, ok)
	}
	if got, ok := base.Get("SHADOWED"); !ok || got.Value != "base-value" {
		t.Fatalf("base Get(SHADOWED) = %#v, %v; base was mutated", got, ok)
	}

	var keys []string
	var values []any
	for key, value := range vars.All() {
		keys = append(keys, key)
		values = append(values, value.Value)
	}
	if want := []string{"FIRST", "SHADOWED", "LAST"}; !reflect.DeepEqual(keys, want) {
		t.Fatalf("All() keys = %v, want %v", keys, want)
	}
	if want := []any{"base-first", "local-value", "local-last"}; !reflect.DeepEqual(values, want) {
		t.Fatalf("All() values = %v, want %v", values, want)
	}

	if got, want := vars.ToCacheMap(), map[string]any{
		"FIRST": "base-first", "SHADOWED": "local-value", "LAST": "local-last",
	}; !reflect.DeepEqual(got, want) {
		t.Fatalf("ToCacheMap() = %v, want %v", got, want)
	}

	copy := vars.DeepCopy()
	copy.Set("SHADOWED", ast.Var{Value: "copy-value"})
	if got, _ := vars.Get("SHADOWED"); got.Value != "local-value" {
		t.Fatalf("original value changed after DeepCopy mutation: %v", got.Value)
	}
}
