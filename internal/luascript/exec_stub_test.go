//go:build !cgo

package luascript

import (
	"context"
	"errors"
	"testing"
)

func TestRunReportsCGODisabled(t *testing.T) {
	err := Run(context.Background(), &Options{Script: `return true`})
	if !errors.Is(err, ErrCGODisabled) {
		t.Fatalf("Run() error = %v, want %v", err, ErrCGODisabled)
	}
}
