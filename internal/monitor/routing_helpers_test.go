package monitor

import (
	"testing"

	"easy_proxies/internal/config"
)

func TestValidateRoutingRuleExclusive(t *testing.T) {
	if err := validateRoutingRule(config.RoutingRule{Target: "proxy-pool", Category: "openai"}); err != nil {
		t.Fatalf("category rule should pass: %v", err)
	}
	if err := validateRoutingRule(config.RoutingRule{Target: "proxy-pool", DomainSuffix: []string{"example.com"}}); err != nil {
		t.Fatalf("custom rule should pass: %v", err)
	}
	if err := validateRoutingRule(config.RoutingRule{Target: "proxy-pool", Category: "openai", DomainSuffix: []string{"example.com"}}); err == nil {
		t.Fatal("both category and domains should be rejected")
	}
	if err := validateRoutingRule(config.RoutingRule{Target: "proxy-pool"}); err == nil {
		t.Fatal("rule with no matcher should be rejected")
	}
}

func TestAutoRuleName(t *testing.T) {
	if got := autoRuleName(config.RoutingRule{Category: "openai", Target: "proxy-pool"}); got != "openai-proxy-pool" {
		t.Fatalf("category name = %q", got)
	}
	if got := autoRuleName(config.RoutingRule{DomainSuffix: []string{"openai.com"}}); got != "openai.com" {
		t.Fatalf("custom name = %q", got)
	}
}

func TestRoutingRuleIndexByID(t *testing.T) {
	rules := []config.RoutingRule{{ID: "rule-1", Name: "a"}, {ID: "rule-2", Name: "b"}}
	if idx := routingRuleIndexByIDLocked(rules, "rule-2"); idx != 1 {
		t.Fatalf("id index = %d", idx)
	}
	if idx := routingRuleIndexByIDLocked(rules, "missing"); idx != -1 {
		t.Fatalf("missing index = %d", idx)
	}
}
