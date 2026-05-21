package service

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"log/slog"
	"sort"
	"strings"
	"unicode/utf8"

	sqlite_vec "github.com/asg017/sqlite-vec-go-bindings/cgo"
	"github.com/daveontour/aimuseum/internal/repository"
	"github.com/daveontour/aimuseum/internal/sqlutil"
)

const mediaTagEmbeddingsVecTable = "media_tag_embeddings"

// tagSearchNoiseWords drops common articles, prepositions, pronouns, and coordinating
// conjunctions from keyword tokens so queries like "sunset at the beach" match tag text
// without brittle matches on filler words.
var tagSearchNoiseWords = map[string]struct{}{
	"a": {}, "an": {}, "the": {}, "some": {}, "any": {}, "every": {}, "each": {}, "either": {}, "neither": {},
	"at": {}, "by": {}, "for": {}, "from": {}, "in": {}, "into": {}, "of": {}, "on": {}, "onto": {}, "off": {},
	"over": {}, "to": {}, "toward": {}, "towards": {}, "under": {}, "upon": {}, "up": {}, "down": {},
	"with": {}, "within": {}, "without": {}, "via": {}, "per": {}, "near": {}, "past": {}, "about": {},
	"across": {}, "against": {}, "along": {}, "among": {}, "around": {}, "before": {}, "behind": {}, "below": {},
	"beneath": {}, "beside": {}, "besides": {}, "between": {}, "beyond": {}, "during": {}, "except": {}, "inside": {},
	"outside": {}, "until": {}, "since": {}, "through": {}, "throughout": {}, "underneath": {},
	"and": {}, "but": {}, "nor": {}, "or": {}, "yet": {}, "so": {},
	"as": {}, "if": {}, "than": {}, "then": {}, "once": {},
	"that": {}, "this": {}, "these": {}, "those": {},
	"it": {}, "its": {}, "is": {}, "am": {}, "are": {}, "was": {}, "were": {}, "be": {}, "been": {}, "being": {},
	"have": {}, "has": {}, "had": {}, "do": {}, "does": {}, "did": {},
	"will": {}, "would": {}, "shall": {}, "should": {}, "may": {}, "might": {}, "must": {}, "can": {}, "could": {},
	"not": {},
}

// NormalizeTagsForEmbedding splits comma-separated tags, trims, lowercases, dedupes,
// sorts alphabetically, and rejoins with ", " for stable embedding input.
func NormalizeTagsForEmbedding(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	parts := strings.Split(raw, ",")
	seen := make(map[string]struct{})
	for _, p := range parts {
		t := strings.ToLower(strings.TrimSpace(p))
		if t != "" {
			seen[t] = struct{}{}
		}
	}
	if len(seen) == 0 {
		return ""
	}
	out := make([]string, 0, len(seen))
	for t := range seen {
		out = append(out, t)
	}
	sort.Strings(out)
	return strings.Join(out, ", ")
}

// KeywordsForTagSearch returns distinct lowercase tokens for substring-matching the tags column:
// comma-separated phrases from NormalizeTagsForEmbedding(raw), plus each whitespace-separated
// word from raw (commas treated as separators). Tokens shorter than 2 runes are skipped.
// Noise words (articles, prepositions, etc.) apply only to whitespace tokens from raw text;
// each comma-normalized segment is kept as typed (e.g. tags "and", "or") so OR-based tag
// search still runs. Multi-word normalized phrases are stored verbatim.
func KeywordsForTagSearch(raw string) []string {
	norm := NormalizeTagsForEmbedding(raw)
	seen := make(map[string]struct{})
	out := make([]string, 0)
	addUnique := func(s string) {
		if _, ok := seen[s]; ok {
			return
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	addNormSegment := func(s string) {
		s = strings.TrimSpace(s)
		if utf8.RuneCountInString(s) < 2 {
			return
		}
		addUnique(strings.ToLower(s))
	}
	addLooseWord := func(s string) {
		s = strings.TrimSpace(s)
		if utf8.RuneCountInString(s) < 2 {
			return
		}
		s = strings.ToLower(s)
		if _, noise := tagSearchNoiseWords[s]; noise {
			return
		}
		addUnique(s)
	}
	if norm != "" {
		for _, p := range strings.Split(norm, ", ") {
			addNormSegment(p)
		}
	}
	relaxed := strings.ReplaceAll(strings.TrimSpace(raw), ",", " ")
	for _, f := range strings.Fields(relaxed) {
		addLooseWord(f)
	}
	return out
}

// TagEmbeddingSignature is a stable short fingerprint of normalized tag text stored in vec int_ids for skip-on-backfill.
func TagEmbeddingSignature(norm string) string {
	sum := sha256.Sum256([]byte(norm))
	return hex.EncodeToString(sum[:])
}

// MediaTagEmbeddingHelper upserts or deletes sqlite-vec rows for media_items.tags (local AI embeddings only).
type MediaTagEmbeddingHelper struct {
	pool *sql.DB
	repo *repository.ImageRepo
	svc  *EmbeddingService
}

// NewMediaTagEmbeddingHelper creates a helper; svc may be nil to disable sync.
func NewMediaTagEmbeddingHelper(pool *sql.DB, repo *repository.ImageRepo, svc *EmbeddingService) *MediaTagEmbeddingHelper {
	if pool == nil || repo == nil {
		return nil
	}
	return &MediaTagEmbeddingHelper{pool: pool, repo: repo, svc: svc}
}

// Sync refreshes or removes the vec row for one media item from current DB tags.
func (h *MediaTagEmbeddingHelper) Sync(ctx context.Context, mediaItemID int64) {
	if h == nil || h.pool == nil || h.repo == nil {
		return
	}
	if h.svc == nil || !h.svc.IsAvailable() {
		return
	}
	item, err := h.repo.GetMediaItemByID(ctx, mediaItemID)
	if err != nil || item == nil {
		return
	}
	var tagStr string
	if item.Tags != nil {
		tagStr = *item.Tags
	}
	norm := NormalizeTagsForEmbedding(tagStr)
	if norm == "" {
		if _, delErr := h.pool.ExecContext(ctx, `DELETE FROM `+mediaTagEmbeddingsVecTable+` WHERE rowid = ?`, mediaItemID); delErr != nil {
			slog.Warn("media tag embedding delete", "media_item_id", mediaItemID, "err", delErr)
		}
		return
	}
	vec, err := h.svc.EmbedText(ctx, norm)
	if err != nil {
		slog.Warn("media tag embedding embed", "media_item_id", mediaItemID, "err", err)
		return
	}
	blob, err := sqlite_vec.SerializeFloat32(vec)
	if err != nil {
		slog.Warn("media tag embedding serialize", "media_item_id", mediaItemID, "err", err)
		return
	}
	sig := TagEmbeddingSignature(norm)
	if err := sqlutil.Vec0Upsert(ctx, h.pool, mediaTagEmbeddingsVecTable, mediaItemID, blob, sig); err != nil {
		slog.Warn("media tag embedding upsert", "media_item_id", mediaItemID, "err", err)
	}
}

// MediaTagEmbeddingSignatureMatches reports whether the vec row for mediaItemID was built for the same normalized tags.
func MediaTagEmbeddingSignatureMatches(ctx context.Context, pool *sql.DB, mediaItemID int64, norm string) (bool, error) {
	if pool == nil || norm == "" {
		return false, nil
	}
	want := TagEmbeddingSignature(norm)
	var got string
	err := pool.QueryRowContext(ctx,
		`SELECT int_ids FROM `+mediaTagEmbeddingsVecTable+` WHERE rowid = ?`, mediaItemID,
	).Scan(&got)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return got != "" && got == want, nil
}
