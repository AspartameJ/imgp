//go:build !windows

package main

import "os"

func isDoubleClick() bool {
	if len(os.Args) > 1 {
		return false
	}
	fiOut, _ := os.Stdout.Stat()
	if fiOut == nil || (fiOut.Mode()&os.ModeCharDevice) != 0 {
		return false
	}
	fiIn, _ := os.Stdin.Stat()
	return fiIn != nil && (fiIn.Mode()&os.ModeCharDevice) == 0
}
