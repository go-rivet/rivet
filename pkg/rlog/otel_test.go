package rlog

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"testing"
)

func TestOTelHandlerPreservesSlogRecord(t *testing.T) {
	var output bytes.Buffer
	logger := slog.New(newOTelHandler(&output, &slog.HandlerOptions{}))

	logger.Info("task finished", "task", "build", "duration", 42)

	var record map[string]any
	if err := json.Unmarshal(output.Bytes(), &record); err != nil {
		t.Fatalf("decode exported log record: %v", err)
	}
	body, ok := record["Body"].(map[string]any)
	if !ok || body["Value"] != "task finished" {
		t.Fatalf("body = %v, want %q", record["Body"], "task finished")
	}
	attributes, ok := record["Attributes"].([]any)
	if !ok {
		t.Fatalf("attributes = %T, want array", record["Attributes"])
	}
	values := make(map[string]any, len(attributes))
	for _, attribute := range attributes {
		entry := attribute.(map[string]any)
		value := entry["Value"].(map[string]any)
		values[entry["Key"].(string)] = value["Value"]
	}
	if got := values["task"]; got != "build" {
		t.Fatalf("task attribute = %v, want %q", got, "build")
	}
	if got := values["duration"]; got != float64(42) {
		t.Fatalf("duration attribute = %v, want %d", got, 42)
	}
}
