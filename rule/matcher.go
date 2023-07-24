package rule

import (
	"errors"
	"net"
	"os"
	"regexp"
	"strings"

	"github.com/chenen3/yeager/rule/pb"
	"google.golang.org/protobuf/proto"
)

type matcher interface {
	Match(host) bool
}

type domainMatcher string

func (d domainMatcher) Match(h host) bool {
	return h.Domain == string(d)
}

type domainKeywordMatcher string

func (key domainKeywordMatcher) Match(h host) bool {
	return strings.Contains(h.Domain, string(key))
}

type domainSuffixMatcher string

func (m domainSuffixMatcher) Match(h host) bool {
	if h.Domain == "" {
		return false
	}
	if !strings.HasSuffix(h.Domain, string(m)) {
		return false
	}

	domain := h.Domain
	return len(m) == len(domain) || domain[len(domain)-len(m)-1] == '.'
}

type domainRegexMatcher struct {
	re *regexp.Regexp
}

func newRegexMatcher(expr string) (*domainRegexMatcher, error) {
	re, err := regexp.Compile(expr)
	if err != nil {
		return nil, err
	}
	return &domainRegexMatcher{re: re}, nil
}

func (m *domainRegexMatcher) Match(h host) bool {
	return h.Domain != "" && m.re.MatchString(h.Domain)
}

type cidrMatcher struct {
	*net.IPNet
}

func newCIDRMatcher(s string) (*cidrMatcher, error) {
	_, ipNet, err := net.ParseCIDR(s)
	if err != nil {
		return nil, err
	}
	return &cidrMatcher{ipNet}, nil
}

func (c *cidrMatcher) Match(h host) bool {
	return h.IsIPv4 && c.Contains(h.IP)
}

const geositeFile = "/usr/local/etc/yeager/geosite.dat"

func loadGeoSite() (*pb.GeoSiteList, error) {
	data, err := os.ReadFile(geositeFile)
	if err != nil {
		return nil, err
	}
	var geoSiteList pb.GeoSiteList
	if err = proto.Unmarshal(data, &geoSiteList); err != nil {
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

type finalMatcher struct{}

func (f finalMatcher) Match(h host) bool {
	return true
}
