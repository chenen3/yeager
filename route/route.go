package route

import (
	"errors"
	"net"
	"strings"
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
	matcher
	rtype  string
	value  string
	policy string
}

func newRule(ruleType string, value string, policy string) (*rule, error) {
	m, err := newRuleMatcher(ruleType, value)
	if err != nil {
		return nil, err
	}
	r := &rule{
		rtype:   strings.ToLower(ruleType),
		value:   value,
		policy:  strings.ToLower(policy),
		matcher: m,
	}
	return r, nil
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

	// parsing rules of geosite.dat boosted memory usage,
	// set nil to release heap objects in future GC
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

func (r *Router) Dispatch(host string) (policy string, err error) {
	h, err := parseHost(host)
	if err != nil {
		return "", err
	}
	for _, ru := range r.rules {
		if ru.Match(h) {
			return ru.policy, nil
		}
	}
	return defaultFinalRule.policy, nil
}

type host struct {
	IsDomain bool
	Domain   string
	IsIPv4   bool
	IP       net.IP
}

func parseHost(s string) (host, error) {
	if len(s) == 0 || len(s) > 255 {
		return host{}, errors.New("bad domain name")
	}

	var h host
	if ip := net.ParseIP(s); ip == nil {
		h.IsDomain = true
		h.Domain = s
	} else if ipv4 := ip.To4(); ipv4 != nil {
		h.IsIPv4 = true
		h.IP = ipv4
	}
	return h, nil
}
