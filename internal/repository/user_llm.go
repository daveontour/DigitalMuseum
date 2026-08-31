package repository

import (
	"context"
	"strings"
)

// mergeLLMStoredWithPatch applies p onto cur (nil pointer fields in p leave cur unchanged).
func mergeLLMStoredWithPatch(cur UserLLMStored, p UserLLMPatch) UserLLMStored {
	ork, tk, rk, ek := cur.OpenRouterAPIKey, cur.TavilyAPIKey, cur.RunpodAPIKey, cur.ElevenLabsAPIKey
	reid, rw := cur.RunpodEndpointID, cur.RunpodWorkers
	if p.OpenRouterAPIKey != nil {
		ork = strings.TrimSpace(*p.OpenRouterAPIKey)
	}
	if p.TavilyAPIKey != nil {
		tk = strings.TrimSpace(*p.TavilyAPIKey)
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
		OpenRouterAPIKey:   ork,
		TavilyAPIKey:       tk,
		RunpodAPIKey:       rk,
		ElevenLabsAPIKey:   ek,
		RunpodEndpointID:   reid,
		RunpodWorkers:      rw,
		AllowServerLLMKeys: cur.AllowServerLLMKeys,
	}
}

// UserLLMStored holds per-user API keys and model overrides (empty = use server defaults).
// AllowServerLLMKeys governs fallback to server env keys when a per-user key is empty.
type UserLLMStored struct {
	OpenRouterAPIKey   string
	TavilyAPIKey       string
	RunpodAPIKey       string
	ElevenLabsAPIKey   string
	RunpodEndpointID   string
	RunpodWorkers      int // 0 = use server default; 1-8 = explicit override
	AllowServerLLMKeys bool
}

// UserLLMPatch updates only fields present in the JSON request (nil = leave unchanged).
type UserLLMPatch struct {
	OpenRouterAPIKey *string
	TavilyAPIKey     *string
	RunpodAPIKey     *string
	ElevenLabsAPIKey *string
	RunpodEndpointID *string
	RunpodWorkers    *int
}

// GetUserLLMStored loads persisted LLM overrides for the user.
func (r *UserRepo) GetUserLLMStored(ctx context.Context, userID int64) (*UserLLMStored, error) {
	var s UserLLMStored
	err := r.pool.QueryRowContext(ctx, `
		SELECT COALESCE(user_openrouter_api_key, ''),
		       COALESCE(user_tavily_api_key, ''),
		       COALESCE(user_runpod_api_key, ''),
		       COALESCE(user_elevenlabs_api_key, ''),
		       COALESCE(user_runpod_endpoint_id, ''),
		       COALESCE(user_runpod_workers, 0),
		       allow_server_llm_keys
		FROM users WHERE id = ?1`,
		userID,
	).Scan(&s.OpenRouterAPIKey, &s.TavilyAPIKey, &s.RunpodAPIKey, &s.ElevenLabsAPIKey, &s.RunpodEndpointID, &s.RunpodWorkers, &s.AllowServerLLMKeys)
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
			user_openrouter_api_key = NULLIF(?2, ''),
			user_tavily_api_key = NULLIF(?3, ''),
			user_runpod_api_key = NULLIF(?4, ''),
			user_elevenlabs_api_key = NULLIF(?5, ''),
			user_runpod_endpoint_id = NULLIF(?6, ''),
			user_runpod_workers = ?7
		WHERE id = ?1`,
		userID, next.OpenRouterAPIKey, next.TavilyAPIKey, next.RunpodAPIKey, next.ElevenLabsAPIKey, next.RunpodEndpointID, runpodWorkersVal,
	)
	return err
}
