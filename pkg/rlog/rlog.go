package rlog

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"runtime"
	"strings"
	"time"
)

const LevelRawOut = slog.Level(11)
const LevelDirectOut = slog.Level(12)
const LevelDirectErr = slog.Level(10)
const LevelTask = slog.Level(2)
const LevelTrace = slog.Level(-8)

type RlogOptions struct {
	Stdout    io.Writer
	Stderr    io.Writer
	Level     *slog.LevelVar
	AddSource bool
	Format    string
	Color     bool

	// AssumeYes  bool
	// AssumeTerm bool // Used for testing'
}

func Init(opts RlogOptions) {
	hOpts := &slog.HandlerOptions{
		Level:     opts.Level,
		AddSource: opts.AddSource,
	}

	log := &RlogHandler{
		Handler: func() slog.Handler {
			switch lf := strings.ToLower(opts.Format); lf {
			case "json":
				return slog.NewJSONHandler(opts.Stdout, hOpts)
			case "text":
				return slog.NewTextHandler(opts.Stdout, hOpts)
			default:
				return NewCliHandler(opts.Stdout, opts.Stderr, opts.Color, hOpts)
			}
		}(),
		Stdout: opts.Stdout,
		Stderr: opts.Stderr,
	}

	if isTestMode {
		globalRouter = NewTestLogRouter(log)
		slog.SetDefault(slog.New(globalRouter))
	} else {
		slog.SetDefault(slog.New(log))
	}
}

// Direct to stdout.
func OutRawf(ctx context.Context, color Color, format string, args ...any) {
	logf(ctx, LevelRawOut, format, args...)
}
func Outf(ctx context.Context, color Color, format string, args ...any) {
	logf(ctx, LevelDirectOut, format, args...)
}

// Direct to stderr.
func Errf(ctx context.Context, color Color, format string, args ...any) {
	logf(ctx, LevelDirectErr, format, args...)
}

// Normal operation, stderr.
func Error(ctx context.Context, msg string, args ...any) { log(ctx, slog.LevelError, msg, args...) }
func Errorf(ctx context.Context, format string, args ...any) {
	logf(ctx, slog.LevelError, format, args...)
}
func Warn(ctx context.Context, msg string, args ...any) { log(ctx, slog.LevelWarn, msg, args...) }
func Warnf(ctx context.Context, format string, args ...any) {
	logf(ctx, slog.LevelWarn, format, args...)
}

// --verbose, -v
func Task(ctx context.Context, msg string, args ...any)     { log(ctx, LevelTask, msg, args...) }
func Taskf(ctx context.Context, format string, args ...any) { logf(ctx, LevelTask, format, args...) }
func Info(ctx context.Context, msg string, args ...any)     { log(ctx, slog.LevelInfo, msg, args...) }
func Infof(ctx context.Context, format string, args ...any) {
	logf(ctx, slog.LevelInfo, format, args...)
}

// --verbose=2, -vv
func Debug(ctx context.Context, msg string, args ...any) { log(ctx, slog.LevelDebug, msg, args...) }
func Debugf(ctx context.Context, format string, args ...any) {
	logf(ctx, slog.LevelDebug, format, args...)
}

// --verbose=3, -vvv
func Trace(ctx context.Context, msg string, args ...any)     { log(ctx, LevelTrace, msg, args...) }
func Tracef(ctx context.Context, format string, args ...any) { logf(ctx, LevelTrace, format, args...) }

func log(ctx context.Context, level slog.Level, msg string, args ...any) {
	logger := slog.Default()
	if !logger.Enabled(ctx, level) {
		return
	}
	var pcs [1]uintptr
	runtime.Callers(3, pcs[:]) // Skip [runtime.Callers, log, Error/Warn/Info]
	r := slog.NewRecord(time.Now(), level, msg, pcs[0])
	r.Add(args...)
	_ = logger.Handler().Handle(ctx, r)
}

func logf(ctx context.Context, level slog.Level, format string, args ...any) {
	logger := slog.Default()
	if !logger.Enabled(ctx, level) {
		return
	}
	var pcs [1]uintptr
	runtime.Callers(3, pcs[:]) // Skip [runtime.Callers, logf, Errorf/Warnf/Infof]
	msg := fmt.Sprintf(format, args...)
	r := slog.NewRecord(time.Now(), level, msg, pcs[0])
	_ = logger.Handler().Handle(ctx, r)
}
