package main

import (
	"fmt"
	"os"

	"github.com/chenen3/yeager/cmd"
	"github.com/chenen3/yeager/util"
)

var host string

func init() {
	rootCmd.AddCommand(certCmd)
	certCmd.Flags().StringVar(&host, "host", "", "comma-separated hostnames and IPs to generate a certificate for")
}

var certCmd = &cmd.Command{
	Name: "cert",
	Desc: "generate certificates for mutual TLS",
	Do: func(self *cmd.Command) {
		if host == "" {
			fmt.Fprint(os.Stderr, "ERROR: required flag \"-host\" not set\n\n")
			self.PrintUsage()
			return
		}

		_, err := util.GenerateCertificate(host, true)
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to generate certificate: %s\n", err)
			return
		}

		fmt.Printf("generate certificate: \n\t%s\n\t%s\n\t%s\n\t%s\n\t%s\n\t%s\n",
			util.CACertFile, util.CAKeyFile,
			util.ServerCertFile, util.ServerKeyFile,
			util.ClientCertFile, util.ClientKeyFile,
		)
		fmt.Printf("please copy %s, %s, and %s to client device\n",
			util.CACertFile, util.ClientCertFile, util.ClientKeyFile,
		)
	},
}
