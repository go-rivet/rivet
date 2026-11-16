// SPDX-FileCopyrightText: 2026 Timothy Rule
// SPDX-License-Identifier: MIT

//go:build !cgo

package lua

import (
	"unsafe"
)

type Runtime struct{}

func NewRuntime() (*Runtime, error) {
	return nil, ErrCGODisabled
}

func (r *Runtime) Close() {}

func (r *Runtime) WithState(func(unsafe.Pointer) error) error {
	return ErrCGODisabled
}
