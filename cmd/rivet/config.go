//go:generate go run ../../cmd/docgen cli
package main

import (
	"cmp"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/go-rivet/rivet/pkg/rivet/errors"
)

type Config struct {
	// Display/Help flags
	Version bool `noenv:"true" doc:"Show Task version."`
	Help    bool `sflag:"h" noenv:"true" doc:"Shows Task usage."`
	Init    bool `sflag:"i" noenv:"true" doc:"Creates a new Taskfile.yml in the current folder."`

	// List flags
	List     bool   `sflag:"l" noenv:"true" doc:"Lists tasks with description of current Taskfile."`
	ListAll  bool   `sflag:"a" noenv:"true" doc:"Lists tasks with or without a description."`
	ListJson bool   `flag:"json" sflag:"j" noenv:"true" doc:"Formats task list as JSON."`
	Sort     string `noenv:"true" doc:"Changes the order of the tasks when listed. [default|alphanumeric|none]."`
	Status   bool   `noenv:"true" doc:"Exits with non-zero exit code if any of the given tasks is not up-to-date."`
	NoStatus bool   `noenv:"true" doc:"Ignore status when listing tasks as JSON"`
	Nested   bool   `noenv:"true" doc:"Nest namespaces when listing tasks as JSON"`

	// Execution flags
	Parallel    bool `sflag:"p" noenv:"true" doc:"Executes tasks provided on command line in parallel."`
	Concurrency int  `sflag:"C" default:"0" doc:"Limit number of tasks to run concurrently."`
	Dry         bool `sflag:"n" doc:"Compiles and prints tasks in the order that they would be run, without executing them."`
	Summary     bool `noenv:"true" doc:"Show summary about a task."`
	ExitCode    bool `sflag:"x" noenv:"true" doc:"Pass-through the exit code of the task command."`
	Failfast    bool `sflag:"F" doc:"When running tasks in parallel, stop all tasks if one fails."`
	Force       bool `noenv:"true" doc:"Forces execution of a task even when up-to-date."`
	ForceAll    bool `sflag:"f" noenv:"true" doc:"Forces execution even when the task is up-to-date."`

	// Directory flags
	Dir        string `sflag:"d" noenv:"true" doc:"Sets the directory in which Task will execute and look for a Taskfile."`
	Entrypoint string `flag:"taskfile" sflag:"t" noenv:"true" doc:"Choose which Taskfile to run. Defaults to \"Taskfile.yml\"."`
	Global     bool   `sflag:"g" noenv:"true" doc:"Runs global Taskfile, from $HOME/{T,t}askfile.{yml,yaml}."`

	// Watch flags
	Watch    bool          `sflag:"w" noenv:"true" doc:"Enables watch of the given task."`
	Interval time.Duration `sflag:"I" noenv:"true" doc:"Interval to watch for changes."`

	// Logging flags
	Verbose   string `noenv:"true" doc:"Log verbosity level [info|debug|trace] or cumulative shorthand [-v|-vv|-vvv]"`
	V1        bool   `flag:"v" noenv:"true"`
	V2        bool   `flag:"vv" noenv:"true"`
	V3        bool   `flag:"vvv" noenv:"true"`
	LogFormat string `flag:"log" noenv:"true" doc:"Log format (\"otel\", \"json\", or \"text\")."`
	Color     bool   `sflag:"c" default:"true" doc:"Colored output. Enabled by default. Set flag to false or use NO_COLOR=1 to disable."`

	// Output flags
	Output               string `doc:"Output configuration."`
	OutputGroupBegin     string `flag:"group-begin" doc:"Group output beginning text."`
	OutputGroupEnd       string `flag:"group-end" doc:"Group output end text."`
	OutputGroupErrorOnly bool   `flag:"group-error-only" doc:"Print group output only on error."`

	// Interaction flags
	DisableFuzzy bool `doc:"Disables fuzzy matching for task names."`
	AssumeYes    bool `flag:"yes" sflag:"y" doc:"Assume \"yes\" as answer to all prompts."`
	Interactive  bool `doc:"Prompt for missing required variables."`

	// Remote Taskfile flags
	Download            bool          `noenv:"true" doc:"Downloads a cached version of a remote Taskfile."`
	Offline             bool          `doc:"Forces Task to only use local or cached Taskfiles."`
	Insecure            bool          `doc:"Forces Task to download Taskfiles over insecure connections."`
	TrustedHosts        string        `doc:"List of trusted hosts for remote Taskfiles (comma-separated)."`
	Timeout             time.Duration `default:"10s" doc:"Timeout for downloading remote Taskfiles."`
	ClearCache          bool          `noenv:"true" doc:"Clear the remote cache."`
	CacheExpiryDuration time.Duration `flag:"expiry" doc:"Expiry duration for cached remote Taskfiles."`
	RemoteCacheDir      string        `flag:"remote-cache" doc:"Directory to cache remote Taskfiles."`

	// TLS/Certificate flags
	CACert  string `flag:"cacert" doc:"Path to a custom CA certificate for HTTPS connections."`
	Cert    string `doc:"Path to a client certificate for HTTPS connections."`
	CertKey string `flag:"cert-key" doc:"Path to a client certificate key for HTTPS connections."`
}

func (c *Config) Adjust() {
	c.Dir = cmp.Or(c.Dir, filepath.Dir(c.Entrypoint))

	switch {
	case c.V3:
		c.Verbose = LevelTrace.String()
	case c.V2:
		c.Verbose = LevelDebug.String()
	case c.V1:
		c.Verbose = LevelInfo.String()
	}

	switch {
	case !c.Color: // Was set false by flag or envar.
	case os.Getenv("NO_COLOR") != "":
		c.Color = false
	case os.Getenv("FORCE_COLOR") != "":
		c.Color = true
	case os.Getenv("CI") == "true":
		c.Color = true
	}
}

func (c *Config) Validate() error {
	if c.Download && c.Offline {
		return errors.New("task: You can't set both --download and --offline flags")
	}

	if c.Download && c.ClearCache {
		return errors.New("task: You can't set both --download and --clear-cache flags")
	}

	if c.Global && c.Dir != "" {
		return errors.New("task: You can't set both --global and --dir")
	}

	if c.Output == "group" {
		if c.OutputGroupBegin != "" {
			return errors.New("task: You can't set --output-group-begin without --output=group")
		}
		if c.OutputGroupEnd != "" {
			return errors.New("task: You can't set --output-group-end without --output=group")
		}
		if c.OutputGroupErrorOnly {
			return errors.New("task: You can't set --output-group-error-only without --output=group")
		}
	}

	if c.List && c.ListAll {
		return errors.New("task: cannot use --list and --list-all at the same time")
	}
	if c.ListJson && !c.List && !c.ListAll {
		return errors.New("task: --json only applies to --list or --list-all")
	}
	if c.NoStatus && !c.ListJson {
		fmt.Println(c.NoStatus)
		fmt.Println(c.ListJson)
		return errors.New("task: --no-status only applies to --json with --list or --list-all")
	}
	if c.Nested && !c.ListJson {
		return errors.New("task: --nested only applies to --json with --list or --list-all")
	}

	if (c.Cert != "" && c.CertKey == "") || (c.Cert == "" && c.CertKey != "") {
		return errors.New("task: --cert and --cert-key must be provided together")
	}

	return nil
}
