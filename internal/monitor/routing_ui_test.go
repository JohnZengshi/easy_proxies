package monitor

import (
	"strings"
	"testing"
)

func TestRoutingUIFallbackAndAccessibleSwitch(t *testing.T) {
	data, err := embeddedFS.ReadFile("assets/index.html")
	if err != nil {
		t.Fatalf("read embedded index: %v", err)
	}
	html := string(data)
	for _, want := range []string{
		`<select id="routingFallback" class="setting-input" style="width:auto;" onchange="routingFallbackChanged(this)">`,
		`<option value="direct">直连</option>`,
		`<option value="proxy-pool">默认代理池</option>`,
		`fetch('/api/routing/fallback')`,
		`role="switch"`,
		`aria-checked`,
		`<span class="track" aria-hidden="true"></span>`,
		`colspan="4"`,
		`enabled: existing ? existing.enabled !== false : true`,
		`Object.assign({}, rule, {enabled})`,
	} {
		if !strings.Contains(html, want) {
			t.Errorf("embedded UI missing %q", want)
		}
	}
	if strings.Contains(html, "<th>启用</th>") {
		t.Error("rule table still has enabled header")
	}
}

func TestRoutingUIHasNoSystemProxyToggle(t *testing.T) {
	data, err := embeddedFS.ReadFile("assets/index.html")
	if err != nil {
		t.Fatalf("read embedded index: %v", err)
	}
	html := string(data)
	for _, stale := range []string{"routingSysProxyState", "toggleRoutingSystemProxy", "loadSysProxy"} {
		if strings.Contains(html, stale) {
			t.Errorf("embedded UI still references %q", stale)
		}
	}
}

func TestRoutingUIHasSystemProxyControl(t *testing.T) {
	data, err := embeddedFS.ReadFile("assets/index.html")
	if err != nil {
		t.Fatalf("read embedded index: %v", err)
	}
	html := string(data)
	for _, want := range []string{
		`id="settingSystemProxy"`,
		`toggleSystemProxy(this)`,
		`fetch('/api/system-proxy')`,
	} {
		if !strings.Contains(html, want) {
			t.Errorf("embedded UI missing %q", want)
		}
	}
}
