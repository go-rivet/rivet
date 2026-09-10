package rlog

import (
	"context"
	"crypto/rand"
	"encoding/hex"
)

type traceContextKey struct{}

// TraceInfo carries the trace and span identifiers for a running task.
type TraceInfo struct {
	TraceID      string
	SpanID       string
	ParentSpanID string
}

// WithTrace attaches trace and span identifiers to ctx for use by logging.
func WithTrace(ctx context.Context, traceID, spanID, parentSpanID string) context.Context {
	return context.WithValue(ctx, traceContextKey{}, TraceInfo{TraceID: traceID, SpanID: spanID, ParentSpanID: parentSpanID})
}

// TraceFromContext returns the trace info attached to ctx, if any.
func TraceFromContext(ctx context.Context) (TraceInfo, bool) {
	info, ok := ctx.Value(traceContextKey{}).(TraceInfo)
	return info, ok
}

// NewTraceID generates a random 16-byte trace ID, hex-encoded.
func NewTraceID() string {
	return randomHex(16)
}

// NewSpanID generates a random 8-byte span ID, hex-encoded.
func NewSpanID() string {
	return randomHex(8)
}

func randomHex(n int) string {
	b := make([]byte, n)
	// crypto/rand.Read does not fail on supported platforms; zero bytes on error.
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
