//go:build windows

package rlog

import (
	"os"
	"syscall"
)

func initTerminal() {
	handle := syscall.Handle(os.Stdout.Fd())
	var mode uint32
	if err := syscall.GetConsoleMode(handle, &mode); err == nil {
		// Enable ENABLE_VIRTUAL_TERMINAL_PROCESSING (0x0004)
		mode |= 0x0004
		_, _, _ = syscall.SyscallN(
			syscall.NewLazyDLL("kernel32.dll").NewProc("SetConsoleMode").Addr(),
			uintptr(handle),
			uintptr(mode),
		)
	}
}
