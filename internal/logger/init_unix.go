//go:build !windows

package logger

func initTerminal() {
	// No-op: Linux and macOS natively interpret ANSI strings out of the box
}
