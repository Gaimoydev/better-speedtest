package geo

import "strings"

type Location struct {
	IP       string            `json:"ip"`
	Carrier  string            `json:"carrier"`
	Province string            `json:"province"`
	City     string            `json:"city"`
	Lat      float64           `json:"lat"`
	Lon      float64           `json:"lon"`
	Region   string            `json:"region"`
	MCC      string            `json:"mcc"`
	PLMN     string            `json:"plmn"`
	Sources  map[string]string `json:"sources"`
}

func (l *Location) setSource(field, provider string) {
	if l.Sources == nil {
		l.Sources = map[string]string{}
	}
	l.Sources[field] = provider
}

func cleanPlace(s string) string {
	s = strings.TrimSpace(s)
	for _, suf := range []string{"特别行政区", "自治区", "省", "市", "地区"} {
		if strings.HasSuffix(s, suf) && len([]rune(s)) > len([]rune(suf)) {
			s = strings.TrimSuffix(s, suf)
			break
		}
	}
	return s
}
