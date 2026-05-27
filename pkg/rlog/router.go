package rlog

import (
	"context"
	"log/slog"
)

var (
	globalRouter *TestLogRouter
	isTestMode   bool
)

type logContextKey struct{}

var contextKey = logContextKey{}

type TestLogRouter struct {
	fallback slog.Handler
}

func SetTestMode() {
	isTestMode = true
}

func NewTestLogRouter(fallback slog.Handler) *TestLogRouter {
	return &TestLogRouter{
		fallback: fallback,
	}
}

func WithContext(ctx context.Context, h slog.Handler) context.Context {
	return context.WithValue(ctx, contextKey, h)
}

func (r *TestLogRouter) Enabled(ctx context.Context, lvl slog.Level) bool {
	if h, ok := ctx.Value(contextKey).(slog.Handler); ok {
		return h.Enabled(ctx, lvl)
	}
	return r.fallback.Enabled(ctx, lvl)
}

func (r *TestLogRouter) Handle(ctx context.Context, rec slog.Record) error {
	if h, ok := ctx.Value(contextKey).(slog.Handler); ok {
		return h.Handle(ctx, rec)
	}
	return r.fallback.Handle(ctx, rec)
}

func (r *TestLogRouter) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &TestLogRouter{
		fallback: r.fallback.WithAttrs(attrs),
	}
}

func (r *TestLogRouter) WithGroup(name string) slog.Handler {
	return &TestLogRouter{
		fallback: r.fallback.WithGroup(name),
	}
}
