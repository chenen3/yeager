package route

import (
	"errors"
	"os"
	"path"
	"strings"

	"github.com/chenen3/yeager/route/pb"
	"google.golang.org/protobuf/proto"
)

const (
	defaultAssetDir = "/usr/local/share/yeager"
	envAssetDir     = "YEAGER_ASSET_DIR"
)

var assetDirs []string

func init() {
	assetDirs = append(assetDirs, defaultAssetDir)
	if d := os.Getenv(envAssetDir); d != "" {
		assetDirs = append(assetDirs, d)
	}
}

func loadGeoSite() (*pb.GeoSiteList, error) {
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

type geoSiteMatcher []matcher

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
		var m matcher
		switch domain.Type {
		case pb.Domain_Plain:
			m = domainKeywordMatcher(domain.Value)
		case pb.Domain_RootDomain:
			m = domainSuffixMatcher(domain.Value)
		case pb.Domain_Full:
			m = domainMatcher(domain.Value)
		case pb.Domain_Regex:
			m, err = newRegexMatcher(domain.Value)
			if err != nil {
				return nil, err
			}
		}
		g = append(g, m)
	}
	return g, nil
}

func (g geoSiteMatcher) Match(h host) bool {
	for _, m := range g {
		if m.Match(h) {
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
