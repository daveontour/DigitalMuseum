package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
)

// VisitorSessionLLMPolicy controls whether a visitor-key session may use the archive owner's
// stored API keys and/or the server's env keys (still subject to users.allow_server_llm_keys).
type VisitorSessionLLMPolicy struct {
	AllowOwnerKeys  bool
	AllowServerKeys bool
}

// GetVisitorSessionLLMPolicy returns LLM policy for an active visitor session.
// Share-link sessions (no visitor_key_hint_id) behave as both flags true.
// Missing or non-visitor sessions return both true so behaviour matches unrestricted merge.
func (r *UserRepo) GetVisitorSessionLLMPolicy(ctx context.Context, sessionID string) (*VisitorSessionLLMPolicy, error) {
	if sessionID == "" {
		return &VisitorSessionLLMPolicy{AllowOwnerKeys: true, AllowServerKeys: true}, nil
	}
	var ao, as bool
	err := r.pool.QueryRowContext(ctx, `
		SELECT
			COALESCE(h.llm_allow_owner_keys, TRUE),
			COALESCE(h.llm_allow_server_keys, TRUE)
		FROM sessions s
		LEFT JOIN visitor_key_hints h ON h.id = s.visitor_key_hint_id
		WHERE s.id = ?1 AND s.expires_at > CURRENT_TIMESTAMP AND s.is_visitor = TRUE`,
		sessionID,
	).Scan(&ao, &as)
	if errors.Is(err, sql.ErrNoRows) {
		return &VisitorSessionLLMPolicy{AllowOwnerKeys: true, AllowServerKeys: true}, nil
	}
	if err != nil {
		return nil, err
	}
	return &VisitorSessionLLMPolicy{AllowOwnerKeys: ao, AllowServerKeys: as}, nil
}

// ErrVisitorSessionLLMNotUpdated is returned when PATCH targets a missing, expired, or non-visitor session.
var ErrVisitorSessionLLMNotUpdated = errors.New("session not found, expired, or not a visitor session")

// sessionLLMJSON is the JSON shape stored in sessions.visitor_llm_overrides.
type sessionLLMJSON struct {
	GeminiAPIKey     string `json:"gemini_api_key,omitempty"`
	AnthropicAPIKey  string `json:"anthropic_api_key,omitempty"`
	GeminiModel      string `json:"gemini_model,omitempty"`
	ClaudeModel      string `json:"claude_model,omitempty"`
	TavilyAPIKey     string `json:"tavily_api_key,omitempty"`
	DeepSeekAPIKey   string `json:"deepseek_api_key,omitempty"`
	DeepSeekModel    string `json:"deepseek_model,omitempty"`
	OpenAIAPIKey     string `json:"openai_api_key,omitempty"`
	OpenAIModel      string `json:"openai_model,omitempty"`
	RunpodAPIKey     string `json:"runpod_api_key,omitempty"`
	ElevenLabsAPIKey string `json:"elevenlabs_api_key,omitempty"`
	RunpodEndpointID string `json:"runpod_endpoint_id,omitempty"`
	RunpodWorkers    int    `json:"runpod_workers,omitempty"`
}

func sessionLLMFromStored(s UserLLMStored) sessionLLMJSON {
	return sessionLLMJSON{
		GeminiAPIKey:     s.GeminiAPIKey,
		AnthropicAPIKey:  s.AnthropicAPIKey,
		GeminiModel:      s.GeminiModel,
		ClaudeModel:      s.ClaudeModel,
		TavilyAPIKey:     s.TavilyAPIKey,
		DeepSeekAPIKey:   s.DeepSeekAPIKey,
		DeepSeekModel:    s.DeepSeekModel,
		OpenAIAPIKey:     s.OpenAIAPIKey,
		OpenAIModel:      s.OpenAIModel,
		RunpodAPIKey:     s.RunpodAPIKey,
		ElevenLabsAPIKey: s.ElevenLabsAPIKey,
		RunpodEndpointID: s.RunpodEndpointID,
		RunpodWorkers:    s.RunpodWorkers,
	}
}

func (j sessionLLMJSON) toStored() UserLLMStored {
	return UserLLMStored{
		GeminiAPIKey:     j.GeminiAPIKey,
		AnthropicAPIKey:  j.AnthropicAPIKey,
		GeminiModel:      j.GeminiModel,
		ClaudeModel:      j.ClaudeModel,
		TavilyAPIKey:     j.TavilyAPIKey,
		DeepSeekAPIKey:   j.DeepSeekAPIKey,
		DeepSeekModel:    j.DeepSeekModel,
		OpenAIAPIKey:     j.OpenAIAPIKey,
		OpenAIModel:      j.OpenAIModel,
		RunpodAPIKey:     j.RunpodAPIKey,
		ElevenLabsAPIKey: j.ElevenLabsAPIKey,
		RunpodEndpointID: j.RunpodEndpointID,
		RunpodWorkers:    j.RunpodWorkers,
	}
}

// GetSessionVisitorLLM loads visitor_llm_overrides for an active visitor session.
// Returns empty values when the session is missing, expired, not visitor, or has no JSON.
func (r *UserRepo) GetSessionVisitorLLM(ctx context.Context, sessionID string) (*UserLLMStored, error) {
	if sessionID == "" {
		return &UserLLMStored{}, nil
	}
	var raw []byte
	err := r.pool.QueryRowContext(ctx, `
		SELECT visitor_llm_overrides
		FROM sessions
		WHERE id = ?1 AND expires_at > CURRENT_TIMESTAMP AND is_visitor = TRUE`,
		sessionID,
	).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return &UserLLMStored{}, nil
	}
	if err != nil {
		return nil, err
	}
	if len(raw) == 0 {
		return &UserLLMStored{}, nil
	}
	var j sessionLLMJSON
	if err := json.Unmarshal(raw, &j); err != nil {
		return &UserLLMStored{}, nil
	}
	out := j.toStored()
	return &out, nil
}

// PatchSessionVisitorLLM merges patch into visitor_llm_overrides for the current visitor session only.
func (r *UserRepo) PatchSessionVisitorLLM(ctx context.Context, sessionID string, p UserLLMPatch) error {
	if sessionID == "" {
		return ErrVisitorSessionLLMNotUpdated
	}
	cur, err := r.GetSessionVisitorLLM(ctx, sessionID)
	if err != nil {
		return err
	}
	next := mergeLLMStoredWithPatch(*cur, p)
	j := sessionLLMFromStored(next)
	payload, err := json.Marshal(j)
	if err != nil {
		return err
	}
	tag, err := r.pool.ExecContext(ctx, `
		UPDATE sessions
		SET visitor_llm_overrides = ?2
		WHERE id = ?1 AND expires_at > CURRENT_TIMESTAMP AND is_visitor = TRUE`,
		sessionID, string(payload),
	)
	if err != nil {
		return err
	}
	if rowsAffectedOrZero(tag) == 0 {
		return ErrVisitorSessionLLMNotUpdated
	}
	return nil
}
