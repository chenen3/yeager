package cmd

import "github.com/chenen3/yeager/cmd/command"

var Root = &command.Command{
	Name: "yeager",
	Desc: "Yeager is a tool for bypass network restriction",
	Do: func(self *command.Command) {
		self.Help()
	},
}
