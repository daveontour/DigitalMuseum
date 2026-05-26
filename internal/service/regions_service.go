package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/daveontour/aimuseum/internal/georegion"
	"github.com/daveontour/aimuseum/internal/model"
	"github.com/daveontour/aimuseum/internal/repository"
)

// RegionsService manages deployment-wide region configuration rows.
type RegionsService struct {
	repo *repository.RegionsRepo
	db   *sql.DB
}

// NewRegionsService creates a RegionsService.
func NewRegionsService(repo *repository.RegionsRepo, db *sql.DB) *RegionsService {
	return &RegionsService{repo: repo, db: db}
}

// ListAll returns all region rows for the Configuration UI.
func (s *RegionsService) ListAll(ctx context.Context) ([]*model.RegionRow, error) {
	rows, err := s.repo.ListAll(ctx)
	if err != nil {
		return nil, err
	}
	if rows == nil {
		return []*model.RegionRow{}, nil
	}
	return rows, nil
}

// GetByID returns one row.
func (s *RegionsService) GetByID(ctx context.Context, id int64) (*model.RegionRow, error) {
	return s.repo.GetByID(ctx, id)
}

// UpdateDefaults updates the reserved default rows.
func (s *RegionsService) UpdateDefaults(ctx context.Context, defaultRegion, defaultLabel string) error {
	defaultRegion = strings.TrimSpace(defaultRegion)
	defaultLabel = strings.TrimSpace(defaultLabel)
	if defaultRegion == "" {
		return fmt.Errorf("default_region is required")
	}
	if defaultLabel == "" {
		return fmt.Errorf("default_label is required")
	}
	if err := s.updateReservedText(ctx, georegion.KeyDefaultRegion, defaultRegion); err != nil {
		return err
	}
	if err := s.updateReservedText(ctx, georegion.KeyDefaultLabel, defaultLabel); err != nil {
		return err
	}
	return s.reload(ctx)
}

func (s *RegionsService) updateReservedText(ctx context.Context, key, value string) error {
	row, err := s.repo.GetByKey(ctx, key)
	if err != nil {
		return err
	}
	text, err := json.Marshal(value)
	if err != nil {
		return err
	}
	if row == nil {
		sortOrder := 0
		if key == georegion.KeyDefaultLabel {
			sortOrder = 1
		}
		_, err = s.repo.Create(ctx, key, sortOrder, string(text))
		return err
	}
	_, err = s.repo.Update(ctx, row.ID, key, row.SortOrder, string(text))
	return err
}

// CreateRegion inserts a bbox region row.
func (s *RegionsService) CreateRegion(ctx context.Context, def georegion.RegionDefinition, sortOrder *int) (*model.RegionRow, error) {
	if err := validateRegionDefinition(&def); err != nil {
		return nil, err
	}
	key := strings.TrimSpace(def.Code)
	if georegion.IsReservedKey(key) {
		return nil, fmt.Errorf("key %q is reserved", key)
	}
	exists, err := s.repo.KeyExists(ctx, key)
	if err != nil {
		return nil, err
	}
	if exists {
		return nil, fmt.Errorf("conflict:region key already exists: %s", key)
	}
	order := 0
	if sortOrder != nil {
		order = *sortOrder
	} else {
		max, err := s.repo.MaxSortOrder(ctx)
		if err != nil {
			return nil, err
		}
		order = max + 1
	}
	text, err := json.Marshal(def)
	if err != nil {
		return nil, err
	}
	row, err := s.repo.Create(ctx, key, order, string(text))
	if err != nil {
		return nil, err
	}
	if err := s.reload(ctx); err != nil {
		return nil, err
	}
	return row, nil
}

// UpdateRegion updates a bbox region row.
func (s *RegionsService) UpdateRegion(ctx context.Context, id int64, def georegion.RegionDefinition, sortOrder *int) (*model.RegionRow, error) {
	row, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if row == nil {
		return nil, nil
	}
	if georegion.IsReservedKey(row.Key) {
		return nil, fmt.Errorf("cannot edit reserved row via region update")
	}
	if err := validateRegionDefinition(&def); err != nil {
		return nil, err
	}
	key := strings.TrimSpace(def.Code)
	if georegion.IsReservedKey(key) {
		return nil, fmt.Errorf("key %q is reserved", key)
	}
	conflict, err := s.repo.KeyExistsExcluding(ctx, key, id)
	if err != nil {
		return nil, err
	}
	if conflict {
		return nil, fmt.Errorf("conflict:region key already exists: %s", key)
	}
	order := row.SortOrder
	if sortOrder != nil {
		order = *sortOrder
	}
	text, err := json.Marshal(def)
	if err != nil {
		return nil, err
	}
	updated, err := s.repo.Update(ctx, id, key, order, string(text))
	if err != nil {
		return nil, err
	}
	if updated == nil {
		return nil, nil
	}
	if err := s.reload(ctx); err != nil {
		return nil, err
	}
	return updated, nil
}

// DeleteRegion removes a non-reserved row.
func (s *RegionsService) DeleteRegion(ctx context.Context, id int64) error {
	row, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return err
	}
	if row == nil {
		return nil
	}
	if georegion.IsReservedKey(row.Key) {
		return fmt.Errorf("cannot delete reserved key %q", row.Key)
	}
	ok, err := s.repo.Delete(ctx, id)
	if err != nil {
		return err
	}
	if !ok {
		return nil
	}
	return s.reload(ctx)
}

// Reorder updates sort_order for multiple rows.
func (s *RegionsService) Reorder(ctx context.Context, items []struct {
	ID        int64
	SortOrder int
}) error {
	if len(items) == 0 {
		return nil
	}
	if err := s.repo.ReorderSortOrder(ctx, items); err != nil {
		return err
	}
	return s.reload(ctx)
}

// ExportConfig builds the regions.json interchange document from DB rows.
func (s *RegionsService) ExportConfig(ctx context.Context) (georegion.Config, error) {
	rows, err := s.repo.ListAll(ctx)
	if err != nil {
		return georegion.Config{}, err
	}
	cfg := georegion.Config{Regions: []georegion.RegionDefinition{}}
	for _, row := range rows {
		switch row.Key {
		case georegion.KeyDefaultRegion:
			var code string
			if err := json.Unmarshal([]byte(row.Text), &code); err != nil {
				return georegion.Config{}, fmt.Errorf("parse %s: %w", row.Key, err)
			}
			cfg.DefaultRegion = code
		case georegion.KeyDefaultLabel:
			var label string
			if err := json.Unmarshal([]byte(row.Text), &label); err != nil {
				return georegion.Config{}, fmt.Errorf("parse %s: %w", row.Key, err)
			}
			cfg.DefaultLabel = label
		default:
			var def georegion.RegionDefinition
			if err := json.Unmarshal([]byte(row.Text), &def); err != nil {
				return georegion.Config{}, fmt.Errorf("parse region %q: %w", row.Key, err)
			}
			cfg.Regions = append(cfg.Regions, def)
		}
	}
	return cfg, nil
}

// ImportConflict describes an uploaded key that already exists in the database.
type ImportConflict struct {
	Key      string                      `json:"key"`
	Existing georegion.RegionDefinition  `json:"existing"`
	Uploaded georegion.RegionDefinition  `json:"uploaded"`
}

// ImportPreviewResult is returned by import preview.
type ImportPreviewResult struct {
	New       []georegion.RegionDefinition `json:"new"`
	Conflicts []ImportConflict             `json:"conflicts"`
}

// ImportPreview parses uploaded JSON and compares keys with the database.
func (s *RegionsService) ImportPreview(ctx context.Context, data []byte) (*ImportPreviewResult, error) {
	cfg, err := parseImportConfig(data)
	if err != nil {
		return nil, err
	}
	result := &ImportPreviewResult{
		New:       []georegion.RegionDefinition{},
		Conflicts: []ImportConflict{},
	}
	for _, def := range cfg.Regions {
		key := strings.TrimSpace(def.Code)
		if key == "" || georegion.IsReservedKey(key) {
			continue
		}
		row, err := s.repo.GetByKey(ctx, key)
		if err != nil {
			return nil, err
		}
		if row == nil {
			result.New = append(result.New, def)
			continue
		}
		var existing georegion.RegionDefinition
		if err := json.Unmarshal([]byte(row.Text), &existing); err != nil {
			return nil, fmt.Errorf("parse existing region %q: %w", key, err)
		}
		result.Conflicts = append(result.Conflicts, ImportConflict{
			Key:      key,
			Existing: existing,
			Uploaded: def,
		})
	}
	return result, nil
}

// ImportApply inserts new keys and applies per-key conflict resolutions, then reloads.
// resolutions maps region code to "keep" or "replace". Defaults from upload apply when present.
func (s *RegionsService) ImportApply(ctx context.Context, data []byte, resolutions map[string]string) error {
	cfg, err := parseImportConfig(data)
	if err != nil {
		return err
	}
	if strings.TrimSpace(cfg.DefaultRegion) != "" {
		if err := s.updateReservedText(ctx, georegion.KeyDefaultRegion, cfg.DefaultRegion); err != nil {
			return err
		}
	}
	if strings.TrimSpace(cfg.DefaultLabel) != "" {
		if err := s.updateReservedText(ctx, georegion.KeyDefaultLabel, cfg.DefaultLabel); err != nil {
			return err
		}
	}
	maxOrder, err := s.repo.MaxSortOrder(ctx)
	if err != nil {
		return err
	}
	nextOrder := maxOrder + 1
	for _, def := range cfg.Regions {
		key := strings.TrimSpace(def.Code)
		if key == "" || georegion.IsReservedKey(key) {
			continue
		}
		if err := validateRegionDefinition(&def); err != nil {
			return err
		}
		row, err := s.repo.GetByKey(ctx, key)
		if err != nil {
			return err
		}
		text, err := json.Marshal(def)
		if err != nil {
			return err
		}
		if row == nil {
			if _, err := s.repo.Create(ctx, key, nextOrder, string(text)); err != nil {
				return err
			}
			nextOrder++
			continue
		}
		action := strings.ToLower(strings.TrimSpace(resolutions[key]))
		if action == "" {
			action = "keep"
		}
		if action == "replace" {
			if _, err := s.repo.Update(ctx, row.ID, key, row.SortOrder, string(text)); err != nil {
				return err
			}
		}
	}
	return s.reload(ctx)
}

func (s *RegionsService) reload(ctx context.Context) error {
	return georegion.ReloadFromDB(ctx, s.db)
}

func parseImportConfig(data []byte) (georegion.Config, error) {
	var cfg georegion.Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return georegion.Config{}, fmt.Errorf("parse import JSON: %w", err)
	}
	if err := georegion.ValidateConfig(&cfg); err != nil {
		return georegion.Config{}, err
	}
	return cfg, nil
}

func validateRegionDefinition(def *georegion.RegionDefinition) error {
	tmp := georegion.Config{
		DefaultRegion: "oth",
		Regions:       []georegion.RegionDefinition{*def},
	}
	return georegion.ValidateConfig(&tmp)
}
