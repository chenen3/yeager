package route

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
		{
			name:   "ip-cidr",
			fields: fields{rules: []string{"ip-cidr,127.0.0.1/8,faketag"}},
			args:   args{"127.0.0.1"},
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

func BenchmarkMatcher(b *testing.B) {
	var rules = []string{
		"ip-cidr,127.0.0.1/8,direct",
		"ip-cidr,192.168.0.0/16,direct",
		"ip-cidr,172.16.0.0/12,direct",
		"ip-cidr,10.0.0.0/8,direct",
		"domain,localhost,direct",
		"geosite,apple@cn,direct",
		"geosite,jd,direct",
		"geosite,alibaba,direct",
		"geosite,baidu,direct",
		"geosite,tencent,direct",
		"geosite,bilibili,direct",
		"geosite,zhihu,direct",
	}
	r, err := New(rules)
	if err != nil {
		b.Fatal(err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := r.Match("iamfake.com"); err != nil {
			b.Fatal(err)
		}
	}
}
