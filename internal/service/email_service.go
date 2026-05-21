// Package service contains business logic that sits between handlers and repositories.
package service

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/daveontour/aimuseum/internal/ai"
	"github.com/daveontour/aimuseum/internal/model"
	"github.com/daveontour/aimuseum/internal/repository"
)

// EmailUpdateParams carries the optional flag fields for PUT /emails/{id}.
type EmailUpdateParams struct {
	IsPersonal  *bool
	IsBusiness  *bool
	IsImportant *bool
	UseByAI     *bool
}

// EmailService coordinates email read and write operations.
type EmailService struct {
	repo    *repository.EmailRepo
	gemini  *ai.GeminiProvider
	billing *repository.BillingRepo
	users   *repository.UserRepo
}

// NewEmailService creates an EmailService.
func NewEmailService(repo *repository.EmailRepo) *EmailService {
	return &EmailService{repo: repo}
}

// WithGemini attaches a Gemini provider for thread summarization.
func (s *EmailService) WithGemini(g *ai.GeminiProvider) {
	s.gemini = g
}

// WithBilling attaches the billing repo and optional user repo (for denormalized identity on billing rows).
func (s *EmailService) WithBilling(b *repository.BillingRepo, users *repository.UserRepo) {
	s.billing = b
	s.users = users
}

// Search returns email metadata matching the given optional filters.
func (s *EmailService) Search(ctx context.Context, p model.EmailSearchParams) ([]model.EmailMetadataResponse, error) {
	emails, err := s.repo.Search(ctx, p)
	if err != nil {
		return nil, err
	}
	return s.hydrateMetadata(ctx, emails)
}

// GetByLabels returns email metadata for emails whose folder matches any label.
func (s *EmailService) GetByLabels(ctx context.Context, labels []string) ([]model.EmailMetadataResponse, error) {
	emails, err := s.repo.GetByLabels(ctx, labels)
	if err != nil {
		return nil, err
	}
	return s.hydrateMetadata(ctx, emails)
}

// GetMetadata returns a single email's metadata, including attachment IDs.
// Returns nil, nil when the email does not exist (handler maps to 404).
func (s *EmailService) GetMetadata(ctx context.Context, id int64) (*model.EmailMetadataResponse, error) {
	email, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if email == nil {
		return nil, nil
	}

	attMap, err := s.repo.GetAttachmentIDsForEmails(ctx, []int64{id})
	if err != nil {
		return nil, fmt.Errorf("get attachment ids: %w", err)
	}

	resp := toMetadataResponse(email, attMap[id])
	return &resp, nil
}

// GetByID returns the raw email record without attachment hydration.
// Returns nil, nil when not found (handler maps to 404).
func (s *EmailService) GetByID(ctx context.Context, id int64) (*model.Email, error) {
	return s.repo.GetByID(ctx, id)
}

// Update modifies flag columns on an email.
// Returns false, nil when the email is not found.
func (s *EmailService) Update(ctx context.Context, id int64, p EmailUpdateParams) (bool, error) {
	return s.repo.Update(ctx, id, p.IsPersonal, p.IsBusiness, p.IsImportant, p.UseByAI)
}

// SoftDelete soft-deletes a single email.
// Returns false, nil when not found.
func (s *EmailService) SoftDelete(ctx context.Context, id int64) (bool, error) {
	return s.repo.SoftDelete(ctx, id)
}

// BulkSoftDelete soft-deletes multiple emails and returns the deleted count.
func (s *EmailService) BulkSoftDelete(ctx context.Context, ids []int64) (int64, error) {
	return s.repo.BulkSoftDelete(ctx, ids)
}

// GetFolders returns the distinct folder/label names from stored emails.
func (s *EmailService) GetFolders(ctx context.Context) ([]string, error) {
	return s.repo.ListFolders(ctx)
}

// emailThreadTranscript builds the same text block used for thread summarization (chronological emails).
func (s *EmailService) emailThreadTranscript(ctx context.Context, participant string) (string, int, error) {
	emails, err := s.repo.GetThreadEmails(ctx, participant)
	if err != nil {
		return "", 0, fmt.Errorf("fetch thread emails: %w", err)
	}
	if len(emails) == 0 {
		return "", 0, nil
	}

	var sb strings.Builder
	ptrStr := func(p *string) string {
		if p == nil {
			return ""
		}
		return *p
	}
	for _, e := range emails {
		dateStr := ""
		if e.Date.Valid {
			dateStr = e.Date.Time.Format(time.RFC1123)
		}
		sb.WriteString(fmt.Sprintf("From: %s\nTo: %s\nDate: %s\nSubject: %s\n",
			ptrStr(e.FromAddress),
			ptrStr(e.ToAddresses),
			dateStr,
			ptrStr(e.Subject),
		))
		if e.PlainText != nil && *e.PlainText != "" {
			body := *e.PlainText
			if len(body) > 2000 {
				body = body[:2000] + "..."
			}
			sb.WriteString(body)
		}
		sb.WriteString("\n---\n")
	}
	return sb.String(), len(emails), nil
}

const emailThreadLLMSystemPrompt = "You are a helpful assistant that summarises email threads concisely."

// SummarizeThread collects all emails involving participant and asks Gemini to
// produce a concise summary. Returns an error if Gemini is unavailable.
func (s *EmailService) SummarizeThread(ctx context.Context, participant string) (string, error) {
	if s.gemini == nil {
		return "", fmt.Errorf("Gemini provider not configured")
	}

	transcript, n, err := s.emailThreadTranscript(ctx, participant)
	if err != nil {
		return "", err
	}
	if n == 0 {
		return "No emails found for this participant.", nil
	}

	prompt := "Please provide a concise summary of the following email conversation:\n\n" + transcript
	result, err := s.gemini.GenerateResponse(ctx,
		ai.GenerateRequest{UserInput: prompt},
		emailThreadLLMSystemPrompt,
		nil,
		nil,
		nil,
	)
	if err != nil {
		stub := result.Usage
		if stub == nil {
			stub = StubLLMUsage("gemini", "")
		}
		MarkUsageServerKey(stub, true)
		RecordLLMUsage(ctx, s.billing, s.users, stub, err)
		return "", fmt.Errorf("Gemini summarize: %w", err)
	}
	MarkUsageServerKey(result.Usage, true)
	RecordLLMUsage(ctx, s.billing, s.users, result.Usage, nil)
	return result.PlainText, nil
}

// RunEmailThreadPrompt sends the thread transcript to Gemini with a caller-supplied instruction.
func (s *EmailService) RunEmailThreadPrompt(ctx context.Context, participant, instruction string) (string, error) {
	if s.gemini == nil {
		return "", fmt.Errorf("Gemini provider not configured")
	}
	transcript, n, err := s.emailThreadTranscript(ctx, participant)
	if err != nil {
		return "", err
	}
	if n == 0 {
		return "", fmt.Errorf("no emails found for this participant")
	}
	prompt := fmt.Sprintf("%s\n\n%s", strings.TrimSpace(instruction), transcript)
	result, err := s.gemini.GenerateResponse(ctx,
		ai.GenerateRequest{UserInput: prompt},
		emailThreadLLMSystemPrompt,
		nil,
		nil,
		nil,
	)
	if err != nil {
		stub := result.Usage
		if stub == nil {
			stub = StubLLMUsage("gemini", "")
		}
		MarkUsageServerKey(stub, true)
		RecordLLMUsage(ctx, s.billing, s.users, stub, err)
		return "", fmt.Errorf("Gemini email thread prompt: %w", err)
	}
	MarkUsageServerKey(result.Usage, true)
	RecordLLMUsage(ctx, s.billing, s.users, result.Usage, nil)
	return result.PlainText, nil
}

// ── helpers ───────────────────────────────────────────────────────────────────

// hydrateMetadata batch-fetches attachment IDs for a slice of emails and returns the
// combined response slice. An empty (not nil) attachment_ids list is always returned.
func (s *EmailService) hydrateMetadata(ctx context.Context, emails []*model.Email) ([]model.EmailMetadataResponse, error) {
	if len(emails) == 0 {
		return []model.EmailMetadataResponse{}, nil
	}

	ids := make([]int64, len(emails))
	for i, e := range emails {
		ids[i] = e.ID
	}

	attMap, err := s.repo.GetAttachmentIDsForEmails(ctx, ids)
	if err != nil {
		return nil, fmt.Errorf("get attachment ids: %w", err)
	}

	result := make([]model.EmailMetadataResponse, len(emails))
	for i, e := range emails {
		result[i] = toMetadataResponse(e, attMap[e.ID])
	}
	return result, nil
}

// toMetadataResponse converts an Email domain model and its attachment IDs into
// the JSON response struct. attachmentIDs may be nil (treated as empty).
func toMetadataResponse(e *model.Email, attachmentIDs []int64) model.EmailMetadataResponse {
	if attachmentIDs == nil {
		attachmentIDs = []int64{}
	}
	return model.EmailMetadataResponse{
		ID:            e.ID,
		UID:           e.UID,
		Folder:        e.Folder,
		Subject:       e.Subject,
		FromAddress:   e.FromAddress,
		ToAddresses:   e.ToAddresses,
		CCAddresses:   e.CCAddresses,
		BCCAddresses:  e.BCCAddresses,
		Date:          e.Date,
		Snippet:       e.Snippet,
		AttachmentIDs: attachmentIDs,
		CreatedAt:     e.CreatedAt,
		UpdatedAt:     e.UpdatedAt,
		IsPersonal:    e.IsPersonal,
		IsBusiness:    e.IsBusiness,
		IsImportant:   e.IsImportant,
		UseByAI:       e.UseByAI,
		Source:        e.Source,
	}
}
