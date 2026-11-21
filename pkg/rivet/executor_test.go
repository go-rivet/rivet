package rivet_test

import (
	"bytes"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/sebdah/goldie/v2"
	"github.com/stretchr/testify/require"

	"github.com/go-rivet/rivet/internal/filepathext"
	"github.com/go-rivet/rivet/pkg/rivet/taskfile/ast"
	"github.com/go-rivet/rivet/pkg/rlog"

	task "github.com/go-rivet/rivet/pkg/rivet"
)

type (
	// A ExecutorTestOption is a function that configures an [ExecutorTest].
	ExecutorTestOption interface {
		applyToExecutorTest(*ExecutorTest)
	}
	// A ExecutorTest is a test wrapper around a [task.Executor] to make it easy
	// to write tests for tasks. See [NewExecutorTest] for information on
	// creating and running ExecutorTests. These tests use fixture files to
	// assert whether the result of a task is correct. If Task's behavior has
	// been changed, the fixture files can be updated by running `task
	// gen:fixtures`.
	ExecutorTest struct {
		TaskTest
		task            string
		vars            map[string]any
		input           string
		executorOpts    []task.ExecutorOption
		wantSetupError  bool
		wantRunError    bool
		wantStatusError bool
	}
)

// NewExecutorTest sets up a new [task.Executor] with the given options and runs
// a task with the given [ExecutorTestOption]s. The output of the task is
// written to a set of fixture files depending on the configuration of the test.
func NewExecutorTest(t *testing.T, opts ...ExecutorTestOption) {
	t.Helper()
	tt := &ExecutorTest{
		task: "default",
		vars: map[string]any{},
		TaskTest: TaskTest{
			fixtureTemplateData: map[string]any{},
		},
	}
	// Apply the functional options
	for _, opt := range opts {
		opt.applyToExecutorTest(tt)
	}
	tt.run(t)
}

// Functional options

// WithInput tells the test to create a reader with the given input. This can be
// used to simulate user input when a task requires it.
func WithInput(input string) ExecutorTestOption {
	return &inputTestOption{input}
}

type inputTestOption struct {
	input string
}

func (opt *inputTestOption) applyToExecutorTest(t *ExecutorTest) {
	t.input = opt.input
}

// WithRunError tells the test to expect an error during the run phase of the
// task execution. A fixture will be created with the output of any errors.
func WithRunError() ExecutorTestOption {
	return &runErrorTestOption{}
}

type runErrorTestOption struct{}

func (opt *runErrorTestOption) applyToExecutorTest(t *ExecutorTest) {
	t.wantRunError = true
}

// WithStatusError tells the test to make an additional call to
// [task.Executor.Status] after the task has been run. A fixture will be created
// with the output of any errors.
func WithStatusError() ExecutorTestOption {
	return &statusErrorTestOption{}
}

type statusErrorTestOption struct{}

func (opt *statusErrorTestOption) applyToExecutorTest(t *ExecutorTest) {
	t.wantStatusError = true
}

// Helpers

// writeFixtureErrRun is a wrapper for writing the output of an error during the
// run phase of the task to a fixture file.
func (tt *ExecutorTest) writeFixtureErrRun(
	t *testing.T,
	g *goldie.Goldie,
	err error,
) {
	t.Helper()
	tt.writeFixture(t, g, "err-run", []byte(err.Error()))
}

// writeFixtureStatus is a wrapper for writing the output of an error when
// making an additional call to [task.Executor.Status] to a fixture file.
func (tt *ExecutorTest) writeFixtureStatus(
	t *testing.T,
	g *goldie.Goldie,
	status string,
) {
	t.Helper()
	tt.writeFixture(t, g, "err-status", []byte(status))
}

// run is the main function for running the test. It sets up the task executor,
// runs the task, and writes the output to a fixture file.
func (tt *ExecutorTest) run(t *testing.T) {
	t.Helper()
	f := func(t *testing.T) {
		t.Helper()
		var buffer SyncBuffer

		// Setup logging.
		levelVar := new(slog.LevelVar)
		levelVar.Set(slog.LevelInfo)
		logOpts := &slog.HandlerOptions{Level: levelVar}
		logHandler := rlog.NewCliHandler(&buffer, &buffer, false, logOpts)
		baseCtx := t.Context()
		ctx := rlog.WithContext(baseCtx, logHandler)

		opts := []task.ExecutorOption{
			task.WithLevelVar(levelVar),
			task.WithStdout(&buffer),
			task.WithStderr(&buffer),
		}
		opts = append(
			opts,
			tt.executorOpts...,
		)

		// If the test has input, create a reader for it and add it to the
		// executor options
		if tt.input != "" {
			var reader bytes.Buffer
			reader.WriteString(tt.input)
			opts = append(opts, task.WithStdin(&reader))
		}

		// Set up the task executor
		e := task.NewExecutor(opts...)

		// Create a golden fixture file for the output
		g := goldie.New(t,
			goldie.WithFixtureDir(filepath.Join(e.Dir, "testdata")),
			goldie.WithEqualFn(NormalizedEqual),
		)

		// Call setup and check for errors
		if err := e.Setup(ctx); tt.wantSetupError {
			require.Error(t, err)
			tt.writeFixtureErrSetup(t, g, err)
			tt.writeFixtureBuffer(t, g, buffer.buf)
			return
		} else {
			require.NoError(t, err)
		}

		// Create the task call
		vars := ast.NewVars()
		for key, value := range tt.vars {
			vars.Set(key, ast.Var{Value: value})
		}
		call := &task.Call{
			Task: tt.task,
			Vars: vars,
		}

		// Run the task and check for errors
		//	ctx := t.Context()
		if err := e.Run(ctx, call); tt.wantRunError {
			require.Error(t, err)
			tt.writeFixtureErrRun(t, g, err)
			tt.writeFixtureBuffer(t, g, buffer.buf)
			return
		} else {
			require.NoError(t, err)
		}

		// If the status flag is set, run the status check
		if tt.wantStatusError {
			if err := e.Status(ctx, call); err != nil {
				tt.writeFixtureStatus(t, g, err.Error())
			}
		}

		tt.writeFixtureBuffer(t, g, buffer.buf)
	}

	// Run the test (with a name if it has one)
	if tt.name != "" {
		t.Run(tt.name, f)
	} else {
		f(t)
	}
}

func TestConcurrency(t *testing.T) {
	t.Parallel()
	NewExecutorTest(t,
		WithExecutorOptions(
			task.WithDir("testdata/concurrency"),
			task.WithConcurrency(1),
		),
		WithPostProcessFn(PPSortedLines),
	)
}

// TODO: mock fs
func TestStatus(t *testing.T) {
	t.Parallel()

	const dir = "testdata/status"

	files := []string{
		"foo.txt",
		"bar.txt",
		"baz.txt",
	}

	for _, f := range files {
		path := filepathext.SmartJoin(dir, f)
		_ = os.Remove(path)
		if _, err := os.Stat(path); err == nil {
			t.Errorf("File should not exist: %v", err)
		}
	}

	// gen-foo creates foo.txt, and will always fail it's status check.
	NewExecutorTest(t,
		WithName("run gen-foo 1 silent"),
		WithExecutorOptions(
			task.WithDir(dir),
			task.WithSilent(true),
		),
		WithTask("gen-foo"),
	)
	// gen-foo creates bar.txt, and will pass its status-check the 3. time it
	// is run. It creates bar.txt, but also lists it as its source. So, the checksum
	// for the file won't match before after the second run as we the file
	// only exists after the first run.
	NewExecutorTest(t,
		WithName("run gen-bar 1 silent"),
		WithExecutorOptions(
			task.WithDir(dir),
			task.WithSilent(true),
		),
		WithTask("gen-bar"),
	)
	// gen-silent-baz is marked as being silent, and should only produce output
	// if e.Verbose is set to true.
	NewExecutorTest(t,
		WithName("run gen-baz silent"),
		WithExecutorOptions(
			task.WithDir(dir),
			task.WithSilent(true),
		),
		WithTask("gen-silent-baz"),
	)

	// for _, f := range files {
	// 	if _, err := os.Stat(filepathext.SmartJoin(dir, f)); err != nil {
	// 		t.Errorf("File should exist: %v", err)
	// 	}
	// }

	// Run gen-bar a second time to produce a checksum file that matches bar.txt
	NewExecutorTest(t,
		WithName("run gen-bar 2 silent"),
		WithExecutorOptions(
			task.WithDir(dir),
			task.WithSilent(true),
		),
		WithTask("gen-bar"),
	)
	// Run gen-bar a third time, to make sure we've triggered the status check.
	NewExecutorTest(t,
		WithName("run gen-bar 3 silent"),
		WithExecutorOptions(
			task.WithDir(dir),
			task.WithSilent(true),
		),
		WithTask("gen-bar"),
	)

	// Now, let's remove source file, and run the task again to to prepare
	// for the next test.
	// err := os.Remove(filepathext.SmartJoin(dir, "bar.txt"))
	// require.NoError(t, err)
	NewExecutorTest(t,
		WithName("run gen-bar 4 silent"),
		WithExecutorOptions(
			task.WithDir(dir),
			task.WithSilent(true),
		),
		WithTask("gen-bar"),
	)
	// all: not up-to-date
	NewExecutorTest(t,
		WithName("run gen-foo 2"),
		WithExecutorOptions(
			task.WithDir(dir),
		),
		WithTask("gen-foo"),
	)
	// status: not up-to-date
	NewExecutorTest(t,
		WithName("run gen-foo 3"),
		WithExecutorOptions(
			task.WithDir(dir),
		),
		WithTask("gen-foo"),
	)
	// sources: not up-to-date
	NewExecutorTest(t,
		WithName("run gen-bar 5"),
		WithExecutorOptions(
			task.WithDir(dir),
			task.WithForce(true),
		),
		WithTask("gen-bar"),
	)
	// // all: up-to-date
	// NewExecutorTest(t,
	// 	WithName("run gen-bar 6"),
	// 	WithExecutorOptions(
	// 		task.WithDir(dir),
	// 	),
	// 	WithTask("gen-bar"),
	// )
}

// func TestPromptInSummary(t *testing.T) {
// 	t.Skip()
// 	t.Parallel()

// 	tests := []struct {
// 		name      string
// 		input     string
// 		wantError bool
// 	}{
// 		{"test short approval", "y\n", false},
// 		{"test long approval", "yes\n", false},
// 		{"test uppercase approval", "Y\n", false},
// 		{"test stops task", "n\n", true},
// 		{"test junk value stops task", "foobar\n", true},
// 		{"test Enter stops task", "\n", true},
// 	}

// 	for _, test := range tests {
// 		t.Run(test.name, func(t *testing.T) {
// 			t.Parallel()

// 			opts := []ExecutorTestOption{
// 				WithName(test.name),
// 				WithExecutorOptions(
// 					task.WithDir("testdata/prompt"),
// 					task.WithAssumeTerm(true),
// 				),
// 				WithTask("foo"),
// 				WithInput(test.input),
// 			}
// 			if test.wantError {
// 				opts = append(opts, WithRunError())
// 			}
// 			NewExecutorTest(t, opts...)
// 		})
// 	}
// }

// func TestPromptWithIndirectTask(t *testing.T) {
// 	t.Skip()
// 	t.Parallel()

// 	NewExecutorTest(t,
// 		WithExecutorOptions(
// 			task.WithDir("testdata/prompt"),
// 			task.WithAssumeTerm(true),
// 		),
// 		WithTask("bar"),
// 		WithInput("y\n"),
// 	)
// }
