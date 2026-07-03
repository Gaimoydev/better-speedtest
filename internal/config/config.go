package config

import (
	"encoding/json"
	"fmt"
	"os"
)

type Config struct {
	Version         int                    `json:"version"`
	Engine          Engine                 `json:"engine"`
	Manual          Manual                 `json:"manual"`
	DetectOrder     DetectOrder            `json:"detect_order"`
	AT              AT                     `json:"at"`
	CarrierMap      map[string]string      `json:"carrier_map"`
	CarrierFallback map[string]string      `json:"carrier_fallback"`
	GeoProviders    map[string]GeoProvider `json:"geo_providers"`
	CNSpeed         CNSpeed                `json:"cnspeed"`
	CDNSources      []CDNSource            `json:"cdn_sources"`
	Ookla           Ookla                  `json:"ookla"`
	Install         Install                `json:"install"`
	Updates         Updates                `json:"updates"`
	HTTP            HTTP                   `json:"http"`
}

type Engine struct {
	ThreadsDL        int    `json:"threads_dl"`
	ThreadsUL        int    `json:"threads_ul"`
	ThreadsDLMax     int    `json:"threads_dl_max"`
	ThreadsULMax     int    `json:"threads_ul_max"`
	DurationS        int    `json:"duration_s"`
	WarmupS          int    `json:"warmup_s"`
	SampleIntervalMS int    `json:"sample_interval_ms"`
	ChunkBytes       int    `json:"chunk_bytes"`
	WANIface         string `json:"wan_iface"`
	LogPath          string `json:"log_path"`
}

type Manual struct {
	Carrier  string `json:"carrier"`
	Province string `json:"province"`
	City     string `json:"city"`
}

type DetectOrder struct {
	Carrier  []string `json:"carrier"`
	Location []string `json:"location"`
}

type AT struct {
	Ports         []string          `json:"ports"`
	ReadTimeoutMS int               `json:"read_timeout_ms"`
	Cmds          map[string]string `json:"cmds"`
}

type GeoProvider struct {
	URL         string                 `json:"url"`
	Fingerprint string                 `json:"fingerprint"`
	Format      string                 `json:"format"`
	Map         map[string]interface{} `json:"map"`
}

type CNSpeed struct {
	Enabled      bool              `json:"enabled"`
	ControlBase  string            `json:"control_base"`
	DownloadBase string            `json:"download_base"`
	Pkg          string            `json:"pkg"`
	DesKey       string            `json:"des_key"`
	Paths        map[string]string `json:"paths"`
	Token        CNToken           `json:"token"`
}

type CNToken struct {
	Algo  string `json:"algo"`
	Part1 string `json:"part1"`
	Part2 string `json:"part2"`
}

type CDNSource struct {
	Name     string `json:"name"`
	DL       string `json:"dl"`
	UL       string `json:"ul"`
	Region   string `json:"region"`
	Enhanced bool   `json:"enhanced"`
	UA       string `json:"ua"` // optional per-source User-Agent (some CDNs 403 the default Chrome UA)
	Note     string `json:"note"`
}

type Ookla struct {
	Enabled      bool   `json:"enabled"`
	ConfigURL    string `json:"config_url"`
	DownloadSize int    `json:"download_size"`
	Scheme       string `json:"scheme"`
	Limit        int    `json:"limit"`
}

type Install struct {
	ManifestURL string `json:"manifest_url"`
	BinaryURL   string `json:"binary_url"`
	Proxy       string `json:"proxy"`
	SHA256      string `json:"sha256"`
	Dest        string `json:"dest"`
	AutoCheck   bool   `json:"auto_check"`
}

type Updates struct {
	MccMncURL              string `json:"mccmnc_url"`
	MccMncRefreshDays      int    `json:"mccmnc_refresh_days"`
	ServerlistRefreshHours int    `json:"serverlist_refresh_hours"`
}

type HTTP struct {
	ChromeUA    string `json:"chrome_ua"`
	UtlsProfile string `json:"utls_profile"`
	TimeoutS    int    `json:"timeout_s"`
}

func Load(defaultJSON []byte, userPath string) (*Config, error) {
	base := map[string]interface{}{}
	if err := json.Unmarshal(defaultJSON, &base); err != nil {
		return nil, fmt.Errorf("parse default config: %w", err)
	}
	if userPath != "" {
		if b, err := os.ReadFile(userPath); err == nil {
			over := map[string]interface{}{}
			if err := json.Unmarshal(b, &over); err != nil {
				return nil, fmt.Errorf("parse user config %s: %w", userPath, err)
			}
			deepMerge(base, over)
		}
	}
	merged, err := json.Marshal(base)
	if err != nil {
		return nil, err
	}
	cfg := &Config{}
	if err := json.Unmarshal(merged, cfg); err != nil {
		return nil, fmt.Errorf("build typed config: %w", err)
	}
	return cfg, nil
}

func deepMerge(dst, src map[string]interface{}) {
	for k, sv := range src {
		if dv, ok := dst[k]; ok {
			if dm, ok1 := dv.(map[string]interface{}); ok1 {
				if sm, ok2 := sv.(map[string]interface{}); ok2 {
					deepMerge(dm, sm)
					continue
				}
			}
		}
		dst[k] = sv
	}
}
