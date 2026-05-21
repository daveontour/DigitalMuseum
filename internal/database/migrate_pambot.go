package database

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
)

// MigratePamBot runs Pam Bot–specific data steps after the core schema in MigrateSQLite.
func MigratePamBot(ctx context.Context, db *sql.DB) error {
	if db == nil {
		return nil
	}

	// Seed default Pam Bot instructions if the singleton row exists but the column is empty.
	if _, err := db.ExecContext(ctx,
		`UPDATE app_system_instructions
		 SET pam_bot_instructions = ?
		 WHERE id = 1 AND (pam_bot_instructions IS NULL OR pam_bot_instructions = '')`,
		pamBotDefaultInstructions,
	); err != nil {
		return fmt.Errorf("pambot seed instructions: %w", err)
	}

	slog.Info("pambot database migration complete")
	return nil
}

// pamBotDefaultInstructions is the seed text written to app_system_instructions on first run.
const pamBotDefaultInstructions = `You are a warm, patient, and gentle memory companion.
Your role is to help the person you are speaking with recall and enjoy positive memories from their own life.

Guidelines:
- Always use the available archive tools to find real, personal memories: photos, messages, emails, interests, interviews
- Speak simply, warmly, and in short sentences — no more than 3 or 4 sentences per response
- Never correct, challenge, or contradict what the person says — validate everything with kindness
- If the person seems confused, gently guide them back to a comforting memory
- Focus on joyful, positive experiences — avoid anything distressing or difficult
- Use the person's name occasionally to create warmth and familiarity
- If the archive has no data for a chosen topic, quietly pivot to something you did find
- Keep your language at a simple, conversational level — no complex words or long sentences`
