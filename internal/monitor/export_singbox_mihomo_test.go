package monitor

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func registerExportNode(t *testing.T, mgr *Manager, tag string, port uint16, available bool) *EntryHandle {
	t.Helper()
	handle := mgr.Register(NodeInfo{
		Tag:           tag,
		Name:          tag,
		URI:           "vless://" + tag + "@" + tag + ".example.com:443",
		ListenAddress: "127.0.0.1",
		Port:          port,
	})
	handle.ref.mu.Lock()
	handle.ref.initialCheckDone = true
	handle.ref.available = available
	handle.ref.mu.Unlock()
	return handle
}

func TestHandleExport_SingBoxIncludesHealthyExcludesUnhealthy(t *testing.T) {
	s, mgr := newExportServer(t)
	_ = registerExportNode(t, mgr, "available", 24000, true)
	_ = registerExportNode(t, mgr, "unavailable", 24001, false)
	_ = registerExportNode(t, mgr, "duplicate", 24000, true)
	_ = registerExportNode(t, mgr, "no-port", 0, true)

	req := httptest.NewRequest("GET", "/api/export?target=sing-box", nil)
	rec := httptest.NewRecorder()
	s.handleExport(rec, req)

	if rec.Code != 200 {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}
	if body := rec.Body.String(); strings.Contains(body, "username") || strings.Contains(body, "password") {
		t.Fatalf("sing-box export contains auth fields:\n%s", body)
	}

	var got struct {
		Outbounds []map[string]any `json:"outbounds"`
		Route     struct {
			Final string `json:"final"`
		} `json:"route"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode sing-box export: %v\n%s", err, rec.Body.String())
	}
	if got.Route.Final != "easy-proxies" {
		t.Errorf("route.final = %q, want easy-proxies", got.Route.Final)
	}
	if len(got.Outbounds) != 2 {
		t.Fatalf("outbounds len = %d, want 2; body=%s", len(got.Outbounds), rec.Body.String())
	}

	var selector []any
	for _, ob := range got.Outbounds {
		switch ob["type"] {
		case "http":
			if ob["tag"] != "available" {
				t.Errorf("http outbound tag = %v, want available", ob["tag"])
			}
			if ob["server"] != "127.0.0.1" || ob["server_port"] != float64(24000) {
				t.Errorf("http outbound = %v", ob)
			}
			if _, ok := ob["username"]; ok {
				t.Errorf("http outbound has username: %v", ob)
			}
			if _, ok := ob["password"]; ok {
				t.Errorf("http outbound has password: %v", ob)
			}
		case "selector":
			if ob["tag"] != "easy-proxies" {
				t.Errorf("selector tag = %v, want easy-proxies", ob["tag"])
			}
			selector, _ = ob["outbounds"].([]any)
		default:
			t.Errorf("unexpected outbound type %v", ob["type"])
		}
	}
	if len(selector) != 1 || selector[0] != "available" {
		t.Errorf("selector outbounds = %v, want [available]", selector)
	}
}

func TestHandleExport_SingBoxPoolModeReturns400(t *testing.T) {
	s, _ := newExportServer(t)
	s.cfgMu.Lock()
	s.cfgSrc.Mode = "pool"
	s.cfgMu.Unlock()

	req := httptest.NewRequest("GET", "/api/export?target=sing-box", nil)
	rec := httptest.NewRecorder()
	s.handleExport(rec, req)

	if rec.Code != 400 {
		t.Fatalf("status = %d, want 400; body=%s", rec.Code, rec.Body.String())
	}
}

func TestHandleExport_SingBoxEmptyNodesReturnsValidEmptyConfig(t *testing.T) {
	s, _ := newExportServer(t)

	req := httptest.NewRequest("GET", "/api/export?target=sing-box", nil)
	rec := httptest.NewRecorder()
	s.handleExport(rec, req)

	if rec.Code != 200 {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	var got struct {
		Outbounds []map[string]any `json:"outbounds"`
		Route     struct {
			Final string `json:"final"`
		} `json:"route"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode empty sing-box export: %v", err)
	}
	if len(got.Outbounds) != 1 {
		t.Fatalf("outbounds len = %d, want selector only", len(got.Outbounds))
	}
	sel := got.Outbounds[0]
	if sel["type"] != "selector" || sel["tag"] != "easy-proxies" {
		t.Fatalf("only outbound = %v, want selector easy-proxies", sel)
	}
	if members, ok := sel["outbounds"].([]any); !ok || len(members) != 0 {
		t.Fatalf("selector outbounds = %v, want empty list", sel["outbounds"])
	}
}

func TestHandleExport_MihomoIncludesHealthyExcludesUnhealthy(t *testing.T) {
	s, mgr := newExportServer(t)
	_ = registerExportNode(t, mgr, "available", 24000, true)
	_ = registerExportNode(t, mgr, "unavailable", 24001, false)
	_ = registerExportNode(t, mgr, "duplicate", 24000, true)
	_ = registerExportNode(t, mgr, "no-port", 0, true)

	req := httptest.NewRequest("GET", "/api/export?target=mihomo", nil)
	rec := httptest.NewRecorder()
	s.handleExport(rec, req)

	if rec.Code != 200 {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	if body := rec.Body.String(); strings.Contains(body, "username") || strings.Contains(body, "password") {
		t.Fatalf("mihomo export contains auth fields:\n%s", body)
	}

	var got struct {
		Proxies     []map[string]any `yaml:"proxies"`
		ProxyGroups []map[string]any `yaml:"proxy-groups"`
		Rules       []string         `yaml:"rules"`
	}
	if err := yaml.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode mihomo export: %v\n%s", err, rec.Body.String())
	}
	if len(got.Proxies) != 1 {
		t.Fatalf("proxies len = %d, want 1; body=%s", len(got.Proxies), rec.Body.String())
	}
	if got.Proxies[0]["name"] != "available" || got.Proxies[0]["type"] != "http" {
		t.Fatalf("proxy = %v, want available/http", got.Proxies[0])
	}
	if got.Proxies[0]["server"] != "127.0.0.1" || got.Proxies[0]["port"] != 24000 {
		t.Fatalf("proxy endpoint = %v, want 127.0.0.1:24000", got.Proxies[0])
	}
	if len(got.ProxyGroups) != 1 || got.ProxyGroups[0]["name"] != "easy-proxies" || got.ProxyGroups[0]["type"] != "select" {
		t.Fatalf("proxy-groups = %v, want one easy-proxies select", got.ProxyGroups)
	}
	if members, ok := got.ProxyGroups[0]["proxies"].([]any); !ok || len(members) != 1 || members[0] != "available" {
		t.Fatalf("group proxies = %v, want [available]", got.ProxyGroups[0]["proxies"])
	}
	if len(got.Rules) != 1 || got.Rules[0] != "MATCH,easy-proxies" {
		t.Fatalf("rules = %v, want [MATCH,easy-proxies]", got.Rules)
	}
}

func TestHandleExport_MihomoPoolModeReturns400(t *testing.T) {
	s, _ := newExportServer(t)
	s.cfgMu.Lock()
	s.cfgSrc.Mode = "pool"
	s.cfgMu.Unlock()

	req := httptest.NewRequest("GET", "/api/export?target=mihomo", nil)
	rec := httptest.NewRecorder()
	s.handleExport(rec, req)

	if rec.Code != 400 {
		t.Fatalf("status = %d, want 400; body=%s", rec.Code, rec.Body.String())
	}
}

func TestHandleExport_MihomoEmptyNodesReturnsValidEmptyConfig(t *testing.T) {
	s, _ := newExportServer(t)

	req := httptest.NewRequest("GET", "/api/export?target=mihomo", nil)
	rec := httptest.NewRecorder()
	s.handleExport(rec, req)

	if rec.Code != 200 {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	var got struct {
		Proxies     []map[string]any `yaml:"proxies"`
		ProxyGroups []map[string]any `yaml:"proxy-groups"`
		Rules       []string         `yaml:"rules"`
	}
	if err := yaml.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode empty mihomo export: %v", err)
	}
	if len(got.Proxies) != 0 || len(got.ProxyGroups) != 1 {
		t.Fatalf("proxies=%d proxy-groups=%d, want 0/1", len(got.Proxies), len(got.ProxyGroups))
	}
	if members, ok := got.ProxyGroups[0]["proxies"].([]any); !ok || len(members) != 0 {
		t.Fatalf("group proxies = %v, want empty list", got.ProxyGroups[0]["proxies"])
	}
}
