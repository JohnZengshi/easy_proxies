package builder

import (
	"slices"
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

func TestBuildRoutingSkipsDisabledRules(t *testing.T) {
	enabled := true
	disabled := false
	cfg := &config.Config{
		Routing: config.RoutingConfig{
			Rules: []config.RoutingRule{
				{Name: "on", DomainSuffix: []string{"on.example.com"}, Target: "ok", Enabled: &enabled},
				{Name: "off", DomainSuffix: []string{"off.example.com"}, Target: "ok", Enabled: &disabled},
			},
		},
	}
	rules := buildRoutingRules(cfg, []string{"ok"})
	if len(rules) != 1 {
		t.Fatalf("expected only enabled rule, got %d", len(rules))
	}
}

func TestBuildRoutingAnthropicCheckerTarget(t *testing.T) {
	enabled := true
	cfg := &config.Config{
		Routing: config.RoutingConfig{
			Rules: []config.RoutingRule{
				{Name: "anthropic", Category: "anthropic", Target: "tw-1-udp", Enabled: &enabled},
			},
		},
	}
	rules := buildRoutingRules(cfg, []string{"tw-1-udp"})
	if len(rules) != 1 {
		t.Fatalf("rules len = %d, want 1", len(rules))
	}
	rule := rules[0].DefaultOptions
	if got := rule.RuleAction.RouteOptions.Outbound; got != "tw-1-udp" {
		t.Fatalf("outbound = %q, want tw-1-udp", got)
	}
	if !slices.Contains([]string(rule.RawDefaultRule.DomainSuffix), "ip.net.coffee") {
		t.Fatalf("Anthropic domains missing ip.net.coffee: %v", rule.RawDefaultRule.DomainSuffix)
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

func TestBuildRoutingFallbackFinal(t *testing.T) {
	cfg := &config.Config{
		Mode:  "pool",
		Nodes: []config.NodeConfig{{Name: "n", URI: "ss://YWVzLTI1Ni1nY206cGFzcw==@127.0.0.1:8388"}},
		Routing: config.RoutingConfig{
			Fallback:           config.RoutingFallbackDirect,
			ChinaDirectEnabled: func() *bool { v := false; return &v }(),
		},
	}
	opts, err := Build(cfg)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if opts.Route == nil {
		t.Fatal("expected route options")
	}
	if opts.Route.Final != "direct-fallback" {
		t.Fatalf("route.final = %q, want direct-fallback", opts.Route.Final)
	}
	foundDirect := false
	for _, ob := range opts.Outbounds {
		if ob.Tag == "direct-fallback" && ob.Type == C.TypeDirect {
			foundDirect = true
		}
	}
	if !foundDirect {
		t.Fatal("expected direct-fallback outbound")
	}

	cfg.Routing.Fallback = config.RoutingFallbackProxyPool
	opts, err = Build(cfg)
	if err != nil {
		t.Fatalf("Build(proxy-pool): %v", err)
	}
	if opts.Route == nil || opts.Route.Final != "proxy-pool" {
		t.Fatalf("route.final = %q, want proxy-pool", opts.Route.Final)
	}
}

func TestBuildRoutingEnabledRuleOverridesDirectFallback(t *testing.T) {
	enabled := true
	disabled := false
	cfg := &config.Config{
		Mode:  "pool",
		Nodes: []config.NodeConfig{{Name: "n", URI: "ss://YWVzLTI1Ni1nY206cGFzcw==@127.0.0.1:8388"}},
		Routing: config.RoutingConfig{
			Fallback: config.RoutingFallbackDirect,
			Rules: []config.RoutingRule{
				{Name: "on", DomainSuffix: []string{"on.example.com"}, Target: "n", Enabled: &enabled},
				{Name: "off", DomainSuffix: []string{"off.example.com"}, Target: "n", Enabled: &disabled},
			},
		},
	}
	opts, err := Build(cfg)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if opts.Route == nil || len(opts.Route.Rules) == 0 {
		t.Fatal("expected enabled custom rule in route rules")
	}
	if got := opts.Route.Rules[0].DefaultOptions.RuleAction.RouteOptions.Outbound; got != "n" {
		t.Fatalf("first rule outbound = %q, want %q", got, "n")
	}
	for _, rule := range opts.Route.Rules {
		raw := rule.DefaultOptions.RawDefaultRule
		if len(raw.DomainSuffix) > 0 && raw.DomainSuffix[0] == "off.example.com" {
			t.Fatalf("disabled rule leaked into route: %+v", rule)
		}
	}
}
