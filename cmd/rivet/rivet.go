package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	"github.com/go-rivet/rivet/internal/config"
	"github.com/go-rivet/rivet/internal/filepathext"
	"github.com/go-rivet/rivet/internal/sort"
	"github.com/go-rivet/rivet/internal/version"
	task "github.com/go-rivet/rivet/pkg/rivet"
	"github.com/go-rivet/rivet/pkg/rivet/args"
	"github.com/go-rivet/rivet/pkg/rivet/errors"
	"github.com/go-rivet/rivet/pkg/rivet/taskfile/ast"
	"github.com/go-rivet/rivet/pkg/rlog"
)

var flagConfig Config

func main() {
	exit := func(err error) {
		if err == nil {
			os.Exit(errors.CodeOk)
		}
		if isGA, _ := strconv.ParseBool(os.Getenv("GITHUB_ACTIONS")); isGA {
			if e, ok := err.(*errors.TaskRunError); ok {
				_, _ = fmt.Fprintf(os.Stdout, "::error title=Task '%s' failed::%v\n", e.TaskName, e.Err)
			} else {
				_, _ = fmt.Fprintf(os.Stdout, "::error title=Task failed::%v\n", err)
			}
		}
		if err, ok := err.(*errors.TaskRunError); ok && flagConfig.ExitCode {
			os.Exit(err.TaskExitCode())
		}
		if err, ok := err.(errors.TaskError); ok {
			os.Exit(err.Code())
		}
		os.Exit(errors.CodeUnknown)
	}

	// Load env file if present before parsing flags.
	var envFile string
	taskfilePath := config.PeekTaskfilePath(os.Args[1:])
	if envFile = config.FindEnvFile(taskfilePath); envFile != "" {
		if err := config.LoadEnv(envFile); err != nil {
			exit(err)
		}
	}

	// Config and flags.
	if err := ParseFlags(&flagConfig, "rivet"); err != nil {
		exit(err)
	}
	flagConfig.Adjust()
	if err := flagConfig.Validate(); err != nil {
		exit(err)
	}

	// Context with signal handling.
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	// Logging.
	vl := VerboseLevel(0)
	if err := vl.Set(flagConfig.Verbose); err != nil {
		exit(err)
	}
	logLevelVar := &slog.LevelVar{}
	logLevelVar.Set(LogLevel(vl))
	rlog.Init(rlog.RlogOptions{
		Stdout: os.Stdout,
		Stderr: os.Stderr,
		Level:  logLevelVar,
		Format: flagConfig.LogFormat,
		Color:  flagConfig.Color,
	})

	// Log the operating conditions.
	rlog.Debug(ctx, "Rivet operating conditions:", "envFile", envFile, "entrypoint", flagConfig.Taskfile)

	// Run rivet.
	if err := run(ctx); err != nil {
		rlog.Errorf(ctx, "%v\n", err)
		exit(err)
	}
	exit(nil)
}

func run(ctx context.Context) error {
	if flagConfig.Version {
		fmt.Println(version.GetVersionWithBuildInfo())
		return nil
	}
	if flagConfig.Help {
		flag.Usage()
		return nil
	}

	if flagConfig.Init {
		cmdArgs, _ := GetCliArgs()
		wd, err := os.Getwd()
		if err != nil {
			return err
		}
		path := wd
		if len(cmdArgs) > 0 {
			name := cmdArgs[0]
			if filepathext.IsExtOnly(name) {
				name = filepathext.SmartJoin(filepath.Dir(name), "Taskfile"+filepath.Ext(name))
			}
			path = filepathext.SmartJoin(wd, name)
		}
		path, err = task.InitTaskfile(path)
		if err != nil {
			return err
		}

		rlog.Debugf(ctx, "%s\n", task.DefaultTaskfile)
		rlog.Infof(ctx, "Taskfile created: %s\n", filepathext.TryAbsToRel(path))
		return nil
	}

	// Setup an executor.
	e := NewExecutor(&flagConfig)
	if err := e.Setup(ctx); err != nil {
		return err
	}

	// Early return conditions.
	if flagConfig.ClearCache {
		cachePath := filepath.Join(e.TempDir.Remote, "remote")
		return os.RemoveAll(cachePath)
	}
	listOptions := task.NewListOptions(
		flagConfig.List,
		flagConfig.ListAll,
		flagConfig.ListJson,
		flagConfig.NoStatus,
		flagConfig.Nested,
	)
	if listOptions.ShouldListTasks() {
		foundTasks, err := e.ListTasks(listOptions)
		if err != nil {
			return err
		}
		if !foundTasks {
			os.Exit(errors.CodeUnknown)
		}
		return nil
	}

	// Final execution conditions.
	cliArgsPreDash, cliArgsPostDash := GetCliArgs()
	calls, globals := args.Parse(cliArgsPreDash...)
	if len(calls) == 0 {
		calls = append(calls, &task.Call{Task: "default"})
	}
	e.Taskfile.Vars.Merge(globals, nil) // Merge CLI variables first (e.g. FOO=bar) so they take priority over Taskfile defaults
	e.Taskfile.Vars.ReverseMerge(specialVars(cliArgsPreDash, cliArgsPostDash), nil)
	if !flagConfig.Watch {
		e.InterceptInterruptSignals()
	}
	if flagConfig.Status {
		return e.Status(ctx, calls...)
	}

	// Run the task(s).
	return e.Run(ctx, calls...)
}

func NewExecutor(c *Config) *task.Executor {
	e := task.NewExecutor(
		func() task.ExecutorOption {
			return &flagsOption{c: c}
		}(),
		task.WithVersionCheck(true),
	)
	return e
}

type flagsOption struct {
	c *Config
}

func (o *flagsOption) ApplyToExecutor(e *task.Executor) {
	c := o.c

	// Set the sorter.
	var sorter sort.Sorter
	switch c.Sort {
	case "none":
		sorter = sort.NoSort
	case "alphanumeric":
		sorter = sort.AlphaNumeric
	}

	// Set the dir to home if global set.
	dir := c.Dir
	if c.Global {
		home, err := os.UserHomeDir()
		if err == nil {
			dir = home
		}
	} else if len(dir) > 0 {
		if d, err := filepath.Abs(dir); err == nil {
			dir = d
		}
	}

	// Set output.
	output := ast.Output{}
	output.Name = c.Output
	output.Group.Begin = c.OutputGroupBegin
	output.Group.End = c.OutputGroupEnd
	output.Group.ErrorOnly = c.OutputGroupErrorOnly

	// Trusted hosts.
	trustedHosts := []string{}
	for _, h := range strings.Split(c.TrustedHosts, ",") {
		h = strings.TrimSpace(h)
		if h != "" {
			trustedHosts = append(trustedHosts, h)
		}
	}

	e.Options(
		task.WithDir(dir),
		task.WithEntrypoint(c.Taskfile),
		task.WithForce(c.Force),
		task.WithForceAll(c.ForceAll),
		task.WithInsecure(c.Insecure),
		task.WithDownload(c.Download),
		task.WithOffline(c.Offline),
		task.WithTrustedHosts(trustedHosts),
		task.WithTimeout(c.Timeout),
		task.WithCacheExpiryDuration(c.CacheExpiryDuration),
		task.WithRemoteCacheDir(c.RemoteCacheDir),
		task.WithCACert(c.CACert),
		task.WithCert(c.Cert),
		task.WithCertKey(c.CertKey),
		task.WithWatch(c.Watch),
		task.WithDisableFuzzy(c.DisableFuzzy),
		task.WithAssumeYes(c.AssumeYes),
		task.WithInteractive(c.Interactive),
		task.WithDry(c.Dry || c.Status),
		task.WithSummary(c.Summary),
		task.WithParallel(c.Parallel),
		task.WithColor(c.Color),
		task.WithConcurrency(c.Concurrency),
		task.WithInterval(c.Interval),
		task.WithOutputStyle(output),
		task.WithTaskSorter(sorter),
		task.WithVersionCheck(true),
		task.WithFailfast(c.Failfast),
		task.WithLogFormat(c.LogFormat),
	)
}

func specialVars(cliArgsPreDash []string, cliArgsPostDash []string) *ast.Vars {
	vars := ast.NewVarsWithCapacity(5)

	cliArgsPostDashQuoted, err := args.ToQuotedString(cliArgsPostDash)
	if err != nil {
		return vars
	}

	vars.Set("CLI_ARGS", ast.Var{Value: cliArgsPostDashQuoted})
	vars.Set("CLI_ARGS_LIST", ast.Var{Value: cliArgsPostDash})
	vars.Set("CLI_FORCE", ast.Var{Value: flagConfig.Force || flagConfig.ForceAll})
	vars.Set("CLI_OFFLINE", ast.Var{Value: flagConfig.Offline})
	vars.Set("CLI_ASSUME_YES", ast.Var{Value: flagConfig.AssumeYes})

	return vars
}
