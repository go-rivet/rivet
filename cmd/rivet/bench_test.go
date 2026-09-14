package main

import (
	"bytes"
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"runtime/pprof"
	"strings"
	"testing"

	task "github.com/go-rivet/rivet/pkg/rivet"
	"github.com/go-rivet/rivet/pkg/rlog"
)

// benchCase describes a single table-driven benchmark scenario that exercises
// rivet starting as close to main() as practical: building an [task.Executor]
// and running its Setup/Run phases (the same steps performed by run() in
// rivet.go).
type benchCase struct {
	name string
	// setup writes the Taskfile (and any other fixtures) into dir. Not timed.
	setup func(tb testing.TB, dir string)
	// calls returns the task calls to run for this case. Called fresh on each
	// iteration since Executor.Run may mutate the calls' Vars.
	calls func() []*task.Call
	// check validates the captured output after the benchmark loop. Not timed.
	check func(tb testing.TB, stdout, stderr string)
}

var benchCases = []benchCase{
	{
		name: "print_message",
		setup: func(tb testing.TB, dir string) {
			tb.Helper()
			taskfile := "version: '3'\ntasks:\n  default:\n    cmds:\n      - echo \"hello from rivet\"\n"
			if err := os.WriteFile(filepath.Join(dir, "Taskfile.yml"), []byte(taskfile), 0o644); err != nil {
				tb.Fatalf("write taskfile: %v", err)
			}
		},
		calls: func() []*task.Call {
			return []*task.Call{{Task: "default"}}
		},
		check: func(tb testing.TB, stdout, stderr string) {
			tb.Helper()
			if !strings.Contains(stdout, "hello from rivet") {
				tb.Fatalf("expected stdout to contain %q, got stdout=%q stderr=%q", "hello from rivet", stdout, stderr)
			}
		},
	},
}

// benchProfileDir, when set via RIVET_BENCH_PROFILE_DIR, causes each benchmark
// case to write its own CPU profile to <dir>/<name>_cpu.out and heap profile to
// <dir>/<name>_mem.out. This is done manually (rather than via go test's
// -cpuprofile/-memprofile flags) so that table-driven sub-benchmarks each get
// their own profiles instead of one combined profile.
var benchProfileDir = os.Getenv("RIVET_BENCH_PROFILE_DIR")

// BenchmarkRivet exercises rivet starting near main(): only Executor
// construction, Setup and Run are timed/profiled. Taskfile setup and output
// assertions run outside the timed loop.
func BenchmarkRivet(b *testing.B) {
	for _, tc := range benchCases {
		b.Run(tc.name, func(b *testing.B) {
			dir := b.TempDir()
			tc.setup(b, dir)

			ctx := context.Background()
			var stdout, stderr bytes.Buffer

			if benchProfileDir != "" {
				f, err := os.Create(filepath.Join(benchProfileDir, tc.name+"_cpu.out"))
				if err != nil {
					b.Fatalf("create profile file: %v", err)
				}
				defer f.Close()
				if err := pprof.StartCPUProfile(f); err != nil {
					b.Fatalf("start cpu profile: %v", err)
				}
				defer pprof.StopCPUProfile()
			}

			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				stdout.Reset()
				stderr.Reset()

				levelVar := &slog.LevelVar{}
				levelVar.Set(slog.LevelInfo)
				rlog.Init(rlog.RlogOptions{
					Stdout: &stdout,
					Stderr: &stderr,
					Level:  levelVar,
					Color:  false,
				})

				e := task.NewExecutor(
					task.WithDir(dir),
					task.WithStdout(&stdout),
					task.WithStderr(&stderr),
					task.WithColor(false),
					task.WithLevelVar(levelVar),
				)
				if err := e.Setup(ctx); err != nil {
					b.Fatalf("setup: %v", err)
				}
				if err := e.Run(ctx, tc.calls()...); err != nil {
					b.Fatalf("run: %v", err)
				}
			}
			b.StopTimer()

			if benchProfileDir != "" {
				writeHeapProfile(b, filepath.Join(benchProfileDir, tc.name+"_mem.out"))
			}

			tc.check(b, stdout.String(), stderr.String())
		})
	}
}

// writeHeapProfile writes a fresh (GC'd) heap snapshot to path, mirroring
// go test's -memprofile behavior for a single benchmark.
func writeHeapProfile(b *testing.B, path string) {
	b.Helper()
	f, err := os.Create(path)
	if err != nil {
		b.Fatalf("create mem profile file: %v", err)
	}
	defer f.Close()
	runtime.GC()
	if err := pprof.WriteHeapProfile(f); err != nil {
		b.Fatalf("write mem profile: %v", err)
	}
}
