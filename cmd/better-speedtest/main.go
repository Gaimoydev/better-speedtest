package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync/atomic"
	"time"

	app "better-speedtest"
	"better-speedtest/internal/config"
	"better-speedtest/internal/engine"
	"better-speedtest/internal/geo"
	"better-speedtest/internal/httpx"
	"better-speedtest/internal/nodes"
	"better-speedtest/internal/report"
	"better-speedtest/internal/selector"
)

const Version = "0.0.2"

const devConfigPath = "/data/plugins/better-speedtest/config.json"

func configPath() string {
	if p := os.Getenv("BETTER_SPEEDTEST_CONFIG"); p != "" {
		return p
	}
	if runtime.GOOS == "linux" {
		return devConfigPath
	}
	if dir, err := os.UserConfigDir(); err == nil {
		return filepath.Join(dir, "better-speedtest", "config.json")
	}
	return "config.json"
}

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	switch os.Args[1] {
	case "version", "-v", "--version":
		fmt.Printf("better-speedtest %s  (multi-source; config=%dB, mcc-mnc=%dB)\n",
			Version, len(app.DefaultConfig), len(app.MccMnc))
	case "ip":
		cmdIP(os.Args[2:])
	case "nodes":
		cmdNodes(os.Args[2:])
	case "test":
		cmdTest(os.Args[2:])
	case "update":
		cmdUpdate(os.Args[2:])
	default:
		usage()
		os.Exit(2)
	}
}

func usage() {
	fmt.Fprint(os.Stderr, `better-speedtest — 多源 · 跨平台命令行测速
用法:
  better-speedtest version                    版本信息
  better-speedtest ip                         定位 + 运营商
  better-speedtest nodes [--src all]          列出节点(cnspeed|cdn|ookla|all)
  better-speedtest test [--auto]              测速(默认自动:中国大陆→cnspeed,海外→CDN+Speedtest.net 择优)
      --src auto|cnspeed|cdn|ookla     指定测速源
      --node <关键字>                  指定节点/源(IP/城市/名称)
      --dur <秒>                       单向时长(默认取配置)
      --json                           输出 NDJSON(供程序调用;默认为人类进度条)
      --no-upload | --no-download      仅测下行 / 仅测上行
      --multi <N>                      多源聚合:同时从最快的 N 个源传输、吞吐求和(打满高带宽口)
  better-speedtest update [--if-stale]        更新 MCC-MNC 运营商表

配置:BETTER_SPEEDTEST_CONFIG 环境变量指定 config.json;否则用平台默认路径。
`)
}

func cmdUpdate(args []string) {
	fs := flag.NewFlagSet("update", flag.ContinueOnError)
	ifStale := fs.Bool("if-stale", false, "仅在本地 MCC-MNC 表缺失或过期时更新")
	quiet := fs.Bool("quiet", false, "减少输出(适合后台自动更新)")
	if err := fs.Parse(args); err != nil {
		os.Exit(2)
	}
	cfg := loadCfg()
	var n int
	var changed bool
	var err error
	if *ifStale {
		n, changed, err = geo.UpdateMccMncIfStale(cfg)
	} else {
		n, err = geo.UpdateMccMnc(cfg)
		changed = err == nil
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "MCC-MNC 表更新失败:", err)
		os.Exit(1)
	}
	if *quiet {
		return
	}
	if changed {
		fmt.Printf("MCC-MNC 表已更新: %d 条 → %s\n", n, geo.MccMncPath())
	} else {
		fmt.Printf("MCC-MNC 表未过期: %s\n", geo.MccMncPath())
	}
}

func loadCfg() *config.Config {
	cfg, err := config.Load(app.DefaultConfig, configPath())
	if err != nil {
		fmt.Fprintln(os.Stderr, "配置加载失败:", err)
		os.Exit(1)
	}
	return cfg
}

func cmdIP(_ []string) {
	cfg := loadCfg()
	geo.StartMccMncAutoUpdate(cfg)
	loc := geo.Resolve(cfg)
	b, _ := json.MarshalIndent(loc, "", "  ")
	fmt.Println(string(b))
}

func cmdNodes(args []string) {
	fs := flag.NewFlagSet("nodes", flag.ContinueOnError)
	src := fs.String("src", "all", "节点来源: cnspeed|cdn|ookla|all")
	full := fs.Bool("full", false, "cnspeed 用全量表(而非就近 mobilematch)")
	if err := fs.Parse(args); err != nil {
		os.Exit(2)
	}
	cfg := loadCfg()
	geo.StartMccMncAutoUpdate(cfg)
	var out []nodes.Node
	if *src == "cdn" || *src == "all" {
		out = append(out, nodes.CDNNodes(cfg)...)
	}
	if *src == "ookla" || *src == "all" {
		if ok, err := nodes.OoklaNodes(cfg); err != nil {
			if *src == "ookla" {
				fmt.Fprintln(os.Stderr, "ookla 源获取失败:", err)
			}
		} else {
			out = append(out, ok...)
		}
	}
	if *src == "cnspeed" || *src == "all" {
		if cfg.CNSpeed.Enabled {
			loc := geo.Resolve(cfg)
			var cn []nodes.Node
			var err error
			if *full {
				cn, err = nodes.FetchServerlist(cfg)
			} else {
				cn, err = nodes.FetchNearby(cfg, loc.Province, loc.City, loc.Carrier)
			}
			if err != nil {
				fmt.Fprintln(os.Stderr, "cnspeed 节点获取失败:", err)
			} else {
				out = append(out, cn...)
			}
		}
	}
	b, _ := json.MarshalIndent(out, "", "  ")
	fmt.Println(string(b))
}

func cmdTest(args []string) {
	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	fs.Bool("auto", false, "自动选点(就近同运营商;cnspeed 默认行为)")
	src := fs.String("src", "auto", "auto|cnspeed|cdn|ookla(auto:大陆→cnspeed,海外→全球CDN+Speedtest.net择优)")
	dur := fs.Int("dur", 0, "测速时长秒(默认取配置 duration_s)")
	nodeKW := fs.String("node", "", "指定节点关键字(IP/城市/名称);--src cdn/ookla 时为源名")
	noUpload := fs.Bool("no-upload", false, "跳过上传(仅下行)")
	noDownload := fs.Bool("no-download", false, "跳过下载(仅上行)")
	jsonOut := fs.Bool("json", false, "输出 NDJSON 供程序调用(默认为人类进度条界面)")
	multi := fs.Int("multi", 1, "多源聚合:同时从最快的 N 个源传输、吞吐求和(用于打满高带宽口;默认 1=单源)")
	if err := fs.Parse(args); err != nil {
		os.Exit(2)
	}
	report.SetJSON(*jsonOut)
	doDownload, doUpload := !*noDownload, !*noUpload
	if !doDownload && !doUpload {
		report.Status("error", "下载和上传不能都跳过")
		report.Done()
		return
	}
	cfg := loadCfg()
	geo.StartMccMncAutoUpdate(cfg)

	duration := time.Duration(cfg.Engine.DurationS) * time.Second
	if *dur > 0 {
		duration = time.Duration(*dur) * time.Second
	}
	if duration <= 0 {
		duration = 15 * time.Second
	}
	warmup := time.Duration(cfg.Engine.WarmupS) * time.Second
	every := time.Duration(cfg.Engine.SampleIntervalMS) * time.Millisecond
	if every <= 0 {
		every = time.Second
	}
	if every > duration { // else the first sample tick fires only after the test window
		every = duration
	}
	dlN, ulN := cfg.Engine.ThreadsDL, cfg.Engine.ThreadsUL

	iface := cfg.Engine.WANIface
	if iface == "" || iface == "auto" {
		iface = engine.DetectWAN()
	}
	if iface != "" {
		report.Status("start", "iface="+iface)
	} else {
		report.Status("start", "准备测速…")
	}

	loc := geo.Resolve(cfg)
	mainland := isMainlandChina(loc)
	preferRegion := "global"
	if mainland {
		preferRegion = "cn"
	}
	src2 := *src
	if src2 == "auto" {
		if mainland {
			src2 = "cnspeed"
		} else {
			report.Status("geo", fmt.Sprintf("%s/%s 非中国大陆 → CDN/Speedtest.net 择优", loc.Carrier, loc.Province))
		}
	}

	if src2 == "cnspeed" {
		testCNSpeed(cfg, iface, *nodeKW, duration, warmup, every, dlN, ulN, doDownload, doUpload, loc, *multi)
		return
	}
	pool := buildCDNPool(cfg, src2, preferRegion, *nodeKW != "")
	testCDN(cfg, iface, *nodeKW, duration, warmup, every, dlN, ulN, doDownload, doUpload, pool, *multi)
}

func buildCDNPool(cfg *config.Config, src, preferRegion string, hasKW bool) []nodes.Node {
	if src == "ookla" {
		ok, err := nodes.OoklaNodes(cfg)
		if err != nil {
			report.Status("error", "Speedtest.net 源获取失败: "+err.Error())
		}
		return ok
	}
	var pool []nodes.Node
	for _, n := range nodes.CDNNodes(cfg) {
		if hasKW || n.Region == preferRegion {
			pool = append(pool, n)
		}
	}
	if src == "auto" {
		if ok, err := nodes.OoklaNodes(cfg); err == nil {
			pool = append(pool, ok...)
		} else {
			report.Status("node", "Speedtest.net 源跳过("+err.Error()+")")
		}
	}
	return pool
}

// mainlandProvinces are the 31 mainland-China provincial regions (excludes
// 香港/澳门/台湾, which cnspeed has no nodes for → those go to CDN).
var mainlandProvinces = map[string]bool{
	"北京": true, "天津": true, "河北": true, "山西": true, "内蒙古": true, "辽宁": true,
	"吉林": true, "黑龙江": true, "上海": true, "江苏": true, "浙江": true, "安徽": true,
	"福建": true, "江西": true, "山东": true, "河南": true, "湖北": true, "湖南": true,
	"广东": true, "广西": true, "海南": true, "重庆": true, "四川": true, "贵州": true,
	"云南": true, "西藏": true, "陕西": true, "甘肃": true, "青海": true, "宁夏": true, "新疆": true,
}

// isMainlandChina decides whether the current location can use cnspeed (mainland
// per-carrier nodes) or should fall back to global CDN sources. A mainland
// province OR a mainland operator qualifies; 香港/澳门/台湾/海外 do not.
func isMainlandChina(loc *geo.Location) bool {
	p := strings.TrimSpace(loc.Province)
	for _, suf := range []string{"特别行政区", "自治区", "省", "市", "地区"} {
		if strings.HasSuffix(p, suf) && len([]rune(p)) > len([]rune(suf)) {
			p = strings.TrimSuffix(p, suf)
			break
		}
	}
	switch p {
	case "香港", "澳门", "台湾":
		return false
	}
	// Prefix match: an ethnic autonomous region keeps its modifier after the
	// "自治区" strip (广西壮族 / 宁夏回族 / 新疆维吾尔), so exact lookup would miss it.
	for key := range mainlandProvinces {
		if strings.HasPrefix(p, key) {
			return true
		}
	}
	switch loc.Carrier {
	case "移动", "联通", "电信", "广电":
		return true
	}
	return false
}

// rankCDN concurrently probes each candidate's real download throughput for
// `probe` seconds and returns the pool sorted fastest-first. Throughput (not
// latency) is the metric on purpose: latency doesn't capture peering quality or
// a source whose URL points to a tiny file (which can't saturate the link).
func rankCDN(iface string, cands []nodes.Node, probe time.Duration) []nodes.Node {
	type res struct {
		i   int
		bps float64
	}
	ch := make(chan res, len(cands))
	for i := range cands {
		go func(i int) {
			client := httpx.CDNTransfer(probe + 8*time.Second)
			r := engine.RunDirection(iface, false, 2, probe, 0, probe,
				engine.CDNDownload(client, cands[i].DLURL, cands[i].UA), nil)
			bps := 0.0
			if s := probe.Seconds(); s > 0 {
				bps = float64(r.AppBytes) * 8 / s
			}
			ch <- res{i, bps}
		}(i)
	}
	score := make([]float64, len(cands))
	for k := 0; k < len(cands); k++ {
		r := <-ch
		score[r.i] = r.bps
		report.Status("probe", fmt.Sprintf("%s ≈ %.0f Mbps", cands[r.i].Name, r.bps/1e6))
	}
	idx := make([]int, len(cands))
	for i := range idx {
		idx[i] = i
	}
	sort.SliceStable(idx, func(a, b int) bool { return score[idx[a]] > score[idx[b]] })
	out := make([]nodes.Node, len(cands))
	for i, id := range idx {
		out[i] = cands[id]
	}
	return out
}

// capMax returns a valid ramp ceiling: at least min, so a 0/low config value
// disables ramping instead of clamping below the start count.
func capMax(max, min int) int {
	if max < min {
		return min
	}
	return max
}

func testCNSpeed(cfg *config.Config, iface, nodeKW string, duration, warmup, every time.Duration, dlN, ulN int, doDownload, doUpload bool, loc *geo.Location, multi int) {
	report.Status("geo", fmt.Sprintf("%s/%s/%s", loc.Carrier, loc.Province, loc.City))

	var cand []nodes.Node
	if nodeKW != "" {
		full, err := nodes.FetchServerlist(cfg)
		if err != nil {
			report.Status("error", "全量表: "+err.Error())
			report.Done()
			return
		}
		cand = selector.MatchNode(full, nodeKW)
	} else {
		nearby, err := nodes.FetchNearby(cfg, loc.Province, loc.City, loc.Carrier)
		if err != nil {
			report.Status("error", "就近节点: "+err.Error())
			report.Done()
			return
		}
		cand = selector.OrderCNSpeed(nearby, loc.Carrier, loc.Province, loc.City)
	}
	if len(cand) == 0 {
		report.Status("error", "无候选节点")
		report.Done()
		return
	}

	client := httpx.Plain(12 * time.Second)
	if multi > 1 {
		testCNSpeedMulti(cfg, iface, client, cand, loc, duration, warmup, every, dlN, ulN, doDownload, doUpload, multi)
		return
	}
	for i, n := range cand {
		if i >= 8 {
			break
		}
		pavg, pjit, ok := engine.TCPPing(n.HostIP, n.Port, 4, 3*time.Second)
		if !ok {
			report.Status("node", n.Name+" 不可达")
			continue
		}
		key, err := nodes.Dovalid(client, cfg, n.HostIP, n.Port)
		if err != nil {
			report.Status("node", n.Name+" dovalid: "+err.Error())
			continue
		}
		k := strings.TrimPrefix(key, "1-")
		var dl, ul engine.Result
		if doDownload {
			report.Status("node", fmt.Sprintf("%s %s:%s ping=%.0fms 下载中", n.Name, n.HostIP, n.Port, pavg))
			dl = engine.RunDirectionAdaptive(iface, false, dlN, capMax(cfg.Engine.ThreadsDLMax, dlN), duration, warmup, every,
				engine.CNSpeedDownload(cfg, n.HostIP, n.Port, k),
				func(t, mbps, peak float64) { report.Line("download", t, mbps, peak) })
			if dl.AppBytes == 0 {
				report.Status("node", n.Name+" 下载失败(403/0字节),换下一个")
				continue
			}
		}
		if doUpload {
			report.Status("node", n.Name+" 上传中")
			ul = engine.RunDirectionAdaptive(iface, true, ulN, capMax(cfg.Engine.ThreadsULMax, ulN), duration, warmup, every,
				engine.CNSpeedUpload(cfg, n.HostIP, n.Port, k),
				func(t, mbps, peak float64) { report.Line("upload", t, mbps, peak) })
			if !doDownload && ul.AppBytes == 0 {
				report.Status("node", n.Name+" 上传失败(0字节),换下一个")
				continue
			}
		}
		report.Final(report.Result{Node: n.Name, Source: "cnspeed", Carrier: loc.Carrier, City: n.City,
			DLAvg: dl.AvgBps / 1e6, DLPeak: dl.PeakBps / 1e6,
			ULAvg: ul.AvgBps / 1e6, ULPeak: ul.PeakBps / 1e6, PingMS: pavg, Jitter: pjit,
			DLConns: dl.Conns, ULConns: ul.Conns})
		return
	}
	report.Status("error", "所有就近节点测速失败(见上)")
	report.Done()
}

// testCNSpeedMulti aggregates up to `multi` nearby cnspeed nodes: it validates
// them (TCPPing + dovalid), then downloads/uploads from all of them at once and
// sums the throughput — the domestic equivalent of --multi, to push past a
// single node's per-connection cap on a high-bandwidth line.
func testCNSpeedMulti(cfg *config.Config, iface string, client *http.Client, cand []nodes.Node, loc *geo.Location, duration, warmup, every time.Duration, dlN, ulN int, doDownload, doUpload bool, multi int) {
	type vnode struct {
		n         nodes.Node
		key       string
		ping, jit float64
	}
	var valid []vnode
	for i, n := range cand {
		if i >= 8 || len(valid) >= multi {
			break
		}
		pavg, pjit, ok := engine.TCPPing(n.HostIP, n.Port, 4, 3*time.Second)
		if !ok {
			report.Status("node", n.Name+" 不可达")
			continue
		}
		key, err := nodes.Dovalid(client, cfg, n.HostIP, n.Port)
		if err != nil {
			report.Status("node", n.Name+" dovalid: "+err.Error())
			continue
		}
		valid = append(valid, vnode{n, strings.TrimPrefix(key, "1-"), pavg, pjit})
	}
	if len(valid) == 0 {
		report.Status("error", "所有就近节点测速失败(见上)")
		report.Done()
		return
	}
	name := valid[0].n.Name
	if len(valid) > 1 {
		name = fmt.Sprintf("%s 等 %d 源", valid[0].n.Name, len(valid))
	}
	pavg, pjit := valid[0].ping, valid[0].jit
	var dl, ul engine.Result
	if doDownload {
		mks := make([]engine.WorkerFactory, 0, len(valid))
		for _, v := range valid {
			mks = append(mks, engine.CNSpeedDownload(cfg, v.n.HostIP, v.n.Port, v.key))
		}
		report.Status("node", fmt.Sprintf("%s ping=%.0fms 下载中", name, pavg))
		dl = engine.RunDirectionPool(iface, false, mks, dlN, capMax(cfg.Engine.ThreadsDLMax, dlN), duration, warmup, every,
			func(t, mbps, peak float64) { report.Line("download", t, mbps, peak) })
		if dl.AppBytes == 0 {
			report.Status("error", name+" 下载失败(403/0字节)")
			report.Done()
			return
		}
	}
	if doUpload {
		mks := make([]engine.WorkerFactory, 0, len(valid))
		for _, v := range valid {
			mks = append(mks, engine.CNSpeedUpload(cfg, v.n.HostIP, v.n.Port, v.key))
		}
		report.Status("node", name+" 上传中")
		ul = engine.RunDirectionPool(iface, true, mks, ulN, capMax(cfg.Engine.ThreadsULMax, ulN), duration, warmup, every,
			func(t, mbps, peak float64) { report.Line("upload", t, mbps, peak) })
		if !doDownload && ul.AppBytes == 0 {
			report.Status("error", name+" 上传失败(0字节)")
			report.Done()
			return
		}
	}
	report.Final(report.Result{Node: name, Source: "cnspeed", Carrier: loc.Carrier, City: valid[0].n.City,
		DLAvg: dl.AvgBps / 1e6, DLPeak: dl.PeakBps / 1e6,
		ULAvg: ul.AvgBps / 1e6, ULPeak: ul.PeakBps / 1e6, PingMS: pavg, Jitter: pjit,
		DLConns: dl.Conns, ULConns: ul.Conns})
}

func aggName(srcs []nodes.Node) string {
	if len(srcs) == 0 {
		return ""
	}
	if len(srcs) == 1 {
		return srcs[0].Name
	}
	return fmt.Sprintf("%s 等 %d 源", srcs[0].Name, len(srcs))
}

func dlFactories(client *http.Client, srcs []nodes.Node) []engine.WorkerFactory {
	mks := make([]engine.WorkerFactory, 0, len(srcs))
	for i := range srcs {
		mks = append(mks, engine.CDNDownload(client, srcs[i].DLURL, srcs[i].UA))
	}
	return mks
}

func ulFactories(client *http.Client, srcs []nodes.Node, gotOK *uint32) []engine.WorkerFactory {
	mks := make([]engine.WorkerFactory, 0, len(srcs))
	for i := range srcs {
		mks = append(mks, engine.CDNUpload(client, srcs[i].ULURL, srcs[i].UA, gotOK))
	}
	return mks
}

func nodePingTarget(n nodes.Node) (string, string) {
	if n.HostIP != "" {
		if host, port, err := net.SplitHostPort(n.HostIP); err == nil {
			return host, port
		}
		if n.Port != "" {
			return n.HostIP, n.Port
		}
	}
	raw := n.DLURL
	if raw == "" {
		raw = n.ULURL
	}
	if raw == "" {
		return "", ""
	}
	u, err := url.Parse(raw)
	if err != nil {
		return "", ""
	}
	host, port := u.Hostname(), u.Port()
	if port == "" {
		if u.Scheme == "http" {
			port = "80"
		} else {
			port = "443"
		}
	}
	return host, port
}

func pingNode(n nodes.Node) (float64, float64, bool) {
	host, port := nodePingTarget(n)
	if host == "" || port == "" {
		return 0, 0, false
	}
	return engine.TCPPing(host, port, 4, 3*time.Second)
}

func testCDN(cfg *config.Config, iface, kw string, duration, warmup, every time.Duration, dlN, ulN int, doDownload, doUpload bool, pool []nodes.Node, multi int) {
	if multi < 1 {
		multi = 1
	}
	var ranked []nodes.Node
	if kw != "" { // explicit --node: that one source only
		for i := range pool {
			if strings.Contains(pool[i].Name, kw) {
				ranked = []nodes.Node{pool[i]}
				break
			}
		}
		multi = 1
	} else if len(pool) > 0 {
		ranked = pool
		if len(pool) > 1 && doDownload { // ranking is download-throughput based; skip it on upload-only
			report.Status("node", fmt.Sprintf("并发探测 %d 个源、按实测吞吐择优…", len(pool)))
			ranked = rankCDN(iface, pool, 2500*time.Millisecond)
		}
	}
	if len(ranked) == 0 {
		report.Status("error", "无可用测速源")
		report.Done()
		return
	}
	want := multi
	if want > len(ranked) {
		want = len(ranked)
	}
	dlSrcs := ranked[:want]

	// Upload sources: the fastest `want` that support upload (have a ULURL);
	// prefer the download winner if it can, then Speedtest.net (Ookla), then any.
	var ulSrcs []nodes.Node
	seen := map[string]bool{}
	tryUL := func(n nodes.Node) {
		if n.ULURL != "" && !seen[n.Name] && len(ulSrcs) < want {
			ulSrcs = append(ulSrcs, n)
			seen[n.Name] = true
		}
	}
	if ranked[0].ULURL != "" {
		tryUL(ranked[0])
	}
	for _, n := range ranked {
		if n.Source == "ookla" {
			tryUL(n)
		}
	}
	for _, n := range ranked {
		tryUL(n)
	}

	dlName := aggName(dlSrcs)
	pingTarget := ranked[0]
	if doDownload && len(dlSrcs) > 0 {
		pingTarget = dlSrcs[0]
	} else if len(ulSrcs) > 0 {
		pingTarget = ulSrcs[0]
	}
	pavg, pjit, pok := pingNode(pingTarget)
	if pok {
		report.Status("node", fmt.Sprintf("%s ping=%.0fms", pingTarget.Name, pavg))
	}
	client := httpx.CDNTransfer(duration + 15*time.Second)
	var dl, ul engine.Result
	var ulMsg, ulNode string

	if doDownload {
		report.Status("node", dlName+" 下载中")
		dl = engine.RunDirectionPool(iface, false, dlFactories(client, dlSrcs), dlN, capMax(cfg.Engine.ThreadsDLMax, dlN), duration, warmup, every,
			func(t, mbps, peak float64) { report.Line("download", t, mbps, peak) })
		if dl.AppBytes == 0 {
			report.Status("error", dlName+" 下载失败(URL 失效/不可达?)")
			report.Done()
			return
		}
	}
	if doUpload {
		if len(ulSrcs) == 0 {
			ulMsg = "无支持上传的源(CDN 源多不支持上传)"
			report.Status("node", ulMsg)
		} else {
			ulName := aggName(ulSrcs)
			if ulSrcs[0].Name != dlSrcs[0].Name {
				report.Status("node", "下载源不支持上传,上传改用 "+ulName)
				ulNode = ulName
			}
			report.Status("node", ulName+" 上传中")
			var gotOK uint32
			ul = engine.RunDirectionPool(iface, true, ulFactories(client, ulSrcs, &gotOK), ulN, capMax(cfg.Engine.ThreadsULMax, ulN), duration, warmup, every,
				func(t, mbps, peak float64) { report.Line("upload", t, mbps, peak) })
			// Supported if the server returned 2xx, OR it accepted a substantial
			// streamed body (Ookla /upload is one long POST that only responds at
			// ctx-cancel, so gotOK never sets even though bytes were accepted).
			if atomic.LoadUint32(&gotOK) == 0 && ul.AppBytes < 65536 {
				ulMsg = "该源不支持上传测试"
				ul = engine.Result{}
				ulNode = ""
				report.Status("node", ulName+" 上传不支持(源不接受 POST)")
			}
		}
	}
	node := dlName
	if !doDownload && len(ulSrcs) > 0 {
		node = aggName(ulSrcs)
		ulNode = ""
	}
	report.Final(report.Result{Node: node, Source: ranked[0].Source,
		DLAvg: dl.AvgBps / 1e6, DLPeak: dl.PeakBps / 1e6,
		ULAvg: ul.AvgBps / 1e6, ULPeak: ul.PeakBps / 1e6, ULMsg: ulMsg, ULNode: ulNode,
		DLConns: dl.Conns, ULConns: ul.Conns, PingMS: pavg, Jitter: pjit})
}
