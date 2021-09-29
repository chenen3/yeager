package cmd

import (
	"fmt"

	"github.com/chenen3/yeager/util"
	"github.com/spf13/cobra"
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
	RunE: func(cmd *cobra.Command, args []string) error {
		_, err := util.GenerateCertificate(host, true)
		if err != nil {
			return err
		}

		fmt.Println("next please copy the files ca-cert.pem, client-cert.pem, client-key.pem to client device")
		return nil
	},
}
