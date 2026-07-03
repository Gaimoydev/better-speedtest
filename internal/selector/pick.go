package selector

import (
	"sort"
	"strings"

	"better-speedtest/internal/nodes"
)

func OrderCNSpeed(list []nodes.Node, carrier, province, city string) []nodes.Node {
	same := make([]nodes.Node, 0, len(list))
	for _, n := range list {
		if carrier == "" || n.Carrier == "" || carrierMatch(n.Carrier, carrier) {
			same = append(same, n)
		}
	}
	if len(same) == 0 {
		same = append(same, list...)
	}
	tier := func(n nodes.Node) int {
		loc := n.Province + n.City + n.Name
		if city != "" && strings.Contains(loc, city) {
			return 0
		}
		if province != "" && strings.Contains(loc, province) {
			return 1
		}
		return 2
	}
	sort.SliceStable(same, func(i, j int) bool { return tier(same[i]) < tier(same[j]) })
	return same
}

func MatchNode(list []nodes.Node, kw string) []nodes.Node {
	var out []nodes.Node
	for _, n := range list {
		if strings.Contains(n.HostIP, kw) || strings.Contains(n.City, kw) ||
			strings.Contains(n.Name, kw) || strings.Contains(n.Province, kw) {
			out = append(out, n)
		}
	}
	return out
}

func carrierMatch(a, b string) bool {
	return strings.Contains(a, b) || strings.Contains(b, a)
}
