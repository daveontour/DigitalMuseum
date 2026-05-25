package ai

import "testing"

func TestParseToolAccessPolicyJSON_legacyEnabledAppliesToBothTiers(t *testing.T) {
	p, err := ParseToolAccessPolicyJSON(`{"tools":{"get_current_time":{"enabled":true}}}`)
	if err != nil {
		t.Fatal(err)
	}
	rule := p["get_current_time"]
	if !rule.Enabled || !rule.VisitorEnabled {
		t.Fatalf("expected both tiers enabled, got %+v", rule)
	}
}

func TestParseToolAccessPolicyJSON_separateVisitorFlag(t *testing.T) {
	p, err := ParseToolAccessPolicyJSON(`{"tools":{"search_tavily":{"enabled":true,"visitor_enabled":false}}}`)
	if err != nil {
		t.Fatal(err)
	}
	rule := p["search_tavily"]
	if !rule.Enabled || rule.VisitorEnabled {
		t.Fatalf("expected master only, got %+v", rule)
	}
}

func TestPolicyAllows_tierSplit(t *testing.T) {
	p := ToolAccessPolicy{
		"search_tavily": {Enabled: true, VisitorEnabled: false},
		"get_current_time": {Enabled: false, VisitorEnabled: true},
	}
	if !PolicyAllows(p, "search_tavily", TierMaster) {
		t.Fatal("master should allow search_tavily")
	}
	if PolicyAllows(p, "search_tavily", TierVisitor) {
		t.Fatal("visitor should not allow search_tavily")
	}
	if PolicyAllows(p, "get_current_time", TierMaster) {
		t.Fatal("master should not allow get_current_time")
	}
	if !PolicyAllows(p, "get_current_time", TierVisitor) {
		t.Fatal("visitor should allow get_current_time")
	}
}
