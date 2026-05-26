package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/daveontour/aimuseum/internal/model"
	"github.com/daveontour/aimuseum/internal/repository"
)

// GuideTopicInput holds the editable fields for one guide topic.
type GuideTopicInput struct {
	Key         string           `json:"key"`
	Title       string           `json:"title"`
	Description string           `json:"description,omitempty"`
	Category    string           `json:"category"`
	Recommended bool             `json:"recommended,omitempty"`
	Steps       []map[string]any `json:"steps"`
}

// GuideTopicsService manages deployment-wide guide topic configuration rows.
type GuideTopicsService struct {
	repo *repository.GuideTopicsRepo
}

// NewGuideTopicsService creates a GuideTopicsService.
func NewGuideTopicsService(repo *repository.GuideTopicsRepo) *GuideTopicsService {
	return &GuideTopicsService{repo: repo}
}

// ListAll returns all rows for the Configuration UI.
func (s *GuideTopicsService) ListAll(ctx context.Context) ([]*model.GuideTopicRow, error) {
	rows, err := s.repo.ListAll(ctx)
	if err != nil {
		return nil, err
	}
	if rows == nil {
		return []*model.GuideTopicRow{}, nil
	}
	return rows, nil
}

// GetByID returns one row.
func (s *GuideTopicsService) GetByID(ctx context.Context, id int64) (*model.GuideTopicRow, error) {
	return s.repo.GetByID(ctx, id)
}

// BuildTopicsDocument assembles the topics map from DB rows (keyed by topic key).
func (s *GuideTopicsService) BuildTopicsDocument(ctx context.Context) (map[string]any, error) {
	rows, err := s.repo.ListAll(ctx)
	if err != nil {
		return nil, err
	}
	topics := map[string]any{}
	for _, row := range rows {
		var item map[string]any
		if err := json.Unmarshal([]byte(row.Text), &item); err != nil {
			return nil, fmt.Errorf("parse guide topic %q: %w", row.Key, err)
		}
		// Ensure the key field in the stored JSON matches the row key.
		item["key"] = row.Key
		topics[row.Key] = item
	}
	return map[string]any{"topics": topics}, nil
}

// Create inserts a new guide topic row.
func (s *GuideTopicsService) Create(ctx context.Context, in GuideTopicInput) (*model.GuideTopicRow, error) {
	key, text, err := buildGuideTopicRow(in)
	if err != nil {
		return nil, err
	}
	exists, err := s.repo.KeyExists(ctx, key)
	if err != nil {
		return nil, err
	}
	if exists {
		return nil, fmt.Errorf("conflict:guide topic key already exists: %s", key)
	}
	return s.repo.Create(ctx, key, text)
}

// Update replaces an existing guide topic row.
func (s *GuideTopicsService) Update(ctx context.Context, id int64, in GuideTopicInput) (*model.GuideTopicRow, error) {
	row, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if row == nil {
		return nil, nil
	}
	key, text, err := buildGuideTopicRow(in)
	if err != nil {
		return nil, err
	}
	conflict, err := s.repo.KeyExistsExcluding(ctx, key, id)
	if err != nil {
		return nil, err
	}
	if conflict {
		return nil, fmt.Errorf("conflict:guide topic key already exists: %s", key)
	}
	return s.repo.Update(ctx, id, key, text)
}

// Delete removes a guide topic row.
func (s *GuideTopicsService) Delete(ctx context.Context, id int64) error {
	_, err := s.repo.Delete(ctx, id)
	return err
}

// ExportDocument returns the guide_topics.json interchange shape.
func (s *GuideTopicsService) ExportDocument(ctx context.Context) (map[string]any, error) {
	rows, err := s.repo.ListAll(ctx)
	if err != nil {
		return nil, err
	}
	topics := make([]any, 0, len(rows))
	for _, row := range rows {
		var item map[string]any
		if err := json.Unmarshal([]byte(row.Text), &item); err != nil {
			return nil, fmt.Errorf("parse guide topic %q: %w", row.Key, err)
		}
		item["key"] = row.Key
		topics = append(topics, item)
	}
	return map[string]any{"topics": topics}, nil
}

// GuideTopicImportConflict describes an uploaded key that already exists in the database.
type GuideTopicImportConflict struct {
	Key      string         `json:"key"`
	Existing map[string]any `json:"existing"`
	Uploaded map[string]any `json:"uploaded"`
}

// GuideTopicImportPreviewResult is returned by import preview.
type GuideTopicImportPreviewResult struct {
	New       []map[string]any           `json:"new"`
	Conflicts []GuideTopicImportConflict `json:"conflicts"`
}

// ImportPreview parses uploaded JSON and compares keys with the database.
func (s *GuideTopicsService) ImportPreview(ctx context.Context, data []byte) (*GuideTopicImportPreviewResult, error) {
	items, err := parseGuideTopicsImport(data)
	if err != nil {
		return nil, err
	}
	result := &GuideTopicImportPreviewResult{
		New:       []map[string]any{},
		Conflicts: []GuideTopicImportConflict{},
	}
	for _, item := range items {
		key, _ := item["key"].(string)
		if key == "" {
			continue
		}
		row, err := s.repo.GetByKey(ctx, key)
		if err != nil {
			return nil, err
		}
		if row == nil {
			result.New = append(result.New, item)
			continue
		}
		var existing map[string]any
		if err := json.Unmarshal([]byte(row.Text), &existing); err != nil {
			return nil, fmt.Errorf("parse existing guide topic %q: %w", key, err)
		}
		result.Conflicts = append(result.Conflicts, GuideTopicImportConflict{
			Key:      key,
			Existing: existing,
			Uploaded: item,
		})
	}
	return result, nil
}

// ImportApply inserts new keys and applies per-key conflict resolutions.
func (s *GuideTopicsService) ImportApply(ctx context.Context, data []byte, resolutions map[string]string) error {
	items, err := parseGuideTopicsImport(data)
	if err != nil {
		return err
	}
	for _, item := range items {
		key, _ := item["key"].(string)
		if key == "" {
			continue
		}
		text, err := json.Marshal(item)
		if err != nil {
			return err
		}
		row, err := s.repo.GetByKey(ctx, key)
		if err != nil {
			return err
		}
		if row == nil {
			if _, err := s.repo.Create(ctx, key, string(text)); err != nil {
				return err
			}
			continue
		}
		action := strings.ToLower(strings.TrimSpace(resolutions[key]))
		if action == "" {
			action = "keep"
		}
		if action == "replace" {
			if _, err := s.repo.Update(ctx, row.ID, key, string(text)); err != nil {
				return err
			}
		}
	}
	return nil
}

// ParseGuideTopicRow decodes a stored row into a map for admin responses.
func ParseGuideTopicRow(row *model.GuideTopicRow) (map[string]any, error) {
	item := map[string]any{}
	if err := json.Unmarshal([]byte(row.Text), &item); err != nil {
		return nil, err
	}
	item["id"] = row.ID
	item["key"] = row.Key
	return item, nil
}

func buildGuideTopicRow(in GuideTopicInput) (string, string, error) {
	key := strings.TrimSpace(in.Key)
	if key == "" {
		return "", "", fmt.Errorf("key is required")
	}
	if strings.TrimSpace(in.Title) == "" {
		return "", "", fmt.Errorf("title is required")
	}
	if strings.TrimSpace(in.Category) == "" {
		return "", "", fmt.Errorf("category is required")
	}

	item := map[string]any{
		"key":      key,
		"title":    strings.TrimSpace(in.Title),
		"category": strings.TrimSpace(in.Category),
	}
	if d := strings.TrimSpace(in.Description); d != "" {
		item["description"] = d
	}
	if in.Recommended {
		item["recommended"] = true
	}
	if len(in.Steps) > 0 {
		item["steps"] = in.Steps
	} else {
		item["steps"] = []map[string]any{}
	}

	text, err := json.Marshal(item)
	if err != nil {
		return "", "", err
	}
	return key, string(text), nil
}

func parseGuideTopicsImport(data []byte) ([]map[string]any, error) {
	var root map[string]any
	if err := json.Unmarshal(data, &root); err != nil {
		return nil, fmt.Errorf("parse import JSON: %w", err)
	}
	rawTopics, ok := root["topics"].([]any)
	if !ok || len(rawTopics) == 0 {
		return nil, fmt.Errorf("topics must not be empty")
	}
	var out []map[string]any
	for _, rt := range rawTopics {
		item, ok := rt.(map[string]any)
		if !ok {
			continue
		}
		key, _ := item["key"].(string)
		if strings.TrimSpace(key) == "" {
			continue
		}
		out = append(out, item)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("no valid guide topics found in import")
	}
	return out, nil
}
