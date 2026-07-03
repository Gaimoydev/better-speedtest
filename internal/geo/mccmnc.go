package geo

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"time"

	app "better-speedtest"
	"better-speedtest/internal/config"
	"better-speedtest/internal/httpx"
)

func MccMncPath() string {
	if p := os.Getenv("BETTER_SPEEDTEST_CONFIG"); p != "" {
		return filepath.Join(filepath.Dir(p), "mcc-mnc.csv")
	}
	if runtime.GOOS == "linux" {
		return "/data/plugins/better-speedtest/mcc-mnc.csv"
	}
	if dir, err := os.UserConfigDir(); err == nil {
		return filepath.Join(dir, "better-speedtest", "mcc-mnc.csv")
	}
	return "mcc-mnc.csv"
}

type Table struct{ byPLMN map[string]mccRow }

type mccRow struct{ Region, Country, Operator, Brand string }

var autoMccMncUpdateStarted atomic.Bool

func NewTable(_ *config.Config) *Table {
	data := app.MccMnc
	if b, err := os.ReadFile(MccMncPath()); err == nil && validMccHeader(b) {
		data = b
	}
	return &Table{byPLMN: parseMcc(data)}
}

func UpdateMccMnc(cfg *config.Config) (int, error) {
	unlock, ok := acquireMccMncLock()
	if !ok {
		return 0, errors.New("MCC-MNC 表正在更新")
	}
	defer unlock()
	return downloadMccMnc(cfg)
}

func UpdateMccMncIfStale(cfg *config.Config) (int, bool, error) {
	if !MccMncNeedsRefresh(cfg) {
		return 0, false, nil
	}
	unlock, ok := acquireMccMncLock()
	if !ok {
		return 0, false, nil
	}
	defer unlock()
	if !MccMncNeedsRefresh(cfg) {
		return 0, false, nil
	}
	n, err := downloadMccMnc(cfg)
	return n, err == nil, err
}

func StartMccMncAutoUpdate(cfg *config.Config) bool {
	if !MccMncNeedsRefresh(cfg) || !autoMccMncUpdateStarted.CompareAndSwap(false, true) {
		return false
	}
	if exe, err := os.Executable(); err == nil {
		cmd := exec.Command(exe, "update", "--if-stale", "--quiet")
		cmd.Env = os.Environ()
		if err := cmd.Start(); err == nil {
			go func() { _ = cmd.Wait() }()
			return true
		}
	}
	go func() {
		_, _, _ = UpdateMccMncIfStale(cfg)
	}()
	return true
}

func MccMncNeedsRefresh(cfg *config.Config) bool {
	if cfg == nil || cfg.Updates.MccMncURL == "" || cfg.Updates.MccMncRefreshDays <= 0 {
		return false
	}
	path := MccMncPath()
	st, err := os.Stat(path)
	if err != nil || st.IsDir() {
		return true
	}
	if b, err := os.ReadFile(path); err != nil || !validMccHeader(b) {
		return true
	}
	maxAge := time.Duration(cfg.Updates.MccMncRefreshDays) * 24 * time.Hour
	return time.Since(st.ModTime()) > maxAge
}

func acquireMccMncLock() (func(), bool) {
	lock := MccMncPath() + ".lock"
	if st, err := os.Stat(lock); err == nil && time.Since(st.ModTime()) > 15*time.Minute {
		_ = os.Remove(lock)
	}
	f, err := os.OpenFile(lock, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, false
	}
	_, _ = fmt.Fprintf(f, "%d\n", os.Getpid())
	_ = f.Close()
	return func() { _ = os.Remove(lock) }, true
}

func downloadMccMnc(cfg *config.Config) (int, error) {
	url := cfg.Updates.MccMncURL
	if url == "" {
		return 0, errors.New("updates.mccmnc_url 未配置")
	}
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return 0, err
	}
	if cfg.HTTP.ChromeUA != "" {
		req.Header.Set("User-Agent", cfg.HTTP.ChromeUA)
	}
	resp, err := httpx.Plain(30 * time.Second).Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return 0, err
	}
	if !validMccHeader(body) {
		return 0, errors.New("下载的表头校验失败(非 MCC;MNC;PLMN...)")
	}
	dst := MccMncPath()
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return 0, err
	}
	tmp := fmt.Sprintf("%s.tmp.%d", dst, os.Getpid())
	defer os.Remove(tmp)
	if err := os.WriteFile(tmp, body, 0o644); err != nil {
		return 0, err
	}
	if runtime.GOOS == "windows" {
		_ = os.Remove(dst)
	}
	if err := os.Rename(tmp, dst); err != nil {
		return 0, err
	}
	return len(parseMcc(body)), nil
}

func validMccHeader(b []byte) bool {
	b = bytes.TrimPrefix(b, []byte("\xef\xbb\xbf"))
	line := b
	if i := bytes.IndexByte(b, '\n'); i >= 0 {
		line = b[:i]
	}
	return bytes.HasPrefix(line, []byte("MCC;MNC;PLMN"))
}

func parseMcc(b []byte) map[string]mccRow {
	b = bytes.TrimPrefix(b, []byte("\xef\xbb\xbf"))
	out := map[string]mccRow{}
	for i, ln := range strings.Split(string(b), "\n") {
		ln = strings.TrimRight(ln, "\r")
		if i == 0 || ln == "" {
			continue
		}
		f := strings.Split(ln, ";")
		if len(f) < 8 {
			continue
		}
		plmn := strings.TrimSpace(f[2])
		if plmn == "" {
			continue
		}
		out[plmn] = mccRow{Region: f[3], Country: f[4], Operator: f[6], Brand: f[7]}
	}
	return out
}

func (t *Table) Lookup(plmn string) (mccRow, bool) { r, ok := t.byPLMN[plmn]; return r, ok }

func CarrierName(cfg *config.Config, plmn string, t *Table) string {
	if c, ok := cfg.CarrierMap[plmn]; ok {
		return c
	}
	if r, ok := t.Lookup(plmn); ok {
		if r.Brand != "" {
			return r.Brand
		}
		return r.Operator
	}
	return ""
}

func RegionOf(_ *config.Config, plmn string, t *Table) string {
	if r, ok := t.Lookup(plmn); ok && r.Region != "" {
		return r.Region
	}
	if len(plmn) >= 1 {
		switch plmn[0] {
		case '2':
			return "Europe"
		case '3':
			return "North America"
		case '4':
			return "Asia"
		case '5':
			return "Oceania"
		case '6':
			return "Africa"
		case '7':
			return "South America"
		}
	}
	return ""
}

var carrierAliases = []struct{ needle, name string }{
	{"移动", "移动"}, {"china mobile", "移动"}, {"cmcc", "移动"},
	{"联通", "联通"}, {"china unicom", "联通"},
	{"电信", "电信"}, {"china telecom", "电信"}, {"chinanet", "电信"},
	{"广电", "广电"}, {"china broadnet", "广电"}, {"china broadcast", "广电"},
}

func normalizeCarrier(s string) string {
	low := strings.ToLower(strings.TrimSpace(s))
	for _, a := range carrierAliases {
		if strings.Contains(low, a.needle) || strings.Contains(s, a.needle) {
			return a.name
		}
	}
	return strings.TrimSpace(s)
}
