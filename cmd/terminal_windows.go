//go:build windows

package cmd

import (
	"syscall"
	"unsafe"
)

func init() {
	kernel32 := syscall.NewLazyDLL("kernel32.dll")
	getStdHandle := kernel32.NewProc("GetStdHandle")
	getConsoleMode := kernel32.NewProc("GetConsoleMode")
	setConsoleMode := kernel32.NewProc("SetConsoleMode")

	h, _, _ := getStdHandle.Call(uintptr(0xFFFFFFF5))
	if h == 0 || h == uintptr(syscall.InvalidHandle) {
		return
	}
	var mode uint32
	ret, _, _ := getConsoleMode.Call(h, uintptr(unsafe.Pointer(&mode)))
	if ret == 0 {
		return
	}
	setConsoleMode.Call(h, uintptr(mode|0x0004))
}
