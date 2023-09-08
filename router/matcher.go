package router

import (
	"errors"
	"net"
	"os"
	"regexp"
	"strings"

	"github.com/chenen3/yeager/router/pb"
	"google.golang.org/protobuf/proto"
)

// see similar implementation in golang.org/x/net/http/httpproxy/proxy.go

type matcher interface {
	// match returns true if the host and ip are allowed
	match(host string, ip net.IP) bool
}

type allMatch struct{}

func (a allMatch) match(host string, ip net.IP) bool {
	return true
}

type domainMatch string

func (d domainMatch) match(host string, ip net.IP) bool {
	return host == string(d)
}

type domainKeywordMatch string

func (key domainKeywordMatch) match(host string, ip net.IP) bool {
	return strings.Contains(host, string(key))
}

type domainSuffixMatch string

func (m domainSuffixMatch) match(host string, ip net.IP) bool {
	if host == "" {
		return false
	}
	if !strings.HasSuffix(host, string(m)) {
		return false
	}

	return len(m) == len(host) || host[len(host)-len(m)-1] == '.'
}

type domainRegexMatch struct {
	re *regexp.Regexp
}

func (m domainRegexMatch) match(host string, ip net.IP) bool {
	return host != "" && m.re.MatchString(host)
}

type cidrMatch struct {
	cidr *net.IPNet
}

func (m cidrMatch) match(host string, ip net.IP) bool {
	return m.cidr.Contains(ip)
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

// benchmark indicates a geoSiteMatch composed of function
// is nearly 40% faster than one composed of interface
type geoSiteMatch []func(host string, ip net.IP) bool

func newGeoSiteMatch(value string) (geoSiteMatch, error) {
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

	g := make(geoSiteMatch, 0, len(sites))
	for _, domain := range sites {
		if len(attrs) > 0 && !domainContainsAnyAttr(domain, attrs) {
			continue
		}
		var f func(host string, ip net.IP) bool
		switch domain.Type {
		case pb.Domain_Plain:
			f = domainKeywordMatch(domain.Value).match
		case pb.Domain_RootDomain:
			f = domainSuffixMatch(domain.Value).match
		case pb.Domain_Full:
			f = domainMatch(domain.Value).match
		case pb.Domain_Regex:
			re, err := regexp.Compile(domain.Value)
			if err != nil {
				return nil, err
			}
			f = domainRegexMatch{re}.match
		}
		g = append(g, f)
	}
	return g, nil
}

func (g geoSiteMatch) match(host string, _ net.IP) bool {
	if host == "" {
		return false
	}
	for _, f := range g {
		if f(host, nil) {
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
