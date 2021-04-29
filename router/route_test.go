package router

import (
	"os"
	"testing"
	"yeager/protocol"
)

func TestMain(m *testing.M) {
	RegisterAssetsDir("../config/dev")
	os.Exit(m.Run())
}

func TestRouter_Dispatch(t *testing.T) {
	type fields struct {
		rules []string
	}
	type args struct {
		addr *protocol.Address
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
			args:    args{addr: protocol.NewAddress("www.apple.com", 80)},
			wantErr: true,
		},
		{
			name: "domain",
			fields: fields{rules: []string{
				"domain,apple.com,direct",
				"final,faketag",
			}},
			args: args{addr: protocol.NewAddress("www.apple.com", 80)},
			want: "faketag",
		},
		{
			name: "domain-suffix",
			fields: fields{rules: []string{
				"domain-suffix,le.com,direct",
				"final,faketag",
			}},
			args: args{addr: protocol.NewAddress("www.google.com", 443)},
			want: "faketag",
		},
		{
			name:   "domain-keyword",
			fields: fields{rules: []string{"domain-keyword,apple,faketag"}},
			args:   args{addr: protocol.NewAddress("www.apple.com", 80)},
			want:   "faketag",
		},
		{
			name:   "geosite",
			fields: fields{rules: []string{"geosite,private,faketag"}},
			args:   args{addr: protocol.NewAddress("localhost", 80)},
			want:   "faketag",
		},
		{
			name:   "ip",
			fields: fields{rules: []string{"ip,127.0.0.1,faketag"}},
			args:   args{addr: protocol.NewAddress("127.0.0.1", 80)},
			want:   "faketag",
		},
		{
			name:   "geoip",
			fields: fields{rules: []string{"geoip,private,faketag"}},
			args:   args{addr: protocol.NewAddress("192.168.1.1", 80)},
			want:   "faketag",
		},
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
			if got := r.Dispatch(tt.args.addr); got != tt.want {
				t.Errorf("Dispatch() = %v, want %v", got, tt.want)
			}
		})
	}
}
