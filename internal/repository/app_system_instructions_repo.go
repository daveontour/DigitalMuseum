package repository

import (
	"context"
	"database/sql"
	"fmt"
)

// AppSystemInstructions is the singleton universal LLM system prompts (id = 1).
type AppSystemInstructions struct {
	ChatInstructions     string
	CoreInstructions     string
	QuestionInstructions string
	PamBotInstructions   string
}

// AppSystemInstructionsRepo reads/writes app_system_instructions.
type AppSystemInstructionsRepo struct {
	pool *sql.DB
}

// NewAppSystemInstructionsRepo creates an AppSystemInstructionsRepo.
func NewAppSystemInstructionsRepo(pool *sql.DB) *AppSystemInstructionsRepo {
	return &AppSystemInstructionsRepo{pool: pool}
}

// Get returns row id=1 or empty strings if missing.
func (r *AppSystemInstructionsRepo) Get(ctx context.Context) (*AppSystemInstructions, error) {
	row := r.pool.QueryRowContext(ctx, `
		SELECT chat_instructions, core_instructions, question_instructions,
		       COALESCE(pam_bot_instructions, '')
		FROM app_system_instructions WHERE id = 1`)
	var out AppSystemInstructions
	err := row.Scan(&out.ChatInstructions, &out.CoreInstructions, &out.QuestionInstructions, &out.PamBotInstructions)
	if err != nil {
		if isNoRows(err) {
			return &AppSystemInstructions{}, nil
		}
		return nil, fmt.Errorf("AppSystemInstructions Get: %w", err)
	}
	return &out, nil
}

// UpsertPamBotInstructions updates only the pam_bot_instructions column on row id=1.
// If the row does not yet exist it is created with empty chat/core/question fields.
func (r *AppSystemInstructionsRepo) UpsertPamBotInstructions(ctx context.Context, instructions string) error {
	_, err := r.pool.ExecContext(ctx, `
		INSERT INTO app_system_instructions (id, chat_instructions, core_instructions, question_instructions, pam_bot_instructions, user_id, updated_at)
		VALUES (1, '', '', '', ?1, NULL, CURRENT_TIMESTAMP)
		ON CONFLICT (id) DO UPDATE SET
			pam_bot_instructions = EXCLUDED.pam_bot_instructions,
			updated_at = CURRENT_TIMESTAMP`, instructions)
	if err != nil {
		return fmt.Errorf("UpsertPamBotInstructions: %w", err)
	}
	return nil
}

// Upsert replaces the singleton row (insert or update).
func (r *AppSystemInstructionsRepo) Upsert(ctx context.Context, chat, core, question string) error {
	_, err := r.pool.ExecContext(ctx, `
		INSERT INTO app_system_instructions (id, chat_instructions, core_instructions, question_instructions, user_id, updated_at)
		VALUES (1, ?1, ?2, ?3, NULL, CURRENT_TIMESTAMP)
		ON CONFLICT (id) DO UPDATE SET
			chat_instructions = EXCLUDED.chat_instructions,
			core_instructions = EXCLUDED.core_instructions,
			question_instructions = EXCLUDED.question_instructions,
			updated_at = CURRENT_TIMESTAMP`, chat, core, question)
	if err != nil {
		return fmt.Errorf("AppSystemInstructions Upsert: %w", err)
	}
	return nil
}
