package nodes

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strconv"
	"time"

	"better-speedtest/internal/config"
	"better-speedtest/internal/httpx"
)

type ooklaConfig struct {
	Servers []struct {
		Host     string  `json:"host"`
		Sponsor  string  `json:"sponsor"`
		Name     string  `json:"name"`
		Country  string  `json:"country"`
		CC       string  `json:"cc"`
		Distance float64 `json:"distance"`
	} `json:"servers"`
}

// OoklaNodes fetches the speedtest.net (Ookla) server list near the requesting
// IP and returns them as transfer nodes. Each server exposes /download?size=N
// and /upload; those URLs are prebuilt so the CDN engine can drive them
// unchanged. Servers are returned nearest-first, capped at cfg.Ookla.Limit.
func OoklaNodes(cfg *config.Config) ([]Node, error) {
	if !cfg.Ookla.Enabled || cfg.Ookla.ConfigURL == "" {
		return nil, fmt.Errorf("ookla 未启用")
	}
	req, err := http.NewRequest(http.MethodGet, cfg.Ookla.ConfigURL, nil)
	if err != nil {
		return nil, err
	}
	if cfg.HTTP.ChromeUA != "" {
		req.Header.Set("User-Agent", cfg.HTTP.ChromeUA)
	}
	resp, err := httpx.Plain(15 * time.Second).Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("config-sdk HTTP %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return nil, err
	}
	var oc ooklaConfig
	if err := json.Unmarshal(body, &oc); err != nil {
		return nil, fmt.Errorf("config-sdk parse: %w", err)
	}
	sort.SliceStable(oc.Servers, func(i, j int) bool { return oc.Servers[i].Distance < oc.Servers[j].Distance })

	limit := cfg.Ookla.Limit
	if limit <= 0 || limit > len(oc.Servers) {
		limit = len(oc.Servers)
	}
	scheme := cfg.Ookla.Scheme
	if scheme == "" {
		scheme = "https"
	}
	size := cfg.Ookla.DownloadSize
	if size <= 0 {
		size = 25000000
	}
	out := make([]Node, 0, limit)
	for _, s := range oc.Servers[:limit] {
		if s.Host == "" {
			continue
		}
		nc := randNocache()
		base := scheme + "://" + s.Host
		name := s.Sponsor
		if s.Name != "" {
			name += " " + s.Name
		}
		out = append(out, Node{
			Name:     name + " (Ookla)",
			Source:   "ookla",
			Region:   "global",
			Carrier:  s.Sponsor,
			Province: s.Country,
			City:     s.Name,
			HostIP:   s.Host,
			DLURL:    base + "/download?nocache=" + nc + "&size=" + strconv.Itoa(size),
			ULURL:    base + "/upload?nocache=" + nc,
		})
	}
	return out, nil
}

func randNocache() string {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return strconv.FormatInt(time.Now().UnixNano(), 16)
	}
	return hex.EncodeToString(b)
}
