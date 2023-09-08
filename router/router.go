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
	kind    string
	route   string
	matcher matcher
}

// there are two form of rules:
//   - ordinary rule: type,value,route
//   - final rule: FINAL,route
func parseRule(s string) (rule, error) {
	var kind, value, route string
	parts := strings.Split(s, ",")
	switch len(parts) {
	case 2:
		kind = parts[0]
		if !strings.EqualFold(kind, finalRule) {
			return rule{}, errors.New("bad final rule: " + s)
		}
		route = parts[1]
	case 3:
		kind = parts[0]
		value = parts[1]
		if value == "" {
			return rule{}, errors.New("empty rule value: " + s)
		}
		route = parts[2]
	default:
		return rule{}, errors.New("bad rule: " + s)
	}

	var m matcher
	switch strings.ToLower(kind) {
	case domainRule:
		m = domainMatch(value)
	case domainSuffixRule:
		m = domainSuffixMatch(value)
	case domainKeywordRule:
		m = domainKeywordMatch(value)
	case geoSiteRule:
		gm, err := newGeoSiteMatch(value)
		if err != nil {
			return rule{}, err
		}
		m = gm
	case ipCIDRRule:
		_, ipNet, err := net.ParseCIDR(value)
		if err != nil {
			return rule{}, err
		}
		m = cidrMatch{ipNet}
	case finalRule:
		m = allMatch{}
	default:
		return rule{}, errors.New("unsupported rule type: " + kind)
	}

	r := rule{
		kind:    strings.ToLower(kind),
		route:   strings.ToLower(route),
		matcher: m,
	}
	return r, nil
}

type Router []rule

func New(rules []string) (Router, error) {
	if len(rules) == 0 {
		return nil, errors.New("empty rules")
	}

	parsed := make([]rule, len(rules))
	for i, s := range rules {
		ru, err := parseRule(s)
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
	ip := net.ParseIP(host)
	// did consider using cache to speed up the matching,
	// but here is not the performance bottleneck
	for _, rule := range r {
		// do not dive deep if the rule type does not match
		switch rule.kind {
		case domainRule, domainSuffixRule, domainKeywordRule, geoSiteRule:
			if host == "" {
				continue
			}
		case ipCIDRRule:
			if ip == nil {
				continue
			}
		}
		if rule.matcher.match(host, ip) {
			return rule.route, nil
		}
	}
	return DefaultRoute, nil
}
