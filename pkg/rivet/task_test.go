package rivet_test

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"maps"
	rand "math/rand/v2"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Masterminds/semver/v3"
	"github.com/sebdah/goldie/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/go-rivet/rivet/internal/filepathext"
	"github.com/go-rivet/rivet/pkg/rivet/errors"
	"github.com/go-rivet/rivet/pkg/rlog"

	task "github.com/go-rivet/rivet/pkg/rivet"
)

func init() {
	_ = os.Setenv("NO_COLOR", "1")
}

type (
	TestOption interface {
		ExecutorTestOption
		FormatterTestOption
	}
	TaskTest struct {
		name                     string
		postProcessFns           []PostProcessFn
		fixtureTemplateData      map[string]any
		fixtureTemplatingEnabled bool
	}
)

func SetupTestLogger(t *testing.T, verbose, silent bool) (context.Context, *SyncBuffer, *slog.LevelVar) {
	t.Helper()

	var buffer SyncBuffer
	levelVar := new(slog.LevelVar)
	levelVar.Set(slog.LevelInfo)
	logOpts := &slog.HandlerOptions{Level: levelVar}
	if silent {
		levelVar.Set(slog.LevelError)
	} else if verbose {
		levelVar.Set(slog.LevelDebug)
	}
	logHandler := rlog.NewCliHandler(&buffer, &buffer, false, logOpts)
	ctx := rlog.WithContext(t.Context(), logHandler)
	return ctx, &buffer, levelVar
}

// goldenFileName makes the file path for fixture files safe for all well-known
// operating systems. Windows in particular has a lot of restrictions the
// characters that can be used in file paths.
func goldenFileName(t *testing.T) string {
	t.Helper()
	name := t.Name()
	for _, c := range []string{` `, `<`, `>`, `:`, `"`, `/`, `\`, `|`, `?`, `*`} {
		name = strings.ReplaceAll(name, c, "-")
	}
	return name
}

// writeFixture writes a fixture file for the test. The fixture file is created
// using the [goldie.Goldie] package. The fixture file is created with the
// output of the task, after any post-process functions have been applied.
func (tt *TaskTest) writeFixture(
	t *testing.T,
	g *goldie.Goldie,
	goldenFileSuffix string,
	b []byte,
) {
	t.Helper()
	// Apply any post-process functions
	for _, fn := range tt.postProcessFns {
		b = fn(t, b)
	}
	// Write the fixture file
	goldenFileName := goldenFileName(t)
	if goldenFileSuffix != "" {
		goldenFileName += "-" + goldenFileSuffix
	}
	// Create a set of data to be made available to every test fixture
	wd, err := os.Getwd()
	require.NoError(t, err)
	if tt.fixtureTemplatingEnabled {
		fixtureTemplateData := map[string]any{
			"TEST_NAME": t.Name(),
			"TEST_DIR":  filepath.ToSlash(wd),
		}
		// If the test has additional template data, copy it into the map
		if tt.fixtureTemplateData != nil {
			maps.Copy(fixtureTemplateData, tt.fixtureTemplateData)
		}
		// Normalize output before comparison (CRLF→LF, backslash→forward slash)
		g.AssertWithTemplate(t, goldenFileName, fixtureTemplateData, normalizeOutput(b))
	} else {
		g.Assert(t, goldenFileName, b)
	}
}

// writeFixtureBuffer is a wrapper for writing the main output of the task to a
// fixture file.
func (tt *TaskTest) writeFixtureBuffer(
	t *testing.T,
	g *goldie.Goldie,
	buffer bytes.Buffer,
) {
	t.Helper()
	tt.writeFixture(t, g, "", buffer.Bytes())
}

// writeFixtureErrSetup is a wrapper for writing the output of an error during
// the setup phase of the task to a fixture file.
func (tt *TaskTest) writeFixtureErrSetup(
	t *testing.T,
	g *goldie.Goldie,
	err error,
) {
	t.Helper()
	tt.writeFixture(t, g, "err-setup", []byte(err.Error()))
}

// Functional options

// WithName gives the test fixture output a name. This should be used when
// running multiple tests in a single test function.
func WithName(name string) TestOption {
	return &nameTestOption{name: name}
}

type nameTestOption struct {
	name string
}

func (opt *nameTestOption) applyToExecutorTest(t *ExecutorTest) {
	t.name = opt.name
}

func (opt *nameTestOption) applyToFormatterTest(t *FormatterTest) {
	t.name = opt.name
}

// WithTask sets the name of the task to run. This should be used when the task
// to run is not the default task.
func WithTask(task string) TestOption {
	return &taskTestOption{task: task}
}

type taskTestOption struct {
	task string
}

func (opt *taskTestOption) applyToExecutorTest(t *ExecutorTest) {
	t.task = opt.task
}

func (opt *taskTestOption) applyToFormatterTest(t *FormatterTest) {
	t.task = opt.task
}

// WithVar sets a variable to be passed to the task. This can be called multiple
// times to set more than one variable.
func WithVar(key string, value any) TestOption {
	return &varTestOption{key: key, value: value}
}

type varTestOption struct {
	key   string
	value any
}

func (opt *varTestOption) applyToExecutorTest(t *ExecutorTest) {
	t.vars[opt.key] = opt.value
}

func (opt *varTestOption) applyToFormatterTest(t *FormatterTest) {
	t.vars[opt.key] = opt.value
}

// WithExecutorOptions sets the [task.ExecutorOption]s to be used when creating
// a [task.Executor].
func WithExecutorOptions(executorOpts ...task.ExecutorOption) TestOption {
	return &executorOptionsTestOption{executorOpts: executorOpts}
}

type executorOptionsTestOption struct {
	executorOpts []task.ExecutorOption
}

func (opt *executorOptionsTestOption) applyToExecutorTest(t *ExecutorTest) {
	t.executorOpts = slices.Concat(t.executorOpts, opt.executorOpts)
}

func (opt *executorOptionsTestOption) applyToFormatterTest(t *FormatterTest) {
	t.executorOpts = slices.Concat(t.executorOpts, opt.executorOpts)
}

// WithPostProcessFn adds a [PostProcessFn] function to the test. Post-process
// functions are run on the output of the task before a fixture is created. This
// can be used to remove absolute paths, sort lines, etc. This can be called
// multiple times to add more than one post-process function.
func WithPostProcessFn(fn PostProcessFn) TestOption {
	return &postProcessFnTestOption{fn: fn}
}

type postProcessFnTestOption struct {
	fn PostProcessFn
}

func (opt *postProcessFnTestOption) applyToExecutorTest(t *ExecutorTest) {
	t.postProcessFns = append(t.postProcessFns, opt.fn)
}

func (opt *postProcessFnTestOption) applyToFormatterTest(t *FormatterTest) {
	t.postProcessFns = append(t.postProcessFns, opt.fn)
}

// WithSetupError sets the test to expect an error during the setup phase of the
// task execution. A fixture will be created with the output of any errors.
func WithSetupError() TestOption {
	return &setupErrorTestOption{}
}

type setupErrorTestOption struct{}

func (opt *setupErrorTestOption) applyToExecutorTest(t *ExecutorTest) {
	t.wantSetupError = true
}

func (opt *setupErrorTestOption) applyToFormatterTest(t *FormatterTest) {
	t.wantSetupError = true
}

// WithFixtureTemplating enables templating for the golden fixture files with
// the default set of data. This is useful if the golden file is dynamic in some
// way (e.g. contains user-specific directories). To add more data, see
// WithFixtureTemplateData.
func WithFixtureTemplating() TestOption {
	return &fixtureTemplatingTestOption{}
}

type fixtureTemplatingTestOption struct{}

func (opt *fixtureTemplatingTestOption) applyToExecutorTest(t *ExecutorTest) {
	t.fixtureTemplatingEnabled = true
}

func (opt *fixtureTemplatingTestOption) applyToFormatterTest(t *FormatterTest) {
	t.fixtureTemplatingEnabled = true
}

// WithFixtureTemplateData adds data to the golden fixture file templates. Keys
// given here will override any existing values. This option will also enable
// global templating, so you do not need to call WithFixtureTemplating as well.
func WithFixtureTemplateData(key string, value any) TestOption {
	return &fixtureTemplateDataTestOption{key, value}
}

type fixtureTemplateDataTestOption struct {
	k string
	v any
}

func (opt *fixtureTemplateDataTestOption) applyToExecutorTest(t *ExecutorTest) {
	t.fixtureTemplatingEnabled = true
	t.fixtureTemplateData[opt.k] = opt.v
}

func (opt *fixtureTemplateDataTestOption) applyToFormatterTest(t *FormatterTest) {
	t.fixtureTemplatingEnabled = true
	t.fixtureTemplateData[opt.k] = opt.v
}

// Post-processing

// A PostProcessFn is a function that can be applied to the output of a test
// fixture before the file is written.
type PostProcessFn func(*testing.T, []byte) []byte

// PPSortedLines sorts the lines of the output of the task. This is useful when
// the order of the output is not important, but the output is expected to be
// the same each time the task is run (e.g. when running tasks in parallel).
func PPSortedLines(t *testing.T, b []byte) []byte {
	t.Helper()
	lines := strings.Split(strings.TrimSpace(string(b)), "\n")
	sort.Strings(lines)
	return []byte(strings.Join(lines, "\n") + "\n")
}

// normalizeOutput normalizes cross-platform differences for byte slice comparison:
// - Converts CRLF and CR to LF (line endings)
// - Converts backslashes to forward slashes (Windows paths)
// - Handles escaped backslashes in JSON (\\) by converting to single forward slash
func normalizeOutput(b []byte) []byte {
	b = bytes.ReplaceAll(b, []byte("\r\n"), []byte("\n"))
	b = bytes.ReplaceAll(b, []byte("\r"), []byte("\n"))
	// First replace escaped backslashes (common in JSON), then single backslashes
	b = bytes.ReplaceAll(b, []byte("\\\\"), []byte("/"))
	b = bytes.ReplaceAll(b, []byte("\\"), []byte("/"))
	return b
}

// normalizePathSeparators converts backslashes to forward slashes for cross-platform path comparison.
func normalizePathSeparators(s string) string {
	return strings.ReplaceAll(s, "\\", "/")
}

// NormalizedEqual compares two byte slices after normalizing output.
// This is used as a custom goldie.EqualFn for cross-platform golden file tests.
func NormalizedEqual(actual, expected []byte) bool {
	return bytes.Equal(normalizeOutput(actual), normalizeOutput(expected))
}

func TestNormalizeOutput(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		input    []byte
		expected []byte
	}{
		{"CRLF to LF", []byte("line1\r\nline2\r\n"), []byte("line1\nline2\n")},
		{"CR to LF", []byte("line1\rline2\r"), []byte("line1\nline2\n")},
		{"Windows path", []byte(`D:\a\task\task`), []byte(`D:/a/task/task`)},
		{"JSON escaped backslash", []byte(`{"path":"D:\\a\\task"}`), []byte(`{"path":"D:/a/task"}`)},
		{"Mixed", []byte("D:\\a\\task\r\n"), []byte("D:/a/task\n")},
		{"Unix path unchanged", []byte("/home/user/task\n"), []byte("/home/user/task\n")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := normalizeOutput(tt.input)
			assert.Equal(t, tt.expected, got)
		})
	}
}

func TestNormalizePathSeparators(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"Windows path", `D:\a\task\task`, `D:/a/task/task`},
		{"Unix path unchanged", `/home/user/task`, `/home/user/task`},
		{"Mixed separators", `C:\Users/name\file`, `C:/Users/name/file`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := normalizePathSeparators(tt.input)
			assert.Equal(t, tt.expected, got)
		})
	}
}

// SyncBuffer is a threadsafe buffer for testing.
// Some times replace stdout/stderr with a buffer to capture output.
// stdout and stderr are threadsafe, but a regular bytes.Buffer is not.
// Using this instead helps prevents race conditions with output.
type SyncBuffer struct {
	buf bytes.Buffer
	mu  sync.Mutex
}

func (sb *SyncBuffer) Write(p []byte) (n int, err error) {
	sb.mu.Lock()
	defer sb.mu.Unlock()
	return sb.buf.Write(p)
}

// fileContentTest provides a basic reusable test-case for running a Taskfile
// and inspect generated files.
type fileContentTest struct {
	Dir        string
	Entrypoint string
	Target     string
	TrimSpace  bool
	Files      map[string]string
}

func (fct fileContentTest) name(file string) string {
	return fmt.Sprintf("target=%q,file=%q", fct.Target, file)
}

func (fct fileContentTest) Run(t *testing.T) {
	t.Helper()

	for f := range fct.Files {
		_ = os.Remove(filepathext.SmartJoin(fct.Dir, f))
	}

	e := task.NewExecutor(
		task.WithDir(fct.Dir),
		task.WithTempDir(task.TempDir{
			Remote:      filepathext.SmartJoin(fct.Dir, ".task"),
			Fingerprint: filepathext.SmartJoin(fct.Dir, ".task"),
		}),
		task.WithEntrypoint(fct.Entrypoint),
		task.WithStdout(io.Discard),
		task.WithStderr(io.Discard),
	)

	ctx, _, _ := SetupTestLogger(t, false, false)

	require.NoError(t, e.Setup(ctx), "e.Setup(ctx)")
	require.NoError(t, e.Run(ctx, &task.Call{Task: fct.Target}), "e.Run(target)")
	for name, expectContent := range fct.Files {
		t.Run(fct.name(name), func(t *testing.T) {
			path := filepathext.SmartJoin(e.Dir, name)
			b, err := os.ReadFile(path)
			require.NoError(t, err, "Error reading file")
			s := string(b)
			if fct.TrimSpace {
				s = strings.TrimSpace(s)
			}
			assert.Equal(t, expectContent, s, "unexpected file content in %s", path)
		})
	}
}

func TestGenerates(t *testing.T) {
	t.Parallel()

	const dir = "testdata/generates"

	const (
		srcTask        = "sub/src.txt"
		relTask        = "rel.txt"
		absTask        = "abs.txt"
		fileWithSpaces = "my text file.txt"
	)

	srcFile := filepathext.SmartJoin(dir, srcTask)

	for _, task := range []string{srcTask, relTask, absTask, fileWithSpaces} {
		path := filepathext.SmartJoin(dir, task)
		_ = os.Remove(path)
		if _, err := os.Stat(path); err == nil {
			t.Errorf("File should not exist: %v", err)
		}
	}

	ctx, buffer, levelVar := SetupTestLogger(t, false, false)
	e := task.NewExecutor(
		task.WithLevelVar(levelVar),
		task.WithDir(dir),
		task.WithStdout(buffer),
		task.WithStderr(buffer),
	)
	require.NoError(t, e.Setup(ctx))

	for _, theTask := range []string{relTask, absTask, fileWithSpaces} {
		destFile := filepathext.SmartJoin(dir, theTask)
		upToDate := fmt.Sprintf("task: Task \"%s\" is up to date\n", srcTask) +
			fmt.Sprintf("task: Task \"%s\" is up to date\n", theTask)

		// Run task for the first time.
		require.NoError(t, e.Run(ctx, &task.Call{Task: theTask}))

		if _, err := os.Stat(srcFile); err != nil {
			t.Errorf("File should exist: %v", err)
		}
		if _, err := os.Stat(destFile); err != nil {
			t.Errorf("File should exist: %v", err)
		}
		// Ensure task was not incorrectly found to be up-to-date on first run.
		if buffer.buf.String() == upToDate {
			t.Errorf("Wrong output message: %s", buffer.buf.String())
		}
		buffer.buf.Reset()

		// Re-run task to ensure it's now found to be up-to-date.
		require.NoError(t, e.Run(ctx, &task.Call{Task: theTask}))
		if buffer.buf.String() != upToDate {
			t.Errorf("Wrong output message: %s", buffer.buf.String())
		}
		buffer.buf.Reset()
	}
}

// TestStatusTimestamp is a regression test for https://github.com/go-task/task/issues/1230.
// When using method: timestamp, deleting a generated file should cause the task to re-run,
// not be skipped because the timestamp file is still present.
func TestStatusTimestamp(t *testing.T) { // nolint:paralleltest // cannot run in parallel
	const dir = "testdata/timestamp"

	generatedFile := filepathext.SmartJoin(dir, "generated.txt")
	tempDir := task.TempDir{
		Remote:      filepathext.SmartJoin(dir, ".task"),
		Fingerprint: filepathext.SmartJoin(dir, ".task"),
	}

	// Clean up any state from previous runs.
	_ = os.Remove(generatedFile)
	_ = os.RemoveAll(filepathext.SmartJoin(dir, ".task"))

	ctx, buffer, levelVar := SetupTestLogger(t, false, false)
	e := task.NewExecutor(
		task.WithLevelVar(levelVar),
		task.WithDir(dir),
		task.WithStdout(buffer),
		task.WithStderr(buffer),
		task.WithTempDir(tempDir),
	)
	require.NoError(t, e.Setup(ctx))

	// First run: task should execute and create generated.txt.
	require.NoError(t, e.Run(ctx, &task.Call{Task: "build"}))
	_, err := os.Stat(generatedFile)
	require.NoError(t, err, "generated.txt should exist after first run")
	buffer.buf.Reset()

	// Second run: task should be up to date.
	require.NoError(t, e.Run(ctx, &task.Call{Task: "build"}))
	assert.Equal(t, `task: Task "build" is up to date`+"\n", buffer.buf.String())
	buffer.buf.Reset()

	// Delete the generated file (simulate a clean), but leave the timestamp file.
	require.NoError(t, os.Remove(generatedFile))
	_, err = os.Stat(generatedFile)
	require.Error(t, err, "generated.txt should be gone")

	// Third run: task MUST re-run because generated.txt is missing.
	// This is the regression: previously the task was incorrectly skipped.
	require.NoError(t, e.Run(ctx, &task.Call{Task: "build"}))
	assert.NotContains(t, buffer.buf.String(), "is up to date", "task should re-run when generated file is missing")
	_, err = os.Stat(generatedFile)
	require.NoError(t, err, "generated.txt should be recreated after third run")
}

func TestCyclicDep(t *testing.T) {
	t.Parallel()

	const dir = "testdata/cyclic"
	ctx, _, levelVar := SetupTestLogger(t, false, false)

	e := task.NewExecutor(
		task.WithLevelVar(levelVar),
		task.WithDir(dir),
		task.WithStdout(io.Discard),
		task.WithStderr(io.Discard),
	)
	require.NoError(t, e.Setup(ctx))
	err := e.Run(ctx, &task.Call{Task: "task-1"})
	var taskCalledTooManyTimesError *errors.TaskCalledTooManyTimesError
	assert.ErrorAs(t, err, &taskCalledTooManyTimesError)
}

func TestTaskVersion(t *testing.T) {
	t.Parallel()

	tests := []struct {
		Dir     string
		Version *semver.Version
		wantErr bool
	}{
		{"testdata/version/v1", semver.MustParse("1"), true},
		{"testdata/version/v2", semver.MustParse("2"), true},
		{"testdata/version/v3", semver.MustParse("3"), false},
		{"testdata/version/v42", semver.MustParse("42"), false},
	}

	for _, test := range tests {
		t.Run(test.Dir, func(t *testing.T) {
			t.Parallel()

			ctx, _, levelVar := SetupTestLogger(t, false, false)
			e := task.NewExecutor(
				task.WithLevelVar(levelVar),
				task.WithDir(test.Dir),
				task.WithStdout(io.Discard),
				task.WithStderr(io.Discard),
				task.WithVersionCheck(true),
			)
			err := e.Setup(ctx)
			if test.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, test.Version, e.Taskfile.Version)
			assert.Equal(t, 2, e.Taskfile.Tasks.Len())
		})
	}
}

func TestExpand(t *testing.T) {
	t.Parallel()

	const dir = "testdata/expand"

	home, err := os.UserHomeDir()
	if err != nil {
		t.Errorf("Couldn't get $HOME: %v", err)
	}
	ctx, buffer, levelVar := SetupTestLogger(t, false, true)

	e := task.NewExecutor(
		task.WithLevelVar(levelVar),
		task.WithDir(dir),
		task.WithStdout(buffer),
		task.WithStderr(buffer),
	)
	require.NoError(t, e.Setup(ctx))
	require.NoError(t, e.Run(ctx, &task.Call{Task: "pwd"}))
	assert.Equal(t, home, strings.TrimSpace(buffer.buf.String()))
}

func TestIncludesRemote(t *testing.T) {
	dir := "testdata/includes_remote"
	_ = os.RemoveAll(filepath.Join(dir, ".task", "remote"))

	srv := httptest.NewServer(http.FileServer(http.Dir(dir)))
	defer srv.Close()

	tcs := []struct {
		firstRemote  string
		secondRemote string
	}{
		{
			firstRemote:  srv.URL + "/first/Taskfile.yml",
			secondRemote: srv.URL + "/first/second/Taskfile.yml",
		},
		{
			firstRemote:  srv.URL + "/first/Taskfile.yml",
			secondRemote: "./second/Taskfile.yml",
		},
		{
			firstRemote:  srv.URL + "/first/",
			secondRemote: srv.URL + "/first/second/",
		},
	}

	taskCalls := []*task.Call{
		{Task: "first:write-file"},
		{Task: "first:second:write-file"},
	}

	for i, tc := range tcs {
		t.Run(fmt.Sprint(i), func(t *testing.T) {
			t.Setenv("FIRST_REMOTE_URL", tc.firstRemote)
			t.Setenv("SECOND_REMOTE_URL", tc.secondRemote)

			ctx, buffer, levelVar := SetupTestLogger(t, false, false)

			// Extract host from server URL for trust testing
			parsedURL, err := url.Parse(srv.URL)
			require.NoError(t, err)
			trustedHost := parsedURL.Host

			executors := []struct {
				name     string
				executor *task.Executor
			}{
				{
					name: "online, always download",
					executor: task.NewExecutor(
						task.WithLevelVar(levelVar),
						task.WithDir(dir),
						task.WithStdout(buffer),
						task.WithStderr(buffer),
						task.WithTimeout(time.Minute),
						task.WithInsecure(true),
						task.WithVerbose(true),

						// Without caching
						task.WithAssumeYes(true),
						task.WithDownload(true),
					),
				},
				{
					name: "offline, use cache",
					executor: task.NewExecutor(
						task.WithLevelVar(levelVar),
						task.WithDir(dir),
						task.WithStdout(buffer),
						task.WithStderr(buffer),
						task.WithTimeout(time.Minute),
						task.WithInsecure(true),
						task.WithVerbose(true),

						// With caching
						task.WithAssumeYes(false),
						task.WithDownload(false),
						task.WithOffline(true),
					),
				},
				{
					name: "with trusted hosts, no prompts",
					executor: task.NewExecutor(
						task.WithLevelVar(levelVar),
						task.WithDir(dir),
						task.WithStdout(buffer),
						task.WithStderr(buffer),
						task.WithTimeout(time.Minute),
						task.WithInsecure(true),
						task.WithStdout(buffer),
						task.WithStderr(buffer),
						task.WithVerbose(true),

						// With trusted hosts
						task.WithTrustedHosts([]string{trustedHost}),
						task.WithDownload(true),
					),
				},
			}

			for _, e := range executors {
				t.Run(e.name, func(t *testing.T) {
					require.NoError(t, e.executor.Setup(ctx))

					for k, taskCall := range taskCalls {
						t.Run(taskCall.Task, func(t *testing.T) {
							expectedContent := fmt.Sprint(rand.Int64()) //nolint:gosec
							t.Setenv("CONTENT", expectedContent)

							outputFile := fmt.Sprintf("%d.%d.txt", i, k)
							t.Setenv("OUTPUT_FILE", outputFile)

							path := filepath.Join(dir, outputFile)
							require.NoError(t, os.RemoveAll(path))

							require.NoError(t, e.executor.Run(t.Context(), taskCall))

							actualContent, err := os.ReadFile(path)
							require.NoError(t, err)
							assert.Equal(t, expectedContent, strings.TrimSpace(string(actualContent)))
						})
					}
				})
			}

			t.Log("\noutput:\n", buffer.buf.String())
		})
	}
}

func TestIncludesHttp(t *testing.T) {
	t.Skip("possible race in the reader")

	dir, err := filepath.Abs("testdata/includes_http")
	require.NoError(t, err)

	srv := httptest.NewServer(http.FileServer(http.Dir(dir)))
	defer srv.Close()

	t.Cleanup(func() {
		// This test fills the .task/remote directory with cache entries because the include URL
		// is different on every test due to the dynamic nature of the TCP port in srv.URL
		if err := os.RemoveAll(filepath.Join(dir, ".task")); err != nil {
			t.Logf("error cleaning up: %s", err)
		}
	})

	taskfiles, err := fs.Glob(os.DirFS(dir), "root-taskfile-*.yml")
	require.NoError(t, err)

	remotes := []struct {
		name string
		root string
	}{
		{
			name: "local",
			root: ".",
		},
		{
			name: "http-remote",
			root: srv.URL,
		},
	}

	for _, taskfile := range taskfiles {
		t.Run(taskfile, func(t *testing.T) {
			for _, remote := range remotes {
				t.Run(remote.name, func(t *testing.T) {
					t.Setenv("INCLUDE_ROOT", remote.root)
					entrypoint := filepath.Join(dir, taskfile)

					ctx, buffer, levelVar := SetupTestLogger(t, true, false)
					e := task.NewExecutor(
						task.WithLevelVar(levelVar),
						task.WithEntrypoint(entrypoint),
						task.WithDir(dir),
						task.WithStdout(buffer),
						task.WithStderr(buffer),
						task.WithInsecure(true),
						task.WithDownload(true),
						task.WithAssumeYes(true),
						task.WithVerbose(true),
						task.WithTimeout(time.Minute),
					)
					require.NoError(t, e.Setup(ctx))
					defer func() { t.Log("output:", buffer.buf.String()) }()

					tcs := []struct {
						name, dir string
					}{
						{
							name: "second-with-dir-1:third-with-dir-1:default",
							dir:  filepath.Join(dir, "dir-1", "dir-1"),
						},
						{
							name: "second-with-dir-1:third-with-dir-2:default",
							dir:  filepath.Join(dir, "dir-1", "dir-2"),
						},
					}

					for _, tc := range tcs {
						t.Run(tc.name, func(t *testing.T) {
							t.Parallel()
							task, err := e.CompiledTask(&task.Call{Task: tc.name})
							require.NoError(t, err)
							assert.Equal(t, tc.dir, task.Dir)
						})
					}
				})
			}
		})
	}
}

func TestIncludesHttpNest(t *testing.T) {
	dir, err := filepath.Abs("testdata/includes_http_nest")
	require.NoError(t, err)

	srv := httptest.NewServer(http.FileServer(http.Dir(dir)))
	defer srv.Close()

	t.Cleanup(func() {
		// This test fills the .task/remote directory with cache entries because the include URL
		// is different on every test due to the dynamic nature of the TCP port in srv.URL
		if err := os.RemoveAll(filepath.Join(dir, ".task")); err != nil {
			t.Logf("error cleaning up: %s", err)
		}
	})

	taskfiles, err := fs.Glob(os.DirFS(dir), "root-taskfile-*.yml")
	require.NoError(t, err)

	remotes := []struct {
		name string
		root string
	}{
		{
			name: "local",
			root: ".",
		},
		{
			name: "http-remote",
			root: srv.URL,
		},
	}

	for _, taskfile := range taskfiles {
		t.Run(taskfile, func(t *testing.T) {
			for _, remote := range remotes {
				t.Run(remote.name, func(t *testing.T) {
					t.Setenv("INCLUDE_ROOT", remote.root)
					entrypoint := filepath.Join(dir, taskfile)

					ctx, buffer, levelVar := SetupTestLogger(t, true, false)
					e := task.NewExecutor(
						task.WithLevelVar(levelVar),
						task.WithEntrypoint(entrypoint),
						task.WithDir(dir),
						task.WithStdout(buffer),
						task.WithStderr(buffer),
						task.WithInsecure(true),
						task.WithDownload(true),
						task.WithAssumeYes(true),
						task.WithVerbose(true),
						task.WithTimeout(time.Minute),
					)
					require.NoError(t, e.Setup(ctx))
					defer func() { t.Log("output:", buffer.buf.String()) }()

					tcs := []struct {
						name, dir string
					}{
						{
							name: "second-with-dir-1:third-with-dir-1:default",
							dir:  filepath.Join(dir, "dir-1", "dir-1"),
						},
						{
							name: "second-with-dir-1:third-with-dir-2:default",
							dir:  filepath.Join(dir, "dir-1", "dir-2"),
						},
					}

					for _, tc := range tcs {
						t.Run(tc.name, func(t *testing.T) {
							t.Parallel()
							task, err := e.CompiledTask(&task.Call{Task: tc.name})
							require.NoError(t, err)
							assert.Equal(t, tc.dir, task.Dir)
						})
					}
				})
			}
		})
	}
}

func TestIncludesOptionalImplicitFalse(t *testing.T) {
	t.Parallel()

	const dir = "testdata/includes_optional_implicit_false"
	wd, _ := os.Getwd()

	message := "task: No Taskfile found at \"%s/%s/TaskfileOptional.yml\""
	expected := fmt.Sprintf(message, filepath.ToSlash(wd), dir)
	ctx, _, levelVar := SetupTestLogger(t, false, false)

	e := task.NewExecutor(
		task.WithLevelVar(levelVar),
		task.WithDir(dir),
		task.WithStdout(io.Discard),
		task.WithStderr(io.Discard),
	)

	err := e.Setup(ctx)
	require.Error(t, err)
	assert.Equal(t, expected, err.Error())
}

func TestIncludesOptionalExplicitFalse(t *testing.T) {
	t.Parallel()

	const dir = "testdata/includes_optional_explicit_false"
	wd, _ := os.Getwd()

	message := "task: No Taskfile found at \"%s/%s/TaskfileOptional.yml\""
	expected := fmt.Sprintf(message, filepath.ToSlash(wd), dir)
	ctx, _, levelVar := SetupTestLogger(t, false, false)

	e := task.NewExecutor(
		task.WithLevelVar(levelVar),
		task.WithDir(dir),
		task.WithStdout(io.Discard),
		task.WithStderr(io.Discard),
	)

	err := e.Setup(ctx)
	require.Error(t, err)
	assert.Equal(t, expected, err.Error())
}

func TestIncludesRelativePath(t *testing.T) {
	t.Parallel()

	const dir = "testdata/includes_rel_path"

	ctx, buffer, levelVar := SetupTestLogger(t, false, false)
	e := task.NewExecutor(
		task.WithLevelVar(levelVar),
		task.WithDir(dir),
		task.WithStdout(buffer),
		task.WithStderr(buffer),
	)

	require.NoError(t, e.Setup(ctx))

	require.NoError(t, e.Run(ctx, &task.Call{Task: "common:pwd"}))
	assert.Contains(t, filepath.ToSlash(buffer.buf.String()), "testdata/includes_rel_path/common")

	buffer.buf.Reset()
	require.NoError(t, e.Run(ctx, &task.Call{Task: "included:common:pwd"}))
	assert.Contains(t, filepath.ToSlash(buffer.buf.String()), "testdata/includes_rel_path/common")
}

func TestIncludedTaskfileVarMerging(t *testing.T) {
	t.Parallel()

	const dir = "testdata/included_taskfile_var_merging"
	tests := []struct {
		name           string
		task           string
		expectedOutput string
	}{
		{"foo", "foo:pwd", "included_taskfile_var_merging/foo\n"},
		{"bar", "bar:pwd", "included_taskfile_var_merging/bar\n"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			ctx, buffer, levelVar := SetupTestLogger(t, false, false)
			e := task.NewExecutor(
				task.WithLevelVar(levelVar),
				task.WithDir(dir),
				task.WithStdout(buffer),
				task.WithStderr(buffer),
				task.WithSilent(true),
			)
			require.NoError(t, e.Setup(ctx))

			err := e.Run(ctx, &task.Call{Task: test.task})
			require.NoError(t, err)
			assert.Contains(t, filepath.ToSlash(buffer.buf.String()), test.expectedOutput)
		})
	}
}

func TestSupportedFileNames(t *testing.T) {
	t.Parallel()

	fileNames := []string{
		"Taskfile.yml",
		"Taskfile.yaml",
		"Taskfile.dist.yml",
		"Taskfile.dist.yaml",
	}
	for _, fileName := range fileNames {
		t.Run(fileName, func(t *testing.T) {
			t.Parallel()

			tt := fileContentTest{
				Dir:       fmt.Sprintf("testdata/file_names/%s", fileName),
				Target:    "default",
				TrimSpace: true,
				Files: map[string]string{
					"output.txt": "hello",
				},
			}
			tt.Run(t)
		})
	}
}

func TestWhenNoDirAttributeItRunsInSameDirAsTaskfile(t *testing.T) {
	t.Parallel()

	const expected = "dir"
	const dir = "testdata/" + expected
	ctx, buffer, levelVar := SetupTestLogger(t, false, false)
	e := task.NewExecutor(
		task.WithLevelVar(levelVar),
		task.WithDir(dir),
		task.WithStdout(buffer),
		task.WithStderr(buffer),
	)

	require.NoError(t, e.Setup(ctx))
	require.NoError(t, e.Run(ctx, &task.Call{Task: "whereami"}))

	// got should be the "dir" part of "testdata/dir"
	// Normalize path separators for cross-platform compatibility (Windows uses backslashes)
	normalized := normalizePathSeparators(buffer.buf.String())
	got := strings.TrimSuffix(filepath.Base(normalized), "\n")
	assert.Equal(t, expected, got, "Mismatch in the working directory")
}

func TestWhenDirAttributeAndDirExistsItRunsInThatDir(t *testing.T) {
	t.Parallel()

	const expected = "exists"
	const dir = "testdata/dir/explicit_exists"
	ctx, buffer, levelVar := SetupTestLogger(t, false, false)
	e := task.NewExecutor(
		task.WithLevelVar(levelVar),
		task.WithDir(dir),
		task.WithStdout(buffer),
		task.WithStderr(buffer),
	)

	require.NoError(t, e.Setup(ctx))
	require.NoError(t, e.Run(ctx, &task.Call{Task: "whereami"}))

	// Normalize path separators for cross-platform compatibility (Windows uses backslashes)
	normalized := normalizePathSeparators(buffer.buf.String())
	got := strings.TrimSuffix(filepath.Base(normalized), "\n")
	assert.Equal(t, expected, got, "Mismatch in the working directory")
}

func TestWhenDirAttributeItCreatesMissingAndRunsInThatDir(t *testing.T) {
	t.Parallel()

	const expected = "createme"
	const dir = "testdata/dir/explicit_doesnt_exist/"
	const toBeCreated = dir + expected
	const target = "whereami"
	ctx, buffer, levelVar := SetupTestLogger(t, false, false)
	e := task.NewExecutor(
		task.WithLevelVar(levelVar),
		task.WithDir(dir),
		task.WithStdout(buffer),
		task.WithStderr(buffer),
	)

	// Ensure that the directory to be created doesn't actually exist.
	_ = os.RemoveAll(toBeCreated)
	if _, err := os.Stat(toBeCreated); err == nil {
		t.Errorf("Directory should not exist: %v", err)
	}
	require.NoError(t, e.Setup(ctx))
	require.NoError(t, e.Run(ctx, &task.Call{Task: target}))

	// Normalize path separators for cross-platform compatibility (Windows uses backslashes)
	normalized := normalizePathSeparators(buffer.buf.String())
	got := strings.TrimSuffix(filepath.Base(normalized), "\n")
	assert.Equal(t, expected, got, "Mismatch in the working directory")

	// Clean-up after ourselves only if no error.
	_ = os.RemoveAll(toBeCreated)
}

func TestDynamicVariablesRunOnTheNewCreatedDir(t *testing.T) {
	t.Parallel()

	const expected = "created"
	const dir = "testdata/dir/dynamic_var_on_created_dir/"
	const toBeCreated = dir + expected
	const target = "default"
	ctx, buffer, levelVar := SetupTestLogger(t, false, false)
	e := task.NewExecutor(
		task.WithLevelVar(levelVar),
		task.WithDir(dir),
		task.WithStdout(buffer),
		task.WithStderr(buffer),
	)

	// Ensure that the directory to be created doesn't actually exist.
	_ = os.RemoveAll(toBeCreated)
	if _, err := os.Stat(toBeCreated); err == nil {
		t.Errorf("Directory should not exist: %v", err)
	}
	require.NoError(t, e.Setup(ctx))
	require.NoError(t, e.Run(ctx, &task.Call{Task: target}))

	// Normalize path separators for cross-platform compatibility (Windows uses backslashes)
	// Take only the first line as Windows may output additional debug info
	normalized := normalizePathSeparators(buffer.buf.String())
	firstLine := strings.Split(normalized, "\n")[0]
	got := filepath.Base(firstLine)
	assert.Equal(t, expected, got, "Mismatch in the working directory")

	// Clean-up after ourselves only if no error.
	_ = os.RemoveAll(toBeCreated)
}

func TestDynamicVariablesShouldRunOnTheTaskDir(t *testing.T) {
	t.Parallel()

	tt := fileContentTest{
		Dir:       "testdata/dir/dynamic_var",
		Target:    "default",
		TrimSpace: false,
		Files: map[string]string{
			"subdirectory/from_root_taskfile.txt":          "subdirectory\n",
			"subdirectory/from_included_taskfile.txt":      "subdirectory\n",
			"subdirectory/from_included_taskfile_task.txt": "subdirectory\n",
			"subdirectory/from_interpolated_dir.txt":       "subdirectory\n",
		},
	}
	t.Run("", func(t *testing.T) {
		t.Parallel()
		tt.Run(t)
	})
}

func TestDynamicVariablesRunOnParentDir(t *testing.T) {
	t.Parallel()

	const expected = "fubar"
	const dir = "testdata/dir/dynamic_var_on_parent_dir/"
	const toBeCreated = dir + "somefolder"
	const target = "default"
	ctx, buffer, levelVar := SetupTestLogger(t, false, false)
	e := task.NewExecutor(
		task.WithLevelVar(levelVar),
		task.WithDir(dir),
		task.WithStdout(buffer),
		task.WithStderr(buffer),
		task.WithSilent(true),
	)

	// Ensure that the directory to be created doesn't actually exist.
	_ = os.RemoveAll(toBeCreated)
	if _, err := os.Stat(toBeCreated); err == nil {
		t.Errorf("Directory should not exist: %v", err)
	}
	require.NoError(t, e.Setup(ctx))
	require.NoError(t, e.Run(ctx, &task.Call{Task: target}))

	normalized := normalizePathSeparators(buffer.buf.String())
	got := strings.TrimSuffix(filepath.Base(normalized), "\n")
	assert.Equal(t, expected, got, "Mismatch message from parent dir")

	// Clean-up after ourselves only if no error.
	_ = os.RemoveAll(toBeCreated)
}

func TestDisplaysErrorOnVersion1Schema(t *testing.T) {
	t.Parallel()

	ctx, _, levelVar := SetupTestLogger(t, false, false)

	e := task.NewExecutor(
		task.WithLevelVar(levelVar),
		task.WithDir("testdata/version/v1"),
		task.WithStdout(io.Discard),
		task.WithStderr(io.Discard),
		task.WithVersionCheck(true),
	)
	err := e.Setup(ctx)
	require.Error(t, err)
	assert.Regexp(t, regexp.MustCompile(`task: Invalid schema version in Taskfile \".*testdata\/version\/v1\/Taskfile\.yml\":\nSchema version \(1\.0\.0\) no longer supported\. Please use v3 or above`), err.Error())
}

func TestDisplaysErrorOnVersion2Schema(t *testing.T) {
	t.Parallel()

	ctx, buffer, levelVar := SetupTestLogger(t, false, false)
	e := task.NewExecutor(
		task.WithLevelVar(levelVar),
		task.WithDir("testdata/version/v2"),
		task.WithStdout(io.Discard),
		task.WithStderr(buffer),
		task.WithVersionCheck(true),
	)
	err := e.Setup(ctx)
	require.Error(t, err)
	assert.Regexp(t, regexp.MustCompile(`task: Invalid schema version in Taskfile \".*testdata\/version\/v2\/Taskfile\.yml\":\nSchema version \(2\.0\.0\) no longer supported\. Please use v3 or above`), err.Error())
}

func TestRunOnlyRunsJobsHashOnce(t *testing.T) {
	t.Parallel()

	tt := fileContentTest{
		Dir:    "testdata/run",
		Target: "generate-hash",
		Files: map[string]string{
			"hash.txt": "starting 1\n1\n2\n",
		},
	}
	t.Run("", func(t *testing.T) {
		t.Parallel()
		tt.Run(t)
	})
}

func TestRunOnlyRunsJobsHashOnceWithWildcard(t *testing.T) {
	t.Parallel()

	tt := fileContentTest{
		Dir:    "testdata/run",
		Target: "deploy",
		Files: map[string]string{
			"wildcard.txt": "Deploy infra\nDeploy js\nDeploy go\n",
		},
	}
	t.Run("", func(t *testing.T) {
		t.Parallel()
		tt.Run(t)
	})
}

func TestRunOnceSharedDeps(t *testing.T) {
	t.Parallel()

	const dir = "testdata/run_once_shared_deps"

	ctx, buffer, levelVar := SetupTestLogger(t, false, false)
	e := task.NewExecutor(
		task.WithLevelVar(levelVar),
		task.WithDir(dir),
		task.WithStdout(buffer),
		task.WithStderr(buffer),
		task.WithForceAll(true),
	)
	require.NoError(t, e.Setup(ctx))
	require.NoError(t, e.Run(ctx, &task.Call{Task: "build"}))

	rx := regexp.MustCompile(`task: \[service-[a,b]:library:build\] echo "build library"`)
	matches := rx.FindAllStringSubmatch(buffer.buf.String(), -1)
	assert.Len(t, matches, 1)
	assert.Contains(t, buffer.buf.String(), `task: [service-a:build] echo "build a"`)
	assert.Contains(t, buffer.buf.String(), `task: [service-b:build] echo "build b"`)
}

func TestRunWhenChanged(t *testing.T) {
	t.Parallel()

	const dir = "testdata/run_when_changed"

	ctx, buffer, levelVar := SetupTestLogger(t, false, false)
	e := task.NewExecutor(
		task.WithLevelVar(levelVar),
		task.WithDir(dir),
		task.WithStdout(buffer),
		task.WithStderr(buffer),
		task.WithForceAll(true),
		task.WithSilent(true),
	)
	require.NoError(t, e.Setup(ctx))
	require.NoError(t, e.Run(ctx, &task.Call{Task: "start"}))
	expectedOutputOrder := strings.TrimSpace(`
login server=fubar user=fubar
login server=foo user=foo
login server=bar user=bar
`)
	assert.Contains(t, buffer.buf.String(), expectedOutputOrder)
}

func TestIncludedVars(t *testing.T) {
	t.Parallel()

	const dir = "testdata/include_with_vars"
	ctx, buffer, levelVar := SetupTestLogger(t, false, false)
	e := task.NewExecutor(
		task.WithLevelVar(levelVar),
		task.WithDir(dir),
		task.WithStdout(buffer),
		task.WithStderr(buffer),
	)
	require.NoError(t, e.Setup(ctx))

	expectedOutputOrder := strings.TrimSpace(`
task: [included1:task1] echo "VAR_1 is included1-var1"
VAR_1 is included1-var1
task: [included1:task1] echo "VAR_2 is included-default-var2"
VAR_2 is included-default-var2
task: [included2:task1] echo "VAR_1 is included2-var1"
VAR_1 is included2-var1
task: [included2:task1] echo "VAR_2 is included-default-var2"
VAR_2 is included-default-var2
task: [included3:task1] echo "VAR_1 is included-default-var1"
VAR_1 is included-default-var1
task: [included3:task1] echo "VAR_2 is included-default-var2"
VAR_2 is included-default-var2
`)
	require.NoError(t, e.Run(ctx, &task.Call{Task: "task1"}))
	t.Log(buffer.buf.String())
	assert.Equal(t, strings.TrimSpace(buffer.buf.String()), expectedOutputOrder)
}

func TestIncludeWithVarsInInclude(t *testing.T) {
	t.Parallel()

	const dir = "testdata/include_with_vars_inside_include"
	ctx, buffer, _ := SetupTestLogger(t, false, false)
	e := task.Executor{
		Dir:    dir,
		Stdout: buffer,
		Stderr: buffer,
	}
	require.NoError(t, e.Setup(ctx))
}

func TestIncludedVarsMultiLevel(t *testing.T) {
	t.Parallel()

	const dir = "testdata/include_with_vars_multi_level"
	ctx, buffer, levelVar := SetupTestLogger(t, false, false)
	e := task.NewExecutor(
		task.WithLevelVar(levelVar),
		task.WithDir(dir),
		task.WithStdout(buffer),
		task.WithStderr(buffer),
	)
	require.NoError(t, e.Setup(ctx))

	expectedOutputOrder := strings.TrimSpace(`
task: [lib:greet] echo 'Hello world'
Hello world
task: [foo:lib:greet] echo 'Hello foo'
Hello foo
task: [bar:lib:greet] echo 'Hello bar'
Hello bar
`)
	require.NoError(t, e.Run(ctx, &task.Call{Task: "default"}))
	t.Log(buffer.buf.String())
	assert.Equal(t, expectedOutputOrder, strings.TrimSpace(buffer.buf.String()))
}

func TestErrorCode(t *testing.T) {
	t.Parallel()

	const dir = "testdata/error_code"
	tests := []struct {
		name     string
		task     string
		expected int
	}{
		{
			name:     "direct task",
			task:     "direct",
			expected: 42,
		}, {
			name:     "indirect task",
			task:     "indirect",
			expected: 42,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			ctx, buffer, levelVar := SetupTestLogger(t, false, false)
			e := task.NewExecutor(
				task.WithLevelVar(levelVar),
				task.WithDir(dir),
				task.WithStdout(buffer),
				task.WithStderr(buffer),
				task.WithSilent(true),
			)
			require.NoError(t, e.Setup(ctx))

			err := e.Run(ctx, &task.Call{Task: test.task})
			require.Error(t, err)
			taskRunErr, ok := err.(*errors.TaskRunError)
			assert.True(t, ok, "cannot cast returned error to *task.TaskRunError")
			assert.Equal(t, test.expected, taskRunErr.TaskExitCode(), "unexpected exit code from task")
		})
	}
}

func TestEvaluateSymlinksInPaths(t *testing.T) { // nolint:paralleltest // cannot run in parallel
	t.Skip("Flakey test, need to resolve")
	const dir = "testdata/evaluate_symlinks_in_paths"
	ctx, buffer, levelVar := SetupTestLogger(t, false, false)
	e := task.NewExecutor(
		task.WithLevelVar(levelVar),
		task.WithDir(dir),
		task.WithStdout(buffer),
		task.WithStderr(buffer),
		task.WithSilent(false),
	)
	tests := []struct {
		name     string
		task     string
		expected string
	}{
		{
			name:     "default (1)",
			task:     "default",
			expected: "task: [default] echo \"some job\"\nsome job",
		},
		{
			name:     "test-sym (1)",
			task:     "test-sym",
			expected: "task: [test-sym] echo \"shared file source changed\" > src/shared/b",
		},
		// {
		// 	name:     "default (2)",
		// 	task:     "default",
		// 	expected: "task: [default] echo \"some job\"\nsome job",
		// },
		{
			name:     "default (3)",
			task:     "default",
			expected: `task: Task "default" is up to date`,
		},
		{
			name:     "reset",
			task:     "reset",
			expected: "task: [reset] echo \"shared file source\" > src/shared/b\ntask: [reset] echo \"file source\" > src/a",
		},
	}
	for _, test := range tests { // nolint:paralleltest // cannot run in parallel
		t.Run(test.name, func(t *testing.T) {
			require.NoError(t, e.Setup(ctx))
			err := e.Run(ctx, &task.Call{Task: test.task})
			require.NoError(t, err)
			assert.Equal(t, test.expected, strings.TrimSpace(buffer.buf.String()))
			buffer.buf.Reset()
		})
	}
	err := os.RemoveAll(dir + "/.task")
	require.NoError(t, err)
}

func TestTaskfileWalk(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		dir      string
		expected string
	}{
		{
			name:     "walk from root directory",
			dir:      "testdata/taskfile_walk",
			expected: "foo\n",
		}, {
			name:     "walk from sub directory",
			dir:      "testdata/taskfile_walk/foo",
			expected: "foo\n",
		}, {
			name:     "walk from sub sub directory",
			dir:      "testdata/taskfile_walk/foo/bar",
			expected: "foo\n",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			ctx, buffer, levelVar := SetupTestLogger(t, false, false)
			e := task.NewExecutor(
				task.WithLevelVar(levelVar),
				task.WithDir(test.dir),
				task.WithStdout(buffer),
				task.WithStderr(buffer),
				task.WithSilent(true),
			)
			require.NoError(t, e.Setup(ctx))
			require.NoError(t, e.Run(ctx, &task.Call{Task: "default"}))
			assert.Equal(t, test.expected, buffer.buf.String())
		})
	}
}

func TestUserWorkingDirectory(t *testing.T) {
	t.Parallel()

	ctx, buffer, levelVar := SetupTestLogger(t, false, true)
	e := task.NewExecutor(
		task.WithLevelVar(levelVar),
		task.WithDir("testdata/user_working_dir"),
		task.WithStdout(buffer),
		task.WithStderr(buffer),
	)
	wd, err := os.Getwd()
	require.NoError(t, err)
	require.NoError(t, e.Setup(ctx))
	require.NoError(t, e.Run(ctx, &task.Call{Task: "default"}))
	// Use filepath.ToSlash because USER_WORKING_DIR uses forward slashes on all platforms
	assert.Equal(t, fmt.Sprintf("%s\n", filepath.ToSlash(wd)), buffer.buf.String())
}

func TestUserWorkingDirectoryWithIncluded(t *testing.T) {
	t.Parallel()

	wd, err := os.Getwd()
	require.NoError(t, err)

	wd = filepath.ToSlash(filepathext.SmartJoin(wd, "testdata/user_working_dir_with_includes/somedir"))

	ctx, buffer, levelVar := SetupTestLogger(t, false, true)
	e := task.NewExecutor(
		task.WithLevelVar(levelVar),
		task.WithDir("testdata/user_working_dir_with_includes"),
		task.WithStdout(buffer),
		task.WithStderr(buffer),
	)
	e.UserWorkingDir = wd

	require.NoError(t, err)
	require.NoError(t, e.Setup(ctx))
	require.NoError(t, e.Run(ctx, &task.Call{Task: "included:echo"}))
	// Normalize path separators for cross-platform compatibility (Windows uses backslashes)
	assert.Equal(t, fmt.Sprintf("%s\n", wd), normalizePathSeparators(buffer.buf.String()))
}

func TestAbsPath(t *testing.T) {
	t.Parallel()

	ctx, buffer, levelVar := SetupTestLogger(t, false, false)
	e := task.NewExecutor(
		task.WithLevelVar(levelVar),
		task.WithDir("testdata/abs_path"),
		task.WithStdout(buffer),
		task.WithStderr(buffer),
		task.WithSilent(true),
	)
	require.NoError(t, e.Setup(ctx))

	err := e.Run(ctx, &task.Call{Task: "default"})
	require.NoError(t, err)

	cwd, err := os.Getwd()
	require.NoError(t, err)
	expected := filepath.Join(cwd, "bar") + "\n"
	assert.Equal(t, expected, buffer.buf.String())
}

func TestForce(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		env      map[string]string
		force    bool
		forceAll bool
	}{
		{
			name:  "force",
			force: true,
		},
		{
			name:     "force-all",
			forceAll: true,
		},
		{
			name:  "force with gentle force experiment",
			force: true,
			env: map[string]string{
				"TASK_X_GENTLE_FORCE": "1",
			},
		},
		{
			name:     "force-all with gentle force experiment",
			forceAll: true,
			env: map[string]string{
				"TASK_X_GENTLE_FORCE": "1",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctx, buffer, levelVar := SetupTestLogger(t, false, false)
			e := task.NewExecutor(
				task.WithLevelVar(levelVar),
				task.WithDir("testdata/force"),
				task.WithStdout(buffer),
				task.WithStderr(buffer),
				task.WithForce(tt.force),
				task.WithForceAll(tt.forceAll),
			)
			require.NoError(t, e.Setup(ctx))
			require.NoError(t, e.Run(ctx, &task.Call{Task: "task-with-dep"}))
		})
	}
}
