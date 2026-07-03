package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadDeepMerge(t *testing.T) {
	def := []byte(`{
		"version":1,
		"engine":{"threads_dl":8,"threads_ul":4,"duration_s":15},
		"manual":{"carrier":"","province":""},
		"cnspeed":{"enabled":true,"des_key":"dw!@#$%^","paths":{"iploc":"/a"}},
		"cdn_sources":[{"name":"X","dl":"u"}]
	}`)
	dir := t.TempDir()
	up := filepath.Join(dir, "config.json")
	if err := os.WriteFile(up, []byte(`{"engine":{"threads_dl":16},"manual":{"carrier":"移动"}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(def, up)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Engine.ThreadsDL != 16 {
		t.Errorf("threads_dl override: got %d want 16", cfg.Engine.ThreadsDL)
	}
	if cfg.Engine.ThreadsUL != 4 {
		t.Errorf("deep-merge lost sibling threads_ul: got %d want 4", cfg.Engine.ThreadsUL)
	}
	if cfg.Manual.Carrier != "移动" {
		t.Errorf("carrier override: got %q want 移动", cfg.Manual.Carrier)
	}
	if !cfg.CNSpeed.Enabled || cfg.CNSpeed.DesKey != "dw!@#$%^" {
		t.Errorf("cnspeed lost: %+v", cfg.CNSpeed)
	}
	if len(cfg.CDNSources) != 1 || cfg.CDNSources[0].Name != "X" {
		t.Errorf("cdn_sources: %+v", cfg.CDNSources)
	}
}

func TestLoadNoUserFile(t *testing.T) {
	def := []byte(`{"version":1,"engine":{"threads_dl":8}}`)
	cfg, err := Load(def, filepath.Join(t.TempDir(), "nonexistent.json"))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Engine.ThreadsDL != 8 {
		t.Errorf("default lost: %d", cfg.Engine.ThreadsDL)
	}
}
