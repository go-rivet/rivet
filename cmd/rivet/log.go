package main

import (
	"log/slog"
	"strings"

	"github.com/go-rivet/rivet/pkg/rlog"
)

type VerboseLevel int

const (
	LevelNone  VerboseLevel = 0
	LevelInfo  VerboseLevel = 1 // Maps to -v     or --verbose=info
	LevelDebug VerboseLevel = 2 // Maps to -vv    or --verbose=debug
	LevelTrace VerboseLevel = 3 // Maps to -vvv   or --verbose=trace
)

func (vl VerboseLevel) String() string {
	switch vl {
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
