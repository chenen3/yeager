package cmd

import (
	"fmt"

	"github.com/chenen3/yeager/cmd/command"
)

// Version is assigned during compilation, for example:
// go build -ldflags="-X github.com/chenen3/yeager/cmd.Version=v0.1"
var Version string

func init() {
	Root.AddCommand(versionCmd)
}

var versionCmd = &command.Command{
	Name: "version",
	Desc: "print yeager version",
	Do: func(_ *command.Command) {
		fmt.Printf("yeager version %s\n", Version)
	},
}
