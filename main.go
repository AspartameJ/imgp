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
		kernel32 := syscall.NewLazyDLL("kernel32.dll")
		getStdHandle := kernel32.NewProc("GetStdHandle")
		getConsoleMode := kernel32.NewProc("GetConsoleMode")

		// STD_OUTPUT_HANDLE = -11
		h, _, _ := getStdHandle.Call(uintptr(0xFFFFFFF5))
		if h == 0 || h == uintptr(syscall.InvalidHandle) {
			return true
		}
		var mode uint32
		ret, _, _ := getConsoleMode.Call(h, uintptr(unsafe.Pointer(&mode)))
		return ret == 0
	}
	fi, _ := os.Stdout.Stat()
	return fi != nil && (fi.Mode()&os.ModeCharDevice) == 0
}
