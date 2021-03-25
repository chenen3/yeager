package route

import (
	"testing"
	"yeager/protocol"
)

func TestRouter_Dispatch(t *testing.T) {
	type fields struct {
		rules []string
	}
	type args struct {
		addr *protocol.Address
	}
	tests := []struct {
		name   string
		fields fields
		args   args
		want   PolicyType
	}{
		{
			name: "domain",
			fields: fields{rules: []string{
				"domain,apple.com,direct",
				"final,fakepolicy",
			}},
			args: args{addr: protocol.NewAddress("www.apple.com", 80)},
			want: "fakepolicy",
		},
		{
			name:   "domain-suffix",
			fields: fields{rules: []string{"domain-suffix,apple.com,fakepolicy"},},
			args:   args{addr: protocol.NewAddress("www.apple.com", 80)},
			want:   "fakepolicy",
		},
		{
			name:   "domain-keyword",
			fields: fields{rules: []string{"domain-keyword,apple,fakepolicy"},},
			args:   args{addr: protocol.NewAddress("www.apple.com", 80)},
			want:   "fakepolicy",
		},
		{
			name:   "geosite",
			fields: fields{rules: []string{"geosite,private,fakepolicy"},},
			args:   args{addr: protocol.NewAddress("localhost", 80)},
			want:   "fakepolicy",
		},
		{
			name:   "ip",
			fields: fields{rules: []string{"ip,127.0.0.1,fakepolicy"},},
			args:   args{addr: protocol.NewAddress("127.0.0.1", 80)},
			want:   "fakepolicy",
		},
		{
			name:   "geoip",
			fields: fields{rules: []string{"geoip,private,fakepolicy"},},
			args:   args{addr: protocol.NewAddress("192.168.1.1", 80)},
			want:   "fakepolicy",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r, err := NewRouter(tt.fields.rules)
			if err != nil {
				t.Error(err)
			}
			if got := r.Dispatch(tt.args.addr); got != tt.want {
				t.Errorf("Dispatch() = %v, want %v", got, tt.want)
			}
		})
	}
}
