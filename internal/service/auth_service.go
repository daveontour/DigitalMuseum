package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/daveontour/aimuseum/internal/appctx"
	appcrypto "github.com/daveontour/aimuseum/internal/crypto"
	"github.com/daveontour/aimuseum/internal/repository"
)

const (
	// authSessionCookieName is the HttpOnly cookie that carries the auth session ID.
	// Separate from dm_keyring_sid which carries the keyring unlock password.
	AuthSessionCookieName = "dm_session"

	authSessionTTL     = 24 * time.Hour
	minPasswordLength  = 1
	sessionCleanupFreq = 30 * time.Minute
)

// ErrInvalidCredentials is returned when username/password does not match.
var ErrInvalidCredentials = errors.New("invalid username or password")

// ErrUsernameTaken is returned when registering with an already-used username.
var ErrUsernameTaken = errors.New("username is already registered")

// ErrRegistrationClosed is returned when a second account is attempted.
var ErrRegistrationClosed = errors.New("an account already exists — only one account is permitted")

// ErrWeakPassword is returned when the password does not meet length requirements.
var ErrWeakPassword = fmt.Errorf("password must be at least %d characters", minPasswordLength)

// ErrUserNotFound is returned when the session points to a deleted/inactive user.
var ErrUserNotFound = errors.New("user not found")

// AuthResult is the outcome of a successful session lookup (see Authenticate).
type AuthResult struct {
	User             *repository.User
	IsVisitor        bool
	Access           appctx.VisitorAccess
	VisitorKeyHintID *int64 // session's visitor_key_hint_id when set (visitor-key login)
}

// AuthService handles user registration, login, session management, and
// password changes.  It is intentionally separate from the keyring-unlock
// SessionMasterStore (which stores the keyring decrypt password in RAM).
type AuthService struct {
	users     *repository.UserRepo
	secure    bool   // Secure flag for Set-Cookie (true behind HTTPS)
	dummyHash string // real argon2id hash used for constant-time login when user not found
}

// NewAuthService creates an AuthService.  secure should be true when the app
// is served over HTTPS.
func NewAuthService(users *repository.UserRepo, secure bool) *AuthService {
	// Pre-compute a genuine argon2id hash so that Login always runs the full
	// verification even when the username is not found, keeping timing constant.
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		panic("auth: failed to generate dummy hash seed: " + err.Error())
	}
	dummyHash, err := appcrypto.HashPassword(hex.EncodeToString(b))
	if err != nil {
		panic("auth: failed to generate dummy hash: " + err.Error())
	}
	svc := &AuthService{users: users, secure: secure, dummyHash: dummyHash}
	go svc.cleanupLoop()
	return svc
}

func canonicalPassword(password string) string {
	return strings.ToLower(password)
}

// verifyPasswordCanonicalFirst checks the canonical lowercase password first.
// If that fails and legacy raw-case hashing matches, it upgrades the stored hash
// to canonical lowercase so future logins follow the new rule.
func (s *AuthService) verifyPasswordCanonicalFirst(ctx context.Context, userID int64, rawPassword, storedHash string) (bool, error) {
	canonical := canonicalPassword(rawPassword)
	ok, err := appcrypto.VerifyPassword(canonical, storedHash)
	if err != nil {
		return false, err
	}
	if ok {
		return true, nil
	}
	// If canonical == raw, there is no legacy variant to try.
	if rawPassword == canonical {
		return false, nil
	}
	legacyOK, err := appcrypto.VerifyPassword(rawPassword, storedHash)
	if err != nil {
		return false, err
	}
	if !legacyOK {
		return false, nil
	}
	// Upgrade legacy hash to canonical lowercase.
	newHash, err := appcrypto.HashPassword(canonical)
	if err != nil {
		return false, err
	}
	if err := s.users.UpdatePasswordHash(ctx, userID, newHash); err != nil {
		return false, err
	}
	return true, nil
}

// ── Registration ─────────────────────────────────────────────────────────────

// Register creates a new user account. Returns ErrUsernameTaken if the username is
// already in use, ErrWeakPassword if the password is too short.
// displayName is stored as display_name (and first_name); familyName is stored as family_name.
func (s *AuthService) Register(ctx context.Context, username, password, displayName, familyName string) (*repository.User, error) {
	username = strings.ToLower(strings.TrimSpace(username))
	password = canonicalPassword(password)
	displayName = strings.TrimSpace(displayName)
	familyName = strings.TrimSpace(familyName)

	if len(password) < minPasswordLength {
		return nil, ErrWeakPassword
	}

	// Single-tenant: only one non-admin account is permitted.
	anyExists, err := s.users.AnyNonAdminUserExists(ctx)
	if err != nil {
		return nil, fmt.Errorf("check existing users: %w", err)
	}
	if anyExists {
		return nil, ErrRegistrationClosed
	}

	exists, err := s.users.EmailExists(ctx, username)
	if err != nil {
		return nil, fmt.Errorf("check username: %w", err)
	}
	if exists {
		return nil, ErrUsernameTaken
	}

	hash, err := appcrypto.HashPassword(password)
	if err != nil {
		return nil, fmt.Errorf("hash password: %w", err)
	}

	user, err := s.users.Create(ctx, username, hash, displayName, displayName, familyName)
	if err != nil {
		return nil, fmt.Errorf("create user: %w", err)
	}
	return user, nil
}

// ── Login ────────────────────────────────────────────────────────────────────

// Login validates credentials and creates a new session.  Returns the session
// ID (to be stored in the cookie) and the authenticated user.
// Returns ErrInvalidCredentials on any mismatch to prevent user enumeration.
func (s *AuthService) Login(ctx context.Context, username, password string) (sessionID string, user *repository.User, err error) {
	username = strings.ToLower(strings.TrimSpace(username))
	rawPassword := password

	u, err := s.users.FindByEmail(ctx, username)
	if err != nil {
		return "", nil, fmt.Errorf("find user: %w", err)
	}

	// Always run VerifyPassword even when the user is not found — constant
	// time behaviour prevents user-enumeration via response timing.
	checkHash := s.dummyHash
	if u != nil {
		checkHash = u.PasswordHash
	}
	ok, verifyErr := appcrypto.VerifyPassword(canonicalPassword(rawPassword), checkHash)
	if verifyErr != nil {
		return "", nil, fmt.Errorf("verify password: %w", verifyErr)
	}
	if !ok && rawPassword != canonicalPassword(rawPassword) {
		legacyOK, legacyErr := appcrypto.VerifyPassword(rawPassword, checkHash)
		if legacyErr != nil {
			return "", nil, fmt.Errorf("verify legacy password: %w", legacyErr)
		}
		ok = legacyOK
	}
	if !ok || u == nil || !u.IsActive || u.IsAdmin {
		return "", nil, ErrInvalidCredentials
	}
	if _, err := s.verifyPasswordCanonicalFirst(ctx, u.ID, rawPassword, u.PasswordHash); err != nil {
		return "", nil, fmt.Errorf("upgrade password hash: %w", err)
	}

	sid, err := randomSessionID()
	if err != nil {
		return "", nil, fmt.Errorf("generate session: %w", err)
	}

	if err := s.users.CreateSession(ctx, sid, u.ID, time.Now().Add(authSessionTTL), false, false, nil); err != nil {
		return "", nil, fmt.Errorf("create session: %w", err)
	}

	s.users.TouchLastLogin(ctx, u.ID)
	return sid, u, nil
}

// ── Session lookup ───────────────────────────────────────────────────────────

// Authenticate looks up the session cookie and returns the user plus visitor access flags.
// Returns (nil, nil) when the session is missing or expired.
func (s *AuthService) Authenticate(ctx context.Context, sessionID string) (*AuthResult, error) {
	if sessionID == "" {
		return nil, nil
	}
	sess, err := s.users.FindSession(ctx, sessionID)
	if err != nil {
		return nil, fmt.Errorf("find session: %w", err)
	}
	if sess == nil {
		return nil, nil
	}

	// Slide the expiry forward on activity.
	_ = s.users.ExtendSession(ctx, sessionID, time.Now().Add(authSessionTTL))

	user, err := s.users.FindByID(ctx, sess.UserID)
	if err != nil {
		return nil, fmt.Errorf("find user: %w", err)
	}
	if user == nil || !user.IsActive {
		return nil, nil
	}

	access := appctx.VisitorAccess{Restricted: false}
	if sess.IsVisitor {
		if sess.ShareLinkSession {
			access = appctx.VisitorAccess{Restricted: false}
		} else if sess.VisitorKeyHintID != nil {
			msgs, em, co, rel, sp, ok, err := s.users.GetVisitorKeyPermissionsForOwner(ctx, *sess.VisitorKeyHintID, user.ID)
			if err != nil {
				return nil, fmt.Errorf("visitor permissions: %w", err)
			}
			if !ok {
				access = appctx.VisitorAccess{Restricted: true}
			} else {
				access = appctx.VisitorAccess{
					Restricted:          true,
					CanMessagesChat:     msgs,
					CanEmails:           em,
					CanContacts:         co,
					CanRelationships:    rel,
					CanSensitivePrivate: sp,
				}
			}
		} else {
			access = appctx.VisitorAccess{Restricted: true}
		}
	}

	return &AuthResult{User: user, IsVisitor: sess.IsVisitor, Access: access, VisitorKeyHintID: sess.VisitorKeyHintID}, nil
}

// ── Logout ───────────────────────────────────────────────────────────────────

// Logout deletes the session from the database.  The caller is responsible for
// expiring the cookie on the HTTP response.
func (s *AuthService) Logout(ctx context.Context, sessionID string) error {
	if sessionID == "" {
		return nil
	}
	return s.users.DeleteSession(ctx, sessionID)
}

// ── Change password ──────────────────────────────────────────────────────────

// ChangePassword verifies currentPassword then stores a new hash.
// Returns ErrInvalidCredentials when currentPassword is wrong.
func (s *AuthService) ChangePassword(ctx context.Context, userID int64, currentPassword, newPassword string) error {
	rawCurrentPassword := currentPassword
	newPassword = canonicalPassword(newPassword)
	if len(newPassword) < minPasswordLength {
		return ErrWeakPassword
	}

	user, err := s.users.FindByID(ctx, userID)
	if err != nil {
		return fmt.Errorf("find user: %w", err)
	}
	if user == nil {
		return ErrUserNotFound
	}

	ok, err := s.verifyPasswordCanonicalFirst(ctx, user.ID, rawCurrentPassword, user.PasswordHash)
	if err != nil {
		return fmt.Errorf("verify password: %w", err)
	}
	if !ok {
		return ErrInvalidCredentials
	}

	newHash, err := appcrypto.HashPassword(newPassword)
	if err != nil {
		return fmt.Errorf("hash new password: %w", err)
	}
	return s.users.UpdatePasswordHash(ctx, userID, newHash)
}

// GetUserLLMStored returns persisted per-user LLM/API overrides.
func (s *AuthService) GetUserLLMStored(ctx context.Context, userID int64) (*repository.UserLLMStored, error) {
	return s.users.GetUserLLMStored(ctx, userID)
}

// PatchUserLLMSettings merges patch into the user's stored LLM/API overrides.
func (s *AuthService) PatchUserLLMSettings(ctx context.Context, userID int64, p repository.UserLLMPatch) error {
	return s.users.PatchUserLLMSettings(ctx, userID, p)
}

// GetSessionVisitorLLM returns LLM overrides stored on the visitor session row (session-scoped only).
func (s *AuthService) GetSessionVisitorLLM(ctx context.Context, sessionID string) (*repository.UserLLMStored, error) {
	return s.users.GetSessionVisitorLLM(ctx, sessionID)
}

// PatchSessionVisitorLLM updates session-scoped LLM overrides (visitor sessions only).
func (s *AuthService) PatchSessionVisitorLLM(ctx context.Context, sessionID string, p repository.UserLLMPatch) error {
	return s.users.PatchSessionVisitorLLM(ctx, sessionID, p)
}

// ── Share sessions ───────────────────────────────────────────────────────────

// CreateShareSession creates a DB session scoped to ownerUserID for a share visitor.
// Unlike Login, no password verification is performed — the share token validates the visitor.
func (s *AuthService) CreateShareSession(ctx context.Context, ownerUserID int64) (string, error) {
	sid, err := randomSessionID()
	if err != nil {
		return "", fmt.Errorf("generate share session: %w", err)
	}
	if err := s.users.CreateSession(ctx, sid, ownerUserID, time.Now().Add(authSessionTTL), true, true, nil); err != nil {
		return "", fmt.Errorf("create share session: %w", err)
	}
	return sid, nil
}

// CreateVisitorKeySession creates a visitor session tied to a visitor_key_hints row
// (share_link_session=false). When visitorKeyHintID is nil, the session is fully restricted.
func (s *AuthService) CreateVisitorKeySession(ctx context.Context, ownerUserID int64, visitorKeyHintID *int64) (string, error) {
	sid, err := randomSessionID()
	if err != nil {
		return "", fmt.Errorf("generate visitor session: %w", err)
	}
	if err := s.users.CreateSession(ctx, sid, ownerUserID, time.Now().Add(authSessionTTL), true, false, visitorKeyHintID); err != nil {
		return "", fmt.Errorf("create visitor session: %w", err)
	}
	return sid, nil
}

// ── Admin bootstrapping ──────────────────────────────────────────────────────

// EnsureAdminUser creates an admin user from env-var credentials if no admin
// exists yet. It is idempotent — safe to call on every startup.
// If the admin email is already registered without the admin flag (e.g. a prior
// normal signup), that user is promoted to admin; password is unchanged.
// Once an admin row exists, env vars are not used to alter existing users.
func (s *AuthService) EnsureAdminUser(ctx context.Context, email, password string) error {
	email = strings.ToLower(strings.TrimSpace(email))
	password = canonicalPassword(password)
	if email == "" || password == "" {
		return nil
	}
	exists, err := s.users.AdminExists(ctx)
	if err != nil {
		return fmt.Errorf("check admin exists: %w", err)
	}
	if exists {
		return nil
	}
	// Same email may already exist without is_admin — promote instead of INSERT.
	if u, ferr := s.users.FindByEmail(ctx, email); ferr != nil {
		return fmt.Errorf("look up admin email: %w", ferr)
	} else if u != nil {
		if err := s.users.SetIsAdmin(ctx, u.ID, true); err != nil {
			return fmt.Errorf("promote existing user to admin: %w", err)
		}
		slog.Info("existing user promoted to admin", "email", email)
		return nil
	}
	if len(password) < minPasswordLength {
		return fmt.Errorf("ADMIN_PASSWORD must be at least %d characters", minPasswordLength)
	}
	hash, err := appcrypto.HashPassword(password)
	if err != nil {
		return fmt.Errorf("hash admin password: %w", err)
	}
	if _, err := s.users.CreateAdmin(ctx, email, hash, "Admin", "Admin", ""); err != nil {
		return fmt.Errorf("create admin user: %w", err)
	}
	slog.Info("admin user created", "email", email)
	return nil
}

// AdminLogin verifies credentials and returns the user only when is_admin = true.
// Returns ErrInvalidCredentials on any mismatch.
func (s *AuthService) AdminLogin(ctx context.Context, email, password string) (*repository.User, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	rawPassword := password
	u, err := s.users.FindByEmail(ctx, email)
	if err != nil {
		return nil, fmt.Errorf("find user: %w", err)
	}
	checkHash := s.dummyHash
	if u != nil {
		checkHash = u.PasswordHash
	}
	ok, verifyErr := appcrypto.VerifyPassword(canonicalPassword(rawPassword), checkHash)
	if verifyErr != nil {
		return nil, fmt.Errorf("verify password: %w", verifyErr)
	}
	if !ok && rawPassword != canonicalPassword(rawPassword) {
		legacyOK, legacyErr := appcrypto.VerifyPassword(rawPassword, checkHash)
		if legacyErr != nil {
			return nil, fmt.Errorf("verify legacy password: %w", legacyErr)
		}
		ok = legacyOK
	}
	if !ok || u == nil || !u.IsActive || !u.IsAdmin {
		return nil, ErrInvalidCredentials
	}
	if _, err := s.verifyPasswordCanonicalFirst(ctx, u.ID, rawPassword, u.PasswordHash); err != nil {
		return nil, fmt.Errorf("upgrade password hash: %w", err)
	}
	return u, nil
}

// ── Cookie helpers ───────────────────────────────────────────────────────────

// CookieMaxAge is the Set-Cookie Max-Age for a new auth session.
const CookieMaxAge = int(authSessionTTL / time.Second)

// ── Internals ────────────────────────────────────────────────────────────────

func randomSessionID() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func (s *AuthService) cleanupLoop() {
	t := time.NewTicker(sessionCleanupFreq)
	defer t.Stop()
	for range t.C {
		if err := s.users.PurgeExpiredSessions(context.Background()); err != nil {
			slog.Warn("session cleanup failed", "err", err)
		}
	}
}
