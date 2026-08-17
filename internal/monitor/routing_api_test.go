package monitor

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"easy_proxies/internal/config"
)

type routingTestNodeManager struct {
	reloads int
}

func (m *routingTestNodeManager) ListConfigNodes(context.Context) ([]config.NodeConfig, error) {
	return nil, nil
}

func (m *routingTestNodeManager) CreateNode(context.Context, config.NodeConfig) (config.NodeConfig, error) {
	return config.NodeConfig{}, nil
}

func (m *routingTestNodeManager) UpdateNode(context.Context, string, config.NodeConfig) (config.NodeConfig, error) {
	return config.NodeConfig{}, nil
}

func (m *routingTestNodeManager) DeleteNode(context.Context, string) error {
	return nil
}

func (m *routingTestNodeManager) TriggerReload(context.Context) error {
	m.reloads++
	return nil
}

func newRoutingAPIServer(t *testing.T) (*Server, *routingTestNodeManager, string) {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	yamlText := `
mode: pool
listener:
  port: 2323
nodes:
  - name: n
    uri: ss://YWVzLTI1Ni1nY206cGFzcw==@127.0.0.1:8388
routing:
  fallback: direct
  rules:
    - id: rule-1
      name: openai
      category: openai
      target: n
`
	if err := os.WriteFile(path, []byte(yamlText), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	nm := &routingTestNodeManager{}
	s := &Server{nodeMgr: nm}
	s.cfgMu.Lock()
	s.cfgSrc = cfg
	s.cfgMu.Unlock()
	return s, nm, path
}

func TestHandleRoutingFallbackGet(t *testing.T) {
	s, _, _ := newRoutingAPIServer(t)
	rec := httptest.NewRecorder()
	s.handleRoutingFallback(rec, httptest.NewRequest(http.MethodGet, "/api/routing/fallback", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	var got struct {
		Fallback string `json:"fallback"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got.Fallback != config.RoutingFallbackDirect {
		t.Fatalf("fallback = %q, want %q", got.Fallback, config.RoutingFallbackDirect)
	}
}

func TestHandleRoutingFallbackPutPersistsAndReloads(t *testing.T) {
	s, nm, path := newRoutingAPIServer(t)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/routing/fallback", strings.NewReader(`{"fallback":"proxy-pool"}`))
	s.handleRoutingFallback(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	if nm.reloads != 1 {
		t.Fatalf("reloads = %d, want 1", nm.reloads)
	}
	reloaded, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.Routing.FallbackOrDefault() != config.RoutingFallbackProxyPool {
		t.Fatalf("reloaded fallback = %q, want %q", reloaded.Routing.FallbackOrDefault(), config.RoutingFallbackProxyPool)
	}
}

func TestHandleRoutingFallbackRejectsInvalidWithoutMutation(t *testing.T) {
	s, nm, path := newRoutingAPIServer(t)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/routing/fallback", strings.NewReader(`{"fallback":"tun"}`))
	s.handleRoutingFallback(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body=%s", rec.Code, rec.Body.String())
	}
	if nm.reloads != 0 {
		t.Fatalf("reloads = %d, want 0", nm.reloads)
	}
	reloaded, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.Routing.FallbackOrDefault() != config.RoutingFallbackDirect {
		t.Fatalf("fallback changed to %q, want direct", reloaded.Routing.FallbackOrDefault())
	}
}

func TestHandleRoutingRuleItemFullPUTPreservesDisabled(t *testing.T) {
	s, nm, path := newRoutingAPIServer(t)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/routing/rules/rule-1", strings.NewReader(`{"id":"rule-1","name":"openai","category":"openai","target":"n","enabled":false}`))
	s.handleRoutingRuleItem(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	if nm.reloads != 1 {
		t.Fatalf("reloads = %d, want 1", nm.reloads)
	}
	reloaded, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(reloaded.Routing.Rules) != 1 {
		t.Fatalf("rules len = %d, want 1", len(reloaded.Routing.Rules))
	}
	if reloaded.Routing.Rules[0].IsEnabled() {
		t.Fatal("full PUT with enabled=false should preserve disabled state")
	}
}

func TestHandleRoutingRuleItemFullPUTEnablesAndPreservesID(t *testing.T) {
	s, nm, path := newRoutingAPIServer(t)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/routing/rules/rule-1", strings.NewReader(`{"id":"wrong-id","name":"openai","category":"openai","target":"n","enabled":true}`))
	s.handleRoutingRuleItem(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	if nm.reloads != 1 {
		t.Fatalf("reloads = %d, want 1", nm.reloads)
	}
	reloaded, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	rule := reloaded.Routing.Rules[0]
	if rule.ID != "rule-1" || !rule.IsEnabled() {
		t.Fatalf("saved rule = %+v, want id rule-1 and enabled", rule)
	}
}

func TestHandleRoutingRuleItemRejectsInvalidWithoutMutation(t *testing.T) {
	s, nm, path := newRoutingAPIServer(t)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/routing/rules/rule-1", strings.NewReader(`{"name":"openai","category":"openai","enabled":true}`))
	s.handleRoutingRuleItem(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body=%s", rec.Code, rec.Body.String())
	}
	if nm.reloads != 0 {
		t.Fatalf("reloads = %d, want 0", nm.reloads)
	}
	reloaded, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	rule := reloaded.Routing.Rules[0]
	if rule.ID != "rule-1" || !rule.IsEnabled() {
		t.Fatalf("saved rule mutated: %+v", rule)
	}
}
