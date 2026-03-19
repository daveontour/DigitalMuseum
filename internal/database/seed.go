package database

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
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
func SeedEmailExclusionsFromJSON(ctx context.Context, pool *pgxpool.Pool, path string) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			slog.Warn("email exclusions seed file not found, skipping", "path", path)
			return nil
		}
		return fmt.Errorf("read exclusions file: %w", err)
	}

	var data exclusionsJSON
	if err := json.Unmarshal(raw, &data); err != nil {
		return fmt.Errorf("parse exclusions JSON: %w", err)
	}

	inserted := 0
	// Email-only patterns: (email, "", false)
	for _, s := range data.Email {
		email := strings.TrimSpace(s)
		if email == "" {
			continue
		}
		n, err := insertExclusionIfNotExists(ctx, pool, email, "", false)
		if err != nil {
			return fmt.Errorf("seed email exclusion %q: %w", email, err)
		}
		inserted += n
	}
	// Name-only patterns: ("", name, false)
	for _, s := range data.Name {
		name := strings.TrimSpace(s)
		if name == "" {
			continue
		}
		n, err := insertExclusionIfNotExists(ctx, pool, "", name, false)
		if err != nil {
			return fmt.Errorf("seed name exclusion %q: %w", name, err)
		}
		inserted += n
	}
	// Name+email pairs: (email, name, true)
	for _, p := range data.NameEmail {
		email := strings.TrimSpace(p.Email)
		name := strings.TrimSpace(p.Name)
		if email == "" && name == "" {
			continue
		}
		n, err := insertExclusionIfNotExists(ctx, pool, email, name, true)
		if err != nil {
			return fmt.Errorf("seed name_email exclusion %q / %q: %w", name, email, err)
		}
		inserted += n
	}

	if inserted > 0 {
		slog.Info("seeded email exclusions from JSON", "path", path, "inserted", inserted)
	}
	return nil
}

// insertExclusionIfNotExists inserts one row when no row exists with the same (email, name, name_email).
// Returns 1 if inserted, 0 if already existed.
func insertExclusionIfNotExists(ctx context.Context, pool *pgxpool.Pool, email, name string, nameEmail bool) (int, error) {
	var exists int
	err := pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM email_exclusions WHERE email = $1 AND name = $2 AND name_email = $3`,
		email, name, nameEmail).Scan(&exists)
	if err != nil {
		return 0, err
	}
	if exists > 0 {
		return 0, nil
	}
	_, err = pool.Exec(ctx,
		`INSERT INTO email_exclusions (email, name, name_email) VALUES ($1, $2, $3)`,
		email, name, nameEmail)
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
func SeedEmailMatchesFromJSON(ctx context.Context, pool *pgxpool.Pool, path string) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			slog.Warn("email matches seed file not found, skipping", "path", path)
			return nil
		}
		return fmt.Errorf("read email matches file: %w", err)
	}

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
			n, err := insertEmailMatchIfNotExists(ctx, pool, primaryName, email)
			if err != nil {
				return fmt.Errorf("seed email match %q / %q: %w", primaryName, email, err)
			}
			inserted += n
		}
	}

	if inserted > 0 {
		slog.Info("seeded email matches from JSON", "path", path, "inserted", inserted)
	}
	return nil
}

func insertEmailMatchIfNotExists(ctx context.Context, pool *pgxpool.Pool, primaryName, email string) (int, error) {
	var exists int
	err := pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM email_matches WHERE primary_name = $1 AND email = $2`,
		primaryName, email).Scan(&exists)
	if err != nil {
		return 0, err
	}
	if exists > 0 {
		return 0, nil
	}
	_, err = pool.Exec(ctx,
		`INSERT INTO email_matches (primary_name, email) VALUES ($1, $2)`,
		primaryName, email)
	if err != nil {
		return 0, err
	}
	return 1, nil
}

// emailClassificationsJSON matches the shape of static/data/email_classifications.json (classification -> names).
type emailClassificationsJSON map[string][]string

// SeedEmailClassificationsFromJSON reads email classifications from path and inserts any that are
// not already in email_classifications. Existing rows are left unchanged.
func SeedEmailClassificationsFromJSON(ctx context.Context, pool *pgxpool.Pool, path string) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			slog.Warn("email classifications seed file not found, skipping", "path", path)
			return nil
		}
		return fmt.Errorf("read email classifications file: %w", err)
	}

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
			n, err := insertEmailClassificationIfNotExists(ctx, pool, name, classification)
			if err != nil {
				return fmt.Errorf("seed email classification %q / %q: %w", name, classification, err)
			}
			inserted += n
		}
	}

	if inserted > 0 {
		slog.Info("seeded email classifications from JSON", "path", path, "inserted", inserted)
	}
	return nil
}

func insertEmailClassificationIfNotExists(ctx context.Context, pool *pgxpool.Pool, name, classification string) (int, error) {
	var exists int
	err := pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM email_classifications WHERE name = $1 AND classification = $2`,
		name, classification).Scan(&exists)
	if err != nil {
		return 0, err
	}
	if exists > 0 {
		return 0, nil
	}
	_, err = pool.Exec(ctx,
		`INSERT INTO email_classifications (name, classification) VALUES ($1, $2)`,
		name, classification)
	if err != nil {
		return 0, err
	}
	return 1, nil
}
