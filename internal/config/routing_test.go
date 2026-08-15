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
