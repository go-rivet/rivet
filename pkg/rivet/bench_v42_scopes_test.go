// SPDX-FileCopyrightText: 2026 Timothy Rule
// SPDX-License-Identifier: MIT

//go:build cgo

package rivet

import (
	"context"
	"fmt"
	"io"
	"testing"

	"github.com/go-rivet/rivet/internal/luascript"
	"github.com/go-rivet/rivet/pkg/rivet/namespace"
	"github.com/go-rivet/rivet/pkg/rivet/taskfile/ast"
)

func BenchmarkV42VariableScopes(b *testing.B) {
	for _, namespaces := range []int{0, 5, 50} {
		b.Run(fmt.Sprintf("namespaces_%d", namespaces), func(b *testing.B) {
			root := &ast.Taskfile{Vars: ast.NewVars()}
			declaration := &ast.Task{Task: "task", Vars: ast.NewVars()}
			if namespaces == 0 {
				root.Tasks = ast.NewTasks()
				root.Tasks.Set(declaration.Task, declaration)
			} else {
				declaration.Task = "ns0:task"
				declaration.Namespace = "ns0"
			}
			for i := 0; i < 10; i++ {
				root.Vars.Set(fmt.Sprintf("ROOT_%d", i), ast.Var{Value: "value"})
			}
			dag, err := namespace.NewDAG(root)
			if err != nil {
				b.Fatal(err)
			}
			for i := 0; i < namespaces; i++ {
				tf := &ast.Taskfile{Namespace: fmt.Sprintf("ns%d", i), Vars: ast.NewVars()}
				if i == 0 {
					tf.Tasks = ast.NewTasks()
					tf.Tasks.Set(declaration.Task, declaration)
				}
				for j := 0; j < 10; j++ {
					value := "value"
					if j == 1 {
						value = "{{.V_0}}-resolved"
					}
					tf.Vars.Set(fmt.Sprintf("V_%d", j), ast.Var{Value: value})
				}
				if err := dag.AddTaskfile(tf); err != nil {
					b.Fatal(err)
				}
			}
			e := &Executor{Taskfile: root, TaskfileDAG: dag, Dir: ".", RootDir: "."}
			e.envOnce.Do(func() {
				e.env = make(map[string]any)
				for i := 0; i < 40; i++ {
					e.env[fmt.Sprintf("ENV_%d", i)] = "environment"
				}
			})
			task := declaration.DeepCopy()
			taskVars, err := e.v42TaskVars(task, nil, true)
			if err != nil {
				b.Fatal(err)
			}
			task.Vars = taskVars
			luaVars, luaCtx, err := e.v42LuaVars(task, nil, taskVars)
			if err != nil {
				b.Fatal(err)
			}
			ctx := context.Background()
			opts := &luascript.Options{
				Script: `assert(ctx.root.ROOT_0 == "value" and vars[":ROOT_0"] == "value")`,
				Vars:   luaVars, Ctx: luaCtx, Stdout: io.Discard,
			}
			b.Run("lua_scopes", func(b *testing.B) {
				b.ReportAllocs()
				for b.Loop() {
					if _, _, err := e.v42LuaVars(task, nil, taskVars); err != nil {
						b.Fatal(err)
					}
				}
			})
			b.Run("lua_runtime", func(b *testing.B) {
				b.ReportAllocs()
				for b.Loop() {
					if err := luascript.Run(ctx, opts); err != nil {
						b.Fatal(err)
					}
				}
			})
			b.Run("shell_vars", func(b *testing.B) {
				b.ReportAllocs()
				for b.Loop() {
					if _, err := e.shellVars(ctx, task, nil); err != nil {
						b.Fatal(err)
					}
				}
			})
		})
	}
}
