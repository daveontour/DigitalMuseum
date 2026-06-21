package repository

import (
	"context"
	"strings"
)

// mergeLLMStoredWithPatch applies p onto cur (nil pointer fields in p leave cur unchanged).
func mergeLLMStoredWithPatch(cur UserLLMStored, p UserLLMPatch) UserLLMStored {
	gk, ak, gm, cm, tk, dk, dm, okKey, om, rk, ek := cur.GeminiAPIKey, cur.AnthropicAPIKey, cur.GeminiModel, cur.ClaudeModel, cur.TavilyAPIKey, cur.DeepSeekAPIKey, cur.DeepSeekModel, cur.OpenAIAPIKey, cur.OpenAIModel, cur.RunpodAPIKey, cur.ElevenLabsAPIKey
	reid, rw := cur.RunpodEndpointID, cur.RunpodWorkers
	if p.GeminiAPIKey != nil {
		gk = strings.TrimSpace(*p.GeminiAPIKey)
	}
	if p.AnthropicAPIKey != nil {
		ak = strings.TrimSpace(*p.AnthropicAPIKey)
	}
	if p.GeminiModel != nil {
		gm = strings.TrimSpace(*p.GeminiModel)
	}
	if p.ClaudeModel != nil {
		cm = strings.TrimSpace(*p.ClaudeModel)
	}
	if p.TavilyAPIKey != nil {
		tk = strings.TrimSpace(*p.TavilyAPIKey)
	}
	if p.DeepSeekAPIKey != nil {
		dk = strings.TrimSpace(*p.DeepSeekAPIKey)
	}
	if p.DeepSeekModel != nil {
		dm = strings.TrimSpace(*p.DeepSeekModel)
	}
	if p.OpenAIAPIKey != nil {
		okKey = strings.TrimSpace(*p.OpenAIAPIKey)
	}
	if p.OpenAIModel != nil {
		om = strings.TrimSpace(*p.OpenAIModel)
	}
	if p.RunpodAPIKey != nil {
		rk = strings.TrimSpace(*p.RunpodAPIKey)
	}
	if p.ElevenLabsAPIKey != nil {
		ek = strings.TrimSpace(*p.ElevenLabsAPIKey)
	}
	if p.RunpodEndpointID != nil {
		reid = strings.TrimSpace(*p.RunpodEndpointID)
	}
	if p.RunpodWorkers != nil {
		rw = *p.RunpodWorkers
		if rw < 0 {
			rw = 0
		}
		if rw > 8 {
			rw = 8
		}
	}
	return UserLLMStored{
		GeminiAPIKey:       gk,
		AnthropicAPIKey:    ak,
		GeminiModel:        gm,
		ClaudeModel:        cm,
		TavilyAPIKey:       tk,
		DeepSeekAPIKey:     dk,
		DeepSeekModel:      dm,
		OpenAIAPIKey:       okKey,
		OpenAIModel:        om,
		RunpodAPIKey:       rk,
		ElevenLabsAPIKey:   ek,
		RunpodEndpointID:   reid,
		RunpodWorkers:      rw,
		AllowServerLLMKeys: cur.AllowServerLLMKeys,
	}
}

// UserLLMStored holds per-user API keys and model overrides (empty = use server defaults).
// AllowServerLLMKeys governs fallback to server env keys when a per-provider user key is empty.
type UserLLMStored struct {
	GeminiAPIKey       string
	AnthropicAPIKey    string
	GeminiModel        string
	ClaudeModel        string
	TavilyAPIKey       string
	DeepSeekAPIKey     string
	DeepSeekModel      string
	OpenAIAPIKey       string
	OpenAIModel        string
	RunpodAPIKey       string
	ElevenLabsAPIKey   string
	RunpodEndpointID   string
	RunpodWorkers      int // 0 = use server default; 1-8 = explicit override
	AllowServerLLMKeys bool
}

// UserLLMPatch updates only fields present in the JSON request (nil = leave unchanged).
type UserLLMPatch struct {
	GeminiAPIKey     *string
	AnthropicAPIKey  *string
	GeminiModel      *string
	ClaudeModel      *string
	TavilyAPIKey     *string
	DeepSeekAPIKey   *string
	DeepSeekModel    *string
	OpenAIAPIKey     *string
	OpenAIModel      *string
	RunpodAPIKey     *string
	ElevenLabsAPIKey *string
	RunpodEndpointID *string
	RunpodWorkers    *int
}

// GetUserLLMStored loads persisted LLM overrides for the user.
func (r *UserRepo) GetUserLLMStored(ctx context.Context, userID int64) (*UserLLMStored, error) {
	var s UserLLMStored
	err := r.pool.QueryRowContext(ctx, `
		SELECT COALESCE(user_gemini_api_key, ''),
		       COALESCE(user_anthropic_api_key, ''),
		       COALESCE(user_gemini_model, ''),
		       COALESCE(user_claude_model, ''),
		       COALESCE(user_tavily_api_key, ''),
		       COALESCE(user_deepseek_api_key, ''),
		       COALESCE(user_deepseek_model, ''),
		       COALESCE(user_openai_api_key, ''),
		       COALESCE(user_openai_model, ''),
		       COALESCE(user_runpod_api_key, ''),
		       COALESCE(user_elevenlabs_api_key, ''),
		       COALESCE(user_runpod_endpoint_id, ''),
		       COALESCE(user_runpod_workers, 0),
		       allow_server_llm_keys
		FROM users WHERE id = ?1`,
		userID,
	).Scan(&s.GeminiAPIKey, &s.AnthropicAPIKey, &s.GeminiModel, &s.ClaudeModel, &s.TavilyAPIKey, &s.DeepSeekAPIKey, &s.DeepSeekModel, &s.OpenAIAPIKey, &s.OpenAIModel, &s.RunpodAPIKey, &s.ElevenLabsAPIKey, &s.RunpodEndpointID, &s.RunpodWorkers, &s.AllowServerLLMKeys)
	if err != nil {
		return nil, err
	}
	return &s, nil
}

// PatchUserLLMSettings merges patch into the user's stored values and saves.
func (r *UserRepo) PatchUserLLMSettings(ctx context.Context, userID int64, p UserLLMPatch) error {
	cur, err := r.GetUserLLMStored(ctx, userID)
	if err != nil {
		return err
	}
	next := mergeLLMStoredWithPatch(*cur, p)
	var runpodWorkersVal any
	if next.RunpodWorkers > 0 {
		runpodWorkersVal = next.RunpodWorkers
	}
	_, err = r.pool.ExecContext(ctx, `
		UPDATE users SET
			user_gemini_api_key = NULLIF(?2, ''),
			user_anthropic_api_key = NULLIF(?3, ''),
			user_gemini_model = NULLIF(?4, ''),
			user_claude_model = NULLIF(?5, ''),
			user_tavily_api_key = NULLIF(?6, ''),
			user_deepseek_api_key = NULLIF(?7, ''),
			user_deepseek_model = NULLIF(?8, ''),
			user_openai_api_key = NULLIF(?9, ''),
			user_openai_model = NULLIF(?10, ''),
			user_runpod_api_key = NULLIF(?11, ''),
			user_elevenlabs_api_key = NULLIF(?12, ''),
			user_runpod_endpoint_id = NULLIF(?13, ''),
			user_runpod_workers = ?14
		WHERE id = ?1`,
		userID, next.GeminiAPIKey, next.AnthropicAPIKey, next.GeminiModel, next.ClaudeModel, next.TavilyAPIKey, next.DeepSeekAPIKey, next.DeepSeekModel, next.OpenAIAPIKey, next.OpenAIModel, next.RunpodAPIKey, next.ElevenLabsAPIKey, next.RunpodEndpointID, runpodWorkersVal,
	)
	return err
}
