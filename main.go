package main

import (
	"gitcode.com/DonaldTom/imgp/cmd"
)

func main() {
	if isDoubleClick() {
		cmd.StartGUI()
		return
	}
	cmd.Execute()
}
