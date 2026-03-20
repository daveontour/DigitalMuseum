package ai

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/daveontour/digitalmuseum/internal/keystore"
)

// LLMToolsAccessStoreKey is the private_store key for LLM tool visibility policy.
const LLMToolsAccessStoreKey = "llm_tools_access_v1"

// UnlockTier describes how the browser session unlocked the keyring for policy checks.
type UnlockTier int

const (
	TierNone UnlockTier = iota
	TierVisitor
	TierMaster
)

// UnlockTierFromSession maps session cookie state to a tier.
func UnlockTierFromSession(store *keystore.SessionMasterStore, r *http.Request) UnlockTier {
	if store == nil || r == nil {
		return TierNone
	}
	unlocked, master := store.SessionStatus(r)
	if !unlocked {
		return TierNone
	}
	if master {
		return TierMaster
	}
	return TierVisitor
}

// ToolAccessRule is stored per tool name. Omitted tools default to denied for everyone.
type ToolAccessRule struct {
	NoKey   bool `json:"no_key"`
	Visitor bool `json:"visitor"`
	Master  bool `json:"master"`
}

// ToolAccessPolicy maps tool name -> rule. Nil or missing entries mean "deny".
type ToolAccessPolicy map[string]ToolAccessRule

// storedPolicyJSON is the on-disk shape in private_store.
type storedPolicyJSON struct {
	Tools map[string]ToolAccessRule `json:"tools"`
}

// ParseToolAccessPolicyJSON parses private_store JSON. Unknown fields ignored.
func ParseToolAccessPolicyJSON(raw string) (ToolAccessPolicy, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ToolAccessPolicy{}, nil
	}
	var s storedPolicyJSON
	if err := json.Unmarshal([]byte(raw), &s); err != nil {
		return nil, err
	}
	if s.Tools == nil {
		return ToolAccessPolicy{}, nil
	}
	return ToolAccessPolicy(s.Tools), nil
}

// MarshalToolAccessPolicyJSON serialises policy for private_store.
func MarshalToolAccessPolicyJSON(p ToolAccessPolicy) (string, error) {
	if p == nil {
		p = ToolAccessPolicy{}
	}
	b, err := json.Marshal(storedPolicyJSON{Tools: map[string]ToolAccessRule(p)})
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// PolicyAllows reports whether the tool may run for this tier. Nil policy => deny all.
func PolicyAllows(policy ToolAccessPolicy, toolName string, tier UnlockTier) bool {
	if policy == nil {
		return false
	}
	rule, ok := policy[toolName]
	if !ok {
		return false
	}
	switch tier {
	case TierNone:
		return rule.NoKey
	case TierVisitor:
		return rule.Visitor
	case TierMaster:
		return rule.Master
	default:
		return false
	}
}

// FilterToolDefinitionsForTier returns tool schema entries allowed for this tier.
func FilterToolDefinitionsForTier(policy ToolAccessPolicy, tier UnlockTier) []map[string]any {
	all := toolDefinitions()
	var out []map[string]any
	for _, td := range all {
		name, _ := td["name"].(string)
		if name == "" {
			continue
		}
		if PolicyAllows(policy, name, tier) {
			out = append(out, td)
		}
	}
	return out
}

// ToolMeta is a stable name + description for UI and API.
type ToolMeta struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

// AllToolMetas lists every tool with description (for settings UI).
func AllToolMetas() []ToolMeta {
	var out []ToolMeta
	for _, td := range toolDefinitions() {
		name, _ := td["name"].(string)
		desc, _ := td["description"].(string)
		if name != "" {
			out = append(out, ToolMeta{Name: name, Description: desc})
		}
	}
	return out
}

// WrapToolExecutorWithPolicy enforces policy on every tool invocation (defense in depth).
func WrapToolExecutorWithPolicy(inner ToolExecutor, policy ToolAccessPolicy, tier UnlockTier) ToolExecutor {
	if inner == nil {
		inner = func(ctx context.Context, name string, args map[string]any) (map[string]any, error) {
			return map[string]any{"error": "tool executor not configured"}, nil
		}
	}
	return func(ctx context.Context, name string, args map[string]any) (map[string]any, error) {
		if !PolicyAllows(policy, name, tier) {
			return map[string]any{
				"error": "this tool is not enabled for your current session (no key / visitor / master policy)",
			}, nil
		}
		return inner(ctx, name, args)
	}
}
