/*
	Package command provide simple API to create modern command-line interface.
	The package is similar to https://github.com/spf13/cobra, cobra is very
	powerful but maybe a little bit too much for this small project.
	杀鸡焉用牛刀
*/
package command

import (
	"flag"
	"fmt"
	"os"
	"strings"
)

type Command struct {
	Name string
	Desc string              // description
	Do   func(self *Command) // specified what the command do

	flags    *flag.FlagSet
	commands []*Command
	args     []string
}

func (c *Command) AddCommand(cmd *Command) {
	c.commands = append(c.commands, cmd)
}

func (c *Command) Flags() *flag.FlagSet {
	if c.flags == nil {
		c.flags = flag.NewFlagSet(c.Name, flag.ContinueOnError)
	}
	return c.flags
}

// input args like:
//     cert -host 127.0.0.1
func extractSubCmd(args []string) (subCmdName string, subCmdArgs []string) {
	if len(args) == 0 {
		return "", nil
	}

	subCmdName, subCmdArgs = args[0], args[1:]
	if strings.HasPrefix(subCmdName, "-") {
		return "", nil
	}

	return subCmdName, subCmdArgs
}

// SetArgs set the args for command.
// If not set, os.Args[1:] by default
func (c *Command) SetArgs(args []string) {
	if args == nil {
		args = []string{}
	}
	c.args = args
}

// Execute use args (default is os.Args[1:])
// to find the matching sub-command and run it.
func (c *Command) Execute() error {
	if c.args == nil {
		c.SetArgs(os.Args[1:])
	}
	subCmdName, subCmdArgs := extractSubCmd(c.args)
	if subCmdName == "" {
		// no sub command in args
		var help bool
		if f := c.Flags().Lookup("help"); f == nil {
			c.Flags().BoolVar(&help, "help", false, "help for "+c.Name)
		}
		err := c.Flags().Parse(c.args)
		if err != nil {
			return err
		}

		if help {
			c.Help()
		} else {
			c.Do(c)
		}
		return nil
	}

	for _, cmd := range c.commands {
		if cmd.Name == subCmdName {
			cmd.SetArgs(subCmdArgs)
			return cmd.Execute()
		}
	}
	fmt.Printf("unsupported sub command: %s\n\n", subCmdName)
	c.PrintUsage()
	return nil
}

func (c *Command) PrintUsage() {
	s := "Usage:\n"
	s += fmt.Sprintf("  %s [flags]\n", c.Name)
	if c.commands != nil {
		s += fmt.Sprintf("  %s [command]\n", c.Name)
		s += "\n"
		s += fmt.Sprintf("Commands:\n")
		for _, cmd := range c.commands {
			// Four spaces before the tab triggers good alignment
			// for both 4- and 8-space tab stops.
			s += fmt.Sprintf("  %s    \t%s\n", cmd.Name, cmd.Desc)
		}
	}

	s += "\n"
	s += "Flags:\n"
	fmt.Fprint(c.Flags().Output(), s)
	c.Flags().PrintDefaults()

	if c.commands != nil {
		fmt.Fprintf(c.Flags().Output(), "\nUse \"%s [command] --help\" for more information about a command.\n", c.Name)
	}
}

func (c *Command) Help() {
	fmt.Fprintf(flag.CommandLine.Output(), "%s\n\n", c.Desc)
	c.PrintUsage()
}
