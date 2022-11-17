package config

import (
	"reflect"
	"testing"

	"gopkg.in/yaml.v3"
)

var rawConf = `verbose: true
socksListen: "127.0.0.1:1081"
httpListen: "127.0.0.1:8081"
tunnelListens:
  - type: grpc
    listen: 127.0.0.1:10812
    certFile: /path/to/server-cert.pem
    keyFile:  /path/to/server-key.pem
    caFile:  /path/to/ca-cert.pem
tunnelClients:
  - policy: proxy
    type: grpc
    address: 127.0.0.1:9000
    certFile:  /path/to/client-cert.pem
    keyFile:  /path/to/client-key.pem
    caFile:  /path/to/ca-cert.pem
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
  - final,proxy
debug: true
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

func TestConfig(t *testing.T) {
	want := Config{
		SOCKSListen: "127.0.0.1:1081",
		HTTPListen:  "127.0.0.1:8081",
		TunnelListens: []TunnelListen{
			{
				Type:     "grpc",
				Listen:   "127.0.0.1:10812",
				CertFile: "/path/to/server-cert.pem",
				KeyFile:  "/path/to/server-key.pem",
				CAFile:   "/path/to/ca-cert.pem",
			},
		},
		TunnelClients: []TunnelClient{
			{
				Policy:                "proxy",
				Type:               "grpc",
				Address:            "127.0.0.1:9000",
				CertFile:           "/path/to/client-cert.pem",
				KeyFile:            "/path/to/client-key.pem",
				CAFile:             "/path/to/ca-cert.pem",
				CAPEM:              caPEM,
				ConnectionPoolSize: 3,
			},
		},
		Rules:   []string{"final,proxy"},
		Verbose: true,
		Debug:   true,
	}

	var got Config
	err := yaml.Unmarshal([]byte(rawConf), &got)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(want, got) {
		t.Fatalf("\nwant %#v, \ngot %#v", want, got)
	}
}
