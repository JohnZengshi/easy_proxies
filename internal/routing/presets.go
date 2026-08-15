package routing

import (
	"embed"
	"encoding/json"
	"sync"
)

//go:embed ruleset_snapshot.json
var rulesetFS embed.FS

// Category is a named set of domain suffixes that identify one service/group.
type Category struct {
	ID           string   `json:"id"`
	Name         string   `json:"name"`
	DomainSuffix []string `json:"domain_suffix"`
}

// ChinaRuleSet contains the domains and IP ranges that should bypass proxies.
type ChinaRuleSet struct {
	DomainSuffix []string `json:"domain_suffix"`
	IPCIDR       []string `json:"ip_cidr"`
}

type snapshot struct {
	Categories []Category   `json:"categories"`
	China      ChinaRuleSet `json:"china"`
}

var (
	loaded snapshot
	once   sync.Once
)

func loadSnapshot() snapshot {
	once.Do(func() {
		data, err := rulesetFS.ReadFile("ruleset_snapshot.json")
		if err != nil {
			panic("routing: load embedded ruleset: " + err.Error())
		}
		if err := json.Unmarshal(data, &loaded); err != nil {
			panic("routing: decode embedded ruleset: " + err.Error())
		}
	})
	return loaded
}

// Categories returns the service categories from the embedded ruleset snapshot.
func Categories() []Category {
	return loadSnapshot().Categories
}

// ExpandCategory returns the domain suffixes for a preset category. It returns
// nil when the category is unknown so callers can fall back to an error.
func ExpandCategory(id string) []string {
	for _, c := range loadSnapshot().Categories {
		if c.ID == id {
			return c.DomainSuffix
		}
	}
	return nil
}

// ChinaRules returns the embedded china-direct rule set.
func ChinaRules() ChinaRuleSet {
	return loadSnapshot().China
}
