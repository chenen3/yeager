package route

import (
	"errors"
	"os"
	"path"
	"strings"

	"github.com/v2fly/v2ray-core/v4/app/router"
	"google.golang.org/protobuf/proto"

	"github.com/chenen3/yeager/log"
	"github.com/chenen3/yeager/util"
)

var assetDirs []string

const (
	defaultAssetDir = "/usr/local/share/yeager"
	envAssetDir     = "YEAGER_ASSET_DIR"
)

func init() {
	assetDirs = append(assetDirs, defaultAssetDir)
	if d := os.Getenv(envAssetDir); d != "" {
		assetDirs = append(assetDirs, d)
	}
}

func loadGeoSiteFile() (*router.GeoSiteList, error) {
	var data []byte
	var err error
	for _, dir := range assetDirs {
		data, err = os.ReadFile(path.Join(dir, "geosite.dat"))
		if err == nil {
			break
		}
	}
	if err != nil {
		log.L().Warnf("By default, yeager loads geosite.dat from %v (and honors %s if set)",
			assetDirs, envAssetDir)
		return nil, err
	}

	geoSiteList := new(router.GeoSiteList)
	err = proto.Unmarshal(data, geoSiteList)
	if err != nil {
		return nil, err
	}

	return geoSiteList, nil
}

var globalGeoSiteList *router.GeoSiteList

func extractCountrySite(country string) ([]*router.Domain, error) {
	if globalGeoSiteList == nil {
		geoSiteList, err := loadGeoSiteFile()
		if err != nil {
			return nil, err
		}
		globalGeoSiteList = geoSiteList
	}

	for _, g := range globalGeoSiteList.Entry {
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

func (g geoSiteMatcher) Match(addr *util.Addr) bool {
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
