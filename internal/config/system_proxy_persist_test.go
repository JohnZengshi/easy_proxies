package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSystemProxyEnabledOrDefault(t *testing.T) {
	cfg := ManagementConfig{}
	if value, present := cfg.SystemProxyEnabledOrDefault(); value || present {
		t.Fatalf("unset SystemProxyEnabled = (%v, %v), want (false, false)", value, present)
	}

	enabled := true
	cfg.SystemProxyEnabled = &enabled
	if value, present := cfg.SystemProxyEnabledOrDefault(); !present || !value {
		t.Fatalf("true SystemProxyEnabled = (%v, %v), want (true, true)", value, present)
	}

	enabled = false
	if value, present := cfg.SystemProxyEnabledOrDefault(); !present || value {
		t.Fatalf("false SystemProxyEnabled = (%v, %v), want (false, true)", value, present)
	}
}

func TestSystemProxyEnabledRoundTripThroughSaveSettings(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	writeFile(t, path, `mode: pool
listener:
  address: 127.0.0.1
  port: 2323
management:
  listen: 127.0.0.1:9091
nodes:
  - name: test
    uri: ss://YWVzLTI1Ni1nY206cGFzcw==@127.0.0.1:8388
`)

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("load config without flag: %v", err)
	}
	if value, present := cfg.Management.SystemProxyEnabledOrDefault(); present || value {
		t.Fatalf("unset flag loaded as (%v, %v), want (false, false)", value, present)
	}

	enabled := true
	cfg.Management.SystemProxyEnabled = &enabled
	if err := cfg.SaveSettings(); err != nil {
		t.Fatalf("save settings: %v", err)
	}
	reloaded, err := Load(path)
	if err != nil {
		t.Fatalf("reload config after save: %v", err)
	}
	if value, present := reloaded.Management.SystemProxyEnabledOrDefault(); !present || !value {
		t.Fatalf("saved flag loaded as (%v, %v), want (true, true)", value, present)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read saved config: %v", err)
	}
	if !strings.Contains(string(data), "system_proxy_enabled: true") {
		t.Fatalf("saved config missing system_proxy_enabled: true\n%s", data)
	}
}

func TestLoadSystemProxyEnabledExplicit(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	writeFile(t, path, `mode: pool
listener:
  address: 127.0.0.1
  port: 2323
management:
  system_proxy_enabled: true
nodes:
  - name: test
    uri: ss://YWVzLTI1Ni1nY206cGFzcw==@127.0.0.1:8388
`)

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("load config with flag: %v", err)
	}
	if value, present := cfg.Management.SystemProxyEnabledOrDefault(); !present || !value {
		t.Fatalf("explicit true flag loaded as (%v, %v), want (true, true)", value, present)
	}
}

func TestLoadSystemProxyEnabledRejectsMalformedValue(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	writeFile(t, path, `mode: pool
listener:
  address: 127.0.0.1
  port: 2323
management:
  system_proxy_enabled: sometimes
nodes:
  - name: test
    uri: ss://YWVzLTI1Ni1nY206cGFzcw==@127.0.0.1:8388
`)

	if _, err := Load(path); err == nil {
		t.Fatal("malformed system_proxy_enabled loaded without error")
	}
}
