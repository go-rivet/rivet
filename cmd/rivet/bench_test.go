package main

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"math/rand"
	"os"
	"os/exec"
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
	mtimeBenchCase(mtimeBenchScenario{name: "500_Files_Depth_3", totalFiles: 500, depth: 3}),
}

type mtimeBenchScenario struct {
	name       string
	totalFiles int
	depth      int
}

var fullMtimeBenchScenarios = []mtimeBenchScenario{
	{name: "Small_100_Files", totalFiles: 100, depth: 2},
	{name: "Standard_10k_Files", totalFiles: 10000, depth: 3},
	{name: "Massive_50k_Files", totalFiles: 50000, depth: 3},
	{name: "Extremely_Deep_Layout", totalFiles: 10000, depth: 6},
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

// manyTasksGlobalVars are the names of the 4 global vars written by
// writeManyTasksTaskfile: the first 2 are plain strings, the last 2 are
// templates derived from the first 2.
var manyTasksGlobalVars = []string{"GLOBAL_VAR_1", "GLOBAL_VAR_2", "GLOBAL_VAR_3", "GLOBAL_VAR_4"}

// writeManyTasksTaskfile writes a Taskfile containing all of manyTasksNames,
// with every other task (indices 0, 2, 4, ...) marked internal: true. To
// approximate a realistic Taskfile, it also declares 4 global vars (2 plain
// strings and 2 templates derived from them) and, per task, 2 task-scoped
// vars named after the task; each task's 3 cmds print the task name followed
// by 2 randomly chosen vars (global or task-scoped).
func writeManyTasksTaskfile(tb testing.TB, dir string) {
	tb.Helper()
	r := rand.New(rand.NewSource(3))
	var sb strings.Builder
	sb.WriteString("version: '3'\n")
	sb.WriteString("vars:\n")
	sb.WriteString("  GLOBAL_VAR_1: \"global value one\"\n")
	sb.WriteString("  GLOBAL_VAR_2: \"global value two\"\n")
	sb.WriteString("  GLOBAL_VAR_3: \"{{.GLOBAL_VAR_1}}-templated\"\n")
	sb.WriteString("  GLOBAL_VAR_4: \"{{.GLOBAL_VAR_2}}-templated\"\n")
	sb.WriteString("tasks:\n")
	for i, name := range manyTasksNames {
		// leading underscore keeps the var name a valid template identifier
		// even though task names may start with a digit.
		var1, var2 := "_"+name+"_VAR1", "_"+name+"_VAR2"
		fmt.Fprintf(&sb, "  %q:\n", name)
		sb.WriteString("    vars:\n")
		fmt.Fprintf(&sb, "      %s: %q\n", var1, name+" text one")
		fmt.Fprintf(&sb, "      %s: %q\n", var2, name+" text two")

		taskVars := append(append([]string{}, manyTasksGlobalVars...), var1, var2)
		sb.WriteString("    cmds:\n")
		sb.WriteString("      - echo \"running task: {{.TASK}}\"\n")
		fmt.Fprintf(&sb, "      - echo \"{{.%s}}\"\n", taskVars[r.Intn(len(taskVars))])
		fmt.Fprintf(&sb, "      - echo \"{{.%s}}\"\n", taskVars[r.Intn(len(taskVars))])
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
		name:  "many_tasks_run_all",
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
				if strings.HasPrefix(line, "running task: ") {
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
	name := manyTasksSingleRunName()

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

func manyTasksSingleRunName() string {
	r := rand.New(rand.NewSource(2))
	return manyTasksRunNames[r.Intn(len(manyTasksRunNames))]
}

// BenchmarkManyTasksCompare compares Rivet with Make for one and all 500
// runnable targets. Run it explicitly with -bench=BenchmarkManyTasksCompare.
func BenchmarkManyTasksCompare(b *testing.B) {
	rivetAll := manyTasksBenchCase()
	b.Run("rivet_many_tasks_run_all", func(b *testing.B) {
		runRivetBenchCase(b, rivetAll)
	})
	b.Run("make_many_tasks_run_all", func(b *testing.B) {
		runManyTasksMakeBench(b, "", manyTasksRunNames)
	})

	singleName := manyTasksSingleRunName()
	rivetSingle := manyTasksSingleCallBenchCase()
	b.Run("rivet_many_tasks_single_call", func(b *testing.B) {
		runRivetBenchCase(b, rivetSingle)
	})
	b.Run("make_many_tasks_single_call", func(b *testing.B) {
		runManyTasksMakeBench(b, singleName, []string{singleName})
	})
}

func runManyTasksMakeBench(b *testing.B, target string, expectedTasks []string) {
	b.Helper()
	dir := b.TempDir()
	writeManyTasksMakefile(b, dir)
	var stdout, stderr bytes.Buffer

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		stdout.Reset()
		stderr.Reset()
		args := []string{"--no-print-directory"}
		if target != "" {
			args = append(args, target)
		}
		cmd := exec.Command("make", args...)
		cmd.Dir = dir
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr
		if err := cmd.Run(); err != nil {
			b.Fatalf("make: %v (stderr=%q)", err, stderr.String())
		}
	}
	b.StopTimer()
	checkManyTasksOutput(b, stdout.String(), stderr.String(), expectedTasks)
}

func writeManyTasksMakefile(tb testing.TB, dir string) {
	tb.Helper()
	var sb strings.Builder
	sb.WriteString("GLOBAL_VAR_1 := global value one\n")
	sb.WriteString("GLOBAL_VAR_2 := global value two\n")
	sb.WriteString("GLOBAL_VAR_3 = $(GLOBAL_VAR_1)-templated\n")
	sb.WriteString("GLOBAL_VAR_4 = $(GLOBAL_VAR_2)-templated\n")
	sb.WriteString(".DEFAULT_GOAL := all\n.PHONY: all")
	for _, name := range manyTasksRunNames {
		fmt.Fprintf(&sb, " %s", name)
	}
	sb.WriteString("\nall:")
	for _, name := range manyTasksRunNames {
		fmt.Fprintf(&sb, " %s", name)
	}
	sb.WriteByte('\n')

	r := rand.New(rand.NewSource(3))
	for i, name := range manyTasksNames {
		var1, var2 := "_"+name+"_VAR1", "_"+name+"_VAR2"
		vars := append(append([]string{}, manyTasksGlobalVars...), var1, var2)
		commandVars := [2]string{
			vars[r.Intn(len(vars))],
			vars[r.Intn(len(vars))],
		}
		if i%2 == 0 {
			continue
		}

		fmt.Fprintf(&sb, "%s: %s := %q\n", name, var1, name+" text one")
		fmt.Fprintf(&sb, "%s: %s := %q\n", name, var2, name+" text two")
		fmt.Fprintf(&sb, "%s:\n\t@echo \"running task: $@\"\n", name)
		fmt.Fprintf(&sb, "\t@echo \"$(%s)\"\n", commandVars[0])
		fmt.Fprintf(&sb, "\t@echo \"$(%s)\"\n", commandVars[1])
	}
	if err := os.WriteFile(filepath.Join(dir, "Makefile"), []byte(sb.String()), 0o644); err != nil {
		tb.Fatalf("write Makefile: %v", err)
	}
}

func checkManyTasksOutput(tb testing.TB, stdout, stderr string, expectedTasks []string) {
	tb.Helper()
	counts := make(map[string]int, len(expectedTasks))
	for _, line := range strings.Split(stdout, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "running task: ") {
			counts[line]++
		}
	}
	for _, name := range expectedTasks {
		want := "running task: " + name
		if counts[want] != 1 {
			tb.Fatalf("expected %q to run exactly once, got %d (stderr=%q)", want, counts[want], stderr)
		}
	}
	if len(counts) != len(expectedTasks) {
		tb.Fatalf("expected exactly %d distinct task outputs, got %d (stdout=%q stderr=%q)", len(expectedTasks), len(counts), stdout, stderr)
	}
}

func mtimeBenchCase(scenario mtimeBenchScenario) benchCase {
	return benchCase{
		name: "mtime_" + scenario.name,
		setup: func(tb testing.TB, dir string) {
			setupMtimeFiles(tb, dir, scenario)

			taskfile := "version: '3'\ntasks:\n  mtime:\n    sources:\n      - tmp/monorepo_bench/**/*\n    generates:\n      - tmp/.marker_task\n    cmds:\n      - touch tmp/.marker_task\n"
			if err := os.WriteFile(filepath.Join(dir, "Taskfile.yml"), []byte(taskfile), 0o644); err != nil {
				tb.Fatalf("write taskfile: %v", err)
			}
		},
		calls: func() []*task.Call {
			return []*task.Call{{Task: "mtime"}}
		},
		check: func(tb testing.TB, stdout, stderr string) {
			tb.Helper()
			output := stdout + stderr
			if !strings.Contains(output, "is up to date") && !strings.Contains(output, "touch tmp/.marker_task") {
				tb.Fatalf("expected mtime task to run or be up to date, got stdout=%q stderr=%q", stdout, stderr)
			}
		},
	}
}

func setupMtimeFiles(tb testing.TB, dir string, scenario mtimeBenchScenario) {
	tb.Helper()
	root := filepath.Join(dir, "tmp", "monorepo_bench")
	groups := []string{"apps", "libs", "tools", "services", "packages"}
	for i := 0; i < scenario.totalFiles; i++ {
		fileDir := root
		for level := 0; level < scenario.depth; level++ {
			if level == 0 {
				fileDir = filepath.Join(fileDir, groups[i%len(groups)])
				continue
			}
			divisor := len(groups)
			for n := 1; n < level; n++ {
				divisor *= 4
			}
			fileDir = filepath.Join(fileDir, fmt.Sprintf("level_%d_%d", level, i/divisor%4))
		}
		if err := os.MkdirAll(fileDir, 0o755); err != nil {
			tb.Fatalf("create mtime fixture directory: %v", err)
		}
		file := filepath.Join(fileDir, fmt.Sprintf("file_%d.go", i))
		if err := os.WriteFile(file, []byte("// mtime source\n"), 0o644); err != nil {
			tb.Fatalf("write mtime fixture: %v", err)
		}
	}
}

func setupMtimeMakeBench(tb testing.TB, dir string, scenario mtimeBenchScenario) string {
	tb.Helper()
	setupMtimeFiles(tb, dir, scenario)
	markerPath := filepath.Join(dir, "tmp", ".marker_make")
	glob := strings.Repeat("*/", scenario.depth) + "*.go"
	makefile := fmt.Sprintf("BENCH_DIR := tmp/monorepo_bench\nALL_FILES := $(wildcard $(BENCH_DIR)/%s)\n\n.DEFAULT_GOAL := all\n.PHONY: all\nall: tmp/.marker_make\n\ntmp/.marker_make: $(ALL_FILES)\n\t@mkdir -p $(@D)\n\t@touch $@\n", glob)
	if err := os.WriteFile(filepath.Join(dir, "Makefile"), []byte(makefile), 0o644); err != nil {
		tb.Fatalf("write Makefile: %v", err)
	}
	return markerPath
}

// BenchmarkMtimeRivetFull compares Rivet and GNU Make across the larger layouts.
// Run it explicitly with -bench=BenchmarkMtimeRivetFull; ordinary BenchmarkRivet
// continues to run only the 500-file case.
func BenchmarkMtimeRivetFull(b *testing.B) {
	for _, scenario := range fullMtimeBenchScenarios {
		rivetCase := mtimeBenchCase(scenario)
		b.Run(rivetCase.name, func(b *testing.B) {
			runRivetBenchCase(b, rivetCase)
		})

		b.Run("make_mtime_"+scenario.name, func(b *testing.B) {
			b.Helper()
			dir := b.TempDir()
			markerPath := setupMtimeMakeBench(b, dir, scenario)
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				cmd := exec.Command("make")
				cmd.Dir = dir
				if err := cmd.Run(); err != nil {
					b.Fatalf("make: %v", err)
				}
			}
			b.StopTimer()
			if _, err := os.Stat(markerPath); err != nil {
				b.Fatalf("expected make marker to exist: %v", err)
			}
		})
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
			runRivetBenchCase(b, tc)
		})
	}
}

func runRivetBenchCase(b *testing.B, tc benchCase) {
	b.Helper()
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
