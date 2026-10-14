package ai

import (
	"reflect"
	"testing"
)

func TestWithRecommendedToolsEnabled(t *testing.T) {
	metas := []ToolMeta{
		{Name: "search_chat_messages_globally"},
		{Name: "get_emails_by_contact"},
		{Name: "get_sensitive_reference_document"},
		{Name: "search_tavily"},
		{Name: "weather:get_forecast"}, // additional (untrusted) MCP server
	}
	policy := ToolAccessPolicy{
		"get_emails_by_contact":            {Enabled: false, VisitorEnabled: true},
		"get_sensitive_reference_document": {Enabled: false, VisitorEnabled: false},
		"not_in_catalog_any_more":          {Enabled: true},
	}

	before := SummarizeToolAccess(policy, metas)
	if before != (ToolAccessSummary{Available: 5, EnabledForOwner: 0, RecommendedOff: 2, RecommendedPresent: 2}) {
		t.Fatalf("summary before = %+v", before)
	}

	got, turnedOn := WithRecommendedToolsEnabled(policy, metas)
	if want := []string{"get_emails_by_contact", "search_chat_messages_globally"}; !reflect.DeepEqual(turnedOn, want) {
		t.Fatalf("turned on %v, want %v", turnedOn, want)
	}
	if r := got["get_emails_by_contact"]; !r.Enabled || !r.VisitorEnabled {
		t.Fatalf("visitor flag must be kept: %+v", r)
	}
	if r := got["search_chat_messages_globally"]; !r.Enabled || r.VisitorEnabled {
		t.Fatalf("new rule must be owner-only: %+v", r)
	}
	for _, name := range []string{"get_sensitive_reference_document", "search_tavily", "weather:get_forecast"} {
		if PolicyAllows(got, name, TierMaster) {
			t.Fatalf("%s must not be enabled", name)
		}
	}
	if !got["not_in_catalog_any_more"].Enabled {
		t.Fatal("unrelated rules must be kept")
	}
	if policy["get_emails_by_contact"].Enabled {
		t.Fatal("input policy must not be modified")
	}

	after := SummarizeToolAccess(got, metas)
	if after.EnabledForOwner != 2 || after.RecommendedOff != 0 {
		t.Fatalf("summary after = %+v", after)
	}
	if _, again := WithRecommendedToolsEnabled(got, metas); len(again) != 0 {
		t.Fatalf("second run should turn nothing on, got %v", again)
	}
}
