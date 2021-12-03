package main

import (
	"fmt"

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
			fmt.Println(`ERROR: required flag(s) "host" not set`)
			self.PrintUsage()
			return
		}

		_, err := util.GenerateCertificate(host, true)
		if err != nil {
			fmt.Println("failed to generate certificate, err: ", err)
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
