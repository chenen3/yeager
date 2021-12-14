package yeager

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"testing"
	"time"

	"github.com/chenen3/yeager/config"
	"github.com/chenen3/yeager/util"
)

func TestYeager(t *testing.T) {
	port, err := util.ChoosePort()
	if err != nil {
		t.Fatal(err)
	}
	addr := fmt.Sprintf("127.0.0.1:%d", port)
	uuid := "ce9f7ded-027c-e7b3-9369-308b7208d498"

	certInfo, err := util.GenerateCertificate("127.0.0.1", false)
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name       string
		serverConf *config.YeagerServer
		clientConf *config.YeagerClient
	}{
		{
			name: "tcp-tls",
			serverConf: &config.YeagerServer{
				Listen:    addr,
				UUID:      uuid,
				Transport: config.TransTCP,
				Security:  config.TLS,
				TLS:       config.Tls{CertPEM: certInfo.ServerCert, KeyPEM: certInfo.ServerKey},
			},
			clientConf: &config.YeagerClient{
				Address:   addr,
				UUID:      uuid,
				Transport: config.TransTCP,
				Security:  config.ClientTLS,
				TLS:       config.ClientTls{Insecure: true},
			},
		},
		{
			name: "tcp-mtls",
			serverConf: &config.YeagerServer{
				Listen:    addr,
				Transport: config.TransTCP,
				Security:  config.TLSMutual,
				MTLS: config.Mtls{
					CertPEM:  certInfo.ServerCert,
					KeyPEM:   certInfo.ServerKey,
					ClientCA: certInfo.RootCert,
				},
			},
			clientConf: &config.YeagerClient{
				Address:   addr,
				Transport: config.TransTCP,
				Security:  config.ClientTLSMutual,
				MTLS: config.ClientMTLS{
					CertPEM: certInfo.ClientCert,
					KeyPEM:  certInfo.ClientKey,
					RootCA:  certInfo.RootCert,
				},
			},
		},
		{
			name: "grpc-plaintext",
			serverConf: &config.YeagerServer{
				Listen:    addr,
				UUID:      uuid,
				Transport: config.TransGRPC,
				Security:  config.NoSecurity,
			},
			clientConf: &config.YeagerClient{
				Address:   addr,
				UUID:      uuid,
				Transport: config.TransGRPC,
				Security:  config.ClientNoSecurity,
			},
		},
		{
			name: "grpc-mtls",
			serverConf: &config.YeagerServer{
				Listen:    fmt.Sprintf("127.0.0.1:%d", port),
				Transport: config.TransGRPC,
				Security:  config.TLSMutual,
				MTLS: config.Mtls{
					CertPEM:  certInfo.ServerCert,
					KeyPEM:   certInfo.ServerKey,
					ClientCA: certInfo.RootCert,
				},
			},
			clientConf: &config.YeagerClient{
				Address:   addr,
				UUID:      uuid,
				Transport: config.TransGRPC,
				Security:  config.ClientTLSMutual,
				MTLS: config.ClientMTLS{
					CertPEM: certInfo.ClientCert,
					KeyPEM:  certInfo.ClientKey,
					RootCA:  certInfo.RootCert,
				},
			},
		},
		{
			name: "quic",
			serverConf: &config.YeagerServer{
				Listen:    addr,
				UUID:      uuid,
				Transport: config.TransQUIC,
				Security:  config.TLS,
				TLS:       config.Tls{CertPEM: certInfo.ServerCert, KeyPEM: certInfo.ServerKey},
			},
			clientConf: &config.YeagerClient{
				Address:   addr,
				UUID:      uuid,
				Transport: config.TransQUIC,
				Security:  config.ClientTLS,
				TLS:       config.ClientTls{Insecure: true},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server, err := NewServer(test.serverConf)
			if err != nil {
				t.Error(err)
				return
			}
			defer server.Close()

			go func() {
				err := server.ListenAndServe(func(ctx context.Context, conn net.Conn, network, addr string) {
					defer conn.Close()
					if addr != "fake.domain.com:1234" {
						t.Errorf("received unexpected dst addr: %s", addr)
						return
					}
					io.Copy(conn, conn)
				})
				if err != nil {
					t.Error(err)
				}
			}()
			<-server.ready

			client, err := NewClient(test.clientConf)
			if err != nil {
				t.Error("NewClient err: " + err.Error())
				return
			}
			defer client.Close()

			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			conn, err := client.DialContext(ctx, "tcp", "fake.domain.com:1234")
			if err != nil {
				t.Error("dial err: " + err.Error())
				return
			}
			defer conn.Close()

			want := []byte("1")
			_, err = conn.Write(want)
			if err != nil {
				t.Error(err)
				return
			}
			got := make([]byte, 1)
			io.ReadFull(conn, got)
			if !bytes.Equal(want, got) {
				t.Errorf("want %v, got %v", want, got)
				return
			}
		})
	}
}
