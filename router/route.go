package router

import (
	"context"
	"errors"
	"strings"
	"sync"

	"github.com/opentracing/opentracing-go"
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

func NewRouter(rules []string) (*Router, error) {
	if len(rules) == 0 {
		return &Router{rules: []*rule{defaultFinalRule}}, nil
	}

	r := new(Router)
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
	if lastRule := r.rules[len(r.rules)-1]; lastRule.type_ != ruleFinal {
		r.rules = append(r.rules, defaultFinalRule)
	}
	return r, nil
}

// 规则格式分两种：
// - 普通规则: ruleType,value,policyType
// - 最终规则: FINAL,policyType
func parseRule(rule string) (*rule, error) {
	parts := strings.Split(rule, ",")
	switch len(parts) {
	case 2:
		if strings.Index(strings.ToLower(rule), string(ruleFinal)) != 0 {
			return nil, errors.New("invalid final rule: " + rule)
		}
		rawRuleType := strings.ToLower(parts[0])
		rawPolicyType := strings.ToLower(parts[1])
		return newRule(ruleType(rawRuleType), "", PolicyType(rawPolicyType))
	case 3:
		rawRuleType := strings.ToLower(parts[0])
		rawRuleValue := parts[1]
		if rawRuleValue == "" {
			return nil, errors.New("empty rule value: " + rule)
		}
		rawPolicyType := strings.ToLower(parts[2])
		return newRule(ruleType(rawRuleType), rawRuleValue, PolicyType(rawPolicyType))
	default:
		return nil, errors.New("invalid rule: " + rule)
	}
}

func (r *Router) Dispatch(ctx context.Context, addr *protocol.Address) PolicyType {
	span, _ := opentracing.StartSpanFromContext(ctx, "router")
	defer span.Finish()

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

	return defaultFinalRule.policy
}
