package rlog

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"math"
	"runtime"
	"sync"
	"time"
)

type otelHandler struct {
	opts   *slog.HandlerOptions
	out    io.Writer
	mu     *sync.Mutex
	attrs  []slog.Attr
	groups []string
}

func newOTelHandler(out io.Writer, opts *slog.HandlerOptions) slog.Handler {
	if opts == nil {
		opts = &slog.HandlerOptions{}
	}
	return &otelHandler{opts: opts, out: out, mu: &sync.Mutex{}}
}

func (h *otelHandler) Enabled(_ context.Context, level slog.Level) bool {
	if h.opts.Level == nil {
		return level >= slog.LevelInfo
	}
	return level >= h.opts.Level.Level()
}

func (h *otelHandler) Handle(ctx context.Context, record slog.Record) error {
	attributes := make([]slog.Attr, 0, len(h.attrs)+record.NumAttrs())
	attributes = append(attributes, h.attrs...)
	record.Attrs(func(attr slog.Attr) bool {
		attributes = append(attributes, attr)
		return true
	})

	value := map[string]any{
		"Timestamp":         record.Time,
		"ObservedTimestamp": time.Now(),
		"Severity":          int(record.Level) + 9,
		"SeverityText":      record.Level.String(),
		"Body":              otelValue(slog.StringValue(record.Message)),
		"Attributes":        h.attributes(attributes),
		"DroppedAttributes": 0,
	}
	if info, ok := TraceFromContext(ctx); ok {
		value["TraceID"] = info.TraceID
		value["SpanID"] = info.SpanID
		value["TraceFlags"] = "00"
		if info.ParentSpanID != "" {
			value["Attributes"] = append(value["Attributes"].([]map[string]any), map[string]any{
				"Key": "parent_span_id", "Value": otelValue(slog.StringValue(info.ParentSpanID)),
			})
		}
	}
	if h.opts.AddSource && record.PC != 0 {
		value["Attributes"] = append(value["Attributes"].([]map[string]any), map[string]any{
			"Key": "code.line.number", "Value": otelValue(slog.IntValue(sourceLine(record.PC))),
		})
	}

	h.mu.Lock()
	defer h.mu.Unlock()
	return json.NewEncoder(h.out).Encode(value)
}

func (h *otelHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	clone := *h
	clone.attrs = append(append([]slog.Attr{}, h.attrs...), attrs...)
	return &clone
}

func (h *otelHandler) WithGroup(name string) slog.Handler {
	if name == "" {
		return h
	}
	clone := *h
	clone.groups = append(append([]string{}, h.groups...), name)
	return &clone
}

func (h *otelHandler) attributes(attrs []slog.Attr) []map[string]any {
	result := make([]map[string]any, 0, len(attrs))
	for _, attr := range attrs {
		attr.Value = attr.Value.Resolve()
		if attr.Equal(slog.Attr{}) {
			continue
		}
		key := attr.Key
		if len(h.groups) > 0 {
			key = joinGroup(h.groups, key)
		}
		result = append(result, map[string]any{"Key": key, "Value": otelValue(attr.Value)})
	}
	return result
}

func otelValue(value slog.Value) map[string]any {
	switch value.Kind() {
	case slog.KindBool:
		return map[string]any{"Type": "BOOL", "Value": value.Bool()}
	case slog.KindDuration:
		return map[string]any{"Type": "INT64", "Value": value.Duration().Nanoseconds()}
	case slog.KindFloat64:
		return map[string]any{"Type": "FLOAT64", "Value": value.Float64()}
	case slog.KindInt64:
		return map[string]any{"Type": "INT64", "Value": value.Int64()}
	case slog.KindString:
		return map[string]any{"Type": "STRING", "Value": value.String()}
	case slog.KindTime:
		return map[string]any{"Type": "INT64", "Value": value.Time().UnixNano()}
	case slog.KindUint64:
		if value.Uint64() <= math.MaxInt64 {
			return map[string]any{"Type": "INT64", "Value": int64(value.Uint64())}
		}
		return map[string]any{"Type": "FLOAT64", "Value": float64(value.Uint64())}
	case slog.KindGroup:
		group := make(map[string]any, len(value.Group()))
		for _, attr := range value.Group() {
			group[attr.Key] = otelValue(attr.Value)
		}
		return map[string]any{"Type": "MAP", "Value": group}
	default:
		return map[string]any{"Type": "STRING", "Value": fmt.Sprint(value.Any())}
	}
}

func sourceLine(pc uintptr) int {
	frame, _ := runtime.CallersFrames([]uintptr{pc}).Next()
	return frame.Line
}

func joinGroup(groups []string, key string) string {
	result := ""
	for _, group := range groups {
		if result != "" {
			result += "."
		}
		result += group
	}
	if result != "" && key != "" {
		result += "."
	}
	return result + key
}
