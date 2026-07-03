package geo

import "better-speedtest/internal/config"

func ManualLocation(cfg *config.Config) *Location {
	m := cfg.Manual
	if m.Carrier == "" && m.Province == "" && m.City == "" {
		return nil
	}
	l := &Location{
		Carrier:  normalizeCarrier(m.Carrier),
		Province: cleanPlace(m.Province),
		City:     cleanPlace(m.City),
	}
	if m.Carrier != "" {
		l.setSource("carrier", "manual")
	}
	if m.Province != "" {
		l.setSource("province", "manual")
	}
	if m.City != "" {
		l.setSource("city", "manual")
	}
	return l
}
