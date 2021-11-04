package main

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/chenen3/yeager/util"
)

var host string

func init() {
	rootCmd.AddCommand(certCmd)
	certCmd.Flags().StringVar(&host, "host", "", "Comma-separated hostnames and IPs to generate a certificate for")
	certCmd.MarkFlagRequired("host")
}

var certCmd = &cobra.Command{
	Use:   "cert",
	Short: "Generate certificates for mutual TLS",
	Run: func(cmd *cobra.Command, args []string) {
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
