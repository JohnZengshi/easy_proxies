package monitor

import (
	"strings"
	"testing"

	"easy_proxies/internal/config"
)

func TestGeneratePACWithRules(t *testing.T) {
	pac := GeneratePAC([]config.RoutingRule{
		{Name: "openai", DomainSuffix: []string{"openai.com"}, DomainKeyword: []string{"chatgpt"}},
		{Name: "github", DomainSuffix: []string{"github.com"}},
	}, 2323)
	for _, want := range []string{
		"function FindProxyForURL(url, host) {",
		`dnsDomainIs(host, ".openai.com")`,
		`shExpMatch(host, "*chatgpt*")`,
		`PROXY 127.0.0.1:2323`,
		`return "DIRECT";`,
	} {
		if !strings.Contains(pac, want) {
			t.Fatalf("PAC missing %q:\n%s", want, pac)
		}
	}
}

func TestGeneratePACEmpty(t *testing.T) {
	pac := GeneratePAC(nil, 2323)
	if strings.Contains(pac, "PROXY") || !strings.Contains(pac, `return "DIRECT";`) {
		t.Fatalf("empty ruleset should be all-direct, got:\n%s", pac)
	}
}

func TestGeneratePACSkipsDisabled(t *testing.T) {
	enabled := true
	disabled := false
	pac := GeneratePAC([]config.RoutingRule{
		{Name: "on", DomainSuffix: []string{"on.example.com"}, Target: "proxy-pool", Enabled: &enabled},
		{Name: "off", DomainSuffix: []string{"off.example.com"}, Target: "proxy-pool", Enabled: &disabled},
	}, 2323)
	if strings.Contains(pac, "off.example.com") {
		t.Fatalf("disabled rule should be skipped:\n%s", pac)
	}
	if !strings.Contains(pac, "on.example.com") {
		t.Fatalf("enabled rule missing from PAC:\n%s", pac)
	}
}
