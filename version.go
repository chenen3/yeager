package main

import (
	"fmt"

	"github.com/chenen3/yeager/cmd"
)

// Version is set at compile time, for example:
// go build -ldflags="-X main.Version=v0.1"
var Version string

func init() {
	rootCmd.AddCommand(versionCmd)
}

var versionCmd = &cmd.Command{
	Name: "version",
	Desc: "print yeager version",
	Do: func(_ *cmd.Command) {
		fmt.Printf("yeager version %s\n", Version)
	},
}
