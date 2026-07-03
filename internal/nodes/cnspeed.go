package nodes

import (
	"crypto/md5"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"better-speedtest/internal/config"
	"better-speedtest/internal/httpx"
)

var reKey = regexp.MustCompile(`1-[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}`)

func md5hex(s string) string {
	h := md5.Sum([]byte(s))
	return hex.EncodeToString(h[:])
}

func GenIMEI() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return "TS" + strings.ToUpper(md5hex(hex.EncodeToString(b))[8:24])
}

func Token(cfg *config.Config, imei string, t int64) string {
	p1 := strings.ReplaceAll(cfg.CNSpeed.Token.Part1, "{imei}", imei)
	p2 := strings.ReplaceAll(cfg.CNSpeed.Token.Part2, "{t}", strconv.FormatInt(t, 10))
	return md5hex(md5hex(p1) + md5hex(p2))
}

func FetchNearby(cfg *config.Config, province, city, oper string) ([]Node, error) {
	q := url.Values{}
	q.Set("ip", "")
	q.Set("network", "4")
	q.Set("province", province)
	q.Set("city", city)
	q.Set("wifioper", oper)
	q.Set("mobileoperid", "0")
	q.Set("ipv6", "0")
	q.Set("model", "Android")
	q.Set("pkg", cfg.CNSpeed.Pkg)
	u := cfg.CNSpeed.ControlBase + cfg.CNSpeed.Paths["mobilematch"] + "?" + q.Encode()
	body, err := httpGet(httpx.Insecure(httpTimeout(cfg)), u, cfg.HTTP.ChromeUA)
	if err != nil {
		return nil, err
	}
	var raw []struct {
		Pname    string `json:"pname"`
		City     string `json:"city"`
		Port     string `json:"port"`
		Hostname string `json:"hostname"`
		HostIP   string `json:"hostip"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("mobilematch parse: %w", err)
	}
	out := make([]Node, 0, len(raw))
	for _, n := range raw {
		out = append(out, Node{
			Name: n.Hostname, Source: "cnspeed", Carrier: oper,
			Province: n.Pname, City: n.City, HostIP: n.HostIP, Port: n.Port,
		})
	}
	return out, nil
}

func FetchServerlist(cfg *config.Config) ([]Node, error) {
	u := cfg.CNSpeed.DownloadBase + cfg.CNSpeed.Paths["serverlist"]
	body, err := httpGet(httpx.Insecure(30*time.Second), u, "okhttp/3")
	if err != nil {
		return nil, err
	}
	plain, err := decodeHexDES([]byte(cfg.CNSpeed.DesKey), []byte(strings.TrimSpace(string(body))))
	if err != nil {
		return nil, fmt.Errorf("serverlist decrypt: %w", err)
	}
	var raw []struct {
		HostIP   string      `json:"hostip"`
		Hostname string      `json:"hostname"`
		Location string      `json:"location"`
		Oper     string      `json:"oper"`
		Pname    string      `json:"pname"`
		Port     json.Number `json:"port"`
	}
	if err := json.Unmarshal(plain, &raw); err != nil {
		return nil, fmt.Errorf("serverlist parse: %w", err)
	}
	out := make([]Node, 0, len(raw))
	for _, n := range raw {
		out = append(out, Node{
			Name: n.Hostname, Source: "cnspeed", Carrier: n.Oper,
			Province: n.Pname, City: n.Location, HostIP: n.HostIP, Port: n.Port.String(),
		})
	}
	return out, nil
}

func Dovalid(client *http.Client, cfg *config.Config, ip, port string) (string, error) {
	imei := GenIMEI()
	t := time.Now().Unix()
	q := url.Values{}
	q.Set("key", "")
	q.Set("flag", "true")
	q.Set("bandwidth", "200")
	q.Set("model", "Android")
	q.Set("imei", imei)
	q.Set("time", strconv.FormatInt(t, 10))
	q.Set("app", "globalspeed")
	q.Set("token", Token(cfg, imei, t))
	q.Set("pkg", cfg.CNSpeed.Pkg)
	u := fmt.Sprintf("http://%s:%s%s?%s", ip, port, cfg.CNSpeed.Paths["node_dovalid"], q.Encode())
	body, err := httpGet(client, u, "Dalvik/2.1.0 (Linux; U; Android 12; RMX3031 Build/SP1A.210812.016)")
	if err != nil {
		return "", err
	}
	s := string(body)
	if m := reKey.FindString(s); m != "" {
		return m, nil
	}
	if strings.Contains(s, "-1-") {
		return "", fmt.Errorf("node busy / queued (-1-)")
	}
	return "", fmt.Errorf("dovalid unexpected response: %q", truncate(s, 48))
}

func httpGet(client *http.Client, u, ua string) ([]byte, error) {
	req, err := http.NewRequest(http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	if ua != "" {
		req.Header.Set("User-Agent", ua)
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	return body, nil
}

func httpTimeout(cfg *config.Config) time.Duration {
	if cfg.HTTP.TimeoutS > 0 {
		return time.Duration(cfg.HTTP.TimeoutS) * time.Second
	}
	return 12 * time.Second
}

func truncate(s string, n int) string {
	if len(s) > n {
		return s[:n]
	}
	return s
}
