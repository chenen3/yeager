package router

import (
	"errors"
	"strings"
	"sync"

	"yeager/protocol"
)

type PolicyType string

const (
	PolicyDirect PolicyType = "direct"
	PolicyReject PolicyType = "reject"
	PolicyProxy  PolicyType = "proxy"
)

type ruleType string

const (
	ruleDomain        ruleType = "domain"         // 精确域名
	ruleDomainSuffix  ruleType = "domain-suffix"  // 根域名
	ruleDomainKeyword ruleType = "domain-keyword" // 域名关键字
	ruleGeoSite       ruleType = "geosite"        // 预定义域名，参考v2ray的domain-list-community
	ruleIP            ruleType = "ip"             // 精确IP
	ruleGeoIP         ruleType = "geoip"          // 预定义IP集，参考v2ray的geoip
	ruleFinal         ruleType = "final"          // 最终规则
)

var defaultFinalRule, _ = newRule(ruleFinal, "", PolicyDirect)

type rule struct {
	type_   ruleType
	value   string
	policy  PolicyType
	matcher matcher
}

func newRule(rt ruleType, value string, pt PolicyType) (*rule, error) {
	ru := &rule{
		type_:  rt,
		value:  value,
		policy: pt,
	}

	var err error
	ru.matcher, err = newRuleMatcher(rt, value)
	return ru, err
}

func (r *rule) Match(addr *protocol.Address) bool {
	switch r.type_ {
	case ruleDomain, ruleDomainSuffix, ruleDomainKeyword, ruleGeoSite:
		if addr.Type != protocol.AddrDomainName {
			return false
		}
	case ruleIP, ruleGeoIP:
		if addr.Type != protocol.AddrIPv4 {
			// ipv6 not supported yet
			return false
		}
	}

	return r.matcher.Match(addr)
}

type Router struct {
	rules []*rule
	cache sync.Map
}

// 规则格式分两种：
// - 普通规则: ruleType,value,policyType
// - 最终规则: FINAL,policyType
func NewRouter(rules []string) (*Router, error) {
	r := &Router{rules: []*rule{defaultFinalRule}}
	if len(rules) == 0 {
		return r, nil
	}

	parsedRules := make([]*rule, 0, len(rules))
	for i, rawRule := range rules {
		var ru *rule
		var err error
		parts := strings.Split(rawRule, ",")
		if i == len(rules)-1 && strings.Index(strings.ToLower(rawRule), string(ruleFinal)) == 0 {
			if len(parts) != 2 {
				return nil, errors.New("invalid final rule: " + rawRule)
			}
			rawRuleType := strings.ToLower(parts[0])
			rawPolicyType := strings.ToLower(parts[1])
			ru, err = newRule(ruleType(rawRuleType), "", PolicyType(rawPolicyType))
			if err != nil {
				return nil, err
			}
		} else {
			if len(parts) != 3 {
				return nil, errors.New("invalid regular rule: " + rawRule)
			}
			rawRuleType := strings.ToLower(parts[0])
			rawRuleValue := parts[1]
			if rawRuleValue == "" {
				return nil, errors.New("empty rule value: " + rawRule)
			}
			rawPolicyType := strings.ToLower(parts[2])
			ru, err = newRule(ruleType(rawRuleType), rawRuleValue, PolicyType(rawPolicyType))
			if err != nil {
				return nil, err
			}
		}
		parsedRules = append(parsedRules, ru)
	}

	r.rules = parsedRules
	return r, nil
}

func (r *Router) Dispatch(addr *protocol.Address) PolicyType {
	i, ok := r.cache.Load(addr.Host)
	if ok {
		return i.(PolicyType)
	}

	for _, ru := range r.rules {
		if ru.Match(addr) {
			r.cache.Store(addr.Host, ru.policy)
			return ru.policy
		}
	}

	// 为了编译成功才写这个return语句，因为r.rules的 FINAL 规则一定匹配成功，实际在上面已 return
	return defaultFinalRule.policy
}
