package main

import (
	"fmt"

	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(versionCmd)
}

var Version string
var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "print yeager version",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Printf("yeager version %s\n", Version)
	},
}
