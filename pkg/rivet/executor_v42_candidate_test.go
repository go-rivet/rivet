package rivet_test

import (
	"cmp"
	"testing"

	"github.com/go-rivet/rivet/pkg/rivet/taskfile/ast"

	task "github.com/go-rivet/rivet/pkg/rivet"
)

func TestEmptyTask(t *testing.T) {
	t.Parallel()
	NewExecutorTest(t,
		WithExecutorOptions(
			task.WithDir("testdata/empty_task"),
		),
	)
}

func TestEmptyTaskfile(t *testing.T) {
	t.Parallel()
	NewExecutorTest(t,
		WithExecutorOptions(
			task.WithDir("testdata/empty_taskfile"),
		),
		WithSetupError(),
		WithFixtureTemplating(),
	)
}

func TestRequires(t *testing.T) {
	t.Parallel()
	NewExecutorTest(t,
		WithName("required var missing"),
		WithExecutorOptions(
			task.WithDir("testdata/requires"),
		),
		WithTask("missing-var"),
		WithRunError(),
	)
	NewExecutorTest(t,
		WithName("required var ok"),
		WithExecutorOptions(
			task.WithDir("testdata/requires"),
		),
		WithTask("missing-var"),
		WithVar("FOO", "bar"),
	)
	NewExecutorTest(t,
		WithName("fails validation"),
		WithExecutorOptions(
			task.WithDir("testdata/requires"),
		),
		WithTask("validation-var"),
		WithVar("ENV", "dev"),
		WithVar("FOO", "bar"),
		WithRunError(),
	)
	NewExecutorTest(t,
		WithName("passes validation"),
		WithExecutorOptions(
			task.WithDir("testdata/requires"),
		),
		WithTask("validation-var"),
		WithVar("FOO", "one"),
		WithVar("ENV", "dev"),
	)
	NewExecutorTest(t,
		WithName("required var missing + fails validation"),
		WithExecutorOptions(
			task.WithDir("testdata/requires"),
		),
		WithTask("validation-var"),
		WithRunError(),
	)
	NewExecutorTest(t,
		WithName("required var missing + fails validation"),
		WithExecutorOptions(
			task.WithDir("testdata/requires"),
		),
		WithTask("validation-var-dynamic"),
		WithVar("FOO", "one"),
		WithVar("ENV", "dev"),
	)
	NewExecutorTest(t,
		WithName("require before compile"),
		WithExecutorOptions(
			task.WithDir("testdata/requires"),
		),
		WithTask("require-before-compile"),
		WithRunError(),
	)
	NewExecutorTest(t,
		WithName("var defined in task"),
		WithExecutorOptions(
			task.WithDir("testdata/requires"),
		),
		WithTask("var-defined-in-task"),
	)
	NewExecutorTest(t,
		WithName("enum ref - passes validation"),
		WithExecutorOptions(
			task.WithDir("testdata/requires"),
		),
		WithTask("validation-var-ref"),
		WithVar("ENV", "dev"),
	)
	NewExecutorTest(t,
		WithName("enum ref - fails validation"),
		WithExecutorOptions(
			task.WithDir("testdata/requires"),
		),
		WithTask("validation-var-ref"),
		WithVar("ENV", "invalid"),
		WithRunError(),
	)
	NewExecutorTest(t,
		WithName("enum ref - ref to non-list"),
		WithExecutorOptions(
			task.WithDir("testdata/requires"),
		),
		WithTask("validation-var-ref-invalid"),
		WithVar("VALUE", "test"),
		WithRunError(),
	)
	NewExecutorTest(t,
		WithName("enum ref - ref to nonexistent var"),
		WithExecutorOptions(
			task.WithDir("testdata/requires"),
		),
		WithTask("validation-var-ref-nonexistent"),
		WithVar("ENV", "dev"),
		WithRunError(),
	)
}

func TestDeps(t *testing.T) {
	t.Parallel()
	NewExecutorTest(t,
		WithExecutorOptions(
			task.WithDir("testdata/deps"),
			task.WithSilent(true),
		),
		WithPostProcessFn(PPSortedLines),
	)
}

func TestPrecondition(t *testing.T) {
	t.Parallel()
	const dir = "testdata/precondition"
	NewExecutorTest(t,
		WithName("a precondition has been met"),
		WithExecutorOptions(
			task.WithDir(dir),
		),
		WithTask("foo"),
	)
	NewExecutorTest(t,
		WithName("a precondition was not met"),
		WithExecutorOptions(
			task.WithDir(dir),
		),
		WithTask("impossible"),
		WithRunError(),
	)
	NewExecutorTest(t,
		WithName("precondition in dependency fails the task"),
		WithExecutorOptions(
			task.WithDir(dir),
		),
		WithTask("depends_on_impossible"),
		WithRunError(),
	)
	NewExecutorTest(t,
		WithName("precondition in cmd fails the task"),
		WithExecutorOptions(
			task.WithDir(dir),
		),
		WithTask("executes_failing_task_as_cmd"),
		WithRunError(),
	)
}

func TestAlias(t *testing.T) {
	t.Parallel()

	NewExecutorTest(t,
		WithName("alias"),
		WithExecutorOptions(
			task.WithDir("testdata/alias"),
		),
		WithTask("f"),
	)

	NewExecutorTest(t,
		WithName("duplicate alias"),
		WithExecutorOptions(
			task.WithDir("testdata/alias"),
		),
		WithTask("x"),
		WithRunError(),
	)

	NewExecutorTest(t,
		WithName("alias summary"),
		WithExecutorOptions(
			task.WithDir("testdata/alias"),
			task.WithSummary(true),
		),
		WithTask("f"),
	)
}

func TestSummaryWithVarsAndRequires(t *testing.T) {
	t.Parallel()

	// Test basic case from prompt.md - vars and requires
	NewExecutorTest(t,
		WithName("vars-and-requires"),
		WithExecutorOptions(
			task.WithDir("testdata/summary-vars-requires"),
			task.WithSummary(true),
		),
		WithTask("mytask"),
	)

	// Test with shell variables
	NewExecutorTest(t,
		WithName("shell-vars"),
		WithExecutorOptions(
			task.WithDir("testdata/summary-vars-requires"),
			task.WithSummary(true),
		),
		WithTask("with-sh-var"),
	)
}

func TestLabel(t *testing.T) {
	t.Parallel()

	NewExecutorTest(t,
		WithName("up to date"),
		WithExecutorOptions(
			task.WithDir("testdata/label_uptodate"),
		),
		WithTask("foo"),
	)

	NewExecutorTest(t,
		WithName("summary"),
		WithExecutorOptions(
			task.WithDir("testdata/label_summary"),
			task.WithSummary(true),
		),
		WithTask("foo"),
	)

	NewExecutorTest(t,
		WithName("status"),
		WithExecutorOptions(
			task.WithDir("testdata/label_status"),
		),
		WithTask("foo"),
		WithStatusError(),
	)

	NewExecutorTest(t,
		WithName("var"),
		WithExecutorOptions(
			task.WithDir("testdata/label_var"),
		),
		WithTask("foo"),
	)

	NewExecutorTest(t,
		WithName("label in summary"),
		WithExecutorOptions(
			task.WithDir("testdata/label_summary"),
		),
		WithTask("foo"),
	)

	NewExecutorTest(t,
		WithName("label in error"),
		WithExecutorOptions(
			task.WithDir("testdata/label_error"),
		),
		WithTask("foo"),
		WithRunError(),
	)
}

func TestPrefix(t *testing.T) {
	t.Parallel()

	NewExecutorTest(t,
		WithName("up to date"),
		WithExecutorOptions(
			task.WithDir("testdata/prefix_uptodate"),
			task.WithOutputStyle(ast.Output{Name: "prefixed"}),
		),
		WithTask("foo"),
	)

	NewExecutorTest(t,
		WithName("up to dat with no output style"),
		WithExecutorOptions(
			task.WithDir("testdata/prefix_uptodate"),
		),
		WithTask("foo"),
	)
}

func TestPromptAssumeYes(t *testing.T) {
	t.Parallel()

	NewExecutorTest(t,
		WithName("--yes flag should skip prompt"),
		WithExecutorOptions(
			task.WithDir("testdata/prompt"),
			task.WithAssumeTerm(true),
			task.WithAssumeYes(true),
		),
		WithTask("foo"),
		WithInput("\n"),
	)

	// NewExecutorTest(t,
	// 	WithName("task should raise errors.TaskCancelledError"),
	// 	WithExecutorOptions(
	// 		task.WithDir("testdata/prompt"),
	// 		task.WithAssumeTerm(true),
	// 	),
	// 	WithTask("foo"),
	// 	WithInput("\n"),
	// 	WithRunError(),
	// )
}

func TestReference(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		call string
	}{
		{
			name: "reference in command",
			call: "ref-cmd",
		},
		{
			name: "reference in dependency",
			call: "ref-dep",
		},
		{
			name: "reference using templating resolver",
			call: "ref-resolver",
		},
		{
			name: "reference using templating resolver and dynamic var",
			call: "ref-resolver-sh",
		},
	}

	for _, test := range tests {
		NewExecutorTest(t,
			WithName(test.name),
			WithExecutorOptions(
				task.WithDir("testdata/var_references"),
				task.WithSilent(true),
				task.WithForce(true),
			),
			WithTask(cmp.Or(test.call, "default")),
		)
	}
}

func TestFuzzyModel(t *testing.T) {
	t.Parallel()

	NewExecutorTest(t,
		WithName("fuzzy"),
		WithExecutorOptions(
			task.WithDir("testdata/fuzzy"),
		),
		WithTask("instal"),
		WithRunError(),
	)

	NewExecutorTest(t,
		WithName("not-fuzzy"),
		WithExecutorOptions(
			task.WithDir("testdata/fuzzy"),
		),
		WithTask("install"),
	)

	NewExecutorTest(t,
		WithName("intern"),
		WithExecutorOptions(
			task.WithDir("testdata/fuzzy"),
		),
		WithTask("intern"),
		WithRunError(),
	)
}

func TestIncludeChecksum(t *testing.T) {
	t.Parallel()

	NewExecutorTest(t,
		WithName("correct"),
		WithExecutorOptions(
			task.WithDir("testdata/includes_checksum/correct"),
		),
	)

	NewExecutorTest(t,
		WithName("incorrect"),
		WithExecutorOptions(
			task.WithDir("testdata/includes_checksum/incorrect"),
		),
		WithSetupError(),
		WithFixtureTemplating(),
	)
}

func TestGeneratesArray(t *testing.T) {
	t.Parallel()

	NewExecutorTest(t,
		WithName("generates-array"),
		WithExecutorOptions(
			task.WithDir("testdata/generates-array"),
			task.WithSilent(true),
			task.WithForce(true),
		),
		WithTask("default"),
	)
}

func TestFailfast(t *testing.T) {
	t.Parallel()

	t.Run("Default", func(t *testing.T) {
		t.Parallel()

		NewExecutorTest(t,
			WithName("default"),
			WithExecutorOptions(
				task.WithDir("testdata/failfast/default"),
				task.WithSilent(true),
			),
			WithPostProcessFn(PPSortedLines),
			WithRunError(),
		)
	})

	t.Run("Option", func(t *testing.T) {
		t.Parallel()

		NewExecutorTest(t,
			WithVar("PUSH", "5"),
			WithName("default"),
			WithExecutorOptions(
				task.WithDir("testdata/failfast/default"),
				task.WithSilent(true),
				task.WithFailfast(true),
			),
			WithPostProcessFn(PPSortedLines),
			WithRunError(),
		)
	})

	t.Run("Task", func(t *testing.T) {
		t.Parallel()

		NewExecutorTest(t,
			WithName("task"),
			WithExecutorOptions(
				task.WithDir("testdata/failfast/task"),
				task.WithSilent(true),
			),
			WithPostProcessFn(PPSortedLines),
			WithRunError(),
		)
	})
}
