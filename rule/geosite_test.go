package rule

import (
	"strings"
	"testing"

	"github.com/chenen3/yeager/rule/pb"
)

func BenchmarkGeoSite(b *testing.B) {
	h := host{Domain: "fake.com"}
	m, err := newGeoSiteMatcher("cn")
	if err != nil {
		b.Fatal(err)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		m.Match(h)
	}
}

func BenchmarkGeoSiteInterface(b *testing.B) {
	h := host{Domain: "fake.com"}
	m, err := newGeoSiteMatcherInterface("cn")
	if err != nil {
		b.Fatal(err)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		m.Match(h)
	}
}

type geoSiteMatcherInterface []matcher

func newGeoSiteMatcherInterface(value string) (geoSiteMatcherInterface, error) {
	parts := strings.Split(value, "@")
	geoValue := strings.TrimSpace(parts[0])
	var attrs []string
	for _, attr := range parts[1:] {
		attrs = append(attrs, strings.TrimSpace(attr))
	}

	sites, err := extractCountrySite(geoValue)
	if err != nil {
		return nil, err
	}

	g := make(geoSiteMatcherInterface, len(sites))
	for i, domain := range sites {
		if len(attrs) > 0 && !domainContainsAnyAttr(domain, attrs) {
			continue
		}
		var m matcher
		switch domain.Type {
		case pb.Domain_Plain:
			m = domainKeywordMatcher(domain.Value)
		case pb.Domain_RootDomain:
			m = domainSuffixMatcher(domain.Value)
		case pb.Domain_Full:
			m = domainMatcher(domain.Value)
		case pb.Domain_Regex:
			rm, err := newRegexMatcher(domain.Value)
			if err != nil {
				return nil, err
			}
			m = rm
		}
		g[i] = m
	}
	return g, nil
}

func (g geoSiteMatcherInterface) Match(h host) bool {
	for _, m := range g {
		if m.Match(h) {
			return true
		}
	}
	return false
}
