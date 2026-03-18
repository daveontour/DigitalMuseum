package service

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/daveontour/digitalmuseum/internal/model"
	"github.com/daveontour/digitalmuseum/internal/repository"
)

// VoiceService manages built-in and custom voices.
type VoiceService struct {
	voiceRepo       *repository.VoiceRepo
	subjectRepo     *repository.SubjectConfigRepo
	pythonStaticDir string
}

// NewVoiceService creates a VoiceService.
func NewVoiceService(voiceRepo *repository.VoiceRepo, subjectRepo *repository.SubjectConfigRepo, pythonStaticDir string) *VoiceService {
	return &VoiceService{
		voiceRepo:       voiceRepo,
		subjectRepo:     subjectRepo,
		pythonStaticDir: pythonStaticDir,
	}
}

// ListAll returns built-in voices (from JSON) merged with custom DB voices.
func (s *VoiceService) ListAll(ctx context.Context) ([]map[string]any, error) {
	// Load subject name for {SUBJECT_NAME} substitution
	subjectName := "Unknown"
	if cfg, _ := s.subjectRepo.GetFirst(ctx); cfg != nil {
		subjectName = cfg.SubjectName
	}

	// Load built-in voices from JSON file
	path := fmt.Sprintf("%s/data/voice_instructions.json", s.pythonStaticDir)
	data, err := os.ReadFile(path)

	var result []map[string]any
	if err == nil {
		var raw map[string]any
		if json.Unmarshal(data, &raw) == nil {
			for key, val := range raw {
				vm, ok := val.(map[string]any)
				if !ok {
					continue
				}
				name := anyStr(vm["name"])
				if key == "owner" {
					name = strings.ReplaceAll(name, "{SUBJECT_NAME}", subjectName)
				}
				result = append(result, map[string]any{
					"key":          key,
					"name":         name,
					"description":  anyStr(vm["description"]),
					"instructions": anyStr(vm["instructions"]),
					"creativity":   vm["creativity"],
					"is_custom":    false,
					"id":           nil,
				})
			}
		}
	}

	// Append custom voices from DB
	customs, err := s.voiceRepo.List(ctx)
	if err != nil {
		return nil, err
	}
	for _, v := range customs {
		desc := ""
		if v.Description != nil {
			desc = *v.Description
		}
		result = append(result, map[string]any{
			"id":           v.ID,
			"key":          v.Key,
			"name":         v.Name,
			"description":  desc,
			"instructions": v.Instructions,
			"creativity":   v.Creativity,
			"is_custom":    true,
		})
	}
	return result, nil
}

// ListCustom returns only the DB custom voices.
func (s *VoiceService) ListCustom(ctx context.Context) ([]*model.CustomVoice, error) {
	return s.voiceRepo.List(ctx)
}

// Create adds a new custom voice.
func (s *VoiceService) Create(ctx context.Context, name string, description *string, instructions string, creativity float64) (*model.CustomVoice, error) {
	key := slugify(name)

	// Check for key collision with built-in voices
	if s.builtinKeyExists(key) {
		return nil, fmt.Errorf("conflict:key '%s' conflicts with a built-in voice. Choose a different name", key)
	}

	// Check DB uniqueness
	existing, err := s.voiceRepo.GetByKey(ctx, key)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		return nil, fmt.Errorf("conflict:a custom voice with key '%s' already exists", key)
	}

	return s.voiceRepo.Create(ctx, key, name, description, instructions, creativity)
}

// Update modifies a custom voice.
func (s *VoiceService) Update(ctx context.Context, id int64, name *string, description *string, instructions *string, creativity *float64) (*model.CustomVoice, error) {
	var newKey *string
	if name != nil {
		k := slugify(*name)
		// Check built-in conflict
		if s.builtinKeyExists(k) {
			return nil, fmt.Errorf("conflict:key '%s' conflicts with a built-in voice", k)
		}
		// Check DB uniqueness (excluding self)
		conflict, err := s.voiceRepo.KeyExistsExcluding(ctx, k, id)
		if err != nil {
			return nil, err
		}
		if conflict {
			return nil, fmt.Errorf("conflict:key '%s' already used by another custom voice", k)
		}
		newKey = &k
	}
	return s.voiceRepo.Update(ctx, id, newKey, name, description, instructions, creativity)
}

// GetByID returns a custom voice by ID.
func (s *VoiceService) GetByID(ctx context.Context, id int64) (*model.CustomVoice, error) {
	return s.voiceRepo.GetByID(ctx, id)
}

// Delete removes a custom voice.
func (s *VoiceService) Delete(ctx context.Context, id int64) error {
	return s.voiceRepo.Delete(ctx, id)
}

// ── helpers ────────────────────────────────────────────────────────────────────

var nonAlnum = regexp.MustCompile(`[^a-z0-9]+`)

func slugify(name string) string {
	slug := strings.ToLower(strings.TrimSpace(name))
	slug = nonAlnum.ReplaceAllString(slug, "_")
	slug = strings.Trim(slug, "_")
	return "custom_" + slug
}

func (s *VoiceService) builtinKeyExists(key string) bool {
	path := fmt.Sprintf("%s/data/voice_instructions.json", s.pythonStaticDir)
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		return false
	}
	_, exists := raw[key]
	return exists
}
