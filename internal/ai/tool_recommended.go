package ai

import "sort"

// Recommended tools — the "Turn on recommended AI tools" shortcut (Configuration → AI Tool Access,
// the chat warning, System status and the setup checklist).
//
// Every tool is denied until an admin enables it (PolicyAllows), which is the right default for a
// newly discovered tool but leaves a brand-new archive with an AI that can't look at anything. The
// shortcut enables, for the owner (master) tier only, the bundled read-only archive tools below
// that are present in the current catalog. It never touches visitor access, never enables a tool
// from an additional (untrusted) MCP server, and leaves out the sensitive-document tools and web
// search (search_tavily sends queries to an outside service) — those stay a deliberate choice.
var recommendedOwnerTools = map[string]bool{
	"get_current_time":                  true,
	"get_imessages_by_chat_session":     true,
	"get_messages_around_in_chat":       true,
	"list_available_chat_sessions":      true,
	"search_chat_messages_globally":     true,
	"search_chat_messages_in_session":   true,
	"search_messages_by_similarity":     true,
	"search_emails_by_similarity":       true,
	"get_emails_by_contact":             true,
	"get_all_messages_by_contact":       true,
	"find_photos_of_person":             true,
	"get_subject_writing_examples":      true,
	"search_facebook_albums":            true,
	"get_album_images":                  true,
	"search_facebook_posts":             true,
	"get_all_facebook_posts":            true,
	"get_unique_tags_count":             true,
	"get_user_interests":                true,
	"get_reference_document":            true,
	"get_available_reference_documents": true,
	"list_interviews":                   true,
	"get_interview":                     true,
	"list_complete_profiles":            true,
	"get_complete_profile":              true,
}

// IsRecommendedOwnerTool reports whether name is one of the recommended bundled tools.
func IsRecommendedOwnerTool(name string) bool { return recommendedOwnerTools[name] }

// ToolAccessSummary counts tools for the owner (master) tier against a catalog.
type ToolAccessSummary struct {
	Available          int `json:"available"`           // tools in the catalog
	EnabledForOwner    int `json:"enabled_for_owner"`   // of those, enabled for the master tier
	RecommendedOff     int `json:"recommended_off"`     // recommended tools present but not enabled
	RecommendedPresent int `json:"recommended_present"` // recommended tools present in the catalog
}

// SummarizeToolAccess counts metas against policy (a nil policy enables nothing).
func SummarizeToolAccess(policy ToolAccessPolicy, metas []ToolMeta) ToolAccessSummary {
	var s ToolAccessSummary
	for _, m := range metas {
		s.Available++
		on := PolicyAllows(policy, m.Name, TierMaster)
		if on {
			s.EnabledForOwner++
		}
		if recommendedOwnerTools[m.Name] {
			s.RecommendedPresent++
			if !on {
				s.RecommendedOff++
			}
		}
	}
	return s
}

// WithRecommendedToolsEnabled returns a copy of policy with every recommended tool present in
// metas enabled for the master tier (existing visitor flags kept), plus the sorted names it
// turned on. Tools not in metas and non-recommended tools are copied unchanged.
func WithRecommendedToolsEnabled(policy ToolAccessPolicy, metas []ToolMeta) (ToolAccessPolicy, []string) {
	out := make(ToolAccessPolicy, len(policy)+len(recommendedOwnerTools))
	for name, rule := range policy {
		out[name] = rule
	}
	var turnedOn []string
	for _, m := range metas {
		if !recommendedOwnerTools[m.Name] {
			continue
		}
		rule := out[m.Name]
		if rule.Enabled {
			continue
		}
		rule.Enabled = true
		out[m.Name] = rule
		turnedOn = append(turnedOn, m.Name)
	}
	sort.Strings(turnedOn)
	return out, turnedOn
}
