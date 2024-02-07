package config

import (
	"encoding/json"
	"reflect"
	"testing"
)

var rawConf = `
{
	"listen": [
		{
			"protocol": "grpc",
			"address": "127.0.0.1:10812",
			"cert_file": "/path/to/server-cert.pem",
			"key_file": "/path/to/server-key.pem",
			"ca_file": "/path/to/ca-cert.pem"
		}
	],
	"transport": {
		"protocol": "grpc",
		"address": "127.0.0.1:9000",
		"cert_file": "/path/to/client-cert.pem",
		"key_file": "/path/to/client-key.pem",
		"ca_file": "/path/to/ca-cert.pem",
		"ca_pem": [
			"-----BEGIN CERTIFICATE-----",
			"MIIBqTCCAU6gAwIBAgIRAJxLfwUAHU2937LQPprCcXwwCgYIKoZIzj0EAwIwJDEQ",
			"MA4GA1UEChMHQWNtZSBDbzEQMA4GA1UEAxMHUm9vdCBDQTAeFw0yMjA2MDIwMzQ5",
			"MjlaFw0yMzA2MDIwMzQ5MjlaMCQxEDAOBgNVBAoTB0FjbWUgQ28xEDAOBgNVBAMT",
			"B1Jvb3QgQ0EwWTATBgcqhkjOPQIBBggqhkjOPQMBBwNCAARu+wIuQiV+xDNRqtxH",
			"3lnSMh3K3sCoBjUwc5zrWHwaHuIKngAw9wk/gyb1lIzMdJA1hKneNv5+EqOxKbJO",
			"uwRio2EwXzAOBgNVHQ8BAf8EBAMCAgQwHQYDVR0lBBYwFAYIKwYBBQUHAwEGCCsG",
			"AQUFBwMCMA8GA1UdEwEB/wQFMAMBAf8wHQYDVR0OBBYEFLbsUXap4IC9bgkxjcc8",
			"eJTckgWQMAoGCCqGSM49BAMCA0kAMEYCIQDRq8M7FRrZuJRBkKoaT4NyANX0TXM+",
			"9CSvf08poZFV5wIhAIl57HSDW2ZjOwHytOMdhVtuIZh8H17jbSHEBoviv+Tl",
			"-----END CERTIFICATE-----"
		]
	},
	"socks_proxy": "127.0.0.1:1081",
	"http_proxy": "127.0.0.1:8081"
}
`

var testCAPEM = `
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
`

func TestConfig(t *testing.T) {
	want := Config{
		Listen: []Transport{
			{
				Protocol: "grpc",
				Address:  "127.0.0.1:10812",
				CertFile: "/path/to/server-cert.pem",
				KeyFile:  "/path/to/server-key.pem",
				CAFile:   "/path/to/ca-cert.pem",
			},
		},
		Transport: Transport{
			Protocol: "grpc",
			Address:  "127.0.0.1:9000",
			CertFile: "/path/to/client-cert.pem",
			KeyFile:  "/path/to/client-key.pem",
			CAFile:   "/path/to/ca-cert.pem",
			CAPEM:    splitLines(testCAPEM),
		},
		SOCKSProxy: "127.0.0.1:1081",
		HTTPProxy:  "127.0.0.1:8081",
	}
	var got Config
	if err := json.Unmarshal([]byte(rawConf), &got); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(want, got) {
		t.Fatalf("\nwant %#v, \ngot %#v", want, got)
	}
}
