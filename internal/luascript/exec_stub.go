// SPDX-FileCopyrightText: 2026 Timothy Rule
// SPDX-License-Identifier: MIT

//go:build !cgo

package luascript

import (
	"context"
	"errors"
)

func Run(_ context.Context, opts *Options) error {
	if opts == nil {
		return errors.New("lua: nil options")
	}
	return ErrCGODisabled
}
