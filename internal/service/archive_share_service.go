package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/daveontour/aimuseum/internal/appctx"
	appcrypto "github.com/daveontour/aimuseum/internal/crypto"
	"github.com/daveontour/aimuseum/internal/model"
	"github.com/daveontour/aimuseum/internal/repository"
)

// ErrShareNotFound is returned when the share token does not exist.
var ErrShareNotFound = errors.New("share token not found")

// ErrShareExpired is returned when the share token has passed its expiry.
var ErrShareExpired = errors.New("share token has expired")

// ErrSharePasswordRequired is returned when the share requires a password but none was supplied.
var ErrSharePasswordRequired = errors.New("this share requires a password")

// ErrSharePasswordInvalid is returned when the supplied password does not match.
var ErrSharePasswordInvalid = errors.New("incorrect share password")

// ArchiveShareService manages archive share tokens and visitor access.
type ArchiveShareService struct {
	repo    *repository.ArchiveShareRepo
	authSvc *AuthService
}

// NewArchiveShareService creates an ArchiveShareService.
func NewArchiveShareService(repo *repository.ArchiveShareRepo, authSvc *AuthService) *ArchiveShareService {
	return &ArchiveShareService{repo: repo, authSvc: authSvc}
}

// ── Owner operations ─────────────────────────────────────────────────────────

// CreateShare generates a new share token for the authenticated user.
// password may be empty (open share). expiresAt may be nil (no expiry).
// toolPolicy is optional JSON describing allowed AI tools.
func (s *ArchiveShareService) CreateShare(ctx context.Context, label, password string, expiresAt *time.Time, toolPolicy string) (*model.ArchiveShare, error) {
	uid := appctx.UserIDFromCtx(ctx)
	if uid == 0 {
		return nil, fmt.Errorf("authentication required to create a share")
	}

	id, err := randomShareID()
	if err != nil {
		return nil, fmt.Errorf("generate share id: %w", err)
	}

	var ph *string
	if pw := strings.TrimSpace(password); pw != "" {
		hash, err := appcrypto.HashPassword(pw)
		if err != nil {
			return nil, fmt.Errorf("hash share password: %w", err)
		}
		ph = &hash
	}

	var labelPtr *string
	if l := strings.TrimSpace(label); l != "" {
		labelPtr = &l
	}

	var policyBytes []byte
	if tp := strings.TrimSpace(toolPolicy); tp != "" {
		// Validate JSON before storing.
		if !json.Valid([]byte(tp)) {
			return nil, fmt.Errorf("tool_access_policy must be valid JSON")
		}
		policyBytes = []byte(tp)
	}

	return s.repo.Create(ctx, id, uid, labelPtr, ph, expiresAt, policyBytes)
}

// ListShares returns all share tokens owned by the authenticated user.
func (s *ArchiveShareService) ListShares(ctx context.Context) ([]*model.ArchiveShare, error) {
	uid := appctx.UserIDFromCtx(ctx)
	if uid == 0 {
		return nil, fmt.Errorf("authentication required")
	}
	return s.repo.ListByUser(ctx, uid)
}

// DeleteShare removes a share token. Only the owning user may delete their shares.
func (s *ArchiveShareService) DeleteShare(ctx context.Context, token string) error {
	uid := appctx.UserIDFromCtx(ctx)
	if uid == 0 {
		return fmt.Errorf("authentication required")
	}
	return s.repo.Delete(ctx, token, uid)
}

// ── Visitor operations ───────────────────────────────────────────────────────

// GetSharePublic looks up the share and returns the public view (no password hash / owner user_id).
// Returns ErrShareNotFound when the token does not exist, ErrShareExpired when past expiry.
func (s *ArchiveShareService) GetSharePublic(ctx context.Context, token string) (*model.ArchiveSharePublic, error) {
	share, err := s.repo.GetByID(ctx, token)
	if err != nil {
		return nil, fmt.Errorf("look up share: %w", err)
	}
	if share == nil {
		return nil, ErrShareNotFound
	}
	if share.ExpiresAt.Valid && time.Now().After(share.ExpiresAt.Time) {
		return nil, ErrShareExpired
	}
	ownerName, _ := s.repo.GetOwnerDisplayName(ctx, share.UserID)
	return &model.ArchiveSharePublic{
		ID:               share.ID,
		Label:            share.Label,
		HasPassword:      share.HasPassword,
		ExpiresAt:        share.ExpiresAt,
		OwnerDisplayName: ownerName,
	}, nil
}

// JoinShare validates a share token and optional password, then creates a session
// scoped to the archive owner's user_id. Returns the new session ID for the visitor.
// The session is a standard auth session — the visitor will see the owner's data.
func (s *ArchiveShareService) JoinShare(ctx context.Context, token, password string) (sessionID string, err error) {
	share, hash, err := s.repo.GetByIDWithHash(ctx, token)
	if err != nil {
		return "", fmt.Errorf("look up share: %w", err)
	}
	if share == nil {
		return "", ErrShareNotFound
	}
	if share.ExpiresAt.Valid && time.Now().After(share.ExpiresAt.Time) {
		return "", ErrShareExpired
	}
	if share.HasPassword {
		if strings.TrimSpace(password) == "" {
			return "", ErrSharePasswordRequired
		}
		ok, err := appcrypto.VerifyPassword(password, hash)
		if err != nil {
			return "", fmt.Errorf("verify password: %w", err)
		}
		if !ok {
			return "", ErrSharePasswordInvalid
		}
	}
	return s.authSvc.CreateShareSession(ctx, share.UserID)
}

// ── helpers ───────────────────────────────────────────────────────────────────

func randomShareID() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
