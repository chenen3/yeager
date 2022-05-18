package config

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
)

func TestConfig(t *testing.T) {
	rawConf := `
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
				},
				"connectionPoolSize": 3
			}
		],
		"rules": [
			"FINAL,PROXY"
		]
	}
	`
	want := Config{
		Inbounds: Inbounds{
			SOCKS: &SOCKS{Listen: ":1081"},
			HTTP:  &HTTP{Listen: ":8081"},
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
				ConnectionPoolSize: 3,
			},
		},
		Rules:   []string{"FINAL,PROXY"},
		Verbose: true,
		Debug:   true,
	}

	got, err := Load(strings.NewReader(rawConf))
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(want, got) {
		// built-in printing does not dereference the pointer of structure field,
		// if something wrong, hard to tell which field is different
		if !reflect.DeepEqual(want.Inbounds.HTTP, got.Inbounds.HTTP) {
			t.Fatalf("want inbound http: %+v, got %+v", want.Inbounds.HTTP, got.Inbounds.HTTP)
		}
		if !reflect.DeepEqual(want.Inbounds.SOCKS, got.Inbounds.SOCKS) {
			t.Fatalf("want inbound socks: %+v, got %+v", want.Inbounds.SOCKS, got.Inbounds.SOCKS)
		}
		if !reflect.DeepEqual(want.Inbounds.Yeager, got.Inbounds.Yeager) {
			t.Fatalf("want inbound yeager: %+v, got %+v", want.Inbounds.Yeager, got.Inbounds.Yeager)
		}
		if !reflect.DeepEqual(want.Outbounds, got.Outbounds) {
			var wob string
			for i := range want.Outbounds {
				wob += fmt.Sprintf("%+v ", want.Outbounds[i])
			}
			var gob string
			for i := range got.Outbounds {
				gob += fmt.Sprintf("%+v ", got.Outbounds[i])
			}
			t.Fatalf("want outbounds: %s, \ngot %v", wob, gob)
		}
		t.Fatalf("want config %+v, got %+v", want, got)
	}
}
