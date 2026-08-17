package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadRoutingRules(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	yaml := `
mode: pool
listener:
  address: 127.0.0.1
  port: 2323
management:
  port: 9091
nodes:
  - name: jp-1
    uri: ss://YWVzLTI1Ni1nY206cGFzcw==@127.0.0.1:8388
routing:
  china_direct_enabled: false
  rules:
    - name: openai
      category: openai
      target: jp-1
`
	if err := os.WriteFile(path, []byte(yaml), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Routing.ChinaDirect() {
		t.Fatal("expected china direct to be disabled")
	}
	if len(cfg.Routing.Rules) != 1 || cfg.Routing.Rules[0].Name != "openai" {
		t.Fatalf("unexpected routing rules: %+v", cfg.Routing.Rules)
	}
}

func TestLoadRoutingRulesRejectsEmptyTarget(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	yaml := `
mode: pool
listener:
  port: 2323
nodes:
  - name: jp-1
    uri: ss://YWVzLTI1Ni1nY206cGFzcw==@127.0.0.1:8388
routing:
  rules:
    - name: bad
      category: openai
`
	if err := os.WriteFile(path, []byte(yaml), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path); err == nil {
		t.Fatal("expected empty target to be rejected")
	}
}

func TestLoadRoutingRulesAddsIDsAndPreservesDisabled(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	yaml := `
mode: pool
listener:
  port: 2323
nodes:
  - name: jp-1
    uri: ss://YWVzLTI1Ni1nY206cGFzcw==@127.0.0.1:8388
routing:
  rules:
    - name: a
      category: openai
      target: proxy-pool
    - name: b
      domain_suffix: [example.com]
      target: proxy-pool
      enabled: false
`
	if err := os.WriteFile(path, []byte(yaml), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Routing.Rules) != 2 {
		t.Fatalf("expected 2 rules, got %d", len(cfg.Routing.Rules))
	}
	if cfg.Routing.Rules[0].ID == "" || cfg.Routing.Rules[1].ID == "" || cfg.Routing.Rules[0].ID == cfg.Routing.Rules[1].ID {
		t.Fatalf("rules should have unique ids: %+v", cfg.Routing.Rules)
	}
	if !cfg.Routing.Rules[0].IsEnabled() {
		t.Fatal("rule without enabled should default to true")
	}
	if cfg.Routing.Rules[1].IsEnabled() {
		t.Fatal("rule with enabled=false should be disabled")
	}
}

func TestLoadRoutingFallbackDefaultsDirect(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	yamlText := `
mode: pool
listener:
  port: 2323
nodes:
  - name: jp-1
    uri: ss://YWVzLTI1Ni1nY206cGFzcw==@127.0.0.1:8388
routing:
  rules: []
`
	if err := os.WriteFile(path, []byte(yamlText), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Routing.FallbackOrDefault() != RoutingFallbackDirect {
		t.Fatalf("FallbackOrDefault = %q, want %q", cfg.Routing.FallbackOrDefault(), RoutingFallbackDirect)
	}
}

func TestLoadRoutingFallbackProxyPoolPersists(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	yamlText := `
mode: pool
listener:
  port: 2323
nodes:
  - name: jp-1
    uri: ss://YWVzLTI1Ni1nY206cGFzcw==@127.0.0.1:8388
routing:
  fallback: proxy-pool
  rules: []
`
	if err := os.WriteFile(path, []byte(yamlText), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Routing.FallbackOrDefault() != RoutingFallbackProxyPool {
		t.Fatalf("FallbackOrDefault = %q, want %q", cfg.Routing.FallbackOrDefault(), RoutingFallbackProxyPool)
	}
	if err := cfg.SaveSettings(); err != nil {
		t.Fatalf("SaveSettings: %v", err)
	}
	reloaded, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.Routing.FallbackOrDefault() != RoutingFallbackProxyPool {
		t.Fatalf("reloaded fallback = %q, want %q", reloaded.Routing.FallbackOrDefault(), RoutingFallbackProxyPool)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "fallback: proxy-pool") {
		t.Fatalf("saved config missing fallback:\n%s", data)
	}
}

func TestLoadRoutingFallbackRejectsInvalid(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	yamlText := `
mode: pool
listener:
  port: 2323
nodes:
  - name: jp-1
    uri: ss://YWVzLTI1Ni1nY206cGFzcw==@127.0.0.1:8388
routing:
  fallback: invalid
  rules: []
`
	if err := os.WriteFile(path, []byte(yamlText), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path); err == nil || !strings.Contains(err.Error(), "routing fallback") {
		t.Fatalf("expected routing fallback load error, got %v", err)
	}
}
