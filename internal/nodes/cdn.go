package nodes

import "better-speedtest/internal/config"

func CDNNodes(cfg *config.Config) []Node {
	out := make([]Node, 0, len(cfg.CDNSources))
	for _, s := range cfg.CDNSources {
		out = append(out, Node{
			Name:     s.Name,
			Source:   "cdn",
			Region:   s.Region,
			DLURL:    s.DL,
			ULURL:    s.UL,
			UA:       s.UA,
			Enhanced: s.Enhanced,
		})
	}
	return out
}
