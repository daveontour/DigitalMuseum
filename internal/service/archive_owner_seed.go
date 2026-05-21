package service

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	appai "github.com/daveontour/aimuseum/internal/ai"
	"github.com/daveontour/aimuseum/internal/repository"
	backgroundjobs "github.com/daveontour/aimuseum/internal/service/background_jobs"
)

// SeedNewOwnerArchiveDefaults enables all LLM tools (private_store policy) and all
// background jobs (auto-start + restart-on-complete) for a freshly created archive
// after the first owner exists and the master keyring is initialised.
func SeedNewOwnerArchiveDefaults(ctx context.Context, db *sql.DB, masterPassword, pepper string) error {
	if db == nil {
		return fmt.Errorf("database is required")
	}
	mp := strings.TrimSpace(masterPassword)
	if mp != "" {
		policyJSON, err := appai.MarshalToolAccessPolicyJSON(appai.AllToolsEnabledPolicy())
		if err != nil {
			return fmt.Errorf("llm tools policy: %w", err)
		}
		privateStore := NewPrivateStoreService(repository.NewPrivateStoreRepo(db), db, pepper)
		if err := privateStore.Upsert(ctx, appai.LLMToolsAccessStoreKey, policyJSON, mp); err != nil {
			return fmt.Errorf("seed llm tools access: %w", err)
		}
	}
	bgJobsRepo := repository.NewBackgroundJobRepo(db)
	if err := backgroundjobs.SeedForNewArchive(ctx, bgJobsRepo); err != nil {
		return fmt.Errorf("background jobs: %w", err)
	}
	return nil
}
