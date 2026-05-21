package backgroundjobs

import (
	"context"
	"fmt"

	"github.com/daveontour/aimuseum/internal/repository"
)

// SeedForNewArchive enables auto-start and restart-on-complete for every registered
// background job on a freshly provisioned archive database.
func SeedForNewArchive(ctx context.Context, repo *repository.BackgroundJobRepo) error {
	for _, d := range DefaultDefinitions {
		if _, err := repo.Upsert(ctx, d.Name, true, true, d.DefaultIntervalSeconds); err != nil {
			return fmt.Errorf("seed background job %s: %w", d.Name, err)
		}
	}
	return nil
}
