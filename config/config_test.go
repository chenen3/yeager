package config

import (
	"reflect"
	"testing"
)

var yamls = `
debug: true
verbose: true
socksListen: ":1081"
httpListen: ":8081"
inbounds:
- listen: 127.0.0.1:10812
  transport: grpc
  tls:
    certFile: /usr/local/etc/yeager/server-cert.pem
    keyFile: /usr/local/etc/yeager/server-key.pem
    caFile: /usr/local/etc/yeager/ca-cert.pem
- listen: 127.0.0.1:10813
  transport: tcp

outbounds:
  - tag: PROXY
    address: 127.0.0.1:9000
    transport: grpc
    tls:
      certFile: /usr/local/etc/yeager/client-cert.pem
      keyFile: /usr/local/etc/yeager/client-key.pem
      caFile: /usr/local/etc/yeager/ca-cert.pem
      caPEM: |-
        -----BEGIN CERTIFICATE-----
        MIIBqTCCAU6gAwIBAgIRAJxLfwUAHU2937LQPprCcXwwCgYIKoZIzj0EAwIwJDEQ
        MA4GA1UEChMHQWNtZSBDbzEQMA4GA1UEAxMHUm9vdCBDQTAeFw0yMjA2MDIwMzQ5
        MjlaFw0yMzA2MDIwMzQ5MjlaMCQxEDAOBgNVBAoTB0FjbWUgQ28xEDAOBgNVBAMT
        B1Jvb3QgQ0EwWTATBgcqhkjOPQIBBggqhkjOPQMBBwNCAARu+wIuQiV+xDNRqtxH
        3lnSMh3K3sCoBjUwc5zrWHwaHuIKngAw9wk/gyb1lIzMdJA1hKneNv5+EqOxKbJO
        uwRio2EwXzAOBgNVHQ8BAf8EBAMCAgQwHQYDVR0lBBYwFAYIKwYBBQUHAwEGCCsG
        AQUFBwMCMA8GA1UdEwEB/wQFMAMBAf8wHQYDVR0OBBYEFLbsUXap4IC9bgkxjcc8
        eJTckgWQMAoGCCqGSM49BAMCA0kAMEYCIQDRq8M7FRrZuJRBkKoaT4NyANX0TXM+
        9CSvf08poZFV5wIhAIl57HSDW2ZjOwHytOMdhVtuIZh8H17jbSHEBoviv+Tl
        -----END CERTIFICATE-----
    connectionPoolSize: 3

rules:
  - FINAL,PROXY
`

var caPEM = `-----BEGIN CERTIFICATE-----
MIIBqTCCAU6gAwIBAgIRAJxLfwUAHU2937LQPprCcXwwCgYIKoZIzj0EAwIwJDEQ
MA4GA1UEChMHQWNtZSBDbzEQMA4GA1UEAxMHUm9vdCBDQTAeFw0yMjA2MDIwMzQ5
MjlaFw0yMzA2MDIwMzQ5MjlaMCQxEDAOBgNVBAoTB0FjbWUgQ28xEDAOBgNVBAMT
B1Jvb3QgQ0EwWTATBgcqhkjOPQIBBggqhkjOPQMBBwNCAARu+wIuQiV+xDNRqtxH
3lnSMh3K3sCoBjUwc5zrWHwaHuIKngAw9wk/gyb1lIzMdJA1hKneNv5+EqOxKbJO
uwRio2EwXzAOBgNVHQ8BAf8EBAMCAgQwHQYDVR0lBBYwFAYIKwYBBQUHAwEGCCsG
AQUFBwMCMA8GA1UdEwEB/wQFMAMBAf8wHQYDVR0OBBYEFLbsUXap4IC9bgkxjcc8
eJTckgWQMAoGCCqGSM49BAMCA0kAMEYCIQDRq8M7FRrZuJRBkKoaT4NyANX0TXM+
9CSvf08poZFV5wIhAIl57HSDW2ZjOwHytOMdhVtuIZh8H17jbSHEBoviv+Tl
-----END CERTIFICATE-----`

func TestLoad(t *testing.T) {
	tests := []struct {
		name    string
		bs      []byte
		want    Config
		wantErr bool
	}{
		{
			name: "yaml",
			bs:   []byte(yamls),
			want: Config{
				SOCKSListen: ":1081",
				HTTPListen:  ":8081",
				Inbounds: []YeagerServer{
					{
						Listen:    "127.0.0.1:10812",
						Transport: "grpc",
						TLS: TLS{
							CertFile: "/usr/local/etc/yeager/server-cert.pem",
							KeyFile:  "/usr/local/etc/yeager/server-key.pem",
							CAFile:   "/usr/local/etc/yeager/ca-cert.pem",
						},
					},
					{
						Listen:    "127.0.0.1:10813",
						Transport: "tcp",
					},
				},
				Outbounds: []YeagerClient{
					{
						Tag:       "PROXY",
						Address:   "127.0.0.1:9000",
						Transport: "grpc",
						TLS: TLS{
							CertFile: "/usr/local/etc/yeager/client-cert.pem",
							KeyFile:  "/usr/local/etc/yeager/client-key.pem",
							CAFile:   "/usr/local/etc/yeager/ca-cert.pem",
							CAPEM:    caPEM,
						},
						ConnectionPoolSize: 3,
					},
				},
				Rules:   []string{"FINAL,PROXY"},
				Verbose: true,
				Debug:   true,
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := Load(test.bs)
			if test.wantErr != (err != nil) {
				t.Errorf("wantErr: %v, got error: %v", test.wantErr, err)
				return
			}
			want := test.want
			if !reflect.DeepEqual(want, got) {
				if !reflect.DeepEqual(want.HTTPListen, got.HTTPListen) {
					t.Errorf("want httpListen: %+v, got %+v", want.HTTPListen, got.HTTPListen)
					return
				}
				if !reflect.DeepEqual(want.SOCKSListen, got.SOCKSListen) {
					t.Errorf("want socksListen: %+v, got %+v", want.SOCKSListen, got.SOCKSListen)
					return
				}
				if !reflect.DeepEqual(want.Inbounds, got.Inbounds) {
					t.Errorf("want inbounds: %+v, \ngot: %+v", want.Inbounds, got.Inbounds)
					return
				}
				if !reflect.DeepEqual(want.Outbounds, got.Outbounds) {
					t.Errorf("want outbounds: %+v, \ngot: %+v", want.Outbounds, got.Outbounds)
					return
				}
				t.Errorf("want config %+v, got %+v", want, got)
				return
			}
		})
	}
}
