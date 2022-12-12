package route

import (
	"errors"
	"net"
	"regexp"
	"strings"
)

type matcher interface {
	Match(host) bool
}

func newRuleMatcher(ruleType string, value string) (m matcher, err error) {
	switch strings.ToLower(ruleType) {
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
	case ruleFinal:
		m = newFinalMatcher()
	default:
		err = errors.New("unsupported rule type: " + ruleType)
	}
	return m, err
}

type domainKeywordMatcher string

func (key domainKeywordMatcher) Match(h host) bool {
	return h.IsDomain && strings.Contains(h.Domain, string(key))
}

type finalMatcher struct{}

func newFinalMatcher() *finalMatcher {
	return &finalMatcher{}
}

func (f *finalMatcher) Match(h host) bool {
	return true
}

type domainMatcher string

func (d domainMatcher) Match(h host) bool {
	return h.IsDomain && string(d) == h.Domain
}

type domainSuffixMatcher string

func (m domainSuffixMatcher) Match(h host) bool {
	if !h.IsDomain {
		return false
	}
	domain := h.Domain
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

func (c *cidrMatcher) Match(h host) bool {
	return h.IsIPv4 && c.Contains(h.IP)
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
	return h.IsDomain && m.re.MatchString(h.Domain)
}
