package rlog

import (
	"io"
	"log/slog"
)

type RlogHandler struct {
	slog.Handler
	Stdout io.Writer
	Stderr io.Writer
}
