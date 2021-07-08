package router

import (
	"errors"
	"os"
	"path"
	"strings"

	"github.com/v2fly/v2ray-core/v4/app/router"
	"google.golang.org/protobuf/proto"
	"yeager/proxy"
)

func loadGeoSiteFile(country string) ([]*router.Domain, error) {
	geositeList := new(router.GeoSiteList)
	var data []byte
	var err error
	for _, dir := range assetDirs {
		data, err = os.ReadFile(path.Join(dir, "geosite.dat"))
		if err == nil {
			break
		}
	}
	if err != nil {
		return nil, err
	}

	err = proto.Unmarshal(data, geositeList)
	if err != nil {
		return nil, err
	}
	for _, g := range geositeList.Entry {
		if strings.EqualFold(g.CountryCode, country) {
			return g.Domain, nil
		}
	}
	return nil, errors.New("unsupported country code: " + country)
}

type geoSiteMatcher []matcher

func newGeoSiteMatcher(value string) (geoSiteMatcher, error) {
	// 配置规则geosite的值可能带有属性，例如 google@ads ，表示只要google所有域名中带有ads属性的域名
	parts := strings.Split(value, "@")
	geoValue := strings.TrimSpace(parts[0])
	var attrs []string
	for _, attr := range parts[1:] {
		attrs = append(attrs, strings.TrimSpace(attr))
	}

	sites, err := loadGeoSiteFile(geoValue)
	if err != nil {
		return nil, err
	}

	g := make(geoSiteMatcher, 0, len(sites))
	for _, domain := range sites {
		if len(attrs) > 0 && !domainContainsAnyAttr(domain, attrs) {
			continue
		}
		var m matcher
		switch domain.Type {
		case router.Domain_Plain:
			m = domainKeywordMatcher(domain.Value)
		case router.Domain_Domain:
			m = domainSuffixMatcher(domain.Value)
		case router.Domain_Full:
			m = domainMatcher(domain.Value)
		case router.Domain_Regex:
			var err error
			m, err = newRegexMatcher(domain.Value)
			if err != nil {
				return nil, err
			}
		}
		g = append(g, m)
	}
	return g, nil
}

func (g geoSiteMatcher) Match(addr *proxy.Address) bool {
	for _, m := range g {
		if m.Match(addr) {
			return true
		}
	}
	return false
}

func domainContainsAnyAttr(domain *router.Domain, attrs []string) bool {
	for _, attr := range attrs {
		for _, dattr := range domain.Attribute {
			if strings.EqualFold(dattr.Key, attr) {
				return true
			}
		}
	}
	return false
}
