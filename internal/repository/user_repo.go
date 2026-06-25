package repository

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	"github.com/daveontour/aimuseum/internal/sqlutil"
)

// ErrAmbiguousUserFullName is returned when more than one user matches FindByFullName.
var ErrAmbiguousUserFullName = errors.New("multiple users match the given full name")

// User represents a row from the users table.
type User struct {
	ID                 int64
	Email              string
	PasswordHash       string
	DisplayName        string
	FirstName          string
	FamilyName         string
	IsActive           bool
	IsAdmin            bool
	AllowServerLLMKeys bool
	CreatedAt          sqlutil.DBTime
	LastLoginAt        sqlutil.NullDBTime
}

// AuthSession represents a row from the sessions table.
type AuthSession struct {
	ID               string
	UserID           int64
	ExpiresAt        sqlutil.DBTime
	IsVisitor        bool
	ShareLinkSession bool
	VisitorKeyHintID *int64
}

// UserRepo handles all database operations for users and auth sessions.
type UserRepo struct {
	pool *sql.DB
}

// NewUserRepo creates a UserRepo.
func NewUserRepo(pool *sql.DB) *UserRepo {
	return &UserRepo{pool: pool}
}

// Create inserts a new non-admin user and returns the created record.
// displayName is stored in display_name (legacy/UI); firstName and familyName are stored explicitly.
func (r *UserRepo) Create(ctx context.Context, email, passwordHash, displayName, firstName, familyName string) (*User, error) {
	var u User
	err := r.pool.QueryRowContext(ctx,
		`INSERT INTO users (email, password_hash, display_name, first_name, family_name, is_admin)
		 VALUES (?1, ?2, NULLIF(?3, ''), NULLIF(?4, ''), NULLIF(?5, ''), FALSE)
		 RETURNING id, email, password_hash,
		           COALESCE(display_name, ''), COALESCE(first_name, ''), COALESCE(family_name, ''),
		           is_active, is_admin, created_at`,
		email, passwordHash, displayName, firstName, familyName,
	).Scan(&u.ID, &u.Email, &u.PasswordHash, &u.DisplayName, &u.FirstName, &u.FamilyName, &u.IsActive, &u.IsAdmin, &u.CreatedAt)
	return &u, err
}

// FindByEmail returns the user with the given email, or nil if not found.
func (r *UserRepo) FindByEmail(ctx context.Context, email string) (*User, error) {
	var u User
	err := r.pool.QueryRowContext(ctx,
		`SELECT id, email, password_hash, COALESCE(display_name, ''), COALESCE(first_name, ''), COALESCE(family_name, ''), is_active, is_admin, created_at
		 FROM users WHERE email = ?1`,
		email,
	).Scan(&u.ID, &u.Email, &u.PasswordHash, &u.DisplayName, &u.FirstName, &u.FamilyName, &u.IsActive, &u.IsAdmin, &u.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return &u, err
}

const findByFullNameSelect = `SELECT id, email, password_hash, COALESCE(display_name, ''), COALESCE(first_name, ''), COALESCE(family_name, ''), is_active, is_admin, created_at
		 FROM users WHERE LOWER(TRIM(COALESCE(first_name, ''))) = LOWER(?1)
		   AND LOWER(TRIM(COALESCE(family_name, ''))) = LOWER(?2)
		 LIMIT 2`

// FindByFullName parses fullName as whitespace-separated tokens: the first token is first_name
// and the remaining tokens (joined with a single space) are family_name.
// Matching is case-insensitive. Returns nil, nil if no user matches.
// Returns ErrAmbiguousUserFullName if more than one user matches.
func (r *UserRepo) FindByFullName(ctx context.Context, fullName string) (*User, error) {
	fullName = strings.TrimSpace(fullName)
	if fullName == "" {
		return nil, nil
	}
	parts := strings.Fields(fullName)
	first := parts[0]
	family := ""
	if len(parts) > 1 {
		family = strings.Join(parts[1:], " ")
	}

	rows, err := r.pool.QueryContext(ctx, findByFullNameSelect, first, family)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var users []*User
	for rows.Next() {
		var u User
		if err := rows.Scan(&u.ID, &u.Email, &u.PasswordHash, &u.DisplayName, &u.FirstName, &u.FamilyName, &u.IsActive, &u.IsAdmin, &u.CreatedAt); err != nil {
			return nil, err
		}
		users = append(users, &u)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	switch len(users) {
	case 0:
		return nil, nil
	case 1:
		return users[0], nil
	default:
		return nil, ErrAmbiguousUserFullName
	}
}

// FindByID returns the user with the given ID, or nil if not found.
func (r *UserRepo) FindByID(ctx context.Context, id int64) (*User, error) {
	var u User
	err := r.pool.QueryRowContext(ctx,
		`SELECT id, email, password_hash, COALESCE(display_name, ''), COALESCE(first_name, ''), COALESCE(family_name, ''), is_active, is_admin, allow_server_llm_keys, created_at
		 FROM users WHERE id = ?1`,
		id,
	).Scan(&u.ID, &u.Email, &u.PasswordHash, &u.DisplayName, &u.FirstName, &u.FamilyName, &u.IsActive, &u.IsAdmin, &u.AllowServerLLMKeys, &u.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return &u, err
}

// UpdatePasswordHash replaces the password hash for a user.
func (r *UserRepo) UpdatePasswordHash(ctx context.Context, id int64, hash string) error {
	_, err := r.pool.ExecContext(ctx,
		`UPDATE users SET password_hash = ?1 WHERE id = ?2`,
		hash, id,
	)
	return err
}

// TouchLastLogin sets last_login_at = CURRENT_TIMESTAMP for the given user.
func (r *UserRepo) TouchLastLogin(ctx context.Context, id int64) {
	// Best-effort — ignore error so login still succeeds if this fails.
	_, _ = r.pool.ExecContext(ctx, `UPDATE users SET last_login_at = CURRENT_TIMESTAMP WHERE id = ?1`, id)
}

// AdminExists reports whether any user has is_admin = true.
func (r *UserRepo) AdminExists(ctx context.Context) (bool, error) {
	var exists bool
	err := r.pool.QueryRowContext(ctx,
		`SELECT EXISTS(SELECT 1 FROM users WHERE is_admin = TRUE)`,
	).Scan(&exists)
	return exists, err
}

// SetIsAdmin sets the is_admin flag for a user.
func (r *UserRepo) SetIsAdmin(ctx context.Context, id int64, isAdmin bool) error {
	_, err := r.pool.ExecContext(ctx,
		`UPDATE users SET is_admin = ?1 WHERE id = ?2`,
		isAdmin, id,
	)
	return err
}

// CreateAdmin inserts a new admin user and returns the created record.
func (r *UserRepo) CreateAdmin(ctx context.Context, email, passwordHash, displayName, firstName, familyName string) (*User, error) {
	var u User
	err := r.pool.QueryRowContext(ctx,
		`INSERT INTO users (email, password_hash, display_name, first_name, family_name, is_admin)
		 VALUES (?1, ?2, NULLIF(?3, ''), NULLIF(?4, ''), NULLIF(?5, ''), TRUE)
		 RETURNING id, email, password_hash,
		           COALESCE(display_name, ''), COALESCE(first_name, ''), COALESCE(family_name, ''),
		           is_active, is_admin, created_at`,
		email, passwordHash, displayName, firstName, familyName,
	).Scan(&u.ID, &u.Email, &u.PasswordHash, &u.DisplayName, &u.FirstName, &u.FamilyName, &u.IsActive, &u.IsAdmin, &u.CreatedAt)
	return &u, err
}

// AnyNonAdminUserExists reports whether at least one non-admin user account exists.
// Used by single-tenant registration to enforce the one-account limit.
// Ignores the reserved placeholder row at users.id=1.
func (r *UserRepo) AnyNonAdminUserExists(ctx context.Context) (bool, error) {
	var exists bool
	err := r.pool.QueryRowContext(ctx,
		`SELECT EXISTS(SELECT 1 FROM users WHERE id > 1 AND (is_admin = FALSE OR is_admin IS NULL))`,
	).Scan(&exists)
	return exists, err
}

// EmailExists reports whether an email address is already registered.
func (r *UserRepo) EmailExists(ctx context.Context, email string) (bool, error) {
	var exists bool
	err := r.pool.QueryRowContext(ctx,
		`SELECT EXISTS(SELECT 1 FROM users WHERE email = ?1)`, email,
	).Scan(&exists)
	return exists, err
}

// HasAnyVisitorKeys reports whether any non-master sensitive_keyring seat exists.
func (r *UserRepo) HasAnyVisitorKeys(ctx context.Context) (bool, error) {
	var exists bool
	err := r.pool.QueryRowContext(ctx,
		`SELECT EXISTS(SELECT 1 FROM sensitive_keyring WHERE is_master = FALSE)`,
	).Scan(&exists)
	if err != nil && strings.Contains(strings.ToLower(err.Error()), "no such table") {
		return false, nil
	}
	return exists, err
}

// ── Sessions ──────────────────────────────────────────────────────────────────

// CreateSession inserts a new authenticated session.
// For owner login: isVisitor=false (shareLinkSession and visitorKeyHintID ignored).
// For share-link visitors: isVisitor=true, shareLinkSession=true, visitorKeyHintID nil.
// For visitor-key login: isVisitor=true, shareLinkSession=false, visitorKeyHintID set when a hint row exists.
func (r *UserRepo) CreateSession(ctx context.Context, id string, userID int64, expiresAt time.Time, isVisitor, shareLinkSession bool, visitorKeyHintID *int64) error {
	var hintArg any
	if visitorKeyHintID != nil {
		hintArg = *visitorKeyHintID
	} else {
		hintArg = nil
	}
	if !isVisitor {
		shareLinkSession = false
		hintArg = nil
	}
	// Use sqlutil.DBTime so SQLite stores RFC3339 text; raw time.Time can persist as Go String()+monotonic.
	exp := sqlutil.DBTime{Time: expiresAt.UTC()}
	_, err := r.pool.ExecContext(ctx,
		`INSERT INTO sessions (id, user_id, expires_at, is_visitor, share_link_session, visitor_key_hint_id)
		 VALUES (?1, ?2, ?3, ?4, ?5, ?6)`,
		id, userID, exp, isVisitor, shareLinkSession, hintArg,
	)
	return err
}

// FindSession returns a non-expired session by ID, or nil if not found/expired.
func (r *UserRepo) FindSession(ctx context.Context, id string) (*AuthSession, error) {
	var s AuthSession
	var hint sql.NullInt64
	err := r.pool.QueryRowContext(ctx,
		`SELECT id, user_id, expires_at, is_visitor, share_link_session, visitor_key_hint_id FROM sessions
		 WHERE id = ?1 AND expires_at > CURRENT_TIMESTAMP`,
		id,
	).Scan(&s.ID, &s.UserID, &s.ExpiresAt, &s.IsVisitor, &s.ShareLinkSession, &hint)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if hint.Valid {
		v := hint.Int64
		s.VisitorKeyHintID = &v
	}
	return &s, nil
}

// visitorKeyHintsTableExists is true when visitor_key_hints exists. The SQLite
// single-user schema omits keyring/hint tables; querying them would error.
func visitorKeyHintsTableExists(ctx context.Context, pool *sql.DB) bool {
	var n int
	err := pool.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='visitor_key_hints'`).Scan(&n)
	if err != nil {
		// PostgreSQL (no sqlite_master) or other driver: assume full schema.
		return true
	}
	return n > 0
}

// GetVisitorKeyPermissionsForOwner returns feature flags for a visitor_key_hints row
// only when its keyring belongs to ownerUserID.
func (r *UserRepo) GetVisitorKeyPermissionsForOwner(ctx context.Context, hintID, ownerUserID int64) (canMessages, canEmails, canContacts, canRelationships, canSensitivePrivate bool, ok bool, err error) {
	if !visitorKeyHintsTableExists(ctx, r.pool) {
		return false, false, false, false, false, false, nil
	}
	e := r.pool.QueryRowContext(ctx, `
		SELECT h.can_messages_chat, h.can_emails, h.can_contacts, h.can_relationship_sensitive, h.can_sensitive_private
		FROM visitor_key_hints h
		INNER JOIN sensitive_keyring k ON k.id = h.keyring_id AND k.is_master = FALSE
		WHERE h.id = ?1 AND k.user_id = ?2`,
		hintID, ownerUserID,
	).Scan(&canMessages, &canEmails, &canContacts, &canRelationships, &canSensitivePrivate)
	if errors.Is(e, sql.ErrNoRows) {
		return false, false, false, false, false, false, nil
	}
	if e != nil {
		return false, false, false, false, false, false, e
	}
	return canMessages, canEmails, canContacts, canRelationships, canSensitivePrivate, true, nil
}

// ExtendSession pushes expires_at forward.
func (r *UserRepo) ExtendSession(ctx context.Context, id string, newExpiry time.Time) error {
	exp := sqlutil.DBTime{Time: newExpiry.UTC()}
	_, err := r.pool.ExecContext(ctx,
		`UPDATE sessions SET expires_at = ?1 WHERE id = ?2`,
		exp, id,
	)
	return err
}

// DeleteSession removes a session (used on logout).
func (r *UserRepo) DeleteSession(ctx context.Context, id string) error {
	_, err := r.pool.ExecContext(ctx, `DELETE FROM sessions WHERE id = ?1`, id)
	return err
}

// PurgeExpiredSessions deletes all sessions past their expiry.  Called
// periodically from the background cleanup goroutine in AuthService.
func (r *UserRepo) PurgeExpiredSessions(ctx context.Context) error {
	_, err := r.pool.ExecContext(ctx, `DELETE FROM sessions WHERE expires_at <= CURRENT_TIMESTAMP`)
	return err
}

// DeleteAllSessions removes every auth session row. Used on server startup so
// cookies from a previous process cannot authenticate after a restart.
func (r *UserRepo) DeleteAllSessions(ctx context.Context) (int64, error) {
	tag, err := r.pool.ExecContext(ctx, `DELETE FROM sessions`)
	if err != nil {
		return 0, err
	}
	return rowsAffectedOrZero(tag), nil
}

// ListAll returns every user ordered by created_at ascending.
func (r *UserRepo) ListAll(ctx context.Context) ([]*User, error) {
	rows, err := r.pool.QueryContext(ctx,
		`SELECT id, email, password_hash, COALESCE(display_name, ''), COALESCE(first_name, ''), COALESCE(family_name, ''), is_active, is_admin, allow_server_llm_keys, created_at
		 FROM users WHERE id > 1 ORDER BY created_at ASC`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var users []*User
	for rows.Next() {
		var u User
		if err := rows.Scan(&u.ID, &u.Email, &u.PasswordHash, &u.DisplayName, &u.FirstName, &u.FamilyName, &u.IsActive, &u.IsAdmin, &u.AllowServerLLMKeys, &u.CreatedAt); err != nil {
			return nil, err
		}
		users = append(users, &u)
	}
	return users, rows.Err()
}

// SetAllowServerLLMKeys sets whether the user may use server GEMINI_API_KEY, ANTHROPIC_API_KEY,
// and TAVILY_API_KEY when they have not set their own (per-provider) keys.
func (r *UserRepo) SetAllowServerLLMKeys(ctx context.Context, userID int64, allow bool) error {
	_, err := r.pool.ExecContext(ctx, `UPDATE users SET allow_server_llm_keys = ?2 WHERE id = ?1`, userID, allow)
	return err
}

// Delete removes a user by ID; all associated data cascades via FK.
func (r *UserRepo) Delete(ctx context.Context, id int64) error {
	_, err := r.pool.ExecContext(ctx, `DELETE FROM users WHERE id = ?1`, id)
	return err
}
