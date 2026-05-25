package service

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/daveontour/aimuseum/internal/config"
	"github.com/daveontour/aimuseum/internal/database"
)

// NewArchiveOpenHook seeds a newly opened archive file (app system instructions).
func NewArchiveOpenHook(cfg *config.Config) database.ArchiveOpenHook {
	_ = cfg
	return func(ctx context.Context, db *sql.DB, profileID string) error {
		if err := database.SeedAppSystemInstructionsFromFiles(ctx, db, "static"); err != nil {
			return fmt.Errorf("seed app system instructions: %w", err)
		}
		return nil
	}
}

// ResolveRequestArchiveProfileID reads X-Archive-Profile-Id, falling back to JSON/form field names when provided.
func ResolveRequestArchiveProfileID(rHeader string, bodyFallback string) string {
	if id := strings.TrimSpace(rHeader); id != "" {
		return id
	}
	return strings.TrimSpace(bodyFallback)
}
