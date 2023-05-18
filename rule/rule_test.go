package rule

import (
	"os"
	"path/filepath"
	"testing"
)

func TestMain(m *testing.M) {
	dir, err := os.Getwd()
	if err != nil {
		panic(err)
	}
	domainListPaths = []string{filepath.Join(dir, "testdata", "geosite.dat")}
	os.Exit(m.Run())
}

func TestRulesMatch(t *testing.T) {
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
			name:   "geosite",
			fields: fields{rules: []string{"geosite,private,faketag"}},
			args:   args{"localhost"},
			want:   "faketag",
		},
		{
			name:   "geosite",
			fields: fields{rules: []string{"geosite,apple@cn,faketag"}},
			args:   args{"apple.cn"},
			want:   "faketag",
		},
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
			r, err := Parse(tt.fields.rules)
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

// 曾考虑引入LRU缓存降低路由耗时，但基准测试表明，示例路由匹配时间约30微秒。
// 对于动辄几十毫秒的网络延迟时间来说，缓存效果并不明显，为避免过早优化，不作缓存。
func BenchmarkRulesMatch(b *testing.B) {
	r, err := Parse([]string{
		"geosite,cn,tag1",
	})
	if err != nil {
		b.Fatal(err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		r.Match("fake.com")
	}
}
