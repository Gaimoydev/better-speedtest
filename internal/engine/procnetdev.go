package engine

import (
	"bufio"
	"os"
	"strconv"
	"strings"
)

func DetectWAN() string {
	f, err := os.Open("/proc/net/route")
	if err != nil {
		return ""
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	first := true
	for sc.Scan() {
		if first {
			first = false
			continue
		}
		fields := strings.Fields(sc.Text())
		if len(fields) >= 4 && fields[1] == "00000000" {
			return fields[0]
		}
	}
	return ""
}

func IfaceBytes(iface string) (rx, tx uint64, ok bool) {
	f, err := os.Open("/proc/net/dev")
	if err != nil {
		return 0, 0, false
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := sc.Text()
		i := strings.IndexByte(line, ':')
		if i < 0 {
			continue
		}
		if strings.TrimSpace(line[:i]) != iface {
			continue
		}
		fields := strings.Fields(line[i+1:])
		if len(fields) < 16 {
			return 0, 0, false
		}
		rx, _ = strconv.ParseUint(fields[0], 10, 64)
		tx, _ = strconv.ParseUint(fields[8], 10, 64)
		return rx, tx, true
	}
	return 0, 0, false
}
