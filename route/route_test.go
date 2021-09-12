package route

import (
	"os"
	"path/filepath"
	"testing"

	"yeager/proxy"
)

func TestMain(m *testing.M) {
	dir, err := os.Getwd()
	if err != nil {
		panic(err)
	}
	assetDirs = append(assetDirs, filepath.Join(dir, "testdata"))
	os.Exit(m.Run())
}

func TestRouter_Dispatch(t *testing.T) {
	type fields struct {
		rules []string
	}
	type args struct {
		addr *proxy.Address
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		want    string
		wantErr bool
	}{
		{
			name: "empty-domain",
			fields: fields{rules: []string{
				"domain,,direct",
			}},
			args:    args{addr: &proxy.Address{Host: "www.apple.com", Type: proxy.AddrDomainName}},
			wantErr: true,
		},
		{
			name: "domain",
			fields: fields{rules: []string{
				"domain,apple.com,direct",
				"final,faketag",
			}},
			args: args{addr: &proxy.Address{Host: "www.apple.com", Type: proxy.AddrDomainName}},
			want: "faketag",
		},
		{
			name: "domain-suffix",
			fields: fields{rules: []string{
				"domain-suffix,le.com,direct",
				"final,faketag",
			}},
			args: args{addr: &proxy.Address{Host: "www.google.com", Type: proxy.AddrDomainName}},
			want: "faketag",
		},
		{
			name:   "domain-keyword",
			fields: fields{rules: []string{"domain-keyword,apple,faketag"}},
			args:   args{addr: &proxy.Address{Host: "www.apple.com", Type: proxy.AddrDomainName}},
			want:   "faketag",
		},
		{
			name:   "geosite",
			fields: fields{rules: []string{"geosite,private,faketag"}},
			args:   args{addr: &proxy.Address{Host: "localhost", Type: proxy.AddrDomainName}},
			want:   "faketag",
		},
		{
			name:   "ip-cidr",
			fields: fields{rules: []string{"ip-cidr,127.0.0.1/8,faketag"}},
			args:   args{addr: &proxy.Address{IP: []byte{127, 0, 0, 1}, Type: proxy.AddrIPv4}},
			want:   "faketag",
		},
		{
			name:   "ip-cidr",
			fields: fields{rules: []string{"ip-cidr,192.168.0.0/16,faketag"}},
			args:   args{addr: &proxy.Address{IP: []byte{192, 168, 1, 1}, Type: proxy.AddrIPv4}},
			want:   "faketag",
		},
		// {
		// 	name:   "geoip",
		// 	fields: fields{rules: []string{"geoip,private,faketag"}},
		// 	args:   args{addr: proxy.NewAddress("192.168.1.1", 80)},
		// 	want:   "faketag",
		// },
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r, err := NewRouter(tt.fields.rules)
			if err != nil {
				if !tt.wantErr {
					t.Error(err)
				}
				return
			}
			got, err := r.Dispatch(tt.args.addr)
			if err != nil {
				t.Error(err)
			}
			if got != tt.want {
				t.Errorf("Dispatch() = %v, want %v", got, tt.want)
			}
		})
	}
}
