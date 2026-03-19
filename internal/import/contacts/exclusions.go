package contacts

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

// NameEmailPair excludes a specific name when paired with a specific email
// (e.g. recipient name incorrectly paired with sender's email in source data)
type NameEmailPair struct {
	Name  string `json:"name"`
	Email string `json:"email"`
}

// ExclusionsConfig holds email and name exclusion patterns
type ExclusionsConfig struct {
	Email     []string        `json:"email"`
	Name      []string        `json:"name"`
	NameEmail []NameEmailPair `json:"name_email"`
}

var defaultNameEmailExclusions = []NameEmailPair{
	{},
}

var defaultExclusions = ExclusionsConfig{
	Email:     []string{},
	Name:      []string{},
	NameEmail: defaultNameEmailExclusions,
}

var exclusions = defaultExclusions

// LoadExclusions loads exclusions from a JSON file
func LoadExclusions(filename string) error {
	raw, err := os.ReadFile(filename)
	if err != nil {
		return fmt.Errorf("failed to read exclusions file: %w", err)
	}
	var cfg ExclusionsConfig
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return fmt.Errorf("failed to parse exclusions JSON: %w", err)
	}
	exclusions = cfg
	exclusions.NameEmail = append(defaultNameEmailExclusions, exclusions.NameEmail...)
	return nil
}

// LoadExclusionsFromDB loads exclusions from the email_exclusions database table.
func LoadExclusionsFromDB(ctx context.Context, db *pgxpool.Pool) error {
	rows, err := db.Query(ctx, "SELECT email, name, name_email FROM email_exclusions")
	if err != nil {
		return fmt.Errorf("query email_exclusions: %w", err)
	}
	defer rows.Close()

	var cfg ExclusionsConfig
	for rows.Next() {
		var email, name string
		var nameEmail bool
		if err := rows.Scan(&email, &name, &nameEmail); err != nil {
			return fmt.Errorf("scan exclusion row: %w", err)
		}
		if nameEmail {
			cfg.NameEmail = append(cfg.NameEmail, NameEmailPair{Name: name, Email: email})
		} else if email != "" {
			cfg.Email = append(cfg.Email, email)
		} else if name != "" {
			cfg.Name = append(cfg.Name, name)
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate exclusion rows: %w", err)
	}
	exclusions = cfg
	exclusions.NameEmail = append(defaultNameEmailExclusions, exclusions.NameEmail...)
	return nil
}

func isExcluded(name, email string) bool {
	for _, exclusion := range exclusions.Email {
		if strings.Contains(email, exclusion) {
			return true
		}
	}
	for _, exclusion := range exclusions.Name {
		if strings.Contains(name, exclusion) {
			return true
		}
	}
	for _, pair := range exclusions.NameEmail {
		if strings.EqualFold(name, pair.Name) && strings.EqualFold(email, pair.Email) {
			return true
		}
	}
	return false
}
