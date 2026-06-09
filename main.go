package main

import (
	"os"

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
	fi, _ := os.Stdout.Stat()
	return fi != nil && (fi.Mode()&os.ModeCharDevice) == 0
}
