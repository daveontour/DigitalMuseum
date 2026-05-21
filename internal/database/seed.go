package database

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"strings"
)

// exclusionsJSON matches the shape of static/data/exclusions.json.
type exclusionsJSON struct {
	Email     []string `json:"email"`
	Name      []string `json:"name"`
	NameEmail []struct {
		Name  string `json:"name"`
		Email string `json:"email"`
	} `json:"name_email"`
}

// SeedEmailExclusionsFromJSON reads exclusions from path and inserts any that are
// not already in email_exclusions. Existing rows are left unchanged.
func SeedEmailExclusionsFromJSON(ctx context.Context, db *sql.DB, path string) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			slog.Warn("email exclusions seed file not found, skipping", "path", path)
			return nil
		}
		return fmt.Errorf("read exclusions file: %w", err)
	}
	return SeedEmailExclusionsFromBytes(ctx, db, raw)
}

// SeedEmailExclusionsFromBytes parses raw JSON and inserts exclusions that are not
// already present. Returns the number of rows inserted.
func SeedEmailExclusionsFromBytes(ctx context.Context, db *sql.DB, raw []byte) error {
	var data exclusionsJSON
	if err := json.Unmarshal(raw, &data); err != nil {
		return fmt.Errorf("parse exclusions JSON: %w", err)
	}

	inserted := 0
	for _, s := range data.Email {
		email := strings.TrimSpace(s)
		if email == "" {
			continue
		}
		n, err := insertExclusionIfNotExists(ctx, db, email, "", false)
		if err != nil {
			return fmt.Errorf("seed email exclusion %q: %w", email, err)
		}
		inserted += n
	}
	for _, s := range data.Name {
		name := strings.TrimSpace(s)
		if name == "" {
			continue
		}
		n, err := insertExclusionIfNotExists(ctx, db, "", name, false)
		if err != nil {
			return fmt.Errorf("seed name exclusion %q: %w", name, err)
		}
		inserted += n
	}
	for _, p := range data.NameEmail {
		email := strings.TrimSpace(p.Email)
		name := strings.TrimSpace(p.Name)
		if email == "" && name == "" {
			continue
		}
		n, err := insertExclusionIfNotExists(ctx, db, email, name, true)
		if err != nil {
			return fmt.Errorf("seed name_email exclusion %q / %q: %w", name, email, err)
		}
		inserted += n
	}

	if inserted > 0 {
		slog.Info("seeded email exclusions", "inserted", inserted)
	}
	return nil
}

// insertExclusionIfNotExists inserts one row when no row exists with the same (email, name, name_email).
// Returns 1 if inserted, 0 if already existed.
func insertExclusionIfNotExists(ctx context.Context, db *sql.DB, email, name string, nameEmail bool) (int, error) {
	var exists int
	err := db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM email_exclusions WHERE email = ? AND name = ? AND name_email = ?`,
		email, name, nameEmail).Scan(&exists)
	if err != nil {
		return 0, err
	}
	if exists > 0 {
		return 0, nil
	}
	_, err = db.ExecContext(ctx,
		`INSERT INTO email_exclusions (email, name, name_email, user_id) VALUES (?, ?, ?, ?)`,
		email, name, nameEmail, nil)
	if err != nil {
		return 0, err
	}
	return 1, nil
}

// emailMatchesJSON matches the shape of static/data/email_matches.json (array of primary_name + emails).
type emailMatchesJSON []struct {
	PrimaryName string   `json:"primary_name"`
	Emails      []string `json:"emails"`
}

// SeedEmailMatchesFromJSON reads email matches from path and inserts any that are
// not already in email_matches. Existing rows are left unchanged.
func SeedEmailMatchesFromJSON(ctx context.Context, db *sql.DB, path string) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			slog.Warn("email matches seed file not found, skipping", "path", path)
			return nil
		}
		return fmt.Errorf("read email matches file: %w", err)
	}
	return SeedEmailMatchesFromBytes(ctx, db, raw)
}

// SeedEmailMatchesFromBytes parses raw JSON and inserts email matches that are not
// already present.
func SeedEmailMatchesFromBytes(ctx context.Context, db *sql.DB, raw []byte) error {
	var data emailMatchesJSON
	if err := json.Unmarshal(raw, &data); err != nil {
		return fmt.Errorf("parse email matches JSON: %w", err)
	}

	inserted := 0
	for _, entry := range data {
		primaryName := strings.TrimSpace(entry.PrimaryName)
		if primaryName == "" {
			continue
		}
		for _, e := range entry.Emails {
			email := strings.TrimSpace(e)
			if email == "" {
				continue
			}
			n, err := insertEmailMatchIfNotExists(ctx, db, primaryName, email)
			if err != nil {
				return fmt.Errorf("seed email match %q / %q: %w", primaryName, email, err)
			}
			inserted += n
		}
	}

	if inserted > 0 {
		slog.Info("seeded email matches", "inserted", inserted)
	}
	return nil
}

func insertEmailMatchIfNotExists(ctx context.Context, db *sql.DB, primaryName, email string) (int, error) {
	var exists int
	err := db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM email_matches WHERE primary_name = ? AND email = ?`,
		primaryName, email).Scan(&exists)
	if err != nil {
		return 0, err
	}
	if exists > 0 {
		return 0, nil
	}
	_, err = db.ExecContext(ctx,
		`INSERT INTO email_matches (primary_name, email, user_id) VALUES (?, ?, ?)`,
		primaryName, email, nil)
	if err != nil {
		return 0, err
	}
	return 1, nil
}

// emailClassificationsJSON matches the shape of static/data/email_classifications.json (classification -> names).
type emailClassificationsJSON map[string][]string

// SeedEmailClassificationsFromJSON reads email classifications from path and inserts any that are
// not already in email_classifications. Existing rows are left unchanged.
func SeedEmailClassificationsFromJSON(ctx context.Context, db *sql.DB, path string) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			slog.Warn("email classifications seed file not found, skipping", "path", path)
			return nil
		}
		return fmt.Errorf("read email classifications file: %w", err)
	}
	return SeedEmailClassificationsFromBytes(ctx, db, raw)
}

// SeedEmailClassificationsFromBytes parses raw JSON and inserts classifications that are
// not already present.
func SeedEmailClassificationsFromBytes(ctx context.Context, db *sql.DB, raw []byte) error {
	var data emailClassificationsJSON
	if err := json.Unmarshal(raw, &data); err != nil {
		return fmt.Errorf("parse email classifications JSON: %w", err)
	}

	inserted := 0
	for classification, names := range data {
		classification = strings.TrimSpace(classification)
		if classification == "" {
			continue
		}
		for _, name := range names {
			name = strings.TrimSpace(name)
			if name == "" {
				continue
			}
			n, err := insertEmailClassificationIfNotExists(ctx, db, name, classification)
			if err != nil {
				return fmt.Errorf("seed email classification %q / %q: %w", name, classification, err)
			}
			inserted += n
		}
	}

	if inserted > 0 {
		slog.Info("seeded email classifications", "inserted", inserted)
	}
	return nil
}

func insertEmailClassificationIfNotExists(ctx context.Context, db *sql.DB, name, classification string) (int, error) {
	var exists int
	err := db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM email_classifications WHERE name = ? AND classification = ?`,
		name, classification).Scan(&exists)
	if err != nil {
		return 0, err
	}
	if exists > 0 {
		return 0, nil
	}
	_, err = db.ExecContext(ctx,
		`INSERT INTO email_classifications (name, classification, user_id) VALUES (?, ?, ?)`,
		name, classification, nil)
	if err != nil {
		return 0, err
	}
	return 1, nil
}

// SeedAppSystemInstructionsFromFiles fills the singleton app_system_instructions row
// from static files when all three stored texts are empty (typical first boot).
func SeedAppSystemInstructionsFromFiles(ctx context.Context, db *sql.DB, staticDir string) error {
	var chat, core, q string
	err := db.QueryRowContext(ctx, `
		SELECT chat_instructions, core_instructions, question_instructions
		FROM app_system_instructions WHERE id = 1`).Scan(&chat, &core, &q)
	if err != nil {
		slog.Warn("app_system_instructions read for seed skipped", "err", err)
		return nil
	}
	if strings.TrimSpace(chat) != "" || strings.TrimSpace(core) != "" || strings.TrimSpace(q) != "" {
		return nil
	}
	read := func(rel string) (string, error) {
		path := strings.TrimSuffix(staticDir, "/") + "/" + rel
		b, err := os.ReadFile(path)
		if err != nil {
			if os.IsNotExist(err) {
				slog.Warn("system instructions seed file missing", "path", path)
				return "", nil
			}
			return "", err
		}
		return string(b), nil
	}
	chat, err = read("data/system_instructions_chat.txt")
	if err != nil {
		return fmt.Errorf("seed chat instructions: %w", err)
	}
	core, err = read("data/system_instructions_core.txt")
	if err != nil {
		return fmt.Errorf("seed core instructions: %w", err)
	}
	q, err = read("data/system_instructions_question.txt")
	if err != nil {
		return fmt.Errorf("seed question instructions: %w", err)
	}
	if chat == "" && core == "" && q == "" {
		return nil
	}
	_, err = db.ExecContext(ctx, `
		UPDATE app_system_instructions SET
			chat_instructions = ?,
			core_instructions = ?,
			question_instructions = ?,
			updated_at = CURRENT_TIMESTAMP
		WHERE id = 1`, chat, core, q)
	if err != nil {
		return fmt.Errorf("seed app_system_instructions update: %w", err)
	}
	slog.Info("seeded app_system_instructions from static files")
	return nil
}
