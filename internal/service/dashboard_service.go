package service

import (
	"context"
	"fmt"
	"maps"
	"math"
	"strings"
	"sync"
	"time"

	"github.com/daveontour/aimuseum/internal/appctx"
	"github.com/daveontour/aimuseum/internal/model"
	"github.com/daveontour/aimuseum/internal/repository"
)

const importModalCacheTTL = 30 * time.Second

type importModalStatsCacheEntry struct {
	value     *model.ImportModalStatsResponse
	expiresAt time.Time
}

type embeddingProgressCacheEntry struct {
	value     model.EmbeddingProgressEntry
	expiresAt time.Time
}

// DashboardService assembles the GET /api/dashboard response.
type DashboardService struct {
	repo             *repository.DashboardRepo
	subjectRepo      *repository.SubjectConfigRepo
	importStatsCache sync.Map
	progressCache    sync.Map
}

// NewDashboardService creates a DashboardService.
func NewDashboardService(repo *repository.DashboardRepo, subjectRepo *repository.SubjectConfigRepo) *DashboardService {
	return &DashboardService{repo: repo, subjectRepo: subjectRepo}
}

// GetDashboard returns the fully-assembled dashboard response.
func (s *DashboardService) GetDashboard(ctx context.Context) (*model.DashboardResponse, error) {
	// ── Fetch subject configuration ─────────────────────────────────────────
	cfg, err := s.subjectRepo.GetFirst(ctx)
	if err != nil {
		return nil, err
	}

	// Build the set of subject names to exclude from the messages-by-contact list.
	subjectNamesLower := make(map[string]struct{})
	if cfg != nil {
		if n := strings.TrimSpace(cfg.SubjectName); n != "" {
			subjectNamesLower[strings.ToLower(n)] = struct{}{}
		}
		if cfg.FamilyName != nil {
			full := strings.TrimSpace(cfg.SubjectName + " " + *cfg.FamilyName)
			if full != "" {
				subjectNamesLower[strings.ToLower(full)] = struct{}{}
			}
		}
	}

	contactNames, err := s.repo.GetSubjectContactNames(ctx)
	if err != nil {
		return nil, err
	}
	for _, n := range contactNames {
		if t := strings.TrimSpace(n); t != "" {
			subjectNamesLower[strings.ToLower(t)] = struct{}{}
		}
	}

	// ── Fetch raw stats ─────────────────────────────────────────────────────
	raw, err := s.repo.GetStats(ctx)
	if err != nil {
		return nil, err
	}

	// ── Filter top senders ──────────────────────────────────────────────────
	var messagesByContact []model.ContactCount
	for _, cc := range raw.TopSenders {
		if _, excluded := subjectNamesLower[strings.ToLower(strings.TrimSpace(cc.Name))]; excluded {
			continue
		}
		messagesByContact = append(messagesByContact, model.ContactCount{
			Name:  strings.TrimSpace(cc.Name),
			Count: cc.Count,
		})
		if len(messagesByContact) >= 20 {
			break
		}
	}
	if messagesByContact == nil {
		messagesByContact = []model.ContactCount{}
	}

	// ── Thumbnail percentage ────────────────────────────────────────────────
	var thumbPct float64
	if raw.TotalImages > 0 {
		thumbPct = math.Round(1000.0*float64(raw.ThumbnailCount)/float64(raw.TotalImages)) / 10.0
	}

	// ── Total messages ──────────────────────────────────────────────────────
	var totalMessages int64
	for _, c := range raw.MessageCounts {
		totalMessages += c
	}

	// ── Subject full name and complete profile check ────────────────────────
	subjectFullName := "Subject"
	var subjectHasCompleteProfile bool

	if cfg != nil {
		fn := ""
		if cfg.FamilyName != nil {
			fn = *cfg.FamilyName
		}
		full := strings.TrimSpace(cfg.SubjectName + " " + fn)
		if full == "" {
			full = strings.TrimSpace(cfg.SubjectName)
		}
		if full == "" {
			full = "Subject"
		}
		subjectFullName = full

		// Build name set for complete_profile lookup
		checkNames := make([]string, 0, len(subjectNamesLower)+1)
		checkNames = append(checkNames, strings.ToLower(subjectFullName))
		checkNames = append(checkNames, strings.ToLower(cfg.SubjectName))
		for n := range subjectNamesLower {
			checkNames = append(checkNames, n)
		}
		// Deduplicate
		seen := make(map[string]struct{})
		unique := checkNames[:0]
		for _, n := range checkNames {
			if n == "" {
				continue
			}
			if _, ok := seen[n]; !ok {
				seen[n] = struct{}{}
				unique = append(unique, n)
			}
		}

		if len(unique) > 0 {
			subjectHasCompleteProfile, err = s.repo.HasCompleteProfileForNames(ctx, unique)
			if err != nil {
				return nil, err
			}
		}
	}

	emailsBySource := raw.EmailsBySource
	if emailsBySource == nil {
		emailsBySource = map[string]int64{}
	}

	return &model.DashboardResponse{
		MessageCounts:                   raw.MessageCounts,
		TotalMessages:                   totalMessages,
		MessagesByYear:                  raw.MessagesByYear,
		EmailsByYear:                    raw.EmailsByYear,
		MessagesByContact:               messagesByContact,
		ContactsCount:                   raw.ContactsCount,
		ContactsByCategory:              raw.ContactsByCategory,
		TotalImages:                     raw.TotalImages,
		FilesystemImagesCount:           raw.FilesystemImagesCount,
		FilesystemImagesEmbeddedCount:   raw.FilesystemImagesEmbeddedCount,
		FilesystemImagesReferencedCount: raw.FilesystemImagesReferencedCount,
		ImportedImages:                  raw.ImportedImages,
		ReferenceImages:                 raw.ReferenceImages,
		ImagesByRegion:                  raw.ImagesByRegion,
		ThumbnailCount:                  raw.ThumbnailCount,
		ThumbnailPercentage:             thumbPct,
		FacebookAlbumsCount:             raw.FacebookAlbumsCount,
		FacebookPostsCount:              raw.FacebookPostsCount,
		LocationsCount:                  raw.LocationsCount,
		PlacesCount:                     raw.PlacesCount,
		EmailsCount:                     raw.EmailsCount,
		EmailsBySource:                  emailsBySource,
		ArtefactsCount:                  raw.ArtefactsCount,
		ReferenceDocsCount:              raw.ReferenceDocsCount,
		ReferenceDocsEnabled:            raw.ReferenceDocsEnabled,
		ReferenceDocsDisabled:           raw.ReferenceDocsCount - raw.ReferenceDocsEnabled,
		CompleteProfilesCount:           raw.CompleteProfilesCount,
		SubjectFullName:                 subjectFullName,
		SubjectHasCompleteProfile:       subjectHasCompleteProfile,
	}, nil
}

// GetImportModalStats returns aggregate counts for the import/maintenance modals.
// Embedding/searchable progress is not included — see GetEmbeddingProgressForSource.
// Results are cached per user for a short window so reopening the dialog does not
// repeat the scans.
func (s *DashboardService) GetImportModalStats(ctx context.Context) (*model.ImportModalStatsResponse, error) {
	key := appctx.UserIDFromCtx(ctx)
	now := time.Now()
	if v, ok := s.importStatsCache.Load(key); ok {
		if entry, ok := v.(importModalStatsCacheEntry); ok && entry.value != nil && now.Before(entry.expiresAt) {
			return cloneImportModalStats(entry.value), nil
		}
	}
	raw, err := s.repo.GetImportModalStats(ctx)
	if err != nil {
		return nil, err
	}
	if raw.EmailsBySource == nil {
		raw.EmailsBySource = map[string]int64{}
	}
	if raw.MessageCounts == nil {
		raw.MessageCounts = map[string]int64{}
	}
	if raw.EmbeddingProgress == nil {
		raw.EmbeddingProgress = map[string]model.EmbeddingProgressEntry{}
	}
	stored := cloneImportModalStats(raw)
	s.importStatsCache.Store(key, importModalStatsCacheEntry{
		value:     stored,
		expiresAt: now.Add(importModalCacheTTL),
	})
	return cloneImportModalStats(stored), nil
}

func cloneImportModalStats(in *model.ImportModalStatsResponse) *model.ImportModalStatsResponse {
	if in == nil {
		return nil
	}
	out := *in
	out.MessageCounts = maps.Clone(in.MessageCounts)
	out.EmailsBySource = maps.Clone(in.EmailsBySource)
	out.EmbeddingProgress = maps.Clone(in.EmbeddingProgress)
	return &out
}

// GetEmbeddingProgressForSource returns how much content of one source still needs an
// AI embedding to be searchable. Callers fetch each source independently so the modal
// can fill progress per source as each one resolves. Results are cached per user and
// source for a short window.
func (s *DashboardService) GetEmbeddingProgressForSource(ctx context.Context, key string) (model.EmbeddingProgressEntry, error) {
	cacheKey := fmt.Sprintf("%d|%s", appctx.UserIDFromCtx(ctx), key)
	now := time.Now()
	if v, ok := s.progressCache.Load(cacheKey); ok {
		if entry, ok := v.(embeddingProgressCacheEntry); ok && now.Before(entry.expiresAt) {
			return entry.value, nil
		}
	}
	entry, err := s.repo.GetEmbeddingProgressForSource(ctx, key)
	if err != nil {
		return model.EmbeddingProgressEntry{}, err
	}
	s.progressCache.Store(cacheKey, embeddingProgressCacheEntry{
		value:     entry,
		expiresAt: now.Add(importModalCacheTTL),
	})
	return entry, nil
}

// GetArchiveDataInventory returns entry counts per data type for conversational AI prompts.
func (s *DashboardService) GetArchiveDataInventory(ctx context.Context) (*model.ArchiveDataInventory, error) {
	raw, err := s.repo.GetArchiveDataInventory(ctx)
	if err != nil {
		return nil, err
	}
	if raw.MessagesByService == nil {
		raw.MessagesByService = map[string]int64{}
	}
	if raw.EmailsBySource == nil {
		raw.EmailsBySource = map[string]int64{}
	}
	return raw, nil
}
