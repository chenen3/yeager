package rule

import (
	"net"
	"regexp"
	"strings"
)

type matcher interface {
	Match(host) bool
}

type domainMatcher string

func (d domainMatcher) Match(h host) bool {
	return h.IsDomain && string(d) == h.Domain
}

type domainKeywordMatcher string

func (key domainKeywordMatcher) Match(h host) bool {
	return h.IsDomain && strings.Contains(h.Domain, string(key))
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

type finalMatcher struct{}

func (f finalMatcher) Match(h host) bool {
	return true
}
