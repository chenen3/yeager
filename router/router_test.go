package router

import "testing"

func TestRouter(t *testing.T) {
	type fields struct {
		rules []string
	}
	type args struct {
		host string
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		want    string
		wantErr bool
	}{
		{
			name:    "no-rule",
			args:    args{"www.apple.com"},
			wantErr: true,
		},
		{
			name:    "empty-domain",
			fields:  fields{rules: []string{"domain,,direct"}},
			args:    args{"www.apple.com"},
			wantErr: true,
		},
		{
			name:   "domain",
			fields: fields{rules: []string{"domain,apple.com,direct", "final,faketag"}},
			args:   args{"www.apple.com"},
			want:   "faketag",
		},
		{
			name:    "domain-suffix",
			fields:  fields{rules: []string{"domain-suffix,le.com,direct", "final,faketag"}},
			args:    args{"www.google.com"},
			want:    "faketag",
			wantErr: false,
		},
		{
			name:   "domain-keyword",
			fields: fields{rules: []string{"domain-keyword,apple,faketag"}},
			args:   args{"www.apple.com"},
			want:   "faketag",
		},
		// {
		// 	name:   "geosite",
		// 	fields: fields{rules: []string{"geosite,private,faketag"}},
		// 	args:   args{"localhost"},
		// 	want:   "faketag",
		// },
		// {
		// 	name:   "geosite",
		// 	fields: fields{rules: []string{"geosite,apple@cn,faketag"}},
		// 	args:   args{"apple.cn"},
		// 	want:   "faketag",
		// },
		{
			name:   "ip-cidr",
			fields: fields{rules: []string{"ip-cidr,127.0.0.1/8,faketag"}},
			args:   args{"127.0.0.1"},
			want:   "faketag",
		},
		{
			name:   "ip-cidr",
			fields: fields{rules: []string{"ip-cidr,192.168.0.0/16,faketag"}},
			args:   args{"192.168.1.1"},
			want:   "faketag",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r, err := New(tt.fields.rules)
			if err != nil {
				if !tt.wantErr {
					t.Errorf("want no error, got error: %v", err)
				}
				return
			}
			got, err := r.Match(tt.args.host)
			if err != nil {
				t.Error(err)
				return
			}
			if got != tt.want {
				t.Errorf("want %v, got %v", tt.want, got)
				return
			}
		})
	}
}
