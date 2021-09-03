package router

import (
	"errors"
	"strings"

	"yeager/proxy"
	"yeager/proxy/direct"
)

// rule type
const (
	ruleDomain        = "domain"         // 域名
	ruleDomainSuffix  = "domain-suffix"  // 根域名
	ruleDomainKeyword = "domain-keyword" // 域名关键字
	ruleGeoSite       = "geosite"        // 预定义域名集合
	ruleIPCIDR        = "ip-cidr"        // 无类别域间路由
	ruleFinal         = "final"          // 最终规则

	// geoip.dat was only 4MB size, but now downloading from upstream
	// it is 46MB. While yeager startup with it, the memory usage raise
	// up to 300MB. Considering GEOIP rule is not the essential feature,
	// now disable it, so that the startup memory would beneath 15MB.

	// deprecated
	ruleGeoIP = "geoip" // 预定义IP集合
)

var defaultFinalRule, _ = newRule(ruleFinal, "", direct.Tag)

type rule struct {
	type_       string
	value       string
	outboundTag string
	matcher     matcher
}

func newRule(type_ string, value string, outboundTag string) (*rule, error) {
	type_ = strings.ToLower(type_)
	outboundTag = strings.ToLower(outboundTag)
	ru := &rule{
		type_:       type_,
		value:       value,
		outboundTag: outboundTag,
	}

	var err error
	ru.matcher, err = newRuleMatcher(type_, value)
	return ru, err
}

func (r *rule) Match(addr *proxy.Address) bool {
	switch r.type_ {
	case ruleDomain, ruleDomainSuffix, ruleDomainKeyword, ruleGeoSite:
		if addr.Type != proxy.AddrDomainName {
			return false
		}
	case ruleIPCIDR:
		if addr.Type != proxy.AddrIPv4 {
			// ipv6 not supported yet
			return false
		}
	}

	return r.matcher.Match(addr)
}

type Router struct {
	rules []*rule
}

func NewRouter(rules []string) (*Router, error) {
	if len(rules) == 0 {
		return new(Router), nil
	}

	var r Router
	parsedRules := make([]*rule, 0, len(rules))
	for i, rawRule := range rules {
		ru, err := parseRule(rawRule)
		if err != nil {
			return nil, err
		}
		if ru.type_ == ruleFinal && i != len(rules)-1 {
			return nil, errors.New("the final rule must be placed at last")
		}
		parsedRules = append(parsedRules, ru)
	}
	r.rules = parsedRules

	// parsing geoip file obviously increase memory usage,
	// set nil to release objects, memory shall release in future GC
	// globalGeoIPList = nil
	globalGeoSiteList = nil
	return &r, nil
}

// 规则格式分两种：
// - 普通规则: ruleType,value,outboundTag
// - 最终规则: FINAL,outboundTag
func parseRule(rule string) (*rule, error) {
	parts := strings.Split(rule, ",")
	switch len(parts) {
	case 2:
		typ := parts[0]
		if !strings.EqualFold(typ, ruleFinal) {
			return nil, errors.New("invalid final rule: " + rule)
		}
		outboundTag := parts[1]
		return newRule(typ, "", outboundTag)
	case 3:
		typ := parts[0]
		val := parts[1]
		if val == "" {
			return nil, errors.New("empty rule value: " + rule)
		}
		outboundTag := parts[2]
		return newRule(typ, val, outboundTag)
	default:
		return nil, errors.New("invalid rule: " + rule)
	}
}

func (r *Router) Dispatch(addr string) (outboundTag string, err error) {
	var dst *proxy.Address
	dst, err = proxy.ParseAddress(addr)
	if err != nil {
		return "", err
	}
	if len(r.rules) == 0 {
		return defaultFinalRule.outboundTag, nil
	}

	for _, ru := range r.rules {
		if ru.Match(dst) {
			return ru.outboundTag, nil
		}
	}

	return defaultFinalRule.outboundTag, nil
}
