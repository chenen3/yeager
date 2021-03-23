package route

import (
	"errors"
	"strings"
)

type matcher interface {
	Match(host string) bool
}

func newRuleMatcher(ruleType ruleType, value string) (m matcher, err error) {
	switch ruleType {
	case ruleDomainKeyword:
		m = newDomainKeywordMatcher(value)
	case ruleIP:
		m = newIpMatcher(value)
	case ruleFinal:
		m = newFinalMatcher()
	default:
		err = errors.New("rule matcher received unsupported rule type: " + string(ruleType))
	}
	return m, err
}

type domainKeywordMatcher struct {
	keyword string
}

func newDomainKeywordMatcher(keyword string) *domainKeywordMatcher {
	return &domainKeywordMatcher{keyword: keyword}
}

func (d *domainKeywordMatcher) Match(host string) bool {
	return strings.Contains(host, d.keyword)
}

type ipMatcher struct {
	ip string
}

func newIpMatcher(ip string) *ipMatcher {
	return &ipMatcher{ip: ip}
}

func (i *ipMatcher) Match(host string) bool {
	return i.ip == host
}

type geositeMatcher struct {
}

func (g *geositeMatcher) Match(host string) bool {
	panic("implement me")
}

type geoipMatcher struct {
}

func (g *geoipMatcher) Match(host string) bool {
	panic("implement me")
}

type finalMatcher struct{}

func newFinalMatcher() *finalMatcher {
	return &finalMatcher{}
}

func (f finalMatcher) Match(host string) bool {
	return true
}
