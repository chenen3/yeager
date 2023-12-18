package main

import (
	"encoding/json"
	"reflect"
	"testing"
)

var rawConf = `
{
	"listenSOCKS": "127.0.0.1:1081",
	"listenHTTP": "127.0.0.1:8081",
	"listen": [
		{
			"proto": "grpc",
			"address": "127.0.0.1:10812",
			"certFile": "/path/to/server-cert.pem",
			"keyFile": "/path/to/server-key.pem",
			"caFile": "/path/to/ca-cert.pem"
		}
	],
	"proxy": {
		"proto": "grpc",
		"address": "127.0.0.1:9000",
		"certFile": "/path/to/client-cert.pem",
		"keyFile": "/path/to/client-key.pem",
		"caFile": "/path/to/ca-cert.pem",
		"caPEM": [
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
	}
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
		ListenSOCKS: "127.0.0.1:1081",
		ListenHTTP:  "127.0.0.1:8081",
		Listen: []TransportConfig{
			{
				Proto:    "grpc",
				Address:  "127.0.0.1:10812",
				CertFile: "/path/to/server-cert.pem",
				KeyFile:  "/path/to/server-key.pem",
				CAFile:   "/path/to/ca-cert.pem",
			},
		},
		Proxy: TransportConfig{
			Proto:    "grpc",
			Address:  "127.0.0.1:9000",
			CertFile: "/path/to/client-cert.pem",
			KeyFile:  "/path/to/client-key.pem",
			CAFile:   "/path/to/ca-cert.pem",
			CAPEM:    splitLine(testCAPEM),
		},
	}
	var got Config
	if err := json.Unmarshal([]byte(rawConf), &got); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(want, got) {
		t.Fatalf("\nwant %#v, \ngot %#v", want, got)
	}
}
