package rule

import (
	"errors"
	"os"
	"runtime/debug"
	"strings"

	"github.com/chenen3/yeager/rule/pb"
	"google.golang.org/protobuf/proto"
)

var domainListPaths = []string{"/usr/local/etc/yeager/geosite.dat"}

func loadGeoSite() (*pb.GeoSiteList, error) {
	var data []byte
	var err error
	for _, p := range domainListPaths {
		data, err = os.ReadFile(p)
		if err == nil {
			break
		}
	}
	if err != nil {
		return nil, err
	}

	var geoSiteList pb.GeoSiteList
	err = proto.Unmarshal(data, &geoSiteList)
	if err != nil {
		return nil, err
	}

	return &geoSiteList, nil
}

var geoSites *pb.GeoSiteList

func extractCountrySite(country string) ([]*pb.Domain, error) {
	if geoSites == nil {
		sites, err := loadGeoSite()
		if err != nil {
			return nil, err
		}
		geoSites = sites
	}

	for _, g := range geoSites.Entry {
		if strings.EqualFold(g.CountryCode, country) {
			return g.Domain, nil
		}
	}
	return nil, errors.New("unsupported country code: " + country)
}

// Cleanup clear domain-list and frees memory,
// because parsing geosite.dat can be memory intensive
func Cleanup() {
	if geoSites == nil {
		return
	}
	// free heap objects
	geoSites = nil
	debug.FreeOSMemory()
}

type matchFunc func(host) bool

// benchmark shows that a geoSiteMatcher composed of function
// is nearly 40% faster than one composed of interface
type geoSiteMatcher []matchFunc

func newGeoSiteMatcher(value string) (geoSiteMatcher, error) {
	// 配置规则geosite的值可能带有属性，例如 google@ads ，表示只要google所有域名中带有ads属性的域名
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

	g := make(geoSiteMatcher, 0, len(sites))
	for _, domain := range sites {
		if len(attrs) > 0 && !domainContainsAnyAttr(domain, attrs) {
			continue
		}
		var f matchFunc
		switch domain.Type {
		case pb.Domain_Plain:
			f = domainKeywordMatcher(domain.Value).Match
		case pb.Domain_RootDomain:
			f = domainSuffixMatcher(domain.Value).Match
		case pb.Domain_Full:
			f = domainMatcher(domain.Value).Match
		case pb.Domain_Regex:
			rm, err := newRegexMatcher(domain.Value)
			if err != nil {
				return nil, err
			}
			f = rm.Match
		}
		g = append(g, f)
	}
	return g, nil
}

func (g geoSiteMatcher) Match(h host) bool {
	if h.Domain == "" {
		return false
	}
	for _, f := range g {
		if f(h) {
			return true
		}
	}
	return false
}

func domainContainsAnyAttr(domain *pb.Domain, attrs []string) bool {
	for _, attr := range attrs {
		for _, dattr := range domain.Attribute {
			if strings.EqualFold(dattr.Key, attr) {
				return true
			}
		}
	}
	return false
}
