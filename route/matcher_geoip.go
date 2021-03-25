package route

import (
	"encoding/binary"
	"errors"
	"io/ioutil"
	"sort"
	"strings"

	"github.com/v2fly/v2ray-core/v4/app/router"
	"google.golang.org/protobuf/proto"
	"yeager/protocol"
)

var geo2CIDR = make(map[string][]*router.CIDR)

func loadGeoIpFile(filename string) error {
	geoIPList := new(router.GeoIPList)
	data, err := ioutil.ReadFile(filename)
	if err != nil {
		return err
	}
	err = proto.Unmarshal(data, geoIPList)
	if err != nil {
		return err
	}

	for _, geoip := range geoIPList.Entry {
		geo2CIDR[strings.ToLower(geoip.CountryCode)] = geoip.Cidr
	}
	return nil
}

type geoIPMatcher struct {
	country  string
	masks    []uint32
	prefixes []uint32
}

func newGeoIPMatcher(country string) (*geoIPMatcher, error) {
	cidrs, ok := geo2CIDR[country]
	if !ok {
		return nil, errors.New("unsupported country code: " + country)
	}
	cidrList := router.CIDRList(cidrs)
	sort.Sort(&cidrList)

	masks := make([]uint32, 0, len(cidrList))
	prefixes := make([]uint32, 0, len(cidrList))
	for _, cidr := range cidrList {
		if len(cidr.Ip) != 4 {
			// ipv6 not supported yet
			continue
		}
		masks = append(masks, mask(binary.BigEndian.Uint32(cidr.Ip), cidr.Prefix))
		prefixes = append(prefixes, cidr.Prefix)
	}

	m := &geoIPMatcher{
		country:  country,
		masks:    masks,
		prefixes: prefixes,
	}
	return m, nil
}

func (g *geoIPMatcher) Match(addr *protocol.Address) bool {
	if len(g.masks) == 0 {
		return false
	}

	ip := binary.BigEndian.Uint32(addr.IP)
	if ip < g.masks[0] {
		return false
	}

	l, r := 0, len(g.masks)
	for l <= r {
		mid := l + ((r - l) >> 1)
		if ip < g.masks[mid] {
			r = mid - 1
			continue
		}
		if g.masks[mid] == mask(ip, g.prefixes[mid]) {
			return true
		}
		l = mid + 1
	}

	return false
}

func mask(ip uint32, prefix uint32) uint32 {
	ip = ip >> (32 - prefix)
	ip = ip << (32 - prefix)
	return ip
}
