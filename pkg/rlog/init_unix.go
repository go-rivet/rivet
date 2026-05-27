//go:build !windows

package rlog

func initTerminal() {
	// No-op: Linux and macOS natively interpret ANSI strings out of the box
}
