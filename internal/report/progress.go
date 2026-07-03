package report

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

var jsonMode bool
var lastPhase string

func SetJSON(v bool) { jsonMode = v }

type Progress struct {
	Phase string  `json:"phase"`
	T     float64 `json:"t,omitempty"`
	Mbps  float64 `json:"mbps,omitempty"`
	Peak  float64 `json:"peak,omitempty"`
	Msg   string  `json:"msg,omitempty"`
}

type Result struct {
	Phase   string  `json:"phase"`
	Node    string  `json:"node"`
	Source  string  `json:"source"`
	Carrier string  `json:"carrier,omitempty"`
	City    string  `json:"city,omitempty"`
	DLAvg   float64 `json:"dl_avg"`
	DLPeak  float64 `json:"dl_peak"`
	ULAvg   float64 `json:"ul_avg"`
	ULPeak  float64 `json:"ul_peak"`
	ULMsg   string  `json:"ul_msg,omitempty"`
	ULNode  string  `json:"ul_node,omitempty"`
	DLConns int     `json:"dl_conns,omitempty"`
	ULConns int     `json:"ul_conns,omitempty"`
	PingMS  float64 `json:"ping"`
	Jitter  float64 `json:"jitter"`
}

func emit(v interface{}) {
	b, _ := json.Marshal(v)
	fmt.Fprintln(os.Stdout, string(b))
}

func Line(phase string, t, mbps, peak float64) {
	if jsonMode {
		emit(Progress{Phase: phase, T: round1(t), Mbps: round2(mbps), Peak: round2(peak)})
		return
	}
	label := ""
	switch phase {
	case "download":
		label = "⬇ 下载"
	case "upload":
		label = "⬆ 上传"
	default:
		return
	}
	if lastPhase != phase && lastPhase != "" {
		fmt.Fprint(os.Stderr, "\n")
	}
	lastPhase = phase
	fmt.Fprintf(os.Stderr, "\r  %s  %s  %8.1f Mbps   峰 %6.1f  ", label, bar(mbps, 1000, 22), mbps, peak)
}

func Status(phase, msg string) {
	if jsonMode {
		emit(Progress{Phase: phase, Msg: msg})
		return
	}
	if lastPhase != "" {
		fmt.Fprint(os.Stderr, "\n")
		lastPhase = ""
	}
	pre := "  · "
	if phase == "error" {
		pre = "  ✗ "
	}
	fmt.Fprintln(os.Stderr, pre+msg)
}

func Final(r Result) {
	r.Phase = "result"
	r.DLAvg, r.DLPeak = round2(r.DLAvg), round2(r.DLPeak)
	r.ULAvg, r.ULPeak = round2(r.ULAvg), round2(r.ULPeak)
	r.PingMS, r.Jitter = round2(r.PingMS), round2(r.Jitter)
	if jsonMode {
		emit(r)
		fmt.Fprintln(os.Stdout, "DONE")
		return
	}
	if lastPhase != "" {
		fmt.Fprint(os.Stderr, "\n")
		lastPhase = ""
	}
	loc := strings.TrimSpace(strings.Join(filterEmpty(r.Carrier, r.City), " "))
	fmt.Printf("\n  节点    %s", r.Node)
	if loc != "" {
		fmt.Printf("  (%s)", loc)
	}
	if r.DLAvg > 0 || r.DLPeak > 0 {
		fmt.Printf("\n  ⬇ 下载  %.1f Mbps   峰 %.1f", r.DLAvg, r.DLPeak)
		if r.DLConns > 0 {
			fmt.Printf("  (%d 连接)", r.DLConns)
		}
	}
	if r.ULMsg != "" {
		fmt.Printf("\n  ⬆ 上传  %s", r.ULMsg)
	} else if r.ULAvg > 0 || r.ULPeak > 0 {
		fmt.Printf("\n  ⬆ 上传  %.1f Mbps   峰 %.1f", r.ULAvg, r.ULPeak)
		if r.ULConns > 0 {
			fmt.Printf("  (%d 连接)", r.ULConns)
		}
		if r.ULNode != "" {
			fmt.Printf("  (via %s)", r.ULNode)
		}
	}
	if r.PingMS > 0 || r.Jitter > 0 {
		fmt.Printf("\n  延迟    %.1f ms    抖动 %.1f ms", r.PingMS, r.Jitter)
	}
	fmt.Print("\n\n")
}

func Done() {
	if jsonMode {
		fmt.Fprintln(os.Stdout, "DONE")
	}
}

func bar(v, max float64, width int) string {
	f := v / max
	if f < 0 {
		f = 0
	}
	if f > 1 {
		f = 1
	}
	n := int(f * float64(width))
	return "[" + strings.Repeat("█", n) + strings.Repeat("░", width-n) + "]"
}

func filterEmpty(ss ...string) []string {
	out := ss[:0]
	for _, s := range ss {
		if s != "" {
			out = append(out, s)
		}
	}
	return out
}

func round1(f float64) float64 { return float64(int(f*10+0.5)) / 10 }
func round2(f float64) float64 { return float64(int(f*100+0.5)) / 100 }
