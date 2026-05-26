package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/daveontour/aimuseum/internal/model"
	"github.com/daveontour/aimuseum/internal/repository"
	"github.com/daveontour/aimuseum/internal/suggestions"
)

// SuggestionInput is the editable fields for one sidebar suggestion entry.
type SuggestionInput struct {
	Category    string   `json:"category"`
	Title       string   `json:"title"`
	Prompt      string   `json:"prompt"`
	Description string   `json:"description,omitempty"`
	ActionLabel string   `json:"action_label,omitempty"`
	Requires    []string `json:"requires,omitempty"`
	Sensitivity string   `json:"sensitivity,omitempty"`
	Function    string   `json:"function,omitempty"`
}

// SuggestionsService manages deployment-wide suggestion configuration rows.
type SuggestionsService struct {
	repo *repository.SuggestionsRepo
}

// NewSuggestionsService creates a SuggestionsService.
func NewSuggestionsService(repo *repository.SuggestionsRepo) *SuggestionsService {
	return &SuggestionsService{repo: repo}
}

// ListAll returns all rows for the Configuration UI.
func (s *SuggestionsService) ListAll(ctx context.Context) ([]*model.SuggestionRow, error) {
	rows, err := s.repo.ListAll(ctx)
	if err != nil {
		return nil, err
	}
	if rows == nil {
		return []*model.SuggestionRow{}, nil
	}
	return rows, nil
}

// GetByID returns one row.
func (s *SuggestionsService) GetByID(ctx context.Context, id int64) (*model.SuggestionRow, error) {
	return s.repo.GetByID(ctx, id)
}

// BuildCategoriesDocument assembles the categories document from DB rows.
func (s *SuggestionsService) BuildCategoriesDocument(ctx context.Context) (map[string]any, error) {
	rows, err := s.repo.ListAll(ctx)
	if err != nil {
		return nil, err
	}
	catOrder := []string{}
	byCat := map[string][]any{}
	for _, row := range rows {
		category, _, err := suggestions.SplitKey(row.Key)
		if err != nil {
			return nil, fmt.Errorf("row id=%d: %w", row.ID, err)
		}
		var item map[string]any
		if err := json.Unmarshal([]byte(row.Text), &item); err != nil {
			return nil, fmt.Errorf("parse suggestion %q: %w", row.Key, err)
		}
		if _, ok := byCat[category]; !ok {
			catOrder = append(catOrder, category)
			byCat[category] = []any{}
		}
		byCat[category] = append(byCat[category], item)
	}
	categories := make([]any, 0, len(catOrder))
	for _, name := range catOrder {
		categories = append(categories, map[string]any{
			"category":    name,
			"suggestions": byCat[name],
		})
	}
	return map[string]any{"categories": categories}, nil
}

// Create inserts a new suggestion row.
func (s *SuggestionsService) Create(ctx context.Context, in SuggestionInput) (*model.SuggestionRow, error) {
	key, text, err := buildSuggestionRow(in)
	if err != nil {
		return nil, err
	}
	exists, err := s.repo.KeyExists(ctx, key)
	if err != nil {
		return nil, err
	}
	if exists {
		return nil, fmt.Errorf("conflict:suggestion key already exists: %s", key)
	}
	return s.repo.Create(ctx, key, text)
}

// Update replaces an existing suggestion row.
func (s *SuggestionsService) Update(ctx context.Context, id int64, in SuggestionInput) (*model.SuggestionRow, error) {
	row, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if row == nil {
		return nil, nil
	}
	key, text, err := buildSuggestionRow(in)
	if err != nil {
		return nil, err
	}
	conflict, err := s.repo.KeyExistsExcluding(ctx, key, id)
	if err != nil {
		return nil, err
	}
	if conflict {
		return nil, fmt.Errorf("conflict:suggestion key already exists: %s", key)
	}
	return s.repo.Update(ctx, id, key, text)
}

// Delete removes a suggestion row.
func (s *SuggestionsService) Delete(ctx context.Context, id int64) error {
	ok, err := s.repo.Delete(ctx, id)
	if err != nil {
		return err
	}
	if !ok {
		return nil
	}
	return nil
}

// ExportDocument returns the suggestions.json interchange shape.
func (s *SuggestionsService) ExportDocument(ctx context.Context) (map[string]any, error) {
	return s.BuildCategoriesDocument(ctx)
}

// SuggestionImportConflict describes an uploaded key that already exists in the database.
type SuggestionImportConflict struct {
	Key      string         `json:"key"`
	Existing map[string]any `json:"existing"`
	Uploaded map[string]any `json:"uploaded"`
}

// SuggestionImportPreviewResult is returned by import preview.
type SuggestionImportPreviewResult struct {
	New       []SuggestionImportPreviewItem `json:"new"`
	Conflicts []SuggestionImportConflict    `json:"conflicts"`
}

// SuggestionImportPreviewItem is a new suggestion from an upload.
type SuggestionImportPreviewItem struct {
	Key        string         `json:"key"`
	Category   string         `json:"category"`
	Suggestion map[string]any `json:"suggestion"`
}

// ImportPreview parses uploaded JSON and compares keys with the database.
func (s *SuggestionsService) ImportPreview(ctx context.Context, data []byte) (*SuggestionImportPreviewResult, error) {
	cats, err := parseSuggestionsImport(data)
	if err != nil {
		return nil, err
	}
	result := &SuggestionImportPreviewResult{
		New:       []SuggestionImportPreviewItem{},
		Conflicts: []SuggestionImportConflict{},
	}
	for category, items := range cats {
		for _, item := range items {
			title, _ := item["title"].(string)
			key, err := suggestions.BuildKey(category, title)
			if err != nil {
				continue
			}
			row, err := s.repo.GetByKey(ctx, key)
			if err != nil {
				return nil, err
			}
			if row == nil {
				result.New = append(result.New, SuggestionImportPreviewItem{
					Key:        key,
					Category:   category,
					Suggestion: item,
				})
				continue
			}
			var existing map[string]any
			if err := json.Unmarshal([]byte(row.Text), &existing); err != nil {
				return nil, fmt.Errorf("parse existing suggestion %q: %w", key, err)
			}
			result.Conflicts = append(result.Conflicts, SuggestionImportConflict{
				Key:      key,
				Existing: existing,
				Uploaded: item,
			})
		}
	}
	return result, nil
}

// ImportApply inserts new keys and applies per-key conflict resolutions.
func (s *SuggestionsService) ImportApply(ctx context.Context, data []byte, resolutions map[string]string) error {
	cats, err := parseSuggestionsImport(data)
	if err != nil {
		return err
	}
	for category, items := range cats {
		for _, item := range items {
			title, _ := item["title"].(string)
			key, err := suggestions.BuildKey(category, title)
			if err != nil {
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
	}
	return nil
}

func buildSuggestionRow(in SuggestionInput) (string, string, error) {
	in.Category = strings.TrimSpace(in.Category)
	in.Title = strings.TrimSpace(in.Title)
	in.Prompt = strings.TrimSpace(in.Prompt)
	if in.Category == "" {
		return "", "", fmt.Errorf("category is required")
	}
	if in.Title == "" {
		return "", "", fmt.Errorf("title is required")
	}
	if in.Prompt == "" && strings.TrimSpace(in.Function) == "" {
		return "", "", fmt.Errorf("prompt or function is required")
	}
	key, err := suggestions.BuildKey(in.Category, in.Title)
	if err != nil {
		return "", "", err
	}
	item := map[string]any{"title": in.Title}
	if in.Prompt != "" {
		item["prompt"] = in.Prompt
	}
	if d := strings.TrimSpace(in.Description); d != "" {
		item["description"] = d
	}
	if a := strings.TrimSpace(in.ActionLabel); a != "" {
		item["action_label"] = a
	}
	if len(in.Requires) > 0 {
		item["requires"] = in.Requires
	}
	if sens := strings.TrimSpace(in.Sensitivity); sens != "" {
		item["sensitivity"] = sens
	}
	if fn := strings.TrimSpace(in.Function); fn != "" {
		item["function"] = fn
	}
	text, err := json.Marshal(item)
	if err != nil {
		return "", "", err
	}
	return key, string(text), nil
}

func parseSuggestionsImport(data []byte) (map[string][]map[string]any, error) {
	var root map[string]any
	if err := json.Unmarshal(data, &root); err != nil {
		return nil, fmt.Errorf("parse import JSON: %w", err)
	}
	rawCats, ok := root["categories"].([]any)
	if !ok || len(rawCats) == 0 {
		return nil, fmt.Errorf("categories must not be empty")
	}
	out := map[string][]map[string]any{}
	for _, rc := range rawCats {
		cm, ok := rc.(map[string]any)
		if !ok {
			continue
		}
		category, _ := cm["category"].(string)
		category = strings.TrimSpace(category)
		if category == "" {
			continue
		}
		rawItems, ok := cm["suggestions"].([]any)
		if !ok {
			continue
		}
		for _, ri := range rawItems {
			item, ok := ri.(map[string]any)
			if !ok {
				continue
			}
			title, _ := item["title"].(string)
			if strings.TrimSpace(title) == "" {
				continue
			}
			out[category] = append(out[category], item)
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("no valid suggestions found in import")
	}
	return out, nil
}

// ParseSuggestionRow decodes a stored row for admin responses.
func ParseSuggestionRow(row *model.SuggestionRow) (category string, item map[string]any, err error) {
	category, _, err = suggestions.SplitKey(row.Key)
	if err != nil {
		return "", nil, err
	}
	item = map[string]any{}
	if err := json.Unmarshal([]byte(row.Text), &item); err != nil {
		return "", nil, err
	}
	return category, item, nil
}
