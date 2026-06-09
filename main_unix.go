//go:build !windows

package main

import "os"

func isDoubleClick() bool {
	if len(os.Args) > 1 {
		return false
	}
	// On Unix, there's no GUI subsystem equivalent of -H=windowsgui,
	// so double-click behavior doesn't apply.
	fi, _ := os.Stdout.Stat()
	return fi != nil && (fi.Mode()&os.ModeCharDevice) == 0
}
