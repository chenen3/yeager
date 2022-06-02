package config

import (
	"reflect"
	"testing"
)

var jsons = `
{
	"debug": true,
	"verbose": true,
	"inbounds": {
		"socks": {
			"listen": ":1081"
		},
		"http": {
			"listen": ":8081"
		},
		"yeager": {
			"listen": "127.0.0.1:10812",
			"transport": "grpc",
			"mtls": {
				"certFile": "/usr/local/etc/yeager/server-cert.pem",
				"keyFile": "/usr/local/etc/yeager/server-key.pem",
				"caFile": "/usr/local/etc/yeager/ca-cert.pem"
			}
		}
	},
	"outbounds": [
		{
			"tag": "PROXY",
			"address": "127.0.0.1:9000",
			"transport": "grpc",
			"mtls": {
				"certFile": "/usr/local/etc/yeager/client-cert.pem",
				"keyFile": "/usr/local/etc/yeager/client-key.pem",
				"caFile": "/usr/local/etc/yeager/ca-cert.pem"
			},
			"connectionPoolSize": 3
		}
	],
	"rules": [
		"FINAL,PROXY"
	]
}
`
var yamls = `
debug: true
verbose: true
inbounds:
  socks:
    listen: ":1081"
  http:
    listen: ":8081"
  yeager:
    listen: 127.0.0.1:10812
    transport: grpc
    mtls:
      certFile: /usr/local/etc/yeager/server-cert.pem
      keyFile: /usr/local/etc/yeager/server-key.pem
      caFile: /usr/local/etc/yeager/ca-cert.pem

outbounds:
  - tag: PROXY
    address: 127.0.0.1:9000
    transport: grpc
    mtls:
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
			name: "json",
			bs:   []byte(jsons),
			want: Config{
				Inbounds: Inbounds{
					SOCKS: &SOCKS{Listen: ":1081"},
					HTTP:  &HTTP{Listen: ":8081"},
					Yeager: &YeagerServer{
						Listen:    "127.0.0.1:10812",
						Transport: "grpc",
						MutualTLS: MutualTLS{
							CertFile: "/usr/local/etc/yeager/server-cert.pem",
							KeyFile:  "/usr/local/etc/yeager/server-key.pem",
							CAFile:   "/usr/local/etc/yeager/ca-cert.pem",
						},
					},
				},
				Outbounds: []YeagerClient{
					{
						Tag:       "PROXY",
						Address:   "127.0.0.1:9000",
						Transport: "grpc",
						MutualTLS: MutualTLS{
							CertFile: "/usr/local/etc/yeager/client-cert.pem",
							KeyFile:  "/usr/local/etc/yeager/client-key.pem",
							CAFile:   "/usr/local/etc/yeager/ca-cert.pem",
						},
						ConnectionPoolSize: 3,
					},
				},
				Rules:   []string{"FINAL,PROXY"},
				Verbose: true,
				Debug:   true,
			},
		},
		{
			name: "yaml",
			bs:   []byte(yamls),
			want: Config{
				Inbounds: Inbounds{
					SOCKS: &SOCKS{Listen: ":1081"},
					HTTP:  &HTTP{Listen: ":8081"},
					Yeager: &YeagerServer{
						Listen:    "127.0.0.1:10812",
						Transport: "grpc",
						MutualTLS: MutualTLS{
							CertFile: "/usr/local/etc/yeager/server-cert.pem",
							KeyFile:  "/usr/local/etc/yeager/server-key.pem",
							CAFile:   "/usr/local/etc/yeager/ca-cert.pem",
						},
					},
				},
				Outbounds: []YeagerClient{
					{
						Tag:       "PROXY",
						Address:   "127.0.0.1:9000",
						Transport: "grpc",
						MutualTLS: MutualTLS{
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
				if !reflect.DeepEqual(want.Inbounds.HTTP, got.Inbounds.HTTP) {
					t.Errorf("want inbound http: %+v, got %+v", want.Inbounds.HTTP, got.Inbounds.HTTP)
					return
				}
				if !reflect.DeepEqual(want.Inbounds.SOCKS, got.Inbounds.SOCKS) {
					t.Errorf("want inbound socks: %+v, got %+v", want.Inbounds.SOCKS, got.Inbounds.SOCKS)
					return
				}
				if !reflect.DeepEqual(want.Inbounds.Yeager, got.Inbounds.Yeager) {
					t.Errorf("want inbound yeager: %+v, \ngot %+v", want.Inbounds.Yeager, got.Inbounds.Yeager)
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
