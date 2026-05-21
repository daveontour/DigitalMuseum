package service

import (
	"context"
	"log/slog"

	appai "github.com/daveontour/aimuseum/internal/ai"
	"github.com/daveontour/aimuseum/internal/appctx"
	"github.com/daveontour/aimuseum/internal/repository"
)

const maxLLMUsageErrorLen = 4000

// StubLLMUsage returns a zero-token usage row for recording failed calls when the provider returned no usage metadata.
func StubLLMUsage(provider, model string) *appai.LLMUsage {
	return &appai.LLMUsage{Provider: provider, Model: model}
}

// RecordLLMUsage inserts a billing row from provider usage; best-effort (logs on failure, never panics).
// callErr should be nil on success. On failure, pass the error; usage may include partial token counts or be from StubLLMUsage.
// If both usage and callErr are nil, the function returns without inserting.
// If usage is nil and callErr is non-nil, no row is inserted (callers should use StubLLMUsage).
// When users is non-nil and the context has a positive user_id, email and first/family name are copied from the main DB as a snapshot on this row.
func RecordLLMUsage(ctx context.Context, billing *repository.BillingRepo, users *repository.UserRepo, usage *appai.LLMUsage, callErr error) {
	if billing == nil {
		return
	}
	if usage == nil {
		if callErr == nil {
			return
		}
		slog.Warn("RecordLLMUsage: nil usage on error, skip billing row", "err", callErr)
		return
	}
	succeeded := callErr == nil
	var errMsg *string
	if !succeeded {
		s := callErr.Error()
		if len(s) > maxLLMUsageErrorLen {
			s = s[:maxLLMUsageErrorLen]
		}
		errMsg = &s
	}
	uid := appctx.UserIDFromCtx(ctx)
	var uidPtr *int64
	if uid > 0 {
		uidPtr = &uid
	}
	isVisitor := appctx.IsVisitorFromCtx(ctx)
	var modelPtr *string
	if usage.Model != "" {
		modelPtr = &usage.Model
	}
	var userEmail, userFirst, userFamily *string
	if uid > 0 && users != nil {
		if u, err := users.FindByID(ctx, uid); err == nil && u != nil {
			if u.Email != "" {
				s := u.Email
				userEmail = &s
			}
			if u.FirstName != "" {
				s := u.FirstName
				userFirst = &s
			}
			if u.FamilyName != "" {
				s := u.FamilyName
				userFamily = &s
			}
		}
	}
	if err := billing.InsertLLMUsage(ctx, usage.Provider, uidPtr, isVisitor, usage.InputTokens, usage.OutputTokens, modelPtr, userEmail, userFirst, userFamily, usage.UsedServerKey, succeeded, errMsg); err != nil {
		slog.Warn("llm billing insert failed", "err", err)
	}
}

// MarkUsageServerKey sets usage.UsedServerKey for callers that use a single server-configured provider (e.g. env Gemini on email/message/admin paths).
func MarkUsageServerKey(usage *appai.LLMUsage, fromServer bool) {
	if usage == nil {
		return
	}
	b := fromServer
	usage.UsedServerKey = &b
}
