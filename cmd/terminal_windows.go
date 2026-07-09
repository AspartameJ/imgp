//go:build windows

package cmd

import (
	"syscall"
	"unsafe"
)

const (
	stdoutHandle = ^uintptr(11) + 1 // -11 = STD_OUTPUT_HANDLE
	stderrHandle = ^uintptr(12) + 1 // -12 = STD_ERROR_HANDLE
)

func init() {
	kernel32 := syscall.NewLazyDLL("kernel32.dll")
	getStdHandle := kernel32.NewProc("GetStdHandle")
	getConsoleMode := kernel32.NewProc("GetConsoleMode")
	setConsoleMode := kernel32.NewProc("SetConsoleMode")
	for _, h := range []uintptr{stdoutHandle, stderrHandle} {
		fd, _, _ := getStdHandle.Call(h)
		if fd == 0 || fd == uintptr(syscall.InvalidHandle) {
			continue
		}
		var mode uint32
		ret, _, _ := getConsoleMode.Call(fd, uintptr(unsafe.Pointer(&mode)))
		if ret == 0 {
			continue
		}
		setConsoleMode.Call(fd, uintptr(mode|0x0004)) // best-effort; failure is non-critical
	}
}
