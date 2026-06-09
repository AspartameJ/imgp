package main

import (
	"os"
	"runtime"
	"syscall"
	"unsafe"

	"gitcode.com/DonaldTom/imgp/cmd"
)

func main() {
	if isDoubleClick() {
		cmd.StartGUI()
		return
	}
	cmd.Execute()
}

func isDoubleClick() bool {
	if len(os.Args) > 1 {
		return false
	}
	if runtime.GOOS == "windows" {
		var mode uint32
		kernel32 := syscall.NewLazyDLL("kernel32.dll")
		getConsoleMode := kernel32.NewProc("GetConsoleMode")
		ret, _, _ := getConsoleMode.Call(uintptr(syscall.Stdout), uintptr(unsafe.Pointer(&mode)))
		return ret == 0
	}
	fi, _ := os.Stdout.Stat()
	return fi != nil && (fi.Mode()&os.ModeCharDevice) == 0
}
