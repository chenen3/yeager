package router

import (
	"errors"
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
	case ruleIP:
		m = ipMatcher(value)
	case ruleGeoIP:
		m, err = newGeoIPMatcher(value)
	case ruleFinal:
		m = newFinalMatcher()
	default:
		err = errors.New("unsupported rule type: " + ruleType)
	}
	return m, err
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

type ipMatcher string

func (i ipMatcher) Match(addr *proxy.Address) bool {
	return string(i) == addr.Host
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
