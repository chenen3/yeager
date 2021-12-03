package cmd

import "testing"

func TestCommand_Execute(t *testing.T) {
	var (
		rootCmd Command
		b       bool
		c       bool
	)
	cmdA := &Command{
		Name: "a",
		Do: func(_ *Command) {
			c = true
		},
	}
	cmdA.Flags().BoolVar(&b, "b", false, "fake flag b")
	rootCmd.AddCommand(cmdA)
	rootCmd.SetArgs([]string{"a", "-b"})

	if err := rootCmd.Execute(); err != nil {
		t.Error(err)
	}
	if !b {
		t.Errorf("failed to parse sub command flag")
	}
	if !c {
		t.Errorf("failed to execute sub command Do()")
	}
}
