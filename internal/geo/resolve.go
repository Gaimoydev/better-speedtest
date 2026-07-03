package geo

import (
	"fmt"
	"strings"

	"better-speedtest/internal/config"
)

func Resolve(cfg *config.Config) *Location { return ResolveWithStatus(cfg, nil) }

func ResolveWithStatus(cfg *config.Config, status func(string)) *Location {
	table := NewTable(cfg)
	out := &Location{Sources: map[string]string{}}
	say := func(s string) {
		if status != nil && s != "" {
			status(s)
		}
	}
	sourceOf := func(l *Location, field, fallback string) string {
		if l != nil && l.Sources != nil && l.Sources[field] != "" {
			return l.Sources[field]
		}
		return fallback
	}
	merge := func(l *Location, src string) bool {
		if l == nil {
			return false
		}
		changed := false
		if out.Carrier == "" && l.Carrier != "" {
			out.Carrier = l.Carrier
			out.setSource("carrier", sourceOf(l, "carrier", src))
			changed = true
		}
		if out.IP == "" && l.IP != "" {
			out.IP = l.IP
			out.setSource("ip", sourceOf(l, "ip", src))
			changed = true
		}
		if out.Province == "" && l.Province != "" {
			out.Province = l.Province
			out.setSource("province", sourceOf(l, "province", src))
			changed = true
		}
		if out.City == "" && l.City != "" {
			out.City = l.City
			out.setSource("city", sourceOf(l, "city", src))
			changed = true
		}
		if out.Lat == 0 && out.Lon == 0 && (l.Lat != 0 || l.Lon != 0) {
			out.Lat, out.Lon = l.Lat, l.Lon
			out.setSource("latlon", sourceOf(l, "latlon", src))
			changed = true
		}
		if out.PLMN == "" && l.PLMN != "" {
			out.PLMN = l.PLMN
			out.MCC = l.MCC
			changed = true
		}
		if out.Region == "" && l.Region != "" {
			out.Region = l.Region
			changed = true
		}
		return changed
	}
	complete := func() bool {
		return out.Carrier != "" && out.Province != "" && out.City != ""
	}
	hasAnyGeo := func() bool {
		return out.IP != "" || out.Carrier != "" || out.Province != "" || out.City != ""
	}
	shortErr := func(err error) string {
		if err == nil {
			return ""
		}
		s := err.Error()
		if len(s) > 80 {
			s = s[:80] + "..."
		}
		return s
	}

	if l := ManualLocation(cfg); l != nil {
		say("使用手动定位覆盖")
		merge(l, "manual")
	}

	if out.Carrier == "" {
		say("检测 AT 运营商…")
		if plmn, err := DetectAT(cfg); err == nil && plmn != "" {
			l := &Location{PLMN: plmn, Carrier: CarrierName(cfg, plmn, table), Region: RegionOf(cfg, plmn, table)}
			if len(plmn) >= 3 {
				l.MCC = plmn[:3]
			}
			if l.Carrier != "" {
				l.setSource("carrier", "at")
			}
			merge(l, "at")
			say("AT 运营商: " + nonEmpty(l.Carrier, plmn))
		} else {
			say("AT 运营商检测失败,继续定位")
		}
	}

	if !complete() {
		say("请求 cnspeed 定位…")
		if l, err := QueryProvider(cfg, "cnspeed"); err == nil && l != nil {
			if merge(l, "geo:cnspeed") {
				say("cnspeed 定位成功: " + locSummary(l))
			} else {
				say("cnspeed 未返回新的定位信息")
			}
		} else {
			say("cnspeed 定位失败: " + shortErr(err))
		}
	}

	if !complete() {
		type result struct {
			name string
			loc  *Location
			err  error
		}
		providers := []string{"aapq", "ipip"}
		ch := make(chan result, len(providers))
		say("并发请求 aapq / ipip 定位…")
		for _, name := range providers {
			go func(name string) {
				l, err := QueryProvider(cfg, name)
				ch <- result{name: name, loc: l, err: err}
			}(name)
		}
		for range providers {
			r := <-ch
			if r.err != nil || r.loc == nil {
				say(fmt.Sprintf("%s 定位失败: %s", r.name, shortErr(r.err)))
				continue
			}
			if merge(r.loc, "geo:"+r.name) {
				say(fmt.Sprintf("%s 定位成功: %s", r.name, locSummary(r.loc)))
			} else {
				say(r.name + " 未返回新的定位信息")
			}
			if complete() {
				break
			}
		}
	}

	if out.Region == "" && out.PLMN != "" {
		out.Region = RegionOf(cfg, out.PLMN, table)
	}
	if hasAnyGeo() {
		say("定位完成: " + locSummary(out))
	} else {
		say("定位失败,继续测速")
	}
	return out
}

func nonEmpty(a, b string) string {
	if strings.TrimSpace(a) != "" {
		return a
	}
	return b
}

func locSummary(l *Location) string {
	if l == nil {
		return "-"
	}
	parts := []string{}
	for _, s := range []string{l.Carrier, l.Province, l.City, l.IP} {
		if strings.TrimSpace(s) != "" {
			parts = append(parts, strings.TrimSpace(s))
		}
	}
	if len(parts) == 0 {
		return "-"
	}
	return strings.Join(parts, "/")
}
