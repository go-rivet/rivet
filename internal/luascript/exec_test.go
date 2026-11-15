//go:build cgo

package luascript

import (
	"bytes"
	"context"
	"errors"
	"io"
	"testing"
	"time"
)

func TestRunUsesFreshStateAndVariableSnapshot(t *testing.T) {
	vars := map[string]any{
		"VALUE":  "original",
		"print":  "print variable",
		"rivet":  "rivet variable",
		"ctx":    "context variable",
		"log":    "log variable",
		"a\x00b": "nul variable",
	}
	if err := Run(context.Background(), &Options{
		Script: `
			assert(ctx.vars.VALUE == "original")
			assert(ctx.vars.print == "print variable" and type(print) == "function")
			assert(ctx.vars.rivet == "rivet variable" and type(rivet.log.error) == "function")
			assert(ctx.vars.ctx == "context variable")
			assert(ctx.vars.log == "log variable" and log == nil)
			assert(ctx.vars["a\0b"] == "nul variable")
			assert(VALUE == nil)
			ctx.vars.VALUE = "changed"
		`,
		Vars: vars,
	}); err != nil {
		t.Fatal(err)
	}
	if vars["VALUE"] != "original" {
		t.Fatalf("Lua mutation changed the Go vars: %v", vars["VALUE"])
	}
	if err := Run(context.Background(), &Options{
		Script: `assert(ctx.vars.VALUE == "original")`,
		Vars:   vars,
	}); err != nil {
		t.Fatalf("second invocation did not receive a fresh Lua state: %v", err)
	}
}

func TestRunCopiesStringValues(t *testing.T) {
	type namedString string
	opts := &Options{
		Vars: map[string]any{
			"":        "",
			"a\x00b":  "x\x00y",
			"UNICODE": "Grüße",
			"NAMED":   namedString("named"),
			"NESTED":  map[string]any{"value": "original"},
		},
		Ctx: map[string]any{"root": map[string]any{"EMPTY": "", "BINARY": "a\x00b"}},
		Script: `
			assert(ctx.vars[""] == "" and vars[""] == "")
			assert(ctx.vars["a\0b"] == "x\0y" and vars["a\0b"] == "x\0y")
			assert(ctx.vars.UNICODE == "Grüße" and vars.NAMED == "named")
			assert(ctx.root.EMPTY == "" and ctx.root.BINARY == "a\0b")
			ctx.vars.NESTED.value = "changed"
			assert(vars.NESTED.value == "original")
		`,
	}
	if err := Run(context.Background(), opts); err != nil {
		t.Fatal(err)
	}
}

func TestRunCreatesEmptyVariableContext(t *testing.T) {
	err := Run(context.Background(), &Options{
		Script: `assert(type(ctx.vars) == "table" and next(ctx.vars) == nil); assert(log == nil)`,
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestRunReturnsLuaErrors(t *testing.T) {
	err := Run(context.Background(), &Options{Script: `error("script failed")`})
	var scriptErr *ScriptError
	if !errors.As(err, &scriptErr) {
		t.Fatalf("Run() error = %v, want *ScriptError", err)
	}
}

func TestRunPreventsLuaFromExitingHostProcess(t *testing.T) {
	err := Run(context.Background(), &Options{Script: `os.exit(0)`})
	var scriptErr *ScriptError
	if !errors.As(err, &scriptErr) {
		t.Fatalf("Run() error = %v, want *ScriptError", err)
	}
}

func TestRunWritesPrintToConfiguredStdout(t *testing.T) {
	var stdout bytes.Buffer
	if err := Run(context.Background(), &Options{
		Script: `print("hello", "lua")`,
		Stdout: &stdout,
	}); err != nil {
		t.Fatal(err)
	}
	if got, want := stdout.String(), "hello\tlua\n"; got != want {
		t.Fatalf("stdout = %q, want %q", got, want)
	}
}

func TestRunRecoversPanickingOutputWriter(t *testing.T) {
	err := Run(context.Background(), &Options{
		Script: `print("hello")`,
		Stdout: panicWriter{},
	})
	var scriptErr *ScriptError
	if !errors.As(err, &scriptErr) {
		t.Fatalf("Run() error = %v, want *ScriptError", err)
	}
}

func TestRunRecoversPanickingContext(t *testing.T) {
	err := Run(panicContext{}, &Options{Script: `return`})
	if err == nil {
		t.Fatal("Run() error = nil, want recovered panic error")
	}
}

func TestRunHonorsContextCancellation(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()

	err := Run(ctx, &Options{Script: `while true do end`})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Run() error = %v, want context deadline exceeded", err)
	}
}

func TestTaskFunctions(t *testing.T) {
	err := Run(context.Background(), &Options{Script: `
		local fn = rivet.fn
		assert(type(fn.OS()) == "string" and type(fn.ARCH()) == "string")
		assert(fn.numCPU() > 0 and type(fn.exeExt()) == "string")
		assert(fn.uuid():match("^%x%x%x%x%x%x%x%x%-%x%x%x%x%-4%x%x%x%-[89ab]%x%x%x%-%x%x%x%x%x%x%x%x%x%x%x%x$"))
		assert(fn.catLines("a\r\nb\nc") == "a b c")
		assert(fn.splitLines("a\r\nb\n")[3] == "")
		assert(fn.toSlash(fn.fromSlash("a/b")) == "a/b")
		assert(fn.joinPath("a", "b") == fn.fromSlash("a/b"))
		assert(fn.relPath(fn.joinPath("a"), fn.joinPath("a", "b")) == "b")
		assert(type(fn.absPath(".")) == "string")
		assert(fn.patsubst(".c", ".o", {"a.c", "b.h"})[1] == "a.o")
		assert(next(fn.patsubst(".c", ".o", {})) == nil)
		assert(type(fn.shellQuote("hello world")) == "string")
		assert(fn.splitArgs([[one "two three"]])[2] == "two three")
		assert(type(fn.joinEnv("one", "two")) == "string")
		assert(fn.joinUrl("https://example.com/a", "b") == "https://example.com/a/b")
		local base = {a = 1, same = "old"}
		local merged = fn.merge(base, {b = 2, same = "new"})
		assert(merged.a == 1 and merged.b == 2 and merged.same == "new" and base.same == "old")
		assert(fn.merge({["a\0b"] = "value"})["a\0b"] == "value")
		local y = fn.fromYaml("list: [one, two]\nflag: true\n")
		assert(y.list[2] == "two" and y.flag)
		local roundtrip = fn.fromYaml(fn.toYaml({items = {"x", "y"}, value = "a\0b"}))
		assert(roundtrip.items[2] == "y" and roundtrip.value == "a\0b")
		assert(fn.fromYaml("null") == nil)
		assert(type(fn.fromYaml(fn.toYaml({}))) == "table")
	`})
	if err != nil {
		t.Fatal(err)
	}
}

func TestTaskFunctionErrors(t *testing.T) {
	err := Run(context.Background(), &Options{Script: `
		local fn = rivet.fn
		local function fails(f, message)
			local ok, err = pcall(f)
			assert(not ok and type(err) == "string" and err:find(message, 1, true), tostring(err))
		end
		fails(function() fn.OS(1) end, "wrong number of arguments")
		fails(function() fn.joinPath("ok", 1) end, "argument 2")
		fails(function() fn.splitArgs('"unterminated') end, "rivet.fn.splitArgs")
		fails(function() fn.fromYaml("[") end, "rivet.fn.fromYaml")
		fails(function() fn.fromYaml("a: null") end, "null values")
		fails(function() fn.fromYaml("1: value") end, "map keys must be strings")
		fails(function() fn.fromYaml("date: 2020-01-02") end, "unsupported result type")
		fails(function() fn.toYaml({[1] = "one", [3] = "three"}) end, "contiguous")
		fails(function() fn.toYaml({[1] = "one", key = "value"}) end, "mix")
		fails(function() fn.toYaml({[true] = "value"}) end, "table keys")
		local cyclic = {}; cyclic.self = cyclic
		fails(function() fn.toYaml(cyclic) end, "cyclic")
		fails(function() fn.toYaml(print) end, "unsupported Lua type")
		fails(function() fn.merge({a = 1}, {"not", "a", "map"}) end, "map")
		fails(function() fn.patsubst(".c", ".o", {"a.c", false}) end, "path 2")
		local nested = {}
		for i = 1, 100 do nested = {child = nested} end
		assert(type(fn.merge(nested)) == "table")
		nested = {child = nested}
		fails(function() fn.merge(nested) end, "nesting exceeds")
		assert(fn.catLines("still\nrunning") == "still running")
	`})
	if err != nil {
		t.Fatal(err)
	}
}

type panicWriter struct{}

func (panicWriter) Write([]byte) (int, error) {
	panic("writer panic")
}

type panicContext struct {
	context.Context
}

func (panicContext) Err() error {
	panic("context panic")
}

var _ io.Writer = panicWriter{}
