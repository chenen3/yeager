package route

import (
	"errors"
	"net"
	"regexp"
	"strings"

	"yeager/proxy"
)

type matcher interface {
	Match(addr *proxy.Address) bool
}

func newRuleMatcher(ruleType string, value string) (m matcher, err error) {
	switch ruleType {
	case ruleDomain:
		m = domainMatcher(value)
	case ruleDomainSuffix:
		m = domainSuffixMatcher(value)
	case ruleDomainKeyword:
		m = domainKeywordMatcher(value)
	case ruleGeoSite:
		m, err = newGeoSiteMatcher(value)
	case ruleIPCIDR:
		m, err = newCIDRMatcher(value)
	// case ruleGeoIP:
	// 	log.Warn("deprecated GEOIP rule, it would be removed in future release")
	// 	// m, err = newGeoIPMatcher(value)
	// 	m = new(nullMatcher)
	case ruleFinal:
		m = newFinalMatcher()
	default:
		err = errors.New("unsupported rule type: " + ruleType)
	}
	return m, err
}

type nullMatcher struct{}

func (n nullMatcher) Match(addr *proxy.Address) bool {
	return false
}

type domainKeywordMatcher string

func (d domainKeywordMatcher) Match(addr *proxy.Address) bool {
	return strings.Contains(addr.Host, string(d))
}

type finalMatcher struct{}

func newFinalMatcher() *finalMatcher {
	return &finalMatcher{}
}

func (f *finalMatcher) Match(addr *proxy.Address) bool {
	return true
}

type domainMatcher string

func (d domainMatcher) Match(addr *proxy.Address) bool {
	return string(d) == addr.Host
}

type domainSuffixMatcher string

func (m domainSuffixMatcher) Match(addr *proxy.Address) bool {
	domain := addr.Host
	if !strings.HasSuffix(domain, string(m)) {
		return false
	}

	return len(m) == len(domain) || domain[len(domain)-len(m)-1] == '.'
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

func (c *cidrMatcher) Match(addr *proxy.Address) bool {
	return c.Contains(addr.IP)
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

func (m *domainRegexMatcher) Match(addr *proxy.Address) bool {
	return m.re.MatchString(addr.Host)
}
