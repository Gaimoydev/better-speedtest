package geo

import (
	"strings"

	"better-speedtest/internal/config"
)

func Resolve(cfg *config.Config) *Location {
	table := NewTable(cfg)
	cache := map[string]*Location{}
	get := func(src string) *Location {
		if l, ok := cache[src]; ok {
			return l
		}
		var l *Location
		switch {
		case src == "manual":
			l = ManualLocation(cfg)
		case src == "at":
			if plmn, err := DetectAT(cfg); err == nil && plmn != "" {
				l = &Location{PLMN: plmn, Carrier: CarrierName(cfg, plmn, table), Region: RegionOf(cfg, plmn, table)}
				if len(plmn) >= 3 {
					l.MCC = plmn[:3]
				}
				if l.Carrier != "" {
					l.setSource("carrier", "at")
				}
			}
		case strings.HasPrefix(src, "geo:"):
			if r, err := QueryProvider(cfg, strings.TrimPrefix(src, "geo:")); err == nil {
				l = r
			}
		}
		cache[src] = l
		return l
	}

	out := &Location{Sources: map[string]string{}}

	for _, src := range cfg.DetectOrder.Carrier {
		l := get(src)
		if l == nil || l.Carrier == "" {
			continue
		}
		out.Carrier = l.Carrier
		out.setSource("carrier", src)
		if l.PLMN != "" {
			out.PLMN, out.MCC = l.PLMN, l.MCC
		}
		if l.Region != "" {
			out.Region = l.Region
		}
		break
	}

	for _, src := range cfg.DetectOrder.Location {
		if out.IP != "" && out.Province != "" && out.City != "" && (out.Lat != 0 || out.Lon != 0) {
			break
		}
		l := get(src)
		if l == nil {
			continue
		}
		if out.IP == "" && l.IP != "" {
			out.IP = l.IP
			out.setSource("ip", src)
		}
		if out.Province == "" && l.Province != "" {
			out.Province = l.Province
			out.setSource("province", src)
		}
		if out.City == "" && l.City != "" {
			out.City = l.City
			out.setSource("city", src)
		}
		if out.Lat == 0 && out.Lon == 0 && (l.Lat != 0 || l.Lon != 0) {
			out.Lat, out.Lon = l.Lat, l.Lon
			out.setSource("latlon", src)
		}
	}

	if out.Region == "" && out.PLMN != "" {
		out.Region = RegionOf(cfg, out.PLMN, table)
	}
	return out
}
