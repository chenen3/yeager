package main

import (
	"fmt"
	"os"

	"github.com/chenen3/yeager/cmd"
)

func main() {
	err := cmd.Root.Execute()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
