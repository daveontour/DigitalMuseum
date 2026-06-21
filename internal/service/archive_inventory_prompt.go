package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/daveontour/aimuseum/internal/appctx"
	"github.com/daveontour/aimuseum/internal/model"
)

const archiveInventoryCacheTTL = 60 * time.Second

type archiveInventoryCacheEntry struct {
	inventory *model.ArchiveDataInventory
	expiresAt time.Time
}

// archiveInventoryInstruction tells the model how to use the JSON inventory block.
const archiveInventoryInstruction = `Use the archive data inventory below to judge whether the archive has enough data to answer the user. If the relevant data type shows count 0 or is clearly insufficient for the question, say so honestly and suggest importing data via **Data Import** or running maintenance via **Data Maintenance** — do not invent archive content.`

// enrichChatSystemPrompt appends reference documents and archive data inventory to a conversational system prompt.
func (s *ChatService) enrichChatSystemPrompt(ctx context.Context, r *http.Request, base string) string {
	return s.enrichChatSystemPromptWithOptions(ctx, r, base, true, true)
}

// enrichChatSystemPromptWithOptions appends optional reference documents and user profile context.
// includeReferenceDocuments controls inlined reference_documents rows.
// includeUserProfile controls archive data inventory (and complements genReq psych/writing profile fields).
func (s *ChatService) enrichChatSystemPromptWithOptions(ctx context.Context, r *http.Request, base string, includeReferenceDocuments, includeUserProfile bool) string {
	if includeReferenceDocuments {
		base = s.appendInlinedReferenceDocumentsToSystemPrompt(ctx, r, base)
	}
	if includeUserProfile {
		base = s.appendArchiveDataInventoryToSystemPrompt(ctx, base)
	}
	return base
}

func (s *ChatService) appendArchiveDataInventoryToSystemPrompt(ctx context.Context, base string) string {
	if s.dashSvc == nil {
		return base
	}
	inv, err := s.cachedArchiveDataInventory(ctx)
	if err != nil {
		slog.Warn("archive data inventory: fetch failed", "err", err)
		return base
	}
	if inv == nil {
		return base
	}
	block, err := formatArchiveInventoryPromptBlock(inv)
	if err != nil {
		slog.Warn("archive data inventory: format failed", "err", err)
		return base
	}
	if block == "" {
		return base
	}
	return base + block
}

func formatArchiveInventoryPromptBlock(inv *model.ArchiveDataInventory) (string, error) {
	if inv == nil {
		return "", nil
	}
	data, err := json.MarshalIndent(inv, "", "  ")
	if err != nil {
		return "", err
	}
	var sb strings.Builder
	sb.WriteString("\n\n---\n\n**Archive data inventory (JSON):**\n")
	sb.WriteString(archiveInventoryInstruction)
	sb.WriteString("\n\n```json\n")
	sb.Write(data)
	sb.WriteString("\n```\n")
	return sb.String(), nil
}

func (s *ChatService) cachedArchiveDataInventory(ctx context.Context) (*model.ArchiveDataInventory, error) {
	uid := appctx.UserIDFromCtx(ctx)
	cacheKey := fmt.Sprintf("uid:%d", uid)

	if entry, ok := s.archiveInventoryCache.Load(cacheKey); ok {
		if cached, ok := entry.(archiveInventoryCacheEntry); ok && time.Now().Before(cached.expiresAt) {
			return cached.inventory, nil
		}
	}

	inv, err := s.dashSvc.GetArchiveDataInventory(ctx)
	if err != nil {
		return nil, err
	}
	s.archiveInventoryCache.Store(cacheKey, archiveInventoryCacheEntry{
		inventory: inv,
		expiresAt: time.Now().Add(archiveInventoryCacheTTL),
	})
	return inv, nil
}
