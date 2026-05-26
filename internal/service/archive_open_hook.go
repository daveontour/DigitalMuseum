package service

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/daveontour/aimuseum/internal/config"
	"github.com/daveontour/aimuseum/internal/database"
	"github.com/daveontour/aimuseum/internal/georegion"
)

// NewArchiveOpenHook seeds a newly opened archive file (app system instructions, regions, suggestions).
func NewArchiveOpenHook(cfg *config.Config) database.ArchiveOpenHook {
	regionsFile := cfg.App.RegionsConfigFile()
	suggestionsFile := cfg.App.SuggestionsConfigFile()
	return func(ctx context.Context, db *sql.DB, profileID string) error {
		if err := database.SeedAppSystemInstructionsFromFiles(ctx, db, "static"); err != nil {
			return fmt.Errorf("seed app system instructions: %w", err)
		}
		if err := database.SeedRegionsFromFileIfMissing(ctx, db, regionsFile); err != nil {
			return fmt.Errorf("seed regions: %w", err)
		}
		if err := georegion.ReloadFromDB(ctx, db); err != nil {
			return fmt.Errorf("load regions from db: %w", err)
		}
		if err := database.SeedSuggestionsFromFileIfMissing(ctx, db, suggestionsFile); err != nil {
			return fmt.Errorf("seed suggestions: %w", err)
		}
		return nil
	}
}
