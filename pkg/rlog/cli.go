package rlog

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"strings"
	"sync"
)

const (
	colorReset  = "\033[0m"
	colorRed    = "\033[31m"
	colorYellow = "\033[33m"
	colorTrace  = "\033[36m"
)

type CliHandler struct {
	opts  *slog.HandlerOptions
	out   io.Writer
	err   io.Writer
	color bool
	mu    *sync.RWMutex // Protects attrs
	attrs []slog.Attr
}

func NewCliHandler(out io.Writer, err io.Writer, color bool, opts *slog.HandlerOptions) *CliHandler {
	if opts == nil {
		opts = &slog.HandlerOptions{}
	}
	return &CliHandler{
		opts:  opts,
		out:   out,
		err:   err,
		color: color,
		mu:    &sync.RWMutex{},
	}
}

func (h *CliHandler) Enabled(ctx context.Context, level slog.Level) bool {
	if h == nil || h.opts == nil {
		return level >= slog.LevelInfo
	}
	if h.opts.Level != nil {
		return level >= h.opts.Level.Level()
	}
	return level >= slog.LevelInfo
}

func (h *CliHandler) Handle(ctx context.Context, r slog.Record) error {
	if r.Level == LevelTask {
		return nil
	}
	h.mu.Lock()
	defer h.mu.Unlock()

	var startColor string
	if h.color {
		switch {
		case r.Level >= slog.LevelError:
			startColor = colorRed
		case r.Level >= slog.LevelWarn:
			startColor = colorYellow
		case r.Level == LevelTrace:
			startColor = colorTrace
		}
	}

	var w = h.err
	switch r.Level {
	case LevelRawOut:
		w = h.out
	case LevelDirectOut:
		w = h.out
	}

	var buf bytes.Buffer
	var log []byte
	if r.Level == LevelRawOut {
		buf.WriteString(r.Message)
	} else {
		buf.WriteString(strings.TrimSuffix(r.Message, "\n"))
	}
	if startColor != "" {
		log = append([]byte(startColor), buf.Bytes()...)
		log = append(log, []byte(colorReset)...)
	} else {
		log = buf.Bytes()
	}
	if r.Level != LevelRawOut {
		log = append(log, '\n')
	}
	_, err := w.Write(log)
	return err
}

func (h *CliHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	if h == nil {
		return h
	}
	h.mu.Lock()
	newAttrs := append(append([]slog.Attr{}, h.attrs...), attrs...)
	h.mu.Unlock()
	return &CliHandler{
		opts:  h.opts,
		out:   h.out,
		err:   h.err,
		mu:    h.mu,
		attrs: newAttrs,
	}
}

func (h *CliHandler) WithGroup(name string) slog.Handler {
	return h
}
