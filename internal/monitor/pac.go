package monitor

import (
	"fmt"
	"net/http"
	"strings"

	"easy_proxies/internal/config"
	"easy_proxies/internal/routing"
)

// GeneratePAC renders a Proxy Auto-Config script from the current routing
// rules. Domains that match an enabled rule are sent to the local mixed proxy;
// unmatched traffic follows fallback. Regex rules are intentionally omitted
// because PAC JavaScript has no general regex matcher.
func GeneratePAC(rules []config.RoutingRule, proxyPort int, fallback string) string {
	var b strings.Builder
	b.WriteString("function FindProxyForURL(url, host) {\n")
	for _, rule := range rules {
		if !rule.IsEnabled() {
			continue
		}
		suffixes := append([]string(nil), rule.DomainSuffix...)
		if rule.Category != "" {
			if expanded := routing.ExpandCategory(rule.Category); expanded != nil {
				suffixes = append(suffixes, expanded...)
			}
		}
		seen := make(map[string]struct{}, len(suffixes))
		var conds []string
		for _, suffix := range suffixes {
			suffix = strings.Trim(strings.TrimSpace(suffix), ".")
			if suffix == "" {
				continue
			}
			escaped := strings.ReplaceAll(suffix, `"`, `\"`)
			if _, ok := seen[escaped]; ok {
				continue
			}
			seen[escaped] = struct{}{}
			conds = append(conds,
				fmt.Sprintf(`host == "%s"`, escaped),
				fmt.Sprintf("dnsDomainIs(host, \".%s\")", escaped),
			)
		}
		for _, keyword := range rule.DomainKeyword {
			keyword = strings.TrimSpace(keyword)
			if keyword == "" {
				continue
			}
			escaped := strings.ReplaceAll(keyword, `"`, `\"`)
			conds = append(conds, fmt.Sprintf("shExpMatch(host, \"*%s*\")", escaped))
		}
		if len(conds) == 0 {
			continue
		}
		b.WriteString("  if (")
		b.WriteString(strings.Join(conds, " || "))
		b.WriteString(") return \"PROXY 127.0.0.1:")
		fmt.Fprintf(&b, "%d\";\n", proxyPort)
	}
	if fallback == config.RoutingFallbackProxyPool {
		fmt.Fprintf(&b, "  return \"PROXY 127.0.0.1:%d\";\n}\n", proxyPort)
	} else {
		b.WriteString("  return \"DIRECT\";\n}\n")
	}
	return b.String()
}

func (s *Server) handleRoutingPAC(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	s.cfgMu.RLock()
	var rules []config.RoutingRule
	fallback := config.RoutingFallbackDirect
	port := 2323
	if s.cfgSrc != nil {
		rules = s.cfgSrc.Routing.Rules
		fallback = s.cfgSrc.Routing.FallbackOrDefault()
		port = int(s.cfgSrc.Listener.Port)
	}
	s.cfgMu.RUnlock()

	w.Header().Set("Content-Type", "application/x-ns-proxy-autoconfig")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write([]byte(GeneratePAC(rules, port, fallback)))
}
