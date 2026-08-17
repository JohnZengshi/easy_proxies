package monitor

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"easy_proxies/internal/config"
)

func TestGeneratePACWithRules(t *testing.T) {
	pac := GeneratePAC([]config.RoutingRule{
		{Name: "openai", DomainSuffix: []string{"openai.com"}, DomainKeyword: []string{"chatgpt"}},
		{Name: "github", DomainSuffix: []string{"github.com"}},
	}, 2323, config.RoutingFallbackDirect)
	for _, want := range []string{
		"function FindProxyForURL(url, host) {",
		`host == "openai.com"`,
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
	pac := GeneratePAC(nil, 2323, config.RoutingFallbackDirect)
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
	}, 2323, config.RoutingFallbackDirect)
	if strings.Contains(pac, "off.example.com") {
		t.Fatalf("disabled rule should be skipped:\n%s", pac)
	}
	if !strings.Contains(pac, "on.example.com") {
		t.Fatalf("enabled rule missing from PAC:\n%s", pac)
	}
}

func TestGeneratePACProxyPoolFallback(t *testing.T) {
	enabled := true
	disabled := false
	pac := GeneratePAC([]config.RoutingRule{
		{Name: "on", DomainSuffix: []string{"on.example.com"}, Target: "proxy-pool", Enabled: &enabled},
		{Name: "off", DomainSuffix: []string{"off.example.com"}, Target: "direct", Enabled: &disabled},
	}, 2323, config.RoutingFallbackProxyPool)
	if !strings.Contains(pac, "on.example.com") || strings.Contains(pac, "off.example.com") {
		t.Fatalf("enabled rule missing or disabled rule leaked:\n%s", pac)
	}
	if !strings.Contains(pac, `return "PROXY 127.0.0.1:2323";`) {
		t.Fatalf("proxy-pool fallback missing:\n%s", pac)
	}
}

func TestGeneratePACAnthropicCategory(t *testing.T) {
	enabled := true
	pac := GeneratePAC([]config.RoutingRule{
		{Name: "anthropic-custom", Category: "anthropic", DomainSuffix: []string{"claude.ai"}, Target: "proxy-pool", Enabled: &enabled},
	}, 2323, config.RoutingFallbackDirect)
	for _, want := range []string{
		`host == "claude.ai"`,
		`dnsDomainIs(host, ".claude.ai")`,
		`host == "anthropic.com"`,
		`dnsDomainIs(host, ".anthropic.com")`,
		`host == "ip.net.coffee"`,
		`dnsDomainIs(host, ".ip.net.coffee")`,
		"PROXY 127.0.0.1:2323",
		`return "DIRECT";`,
	} {
		if !strings.Contains(pac, want) {
			t.Fatalf("PAC missing %q:\n%s", want, pac)
		}
	}
	if got := strings.Count(pac, `dnsDomainIs(host, ".claude.ai")`); got != 1 {
		t.Fatalf("custom suffix duplicated category expansion, got %d matches:\n%s", got, pac)
	}
	directIdx := strings.Index(pac, `return "DIRECT";`)
	for _, host := range []string{"claude.ai", ".claude.ai", "anthropic.com", ".anthropic.com", "ip.net.coffee", ".ip.net.coffee"} {
		if idx := strings.Index(pac, host); idx < 0 || idx >= directIdx {
			t.Fatalf("host %q missing before direct fallback:\n%s", host, pac)
		}
	}
}

func TestGeneratePACSkipsAnthropicCategoryWhenDisabled(t *testing.T) {
	disabled := false
	pac := GeneratePAC([]config.RoutingRule{
		{Name: "anthropic-off", Category: "anthropic", Target: "proxy-pool", Enabled: &disabled},
	}, 2323, config.RoutingFallbackDirect)
	if strings.Contains(pac, "claude.ai") || strings.Contains(pac, "anthropic.com") || strings.Contains(pac, "ip.net.coffee") || strings.Contains(pac, "PROXY") {
		t.Fatalf("disabled Anthropic preset leaked into PAC:\n%s", pac)
	}
	if !strings.Contains(pac, `return "DIRECT";`) {
		t.Fatalf("disabled rule should fall back direct:\n%s", pac)
	}
}

func TestGeneratePACUnknownCategoryFallsThrough(t *testing.T) {
	pac := GeneratePAC([]config.RoutingRule{
		{Name: "unknown-preset", Category: "not-a-preset", Target: "proxy-pool"},
	}, 2323, config.RoutingFallbackDirect)
	if strings.Contains(pac, "PROXY") || strings.Contains(pac, "not-a-preset") {
		t.Fatalf("unknown category must not create PAC proxy condition:\n%s", pac)
	}
	if !strings.Contains(pac, `return "DIRECT";`) {
		t.Fatalf("unknown category should fall back direct:\n%s", pac)
	}
}

func TestHandleRoutingPACFallback(t *testing.T) {
	s := &Server{}
	s.cfgMu.Lock()
	s.cfgSrc = &config.Config{
		Listener: config.ListenerConfig{Port: 2323},
		Routing:  config.RoutingConfig{Fallback: config.RoutingFallbackProxyPool},
	}
	s.cfgMu.Unlock()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/routing.pac", nil)
	s.handleRoutingPAC(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), `return "PROXY 127.0.0.1:2323";`) {
		t.Fatalf("handler PAC does not use proxy-pool fallback:\n%s", rec.Body.String())
	}
	if got := rec.Header().Get("Cache-Control"); got != "no-store" {
		t.Fatalf("Cache-Control = %q, want no-store", got)
	}
}

func TestHandleRoutingPACAnthropicProxyPool(t *testing.T) {
	enabled := true
	s := &Server{}
	s.cfgMu.Lock()
	s.cfgSrc = &config.Config{
		Listener: config.ListenerConfig{Port: 2323},
		Routing: config.RoutingConfig{
			Fallback: config.RoutingFallbackProxyPool,
			Rules: []config.RoutingRule{
				{Name: "anthropic", Category: "anthropic", Target: "proxy-pool", Enabled: &enabled},
			},
		},
	}
	s.cfgMu.Unlock()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/routing.pac", nil)
	s.handleRoutingPAC(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	body := rec.Body.String()
	for _, want := range []string{`host == "claude.ai"`, `.claude.ai`, `host == "anthropic.com"`, `.anthropic.com`, `host == "ip.net.coffee"`, `.ip.net.coffee`, `return "PROXY 127.0.0.1:2323";`} {
		if !strings.Contains(body, want) {
			t.Fatalf("handler PAC missing %q:\n%s", want, body)
		}
	}
	if !strings.Contains(body, "return \"PROXY 127.0.0.1:2323\";\n}\n") {
		t.Fatalf("handler PAC missing final proxy-pool fallback:\n%s", body)
	}
}

func TestHandleRoutingPACIPNetCoffee(t *testing.T) {
	enabled := true
	s := &Server{}
	s.cfgMu.Lock()
	s.cfgSrc = &config.Config{
		Listener: config.ListenerConfig{Port: 2323},
		Routing: config.RoutingConfig{
			Fallback: config.RoutingFallbackDirect,
			Rules: []config.RoutingRule{
				{Name: "anthropic", Category: "anthropic", Target: "proxy-pool", Enabled: &enabled},
			},
		},
	}
	s.cfgMu.Unlock()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/routing.pac", nil)
	s.handleRoutingPAC(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, `host == "ip.net.coffee"`) || !strings.Contains(body, `return "DIRECT";`) {
		t.Fatalf("handler PAC category expansion or direct fallback missing:\n%s", body)
	}
}
