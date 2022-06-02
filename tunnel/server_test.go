package tunnel

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log"
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
			name: "tcp-plaintext",
			serverConf: &config.YeagerServer{
				Listen:    addr,
				Transport: config.TransTCP,
			},
			clientConf: &config.YeagerClient{
				Address:   addr,
				Transport: config.TransTCP,
			},
		},
		{
			name: "grpc-mtls",
			serverConf: &config.YeagerServer{
				Listen:    addr,
				Transport: config.TransGRPC,
				MutualTLS: config.MutualTLS{
					CertPEM: string(certInfo.ServerCert),
					KeyPEM:  string(certInfo.ServerKey),
					CAPEM:   string(certInfo.RootCert),
				},
			},
			clientConf: &config.YeagerClient{
				Address:   addr,
				Transport: config.TransGRPC,
				MutualTLS: config.MutualTLS{
					CertPEM: string(certInfo.ClientCert),
					KeyPEM:  string(certInfo.ClientKey),
					CAPEM:   string(certInfo.RootCert),
				},
			},
		},
		{
			name: "quic-mtls",
			serverConf: &config.YeagerServer{
				Listen:    addr,
				Transport: config.TransQUIC,
				MutualTLS: config.MutualTLS{
					CertPEM: string(certInfo.ServerCert),
					KeyPEM:  string(certInfo.ServerKey),
					CAPEM:   string(certInfo.RootCert),
				},
			},
			clientConf: &config.YeagerClient{
				Address:   addr,
				Transport: config.TransQUIC,
				MutualTLS: config.MutualTLS{
					CertPEM: string(certInfo.ClientCert),
					KeyPEM:  string(certInfo.ClientKey),
					CAPEM:   string(certInfo.RootCert),
				},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			srv, err := NewServer(test.serverConf)
			if err != nil {
				t.Error(err)
				return
			}
			defer func() {
				if er := srv.Shutdown(); er != nil {
					t.Error(er)
				}
			}()

			srv.Handle(func(ctx context.Context, conn net.Conn, addr string) {
				defer conn.Close()
				if addr != "fake.domain.com:1234" {
					panic("received unexpected dst addr: " + addr)
				}
				_, _ = io.Copy(conn, conn)
			})
			go func() {
				e := srv.ListenAndServe()
				if e != nil {
					log.Printf("yeager server exit: %s", err)
				}
			}()

			select {
			case <-time.After(time.Second):
				t.Fatalf("server not ready in time")
			case <-srv.ready:
			}

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
			_, err = io.ReadFull(conn, got)
			if err != nil {
				t.Error(err)
				return
			}
			if !bytes.Equal(want, got) {
				t.Errorf("want %v, got %v", want, got)
				return
			}
		})
	}
}
