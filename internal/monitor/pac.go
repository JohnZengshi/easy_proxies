package monitor

import (
	"fmt"
	"net/http"
	"strings"

	"easy_proxies/internal/config"
)

// GeneratePAC renders a Proxy Auto-Config script from the current routing
// rules. Domains that match a rule are sent to the local mixed proxy; every
// other request goes direct. Regex rules are intentionally omitted because
// PAC JavaScript has no general regex matcher.
func GeneratePAC(rules []config.RoutingRule, proxyPort int) string {
	var b strings.Builder
	b.WriteString("function FindProxyForURL(url, host) {\n")
	for _, rule := range rules {
		if !rule.IsEnabled() {
			continue
		}
		var conds []string
		for _, suffix := range rule.DomainSuffix {
			suffix = strings.TrimSpace(suffix)
			if suffix == "" {
				continue
			}
			escaped := strings.ReplaceAll(suffix, `"`, `\"`)
			conds = append(conds, fmt.Sprintf("dnsDomainIs(host, \".%s\")", escaped))
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
	b.WriteString("  return \"DIRECT\";\n}\n")
	return b.String()
}

func (s *Server) handleRoutingPAC(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	s.cfgMu.RLock()
	var rules []config.RoutingRule
	port := 2323
	if s.cfgSrc != nil {
		rules = s.cfgSrc.Routing.Rules
		port = int(s.cfgSrc.Listener.Port)
	}
	s.cfgMu.RUnlock()

	w.Header().Set("Content-Type", "application/x-ns-proxy-autoconfig")
	_, _ = w.Write([]byte(GeneratePAC(rules, port)))
}
