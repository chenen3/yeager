package route

import (
	"errors"
	"strings"

	ynet "github.com/chenen3/yeager/net"
)

// rule type
const (
	ruleDomain        = "domain"         // 域名
	ruleDomainSuffix  = "domain-suffix"  // 根域名
	ruleDomainKeyword = "domain-keyword" // 域名关键字
	ruleGeoSite       = "geosite"        // 预定义域名集合
	ruleIPCIDR        = "ip-cidr"        // 无类别域间路由
	ruleFinal         = "final"          // 最终规则

	// size of geoip.dat was only 4MB, but now downloading from upstream
	// it is 46MB. While yeager startup with it, the memory usage raise
	// up to 300MB. Considering GEOIP rule is not the essential feature,
	// now disable it, so that the startup memory would beneath 15MB.
	// ruleGeoIP = "geoip" // 预定义IP集合
)

// built-in proxy policy
const (
	Direct = "direct"
	Reject = "reject"
)

var defaultFinalRule, _ = newRule(ruleFinal, "", Direct)

type rule struct {
	rtype   string
	value   string
	policy  string
	matcher matcher
}

func newRule(ruleType string, value string, policy string) (*rule, error) {
	m, err := newRuleMatcher(ruleType, value)
	if err != nil {
		return nil, err
	}
	r := &rule{
		rtype:  strings.ToLower(ruleType),
		value:  value,
		policy: strings.ToLower(policy),
		matcher: m,
	}
	return r, nil
}

func (r *rule) Match(addr *ynet.Addr) bool {
	switch r.rtype {
	case ruleDomain, ruleDomainSuffix, ruleDomainKeyword, ruleGeoSite:
		if addr.Type != ynet.AddrDomainName {
			return false
		}
	case ruleIPCIDR:
		if addr.Type != ynet.AddrIPv4 {
			// ipv6 not supported yet
			return false
		}
	}
	return r.matcher.Match(addr)
}

type Router struct {
	rules []*rule
}

func New(rules []string) (*Router, error) {
	if len(rules) == 0 {
		return nil, errors.New("empty rules")
	}

	var r Router
	parsedRules := make([]*rule, 0, len(rules))
	for i, rawRule := range rules {
		ru, err := parseRule(rawRule)
		if err != nil {
			return nil, err
		}
		if ru.rtype == ruleFinal && i != len(rules)-1 {
			return nil, errors.New("the final rule must be placed at last")
		}
		parsedRules = append(parsedRules, ru)
	}
	r.rules = parsedRules

	// parsing rules of geosite.dat (or geoip.dat) boosted memory usage,
	// set nil to release heap objects in future GC
	// globalGeoIPList = nil
	geoSites = nil
	return &r, nil
}

// there are two form of rules:
//   - ordinary rule: type,value,policy
//   - final rule: FINAL,policy
func parseRule(rule string) (*rule, error) {
	parts := strings.Split(rule, ",")
	switch len(parts) {
	case 2:
		rtype := parts[0]
		if !strings.EqualFold(rtype, ruleFinal) {
			return nil, errors.New("invalid final rule: " + rule)
		}
		return newRule(rtype, "", parts[1])
	case 3:
		typ := parts[0]
		val := parts[1]
		if val == "" {
			return nil, errors.New("empty rule value: " + rule)
		}
		return newRule(typ, val, parts[2])
	default:
		return nil, errors.New("wrong form of rule: " + rule)
	}
}

func (r *Router) Dispatch(addr string) (policy string, err error) {
	var dst *ynet.Addr
	dst, err = ynet.ParseAddr("tcp", addr)
	if err != nil {
		return "", err
	}
	for _, ru := range r.rules {
		if ru.Match(dst) {
			return ru.policy, nil
		}
	}
	return defaultFinalRule.policy, nil
}
