package route

/*
var globalGeoIPList *router.GeoIPList

func extractCountryIP(country string) ([]*router.CIDR, error) {
	if globalGeoIPList == nil {
		geoIPList, err := loadGeoIPFile()
		if err != nil {
			return nil, err
		}
		globalGeoIPList = geoIPList
	}

	for _, geoip := range globalGeoIPList.Entry {
		if strings.EqualFold(geoip.CountryCode, country) {
			return geoip.Cidr, nil
		}
	}
	return nil, errors.New("unsupported geoip country: " + country)
}

func loadGeoIPFile() (*router.GeoIPList, error) {
	var data []byte
	var err error
	for _, dir := range assetDirs {
		data, err = os.ReadFile(path.Join(dir, "geoip.dat"))
		if err == nil {
			break
		}
	}
	if err != nil {
		return nil, err
	}

	geoIPList := new(router.GeoIPList)
	err = proto.Unmarshal(data, geoIPList)
	if err != nil {
		return nil, err
	}
	return geoIPList, nil
}

type geoIPMatcher struct {
	country  string
	masks    []uint32
	prefixes []uint32
}

// deprecated
func newGeoIPMatcher(country string) (*geoIPMatcher, error) {
	cidrs, err := extractCountryIP(country)
	if err != nil {
		return nil, err
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

func (g *geoIPMatcher) Match(addr *proxy.Listen) bool {
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
*/
