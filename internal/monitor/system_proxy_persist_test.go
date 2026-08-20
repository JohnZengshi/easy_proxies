package monitor

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"easy_proxies/internal/config"
	"easy_proxies/internal/sysproxy"
)

type systemProxyStub struct {
	enabled bool
}

func (p *systemProxyStub) Enable(string) error {
	p.enabled = true
	return nil
}

func (p *systemProxyStub) Disable() error {
	p.enabled = false
	return nil
}

func (p *systemProxyStub) CleanupStale(string, string) error {
	return nil
}

func TestHandleSystemProxyPersist(t *testing.T) {
	if !sysproxy.Supported() {
		t.Skip("system proxy is only supported on darwin and windows")
	}

	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	yamlText := `mode: pool
listener:
  address: 127.0.0.1
  port: 2323
management:
  enabled: true
  listen: 127.0.0.1:9091
nodes:
  - name: test
    uri: ss://YWVzLTI1Ni1nY206cGFzcw==@127.0.0.1:8388
`
	if err := os.WriteFile(path, []byte(yamlText), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}

	stub := &systemProxyStub{}
	server := &Server{cfgSrc: cfg, sysProxy: stub}

	post := func(body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "/api/system-proxy", strings.NewReader(body))
		rec := httptest.NewRecorder()
		server.handleSystemProxy(rec, req)
		return rec
	}

	rec := post(`{"enabled":true}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("enable status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	if !stub.enabled {
		t.Fatal("stub proxy was not enabled")
	}

	reloaded, err := config.Load(path)
	if err != nil {
		t.Fatalf("reload config after enable: %v", err)
	}
	if value, present := reloaded.Management.SystemProxyEnabledOrDefault(); !present || !value {
		t.Fatalf("persisted enable = (%v, %v), want (true, true)", value, present)
	}

	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read config before unchanged request: %v", err)
	}
	rec = post(`{"enabled":true}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("unchanged status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read config after unchanged request: %v", err)
	}
	if !bytes.Equal(before, after) {
		t.Fatal("unchanged system-proxy request rewrote config")
	}

	rec = post(`{"enabled":false}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("disable status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	if stub.enabled {
		t.Fatal("stub proxy was not disabled")
	}
	reloaded, err = config.Load(path)
	if err != nil {
		t.Fatalf("reload config after disable: %v", err)
	}
	if value, present := reloaded.Management.SystemProxyEnabledOrDefault(); !present || value {
		t.Fatalf("persisted disable = (%v, %v), want (false, true)", value, present)
	}
}
