package route

import (
	"errors"
	"io/ioutil"
	"regexp"
	"strings"

	"github.com/v2fly/v2ray-core/v4/app/router"
	"google.golang.org/protobuf/proto"
	"yeager/protocol"
)

type matcher interface {
	Match(addr *protocol.Address) bool
}

func newRuleMatcher(ruleType ruleType, value string) (m matcher, err error) {
	switch ruleType {
	case ruleDomain:
		m = domainMatcher(value)
	case ruleDomainSuffix:
		m = domainSuffixMatcher(value)
	case ruleDomainKeyword:
		m = domainKeywordMatcher(value)
	case ruleGeoSite:
		m, err = newGeoSiteMatcher(value)
	case ruleIP:
		m = ipMatcher(value)
	case ruleGeoIP:
		m, err = newGeoIPMatcher(value)
	case ruleFinal:
		m = newFinalMatcher()
	default:
		err = errors.New("rule matcher received unsupported rule type: " + string(ruleType))
	}
	return m, err
}

type domainKeywordMatcher string

func (d domainKeywordMatcher) Match(addr *protocol.Address) bool {
	return strings.Contains(addr.Host, string(d))
}

type finalMatcher struct{}

func newFinalMatcher() *finalMatcher {
	return &finalMatcher{}
}

func (f *finalMatcher) Match(addr *protocol.Address) bool {
	return true
}

type domainMatcher string

func (d domainMatcher) Match(addr *protocol.Address) bool {
	return string(d) == addr.Host
}

type domainSuffixMatcher string

func (d domainSuffixMatcher) Match(addr *protocol.Address) bool {
	return strings.HasSuffix(addr.Host, string(d))
}

type ipMatcher string

func (i ipMatcher) Match(addr *protocol.Address) bool {
	return string(i) == addr.Host
}

var geo2site = make(map[string][]*router.Domain)

func loadGeoSiteFile(filename string) error {
	geositeList := new(router.GeoSiteList)
	data, err := ioutil.ReadFile(filename)
	if err != nil {
		return err
	}
	err = proto.Unmarshal(data, geositeList)
	if err != nil {
		return err
	}
	for _, g := range geositeList.Entry {
		geo2site[strings.ToLower(g.CountryCode)] = g.Domain
	}
	return nil
}

type geoSiteMatcher []matcher

func newGeoSiteMatcher(value string) (geoSiteMatcher, error) {
	// geosite配置规则的值可能带有属性，例如 google@ads ，表示只要google所有域名中带有ads属性的域名
	parts := strings.Split(value, "@")
	geoValue := strings.TrimSpace(parts[0])
	var attrs []string
	for _, attr := range parts[1:] {
		attrs = append(attrs, strings.TrimSpace(attr))
	}

	sites, ok := geo2site[strings.ToLower(geoValue)]
	if !ok {
		return nil, errors.New("unsupported country code: " + geoValue)
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

func (g geoSiteMatcher) Match(addr *protocol.Address) bool {
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

func (m *domainRegexMatcher) Match(addr *protocol.Address) bool {
	return m.re.MatchString(addr.Host)
}
