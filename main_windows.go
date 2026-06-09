//go:build windows

package main

import (
	"os"
	"syscall"
	"unsafe"
)

func isDoubleClick() bool {
	if len(os.Args) > 1 {
		return false
	}
	kernel32 := syscall.NewLazyDLL("kernel32.dll")
	getStdHandle := kernel32.NewProc("GetStdHandle")
	getConsoleMode := kernel32.NewProc("GetConsoleMode")

	// STD_OUTPUT_HANDLE = -11 (0xFFFFFFF5)
	h, _, _ := getStdHandle.Call(uintptr(0xFFFFFFF5))
	if h == 0 || h == uintptr(syscall.InvalidHandle) {
		return true
	}
	var mode uint32
	ret, _, _ := getConsoleMode.Call(h, uintptr(unsafe.Pointer(&mode)))
	return ret == 0
}
