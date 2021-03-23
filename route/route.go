package route

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
	ruleDomain        ruleType = "DOMAIN"         // 精确域名
	ruleDomainSuffix  ruleType = "DOMAIN-SUFFIX"  // 子域名
	ruleDomainKeyword ruleType = "DOMAIN-KEYWORD" // 域名关键字
	ruleGEOSITE       ruleType = "GEOSITE"        // 预定义域名，参见v2ray的domain-list-community
	ruleIP            ruleType = "IP"             // 精确IP
	ruleGEOIP         ruleType = "GEOIP"          // 预定义IP，参见v2ray的GEOIP
	ruleFinal         ruleType = "FINAL"          // 最终规则
)

type rule struct {
	type_   ruleType
	value   string
	policy  PolicyType
	matcher matcher // todo
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
	return r.matcher.Match(addr.Host)
}

type Router struct {
	rules []*rule
	cache sync.Map
}

var defaultFinalRule, _ = newRule(ruleFinal, "", PolicyProxy)

// 规则格式分两种：
// - 普通规则: ruleType,value,policyType  其中value的形式参考v2ray路由规则
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
		if i == len(rules)-1 && strings.Contains(rawRule, string(ruleFinal)) {
			if len(parts) != 2 {
				return nil, errors.New("invalid final rule: " + rawRule)
			}
			ru, err = newRule(ruleType(parts[0]), "", PolicyType(parts[1]))
			if err != nil {
				return nil, err
			}
		} else {
			if len(parts) != 3 {
				return nil, errors.New("invalid regular rule: " + rawRule)
			}
			ru, err = newRule(ruleType(parts[0]), parts[1], PolicyType(parts[2]))
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
