package main

import (
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The v42 benchmarks own their fixtures. They started as copies of the v3
// fixtures and are free to diverge as the v42 schema does. Only the task
// calls and output checks are shared with the v3 cases, as they do not depend
// on the schema.

func writeTaskfileV42(tb testing.TB, dir, content string) {
	tb.Helper()
	if err := os.WriteFile(filepath.Join(dir, "Taskfile.yml"), []byte(content), 0o644); err != nil {
		tb.Fatalf("write taskfile: %v", err)
	}
}

// writeManyTasksTaskfileV42 is the v42 counterpart of writeManyTasksTaskfile.
func writeManyTasksTaskfileV42(tb testing.TB, dir string) {
	tb.Helper()
	r := rand.New(rand.NewSource(3))
	var sb strings.Builder
	sb.WriteString("version: 42\n")
	sb.WriteString("vars:\n")
	sb.WriteString("  GLOBAL_VAR_1: \"global value one\"\n")
	sb.WriteString("  GLOBAL_VAR_2: \"global value two\"\n")
	sb.WriteString("  GLOBAL_VAR_3: \"{{.GLOBAL_VAR_1}}-templated\"\n")
	sb.WriteString("  GLOBAL_VAR_4: \"{{.GLOBAL_VAR_2}}-templated\"\n")
	sb.WriteString("tasks:\n")
	for i, name := range manyTasksNames {
		var1, var2 := "_"+name+"_VAR1", "_"+name+"_VAR2"
		fmt.Fprintf(&sb, "  %q:\n", name)
		sb.WriteString("    vars:\n")
		fmt.Fprintf(&sb, "      %s: %q\n", var1, name+" text one")
		fmt.Fprintf(&sb, "      %s: %q\n", var2, name+" text two")

		taskVars := append(append([]string{}, manyTasksGlobalVars...), var1, var2)
		sb.WriteString("    cmds:\n")
		sb.WriteString("      - 'echo \"running task: {{.TASK}}\"'\n")
		fmt.Fprintf(&sb, "      - echo \"{{.%s}}\"\n", taskVars[r.Intn(len(taskVars))])
		fmt.Fprintf(&sb, "      - echo \"{{.%s}}\"\n", taskVars[r.Intn(len(taskVars))])
		if i%2 == 0 {
			sb.WriteString("    internal: true\n")
		}
	}
	writeTaskfileV42(tb, dir, sb.String())
}

func v42BenchCases() []benchCase {
	printMessage := benchCases[0]
	printMessage.name = "print_message_v42"
	printMessage.setup = func(tb testing.TB, dir string) {
		writeTaskfileV42(tb, dir, "version: 42\ntasks:\n  default:\n    cmds:\n      - echo \"hello from rivet\"\n")
	}

	runAll := manyTasksBenchCase()
	runAll.name += "_v42"
	runAll.setup = writeManyTasksTaskfileV42

	single := manyTasksSingleCallBenchCase()
	single.name += "_v42"
	single.setup = writeManyTasksTaskfileV42

	scenario := mtimeBenchScenario{name: "500_Files_Depth_3", totalFiles: 500, depth: 3}
	mtime := mtimeBenchCase(scenario)
	mtime.name += "_v42"
	mtime.setup = func(tb testing.TB, dir string) {
		setupMtimeFiles(tb, dir, scenario)
		writeTaskfileV42(tb, dir, "version: 42\ntasks:\n  mtime:\n    sources:\n      - tmp/monorepo_bench/**/*\n    generates:\n      - tmp/.marker_task\n    cmds:\n      - touch tmp/.marker_task\n")
	}

	return []benchCase{printMessage, runAll, single, mtime}
}

// BenchmarkRivetV42 is BenchmarkRivet run against schema version 42.
func BenchmarkRivetV42(b *testing.B) {
	for _, tc := range v42BenchCases() {
		b.Run(tc.name, func(b *testing.B) {
			runRivetBenchCase(b, tc)
		})
	}
}

// BenchmarkManyTasksCompareV42 is the Rivet half of BenchmarkManyTasksCompare
// run against schema version 42.
func BenchmarkManyTasksCompareV42(b *testing.B) {
	for _, tc := range v42BenchCases()[1:3] {
		b.Run("rivet_"+tc.name, func(b *testing.B) {
			runRivetBenchCase(b, tc)
		})
	}
}
