package config

import (
	"os"
	"path/filepath"
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
