package geo

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"better-speedtest/internal/config"
	"better-speedtest/internal/httpx"
)

func QueryProvider(cfg *config.Config, name string) (*Location, error) {
	p, ok := cfg.GeoProviders[name]
	if !ok {
		return nil, fmt.Errorf("unknown geo provider %q", name)
	}
	to := time.Duration(cfg.HTTP.TimeoutS) * time.Second
	if to <= 0 {
		to = 12 * time.Second
	}
	var client *http.Client
	switch {
	case p.Fingerprint == "chrome":
		client = httpx.Chrome(to)
	case strings.Contains(p.URL, "cnspeedtest"):
		client = httpx.Insecure(to)
	default:
		client = httpx.Plain(to)
	}
	req, err := http.NewRequest(http.MethodGet, p.URL, nil)
	if err != nil {
		return nil, err
	}
	if cfg.HTTP.ChromeUA != "" {
		req.Header.Set("User-Agent", cfg.HTTP.ChromeUA)
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("provider %q: HTTP %d", name, resp.StatusCode)
	}
	switch p.Format {
	case "json":
		return parseJSONProvider(name, body, p.Map)
	case "pipe":
		return parsePipeProvider(name, string(body), p.Map)
	default:
		return nil, fmt.Errorf("provider %q: unknown format %q", name, p.Format)
	}
}

func parseJSONProvider(name string, body []byte, m map[string]interface{}) (*Location, error) {
	var obj interface{}
	if err := json.Unmarshal(body, &obj); err != nil {
		return nil, err
	}
	l := &Location{}
	if lap, ok := m["loc_array"].(string); ok {
		if ipp, ok := m["ip"].(string); ok {
			l.IP = asString(getPath(obj, ipp))
		}
		arr, _ := getPath(obj, lap).([]interface{})
		pick := func(key string) string {
			idx, ok := m[key].(float64)
			if !ok || int(idx) < 0 || int(idx) >= len(arr) {
				return ""
			}
			return asString(arr[int(idx)])
		}
		l.Province = cleanPlace(pick("province_idx"))
		l.City = cleanPlace(pick("city_idx"))
		l.Carrier = normalizeCarrier(pick("carrier_idx"))
	} else {
		get := func(key string) string {
			if p, ok := m[key].(string); ok {
				return asString(getPath(obj, p))
			}
			return ""
		}
		l.IP = get("ip")
		l.Province = cleanPlace(get("province"))
		l.City = cleanPlace(get("city"))
		l.Carrier = normalizeCarrier(get("carrier"))
		if p, ok := m["lat"].(string); ok {
			l.Lat = asFloat(getPath(obj, p))
		}
		if p, ok := m["lon"].(string); ok {
			l.Lon = asFloat(getPath(obj, p))
		}
	}
	markSources(l, name)
	return l, nil
}

func parsePipeProvider(name, body string, m map[string]interface{}) (*Location, error) {
	parts := strings.Split(strings.TrimSpace(body), "|")
	idx := func(key string) int {
		if f, ok := m[key].(float64); ok {
			return int(f)
		}
		return -1
	}
	l := &Location{}
	if i := idx("ip_field"); i >= 0 && i < len(parts) {
		l.IP = strings.TrimSpace(parts[i])
	}
	if i := idx("loc_array_field"); i >= 0 && i < len(parts) {
		var arr []string
		if err := json.Unmarshal([]byte(parts[i]), &arr); err == nil {
			if len(arr) > 1 {
				l.Province = cleanPlace(arr[1])
			}
			if len(arr) > 2 {
				l.City = cleanPlace(arr[2])
			}
			if len(arr) > 4 {
				l.Carrier = normalizeCarrier(arr[4])
			}
		}
	}
	if i := idx("carrier_field"); i >= 0 && i < len(parts) && strings.TrimSpace(parts[i]) != "" {
		l.Carrier = normalizeCarrier(parts[i])
	}
	markSources(l, name)
	return l, nil
}

func markSources(l *Location, name string) {
	for f, v := range map[string]string{"ip": l.IP, "province": l.Province, "city": l.City, "carrier": l.Carrier} {
		if v != "" {
			l.setSource(f, name)
		}
	}
	if l.Lat != 0 || l.Lon != 0 {
		l.setSource("latlon", name)
	}
}

func getPath(obj interface{}, path string) interface{} {
	cur := obj
	for _, part := range strings.Split(path, ".") {
		m, ok := cur.(map[string]interface{})
		if !ok {
			return nil
		}
		cur = m[part]
	}
	return cur
}

func asString(v interface{}) string {
	switch x := v.(type) {
	case string:
		return x
	case float64:
		return strconv.FormatFloat(x, 'f', -1, 64)
	case bool:
		return strconv.FormatBool(x)
	}
	return ""
}

func asFloat(v interface{}) float64 {
	switch x := v.(type) {
	case float64:
		return x
	case string:
		f, _ := strconv.ParseFloat(strings.TrimSpace(x), 64)
		return f
	}
	return 0
}
