package geo

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"

	"better-speedtest/internal/config"
)

var errNoAT = errors.New("no AT port responded")

var (
	reCOPS = regexp.MustCompile(`\+COPS:\s*\d+,\d+,"(\d+)"`)
	reIMSI = regexp.MustCompile(`\b(\d{15})\b`)
)

func DetectAT(cfg *config.Config) (string, error) {
	ch := make(chan string, 1)
	go func() { p, _ := detectATInner(cfg); ch <- p }()
	select {
	case p := <-ch:
		if p == "" {
			return "", errNoAT
		}
		return p, nil
	case <-time.After(25 * time.Second):
		return "", errNoAT
	}
}

func detectATInner(cfg *config.Config) (string, error) {
	waitSecs := cfg.AT.ReadTimeoutMS / 1000
	if waitSecs < 2 {
		waitSecs = 2
	}
	for _, port := range cfg.AT.Ports {
		if _, err := os.Stat(port); err != nil {
			continue
		}
		out := atExec(port, []string{cfg.AT.Cmds["cops_set"], cfg.AT.Cmds["cops_query"]}, waitSecs)
		if m := reCOPS.FindStringSubmatch(out); m != nil {
			return m[1], nil
		}
		if out := atExec(port, []string{cfg.AT.Cmds["imsi"]}, waitSecs); out != "" {
			if m := reIMSI.FindStringSubmatch(out); m != nil {
				imsi := m[1]
				if len(imsi) >= 6 {
					if _, ok := NewTable(cfg).Lookup(imsi[:6]); ok {
						return imsi[:6], nil
					}
				}
				return imsi[:5], nil
			}
		}
	}
	return "", errNoAT
}

func atExec(port string, cmds []string, waitSecs int) string {
	var w strings.Builder
	for _, c := range cmds {
		if c == "" {
			continue
		}
		w.WriteString("sleep 0.3; printf '%s\\r' '" + shEsc(c) + "' > " + shq(port) + " 2>/dev/null; ")
	}
	script := "cat " + shq(port) + " > /tmp/.at_$$ 2>/dev/null & CAT=$!\n" +
		"( " + w.String() + "true ) & WR=$!\n" +
		"sleep " + strconv.Itoa(waitSecs) + "\n" +
		"kill -9 $CAT $WR 2>/dev/null\n" +
		"cat /tmp/.at_$$ 2>/dev/null\n" +
		"rm -f /tmp/.at_$$\n"

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(waitSecs+5)*time.Second)
	defer cancel()
	out, _ := exec.CommandContext(ctx, "sh", "-c", script).Output()
	return strings.ReplaceAll(string(out), "\r", "")
}

func shq(s string) string   { return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'" }
func shEsc(s string) string { return strings.ReplaceAll(s, "'", `'\''`) }
