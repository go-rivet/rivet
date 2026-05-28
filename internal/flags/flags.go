package flags

import (
	"cmp"
	"log"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/pflag"

	"github.com/go-rivet/rivet/internal/env"
	"github.com/go-rivet/rivet/internal/sort"
	task "github.com/go-rivet/rivet/pkg/rivet"
	"github.com/go-rivet/rivet/pkg/rivet/errors"
	"github.com/go-rivet/rivet/pkg/rivet/taskfile/ast"
	"github.com/go-rivet/rivet/pkg/rlog"
)

const usage = `Usage: task [flags...] [task...]

Runs the specified task(s). Falls back to the "default" task if no task name
was specified, or lists all tasks if an unknown task name was specified.

Example: 'task hello' with the following 'Taskfile.yml' file will generate an
'output.txt' file with the content "hello".

'''
version: '3'
tasks:
  hello:
    cmds:
      - echo "I am going to write a file named 'output.txt' now."
      - echo "hello" > output.txt
    generates:
      - output.txt
'''

Options:
`

type VerboseLevel int

const (
	LevelNone  VerboseLevel = 0
	LevelInfo  VerboseLevel = 1 // Maps to -v     or --verbose=info
	LevelDebug VerboseLevel = 2 // Maps to -vv    or --verbose=debug
	LevelTrace VerboseLevel = 3 // Maps to -vvv   or --verbose=trace
)

func (vl *VerboseLevel) String() string {
	switch *vl {
	case LevelInfo:
		return "info"
	case LevelDebug:
		return "debug"
	case LevelTrace:
		return "trace"
	default:
		return "none"
	}
}

func (vl *VerboseLevel) Type() string {
	return "string/count"
}

func (vl *VerboseLevel) Set(s string) error {
	normalized := strings.ToLower(strings.TrimSpace(s))

	switch normalized {
	case "info":
		*vl = LevelInfo
		return nil
	case "debug":
		*vl = LevelDebug
		return nil
	case "trace":
		*vl = LevelTrace
		return nil
	case "none", "0":
		*vl = LevelNone
		return nil
	default:
		*vl = LevelNone
		return nil
	}
}

var (
	Version             bool
	Help                bool
	Init                bool
	List                bool
	ListAll             bool
	ListJson            bool
	TaskSort            string
	Status              bool
	NoStatus            bool
	Nested              bool
	Insecure            bool
	Force               bool
	ForceAll            bool
	Watch               bool
	Verbose             VerboseLevel
	VLevel              int
	DisableFuzzy        bool
	AssumeYes           bool
	Dry                 bool
	Summary             bool
	ExitCode            bool
	Parallel            bool
	Concurrency         int
	Dir                 string
	Entrypoint          string
	Output              ast.Output
	Color               bool
	Interval            time.Duration
	Failfast            bool
	Global              bool
	Download            bool
	Offline             bool
	TrustedHosts        []string
	ClearCache          bool
	Timeout             time.Duration
	CacheExpiryDuration time.Duration
	RemoteCacheDir      string
	LogFormat           string
	CACert              string
	Cert                string
	CertKey             string
	Interactive         bool
)

func init() {
	var dir, entrypoint string
	fs := pflag.NewFlagSet("experiments", pflag.ContinueOnError)
	fs.StringVarP(&dir, "dir", "d", "", "")
	fs.StringVarP(&entrypoint, "taskfile", "t", "", "")
	fs.Usage = func() {}
	_ = fs.Parse(os.Args[1:])

	dir = cmp.Or(dir, filepath.Dir(entrypoint))

	// Parse the rest of the flags
	log.SetFlags(0)
	log.SetOutput(os.Stderr)
	pflag.Usage = func() {
		log.Print(usage)
		pflag.PrintDefaults()
	}

	pflag.BoolVar(&Version, "version", false, "Show Task version.")
	pflag.BoolVarP(&Help, "help", "h", false, "Shows Task usage.")
	pflag.BoolVarP(&Init, "init", "i", false, "Creates a new Taskfile.yml in the current folder.")
	pflag.BoolVarP(&List, "list", "l", false, "Lists tasks with description of current Taskfile.")
	pflag.BoolVarP(&ListAll, "list-all", "a", false, "Lists tasks with or without a description.")
	pflag.BoolVarP(&ListJson, "json", "j", false, "Formats task list as JSON.")
	pflag.StringVar(&TaskSort, "sort", "", "Changes the order of the tasks when listed. [default|alphanumeric|none].")
	pflag.BoolVar(&Status, "status", false, "Exits with non-zero exit code if any of the given tasks is not up-to-date.")
	pflag.BoolVar(&NoStatus, "no-status", false, "Ignore status when listing tasks as JSON")
	pflag.BoolVar(&Nested, "nested", false, "Nest namespaces when listing tasks as JSON")
	pflag.BoolVar(&Insecure, "insecure", getConfig("REMOTE_INSECURE", false), "Forces Task to download Taskfiles over insecure connections.")
	pflag.BoolVarP(&Watch, "watch", "w", false, "Enables watch of the given task.")

	_ = Verbose.Set("none")
	pflag.Var(&Verbose, "verbose", "Log verbosity level [info|debug|trace] or cumulative shorthand [-v|-vv|-vvv]")
	pflag.CountVarP(&VLevel, "v-count", "v", "Increase verbosity level cumulatively (-v, -vv, -vvv)")
	_ = pflag.CommandLine.MarkHidden("v-count")
	pflag.StringVar(&LogFormat, "log", "", `Log format ("json" or "text").`)

	pflag.BoolVar(&DisableFuzzy, "disable-fuzzy", getConfig("DISABLE_FUZZY", false), "Disables fuzzy matching for task names.")
	pflag.BoolVarP(&AssumeYes, "yes", "y", getConfig("ASSUME_YES", false), "Assume \"yes\" as answer to all prompts.")
	pflag.BoolVar(&Interactive, "interactive", getConfig("INTERACTIVE", false), "Prompt for missing required variables.")
	pflag.BoolVarP(&Parallel, "parallel", "p", false, "Executes tasks provided on command line in parallel.")
	pflag.BoolVarP(&Dry, "dry", "n", getConfig("DRY", false), "Compiles and prints tasks in the order that they would be run, without executing them.")
	pflag.BoolVar(&Summary, "summary", false, "Show summary about a task.")
	pflag.BoolVarP(&ExitCode, "exit-code", "x", false, "Pass-through the exit code of the task command.")
	pflag.StringVarP(&Dir, "dir", "d", "", "Sets the directory in which Task will execute and look for a Taskfile.")
	pflag.StringVarP(&Entrypoint, "taskfile", "t", "", `Choose which Taskfile to run. Defaults to "Taskfile.yml".`)
	pflag.StringVarP(&Output.Name, "output", "o", "", "Sets output style: [interleaved|group|prefixed].")
	pflag.StringVar(&Output.Group.Begin, "output-group-begin", "", "Message template to print before a task's grouped output.")
	pflag.StringVar(&Output.Group.End, "output-group-end", "", "Message template to print after a task's grouped output.")
	pflag.BoolVar(&Output.Group.ErrorOnly, "output-group-error-only", false, "Swallow output from successful tasks.")
	pflag.BoolVarP(&Color, "color", "c", getConfig("COLOR", false), "Colored output. Enabled by default. Set flag to false or use NO_COLOR=1 to disable.")
	pflag.IntVarP(&Concurrency, "concurrency", "C", getConfig("CONCURRENCY", 0), "Limit number of tasks to run concurrently.")
	pflag.DurationVarP(&Interval, "interval", "I", 0, "Interval to watch for changes.")
	pflag.BoolVarP(&Failfast, "failfast", "F", getConfig("FAILFAST", false), "When running tasks in parallel, stop all tasks if one fails.")
	pflag.BoolVarP(&Global, "global", "g", false, "Runs global Taskfile, from $HOME/{T,t}askfile.{yml,yaml}.")

	pflag.BoolVarP(&ForceAll, "force", "f", false, "Forces execution even when the task is up-to-date.")

	// Remote Taskfiles experiment will adds the "download" and "offline" flags
	if true {
		pflag.BoolVar(&Download, "download", false, "Downloads a cached version of a remote Taskfile.")
		pflag.BoolVar(&Offline, "offline", getConfig("REMOTE_OFFLINE", false), "Forces Task to only use local or cached Taskfiles.")
		pflag.StringSliceVar(&TrustedHosts, "trusted-hosts", getConfig("REMOTE_TRUSTED_HOSTS", []string{}), "List of trusted hosts for remote Taskfiles (comma-separated).")
		pflag.DurationVar(&Timeout, "timeout", getConfig("REMOTE_TIMEOUT", time.Second*10), "Timeout for downloading remote Taskfiles.")
		pflag.BoolVar(&ClearCache, "clear-cache", false, "Clear the remote cache.")
		pflag.DurationVar(&CacheExpiryDuration, "expiry", getConfig("REMOTE_CACHE_EXPIRY", time.Duration(0)), "Expiry duration for cached remote Taskfiles.")
		pflag.StringVar(&RemoteCacheDir, "remote-cache-dir", getConfig("REMOTE_CACHE_DIR", env.GetTaskEnv("REMOTE_DIR")), "Directory to cache remote Taskfiles.")
		pflag.StringVar(&CACert, "cacert", getConfig("REMOTE_CACERT", ""), "Path to a custom CA certificate for HTTPS connections.")
		pflag.StringVar(&Cert, "cert", getConfig("REMOTE_CERT", ""), "Path to a client certificate for HTTPS connections.")
		pflag.StringVar(&CertKey, "cert-key", getConfig("REMOTE_CERT_KEY", ""), "Path to a client certificate key for HTTPS connections.")
	}
	pflag.Parse()

	if VLevel > 0 {
		if VLevel > int(LevelTrace) {
			VLevel = int(LevelTrace)
		}
		Verbose = VerboseLevel(VLevel)
	}

	// Auto-detect color based on environment when not explicitly configured
	// Priority: CLI flag > TASK_COLOR env > taskrc config > NO_COLOR > FORCE_COLOR/CI > default
	colorExplicitlySet := pflag.Lookup("color").Changed || env.GetTaskEnv("COLOR") != ""
	if !colorExplicitlySet {
		if os.Getenv("NO_COLOR") != "" {
			Color = false
		} else if os.Getenv("FORCE_COLOR") != "" || isCI() {
			Color = true
		}
	}
}

func LogLevel(level VerboseLevel) slog.Level {
	switch level {
	case LevelNone:
		return slog.LevelWarn
	case LevelInfo:
		return slog.LevelInfo
	case LevelDebug:
		return slog.LevelDebug
	case LevelTrace:
		return rlog.LevelTrace
	default:
		return slog.LevelWarn
	}
}

// isCI returns true if running in a CI environment
func isCI() bool {
	ci, _ := strconv.ParseBool(os.Getenv("CI"))
	return ci
}

func Validate() error {
	if Download && Offline {
		return errors.New("task: You can't set both --download and --offline flags")
	}

	if Download && ClearCache {
		return errors.New("task: You can't set both --download and --clear-cache flags")
	}

	if Global && Dir != "" {
		return errors.New("task: You can't set both --global and --dir")
	}

	if Output.Name != "group" {
		if Output.Group.Begin != "" {
			return errors.New("task: You can't set --output-group-begin without --output=group")
		}
		if Output.Group.End != "" {
			return errors.New("task: You can't set --output-group-end without --output=group")
		}
		if Output.Group.ErrorOnly {
			return errors.New("task: You can't set --output-group-error-only without --output=group")
		}
	}

	if List && ListAll {
		return errors.New("task: cannot use --list and --list-all at the same time")
	}

	if ListJson && !List && !ListAll {
		return errors.New("task: --json only applies to --list or --list-all")
	}

	if NoStatus && !ListJson {
		return errors.New("task: --no-status only applies to --json with --list or --list-all")
	}

	if Nested && !ListJson {
		return errors.New("task: --nested only applies to --json with --list or --list-all")
	}

	// Validate certificate flags
	if (Cert != "" && CertKey == "") || (Cert == "" && CertKey != "") {
		return errors.New("task: --cert and --cert-key must be provided together")
	}

	return nil
}

// WithFlags is a special internal functional option that is used to pass flags
// from the CLI into any constructor that accepts functional options.
func WithFlags() task.ExecutorOption {
	return &flagsOption{}
}

type flagsOption struct{}

func (o *flagsOption) ApplyToExecutor(e *task.Executor) {
	// Set the sorter
	var sorter sort.Sorter
	switch TaskSort {
	case "none":
		sorter = sort.NoSort
	case "alphanumeric":
		sorter = sort.AlphaNumeric
	}

	// Change the directory to the user's home directory if the global flag is set
	dir := Dir
	if Global {
		home, err := os.UserHomeDir()
		if err == nil {
			dir = home
		}
	} else {
		if len(dir) > 0 {
			// Use the cli provided directory.
			if d, err := filepath.Abs(dir); err == nil {
				dir = d
			}
		}
	}

	e.Options(
		task.WithDir(dir),
		task.WithEntrypoint(Entrypoint),
		task.WithForce(Force),
		task.WithForceAll(ForceAll),
		task.WithInsecure(Insecure),
		task.WithDownload(Download),
		task.WithOffline(Offline),
		task.WithTrustedHosts(TrustedHosts),
		task.WithTimeout(Timeout),
		task.WithCacheExpiryDuration(CacheExpiryDuration),
		task.WithRemoteCacheDir(RemoteCacheDir),
		task.WithCACert(CACert),
		task.WithCert(Cert),
		task.WithCertKey(CertKey),
		task.WithWatch(Watch),
		task.WithDisableFuzzy(DisableFuzzy),
		task.WithAssumeYes(AssumeYes),
		task.WithInteractive(Interactive),
		task.WithDry(Dry || Status),
		task.WithSummary(Summary),
		task.WithParallel(Parallel),
		task.WithColor(Color),
		task.WithConcurrency(Concurrency),
		task.WithInterval(Interval),
		task.WithOutputStyle(Output),
		task.WithTaskSorter(sorter),
		task.WithVersionCheck(true),
		task.WithFailfast(Failfast),
		task.WithLogFormat(LogFormat),
	)
}

// getConfig extracts a config value with priority: env var > fallback
func getConfig[T any](envKey string, fallback T) T {
	if envKey != "" {
		if val, ok := getEnvAs[T](envKey); ok {
			return val
		}
	}
	return fallback
}

// getEnvAs parses a TASK_ prefixed env var as type T
func getEnvAs[T any](envKey string) (T, bool) {
	var zero T
	switch any(zero).(type) {
	case bool:
		if val, ok := env.GetTaskEnvBool(envKey); ok {
			return any(val).(T), true
		}
	case int:
		if val, ok := env.GetTaskEnvInt(envKey); ok {
			return any(val).(T), true
		}
	case time.Duration:
		if val, ok := env.GetTaskEnvDuration(envKey); ok {
			return any(val).(T), true
		}
	case string:
		if val, ok := env.GetTaskEnvString(envKey); ok {
			return any(val).(T), true
		}
	case []string:
		if val, ok := env.GetTaskEnvStringSlice(envKey); ok {
			return any(val).(T), true
		}
	}
	return zero, false
}
