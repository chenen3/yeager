package route

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

// built-in pass
const (
	Direct = "direct"
	Reject = "reject"
)

type rule struct {
	kind    string
	pass    string
	matcher matcher
}

// there are two form of rules:
//   - ordinary rule: kind,condition,pass
//   - final rule: FINAL,route
func parseRule(s string) (rule, error) {
	var kind, cond, pass string
	parts := strings.Split(s, ",")
	switch len(parts) {
	case 2:
		kind = parts[0]
		if !strings.EqualFold(kind, finalRule) {
			return rule{}, errors.New("bad final rule: " + s)
		}
		pass = parts[1]
	case 3:
		kind = parts[0]
		cond = parts[1]
		if cond == "" {
			return rule{}, errors.New("empty rule value: " + s)
		}
		pass = parts[2]
	default:
		return rule{}, errors.New("bad rule: " + s)
	}

	var m matcher
	switch strings.ToLower(kind) {
	case domainRule:
		m = domainMatch(cond)
	case domainSuffixRule:
		m = domainSuffixMatch(cond)
	case domainKeywordRule:
		m = domainKeywordMatch(cond)
	case geoSiteRule:
		gm, err := newGeoSiteMatch(cond)
		if err != nil {
			return rule{}, err
		}
		m = gm
	case ipCIDRRule:
		_, ipNet, err := net.ParseCIDR(cond)
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
		pass:    strings.ToLower(pass),
		matcher: m,
	}
	return r, nil
}

type Routes []rule

func New(rules []string) (Routes, error) {
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

func (r Routes) Match(host string) (pass string, err error) {
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
			return rule.pass, nil
		}
	}
	return Direct, nil
}
