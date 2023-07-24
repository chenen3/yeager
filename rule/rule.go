package rule

import (
	"errors"
	"net"
	"runtime/debug"
	"strings"
)

// rule type
const (
	domain        = "domain"         // 域名
	domainSuffix  = "domain-suffix"  // 根域名
	domainKeyword = "domain-keyword" // 域名关键字
	geoSite       = "geosite"        // 预定义域名集合
	ipCIDR        = "ip-cidr"        // 无类别域间路由
	final         = "final"          // 最终规则

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

type rule struct {
	matcher
	rtype  string
	policy string
}

func newRule(ruleType string, value string, policy string) (*rule, error) {
	var m matcher
	switch strings.ToLower(ruleType) {
	case domain:
		m = domainMatcher(value)
	case domainSuffix:
		m = domainSuffixMatcher(value)
	case domainKeyword:
		m = domainKeywordMatcher(value)
	case geoSite:
		gm, err := newGeoSiteMatcher(value)
		if err != nil {
			return nil, err
		}
		m = gm
	case ipCIDR:
		cm, err := newCIDRMatcher(value)
		if err != nil {
			return nil, err
		}
		m = cm
	case final:
		m = finalMatcher{}
	default:
		return nil, errors.New("unsupported rule type: " + ruleType)
	}

	r := &rule{
		rtype:   strings.ToLower(ruleType),
		policy:  strings.ToLower(policy),
		matcher: m,
	}
	return r, nil
}

// there are two form of rules:
//   - ordinary rule: type,value,policy
//   - final rule: FINAL,policy
func parseRule(rule string) (*rule, error) {
	parts := strings.Split(rule, ",")
	switch len(parts) {
	case 2:
		rtype := parts[0]
		if !strings.EqualFold(rtype, final) {
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

type Rules []*rule

func Parse(rules []string) (Rules, error) {
	if len(rules) == 0 {
		return nil, errors.New("empty rules")
	}

	parsed := make([]*rule, len(rules))
	for i, rawRule := range rules {
		ru, err := parseRule(rawRule)
		if err != nil {
			return nil, err
		}
		if ru.rtype == final && i != len(rules)-1 {
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

func (rs Rules) Match(host string) (policy string, err error) {
	h, err := parseHost(host)
	if err != nil {
		return "", err
	}

	// did consider using cache to speed up the matching,
	// but here is not the performance bottleneck
	for _, r := range rs {
		// do not dive deep if the rule type is not match
		switch r.rtype {
		case domain, domainSuffix, domainKeyword, geoSite:
			if h.Domain == "" {
				continue
			}
		case ipCIDR:
			if h.IP == nil {
				continue
			}
		}
		if r.Match(h) {
			return r.policy, nil
		}
	}
	return Direct, nil
}
