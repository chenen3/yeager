package main

import (
	"fmt"
	"os"

	"github.com/chenen3/yeager/cmd"
)

var rootCmd = &cmd.Command{
	Name: "yeager",
	Desc: "Yeager is a tool for bypass network restriction",
	Do: func(self *cmd.Command) {
		self.Help()
	},
}

func main() {
	err := rootCmd.Execute()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
