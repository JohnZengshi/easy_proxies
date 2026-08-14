package monitor

import (
	"log"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"easy_proxies/internal/config"
)

func newExportServer(t *testing.T) (*Server, *Manager) {
	t.Helper()
	mgr, err := NewManager(Config{ProbeTarget: "https://www.google.com"})
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	s := &Server{
		cfg:    Config{},
		mgr:    mgr,
		logger: log.Default(),
	}
	s.cfgMu.Lock()
	s.cfgSrc = &config.Config{
		Mode:     "hybrid",
		Listener: config.ListenerConfig{Address: "127.0.0.1", Port: 2323},
		GeoIP: config.GeoIPConfig{
			Enabled: true,
			Listen:  "127.0.0.1",
			Port:    1221,
		},
	}
	s.cfgMu.Unlock()
	return s, mgr
}

func TestHandleExport_NineRouterIncludesEveryActiveNode(t *testing.T) {
	s, mgr := newExportServer(t)

	available := mgr.Register(NodeInfo{Tag: "available", URI: "vless://a@a.example.com:443", ListenAddress: "127.0.0.1", Port: 24000})
	unavailable := mgr.Register(NodeInfo{Tag: "unavailable", URI: "vless://b@b.example.com:443", ListenAddress: "127.0.0.1", Port: 24001})
	_ = mgr.Register(NodeInfo{Tag: "unchecked", URI: "vless://c@c.example.com:443", ListenAddress: "127.0.0.1", Port: 24002})
	blacklisted := mgr.Register(NodeInfo{Tag: "blacklisted", URI: "vless://d@d.example.com:443", ListenAddress: "127.0.0.1", Port: 24003})
	_ = mgr.Register(NodeInfo{Tag: "duplicate", URI: "vless://e@e.example.com:443", ListenAddress: "127.0.0.1", Port: 24000})
	_ = mgr.Register(NodeInfo{Tag: "no-port", URI: "vless://f@f.example.com:443", ListenAddress: "127.0.0.1"})

	available.ref.mu.Lock()
	available.ref.initialCheckDone = true
	available.ref.available = true
	available.ref.mu.Unlock()

	unavailable.ref.mu.Lock()
	unavailable.ref.initialCheckDone = true
	unavailable.ref.available = false
	unavailable.ref.mu.Unlock()

	blacklisted.ref.mu.Lock()
	blacklisted.ref.initialCheckDone = true
	blacklisted.ref.available = true
	blacklisted.ref.blacklist = true
	blacklisted.ref.mu.Unlock()

	req := httptest.NewRequest("GET", "/api/export?target=9router", nil)
	rec := httptest.NewRecorder()
	s.handleExport(rec, req)

	if rec.Code != 200 {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); ct != "text/plain; charset=utf-8" {
		t.Errorf("Content-Type = %q, want text/plain", ct)
	}

	body := strings.TrimSpace(rec.Body.String())
	lines := strings.Split(body, "\n")
	if len(lines) != 4 {
		t.Fatalf("got %d lines, want 4 (healthy, unhealthy, unchecked, blacklisted; duplicate and no-port excluded)\n%s", len(lines), body)
	}
	for _, line := range lines {
		u, err := url.Parse(line)
		if err != nil {
			t.Fatalf("line %q is not a valid URL: %v", line, err)
		}
		if u.Scheme != "http" || u.Hostname() != "127.0.0.1" || u.Port() == "" || u.User != nil {
			t.Errorf("line %q is not a no-auth loopback HTTP URI", line)
		}
	}
	for _, banned := range []string{"#", "socks", "@", ":2323", ":1221"} {
		if strings.Contains(body, banned) {
			t.Errorf("9router export contains %q:\n%s", banned, body)
		}
	}
}

func TestHandleExport_NineRouterRejectsUnknownTargetAndPreservesDefault(t *testing.T) {
	s, mgr := newExportServer(t)
	node := mgr.Register(NodeInfo{Tag: "node", URI: "vless://a@a.example.com:443", ListenAddress: "127.0.0.1", Port: 24000})
	node.ref.mu.Lock()
	node.ref.initialCheckDone = true
	node.ref.available = true
	node.ref.mu.Unlock()

	unknown := httptest.NewRecorder()
	s.handleExport(unknown, httptest.NewRequest("GET", "/api/export?target=docker", nil))
	if unknown.Code != 400 {
		t.Fatalf("unknown target status = %d, want 400", unknown.Code)
	}

	def := httptest.NewRecorder()
	s.handleExport(def, httptest.NewRequest("GET", "/api/export", nil))
	if def.Code != 200 {
		t.Fatalf("default export status = %d, want 200", def.Code)
	}
	for _, want := range []string{"# Pool 代理池入口", "http://127.0.0.1:2323", "http://127.0.0.1:24000"} {
		if !strings.Contains(def.Body.String(), want) {
			t.Errorf("default export does not preserve %q:\n%s", want, def.Body.String())
		}
	}
}

func TestEmbeddedIndex_ExposesNineRouterExport(t *testing.T) {
	data, err := embeddedFS.ReadFile("assets/index.html")
	if err != nil {
		t.Fatalf("read embedded index: %v", err)
	}
	html := string(data)
	if !strings.Contains(html, "target=9router") {
		t.Fatal("index.html does not reference the 9router export endpoint")
	}
}
