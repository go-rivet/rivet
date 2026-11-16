// SPDX-FileCopyrightText: 2026 Timothy Rule
// SPDX-License-Identifier: MIT

//go:build cgo

package lua

import (
	"sync"
	"testing"
)

func TestRuntimeClose(t *testing.T) {
	runtime, err := NewRuntime()
	if err != nil {
		t.Fatal(err)
	}

	var closers sync.WaitGroup
	for range 8 {
		closers.Add(1)
		go func() {
			defer closers.Done()
			runtime.Close()
		}()
	}
	closers.Wait()

	if runtime.master != nil {
		t.Fatal("Lua state was not closed")
	}
}
