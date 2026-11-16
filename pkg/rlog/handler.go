// SPDX-FileCopyrightText: 2026 Timothy Rule
// SPDX-License-Identifier: MIT

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
