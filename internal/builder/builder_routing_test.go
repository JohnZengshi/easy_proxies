package builder

import (
	"testing"

	"easy_proxies/internal/config"
	"easy_proxies/internal/routing"

	C "github.com/sagernet/sing-box/constant"
)

func TestBuildRoutingRules(t *testing.T) {
	cfg := &config.Config{
		Nodes: []config.NodeConfig{
			{Name: "jp-1", URI: "ss://YWVzLTI1Ni1nY206cGFzcw==@127.0.0.1:8388"},
		},
		Routing: config.RoutingConfig{
			Rules: []config.RoutingRule{
				{Name: "openai", Category: "openai", Target: "jp-1"},
				{Name: "custom", DomainSuffix: []string{"example.com"}, Target: "proxy-pool"},
			},
			ChinaDirectEnabled: boolPtr(true),
		},
	}
	_ = routing.Categories()
	rules := buildRoutingRules(cfg, []string{"jp-1"})
	if len(rules) != 2 {
		t.Fatalf("expected 2 rules, got %d: %+v", len(rules), rules)
	}
	if rules[0].DefaultOptions.RawDefaultRule.DomainSuffix == nil {
		t.Fatal("expected category expansion to produce domain suffix")
	}
}

func boolPtr(v bool) *bool { return &v }

func TestBuildRoutingTargetInvalid(t *testing.T) {
	cfg := &config.Config{
		Routing: config.RoutingConfig{
			Rules: []config.RoutingRule{
				{Name: "bad", DomainSuffix: []string{"example.com"}, Target: "missing"},
			},
		},
	}
	rules := buildRoutingRules(cfg, []string{"ok"})
	if len(rules) != 0 {
		t.Fatalf("expected invalid target to be skipped, got %d", len(rules))
	}
}

func TestBuildIncludesDirectChina(t *testing.T) {
	cfg := &config.Config{
		Mode:  "pool",
		Nodes: []config.NodeConfig{{Name: "n", URI: "ss://YWVzLTI1Ni1nY206cGFzcw==@127.0.0.1:8388"}},
		Routing: config.RoutingConfig{
			ChinaDirectEnabled: func() *bool { v := true; return &v }(),
		},
	}
	opts, err := Build(cfg)
	if err == nil {
		found := false
		for _, ob := range opts.Outbounds {
			if ob.Tag == "direct-cn" && ob.Type == C.TypeDirect {
				found = true
			}
		}
		if !found {
			t.Fatal("expected direct-cn outbound in built options")
		}
	} else {
		t.Fatalf("unexpected error: %v", err)
	}
}
