package router

import (
	"errors"
	"net"
	"runtime/debug"
	"strings"
)

const (
	domainRule        = "domain"         // 域名
	domainSuffixRule  = "domain-suffix"  // 根域名
	domainKeywordRule = "domain-keyword" // 域名关键字
	geoSiteRule       = "geosite"        // 预定义域名集合
	ipCIDRRule        = "ip-cidr"        // 无类别域间路由
	finalRule         = "final"          // 最终规则

	// size of geoip.dat was only 4MB, but now downloading from upstream
	// it is 46MB. While yeager startup with it, the memory usage raise
	// up to 300MB. Considering GEOIP rule is not the essential feature,
	// now disable it, so that the startup memory would beneath 15MB.
	// ruleGeoIP = "geoip" // 预定义IP集合
)

// built-in route
const (
	DirectRoute = "direct"
	RejectRoute = "reject"

	DefaultRoute = DirectRoute
)

type rule struct {
	matcher
	kind  string
	route string
}

func newRule(kind string, value string, route string) (*rule, error) {
	var m matcher
	switch strings.ToLower(kind) {
	case domainRule:
		m = domainMatcher(value)
	case domainSuffixRule:
		m = domainSuffixMatcher(value)
	case domainKeywordRule:
		m = domainKeywordMatcher(value)
	case geoSiteRule:
		gm, err := newGeoSiteMatcher(value)
		if err != nil {
			return nil, err
		}
		m = gm
	case ipCIDRRule:
		cm, err := newCIDRMatcher(value)
		if err != nil {
			return nil, err
		}
		m = cm
	case finalRule:
		m = finalMatcher{}
	default:
		return nil, errors.New("unsupported rule type: " + kind)
	}

	r := &rule{
		kind:    strings.ToLower(kind),
		route:   strings.ToLower(route),
		matcher: m,
	}
	return r, nil
}

// there are two form of rules:
//   - ordinary rule: type,value,route
//   - final rule: FINAL,route
func parseRule(rule string) (*rule, error) {
	parts := strings.Split(rule, ",")
	switch len(parts) {
	case 2:
		rtype := parts[0]
		if !strings.EqualFold(rtype, finalRule) {
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

type host struct {
	Domain string
	IsIPv4 bool
	IP     net.IP
}

func parseHost(s string) (host, error) {
	if s == "" {
		return host{}, errors.New("empty host name")
	}

	var h host
	if ip := net.ParseIP(s); ip == nil {
		h.Domain = s
	} else if ipv4 := ip.To4(); ipv4 != nil {
		h.IsIPv4 = true
		h.IP = ipv4
	}
	return h, nil
}

type Router []*rule

func New(rules []string) (Router, error) {
	if len(rules) == 0 {
		return nil, errors.New("empty rules")
	}

	parsed := make([]*rule, len(rules))
	for i, rawRule := range rules {
		ru, err := parseRule(rawRule)
		if err != nil {
			return nil, err
		}
		if ru.kind == finalRule && i != len(rules)-1 {
			return nil, errors.New("final rule must be placed at last")
		}
		parsed[i] = ru
	}
	// parsing geosite.dat can be memory intensive
	if geoSites != nil {
		geoSites = nil
		debug.FreeOSMemory()
	}
	return parsed, nil
}

func (r Router) Match(host string) (route string, err error) {
	h, err := parseHost(host)
	if err != nil {
		return "", err
	}

	// did consider using cache to speed up the matching,
	// but here is not the performance bottleneck
	for _, rule := range r {
		// do not dive deep if the rule type is not match
		switch rule.kind {
		case domainRule, domainSuffixRule, domainKeywordRule, geoSiteRule:
			if h.Domain == "" {
				continue
			}
		case ipCIDRRule:
			if h.IP == nil {
				continue
			}
		}
		if rule.Match(h) {
			return rule.route, nil
		}
	}
	return DefaultRoute, nil
}
