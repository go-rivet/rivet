package main

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"math/rand"
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
	manyTasksBenchCase(),
	manyTasksSingleCallBenchCase(),
}

// manyTasksNames and manyTasksRunNames hold the 1000 task names (and the 500
// non-internal ones) shared by the many_tasks* bench cases, so both cases
// exercise an identical Taskfile.
var manyTasksNames = randomTaskNames(1000)
var manyTasksRunNames = func() []string {
	var runNames []string
	for i, name := range manyTasksNames {
		if i%2 == 1 {
			runNames = append(runNames, name)
		}
	}
	return runNames
}()

// writeManyTasksTaskfile writes a Taskfile containing all of manyTasksNames,
// with every other task (indices 0, 2, 4, ...) marked internal: true.
func writeManyTasksTaskfile(tb testing.TB, dir string) {
	tb.Helper()
	var sb strings.Builder
	sb.WriteString("version: '3'\ntasks:\n")
	for i, name := range manyTasksNames {
		fmt.Fprintf(&sb, "  %q:\n    cmds:\n      - echo \"running task: {{.TASK}}\"\n", name)
		if i%2 == 0 {
			sb.WriteString("    internal: true\n")
		}
	}
	if err := os.WriteFile(filepath.Join(dir, "Taskfile.yml"), []byte(sb.String()), 0o644); err != nil {
		tb.Fatalf("write taskfile: %v", err)
	}
}

// manyTasksBenchCase builds a benchCase for a Taskfile with 1000 tasks with
// random names (10-40 chars). Half of the tasks are marked internal: true;
// only the 500 non-internal tasks are run, and the check verifies each of
// them printed its "running task: {{.TASK}}" message exactly once.
func manyTasksBenchCase() benchCase {
	return benchCase{
		name:  "many_tasks",
		setup: writeManyTasksTaskfile,
		calls: func() []*task.Call {
			calls := make([]*task.Call, len(manyTasksRunNames))
			for i, name := range manyTasksRunNames {
				calls[i] = &task.Call{Task: name}
			}
			return calls
		},
		check: func(tb testing.TB, stdout, stderr string) {
			tb.Helper()
			counts := make(map[string]int, len(manyTasksRunNames))
			for _, line := range strings.Split(stdout, "\n") {
				line = strings.TrimSpace(line)
				if line != "" {
					counts[line]++
				}
			}
			for _, name := range manyTasksRunNames {
				want := "running task: " + name
				if counts[want] != 1 {
					tb.Fatalf("expected %q to run exactly once, got %d (stderr=%q)", want, counts[want], stderr)
				}
			}
			if len(counts) != len(manyTasksRunNames) {
				tb.Fatalf("expected exactly %d distinct task outputs, got %d (stdout=%q stderr=%q)", len(manyTasksRunNames), len(counts), stdout, stderr)
			}
		},
	}
}

// manyTasksSingleCallBenchCase uses the same 1000-task Taskfile as
// manyTasksBenchCase, but only runs one non-internal task, selected at
// random, to measure lookup/dispatch cost independent of task count run.
func manyTasksSingleCallBenchCase() benchCase {
	r := rand.New(rand.NewSource(2))
	name := manyTasksRunNames[r.Intn(len(manyTasksRunNames))]

	return benchCase{
		name:  "many_tasks_single_call",
		setup: writeManyTasksTaskfile,
		calls: func() []*task.Call {
			return []*task.Call{{Task: name}}
		},
		check: func(tb testing.TB, stdout, stderr string) {
			tb.Helper()
			want := "running task: " + name
			count := 0
			for _, line := range strings.Split(stdout, "\n") {
				if strings.TrimSpace(line) == want {
					count++
				}
			}
			if count != 1 {
				tb.Fatalf("expected %q to run exactly once, got %d (stdout=%q stderr=%q)", want, count, stdout, stderr)
			}
		},
	}
}

// randomTaskNames generates n unique random alphanumeric task names, each
// between 10 and 40 characters long. A fixed seed is used so benchmark runs
// are reproducible.
func randomTaskNames(n int) []string {
	const letters = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	r := rand.New(rand.NewSource(1))
	seen := make(map[string]bool, n)
	names := make([]string, 0, n)
	for len(names) < n {
		length := 10 + r.Intn(31)
		b := make([]byte, length)
		for i := range b {
			b[i] = letters[r.Intn(len(letters))]
		}
		name := string(b)
		if seen[name] {
			continue
		}
		seen[name] = true
		names = append(names, name)
	}
	return names
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
				defer func() { _ = f.Close() }()
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
	defer func() { _ = f.Close() }()
	runtime.GC()
	if err := pprof.WriteHeapProfile(f); err != nil {
		b.Fatalf("write mem profile: %v", err)
	}
}
