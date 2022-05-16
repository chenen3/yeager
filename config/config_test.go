package config

import (
	"reflect"
	"strings"
	"testing"
)

func TestConfig(t *testing.T) {
	rawConf := `
	{
		"debug": true,
		"verbose": true,
		"connectionPoolSize": 3,
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
					"certFile": "/usr/local/etc/yeager/client-cert.pem",
					"keyFile": "/usr/local/etc/yeager/client-key.pem",
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
				}
			}
		],
		"rules": [
			"FINAL,PROXY"
		]
	}
	`
	want := Config{
		Inbounds: Inbounds{
			SOCKS: &socks{Listen: ":1081"},
			HTTP:  &http{Listen: ":8081"},
			Yeager: &YeagerServer{
				Listen:    "127.0.0.1:10812",
				Transport: "grpc",
				MutualTLS: MutualTLS{
					CertFile: "/usr/local/etc/yeager/client-cert.pem",
					KeyFile:  "/usr/local/etc/yeager/client-key.pem",
					CAFile:   "/usr/local/etc/yeager/ca-cert.pem",
				},
			},
		},
		Outbounds: []*YeagerClient{
			{
				Tag:       "PROXY",
				Address:   "127.0.0.1:9000",
				Transport: "grpc",
				MutualTLS: MutualTLS{
					CertFile: "/usr/local/etc/yeager/client-cert.pem",
					KeyFile:  "/usr/local/etc/yeager/client-key.pem",
					CAFile:   "/usr/local/etc/yeager/ca-cert.pem",
				},
			},
		},
		Rules:              []string{"FINAL,PROXY"},
		Verbose:            true,
		ConnectionPoolSize: 3,
		Debug:              true,
	}

	got, err := Load(strings.NewReader(rawConf))
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(want, got) {
		t.Fatalf("want %#v, got %#v", want, got)
	}
}
