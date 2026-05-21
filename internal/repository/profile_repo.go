package repository

import (
	"context"
	"crypto/rand"
	"database/sql"
	"fmt"
	"time"
)

// ArchiveProfile is a row from the archive_profiles table in the billing DB.
type ArchiveProfile struct {
	ID        string     `json:"id"`
	Name      string     `json:"name"`
	Username  *string    `json:"username,omitempty"`
	DBPath    string     `json:"db_path"`
	Enabled   bool       `json:"enabled"`
	IsDefault bool       `json:"is_default"`
	CreatedAt time.Time  `json:"created_at"`
	LastUsed  *time.Time `json:"last_used,omitempty"`
}

// ProfileRepo accesses archive_profiles in the billing database.
type ProfileRepo struct {
	pool *sql.DB
}

// NewProfileRepo creates a ProfileRepo backed by the given connection pool.
func NewProfileRepo(pool *sql.DB) *ProfileRepo {
	return &ProfileRepo{pool: pool}
}

// ListAll returns every profile (including disabled) — for admin use.
func (r *ProfileRepo) ListAll(ctx context.Context) ([]ArchiveProfile, error) {
	rows, err := r.pool.QueryContext(ctx,
		`SELECT id, name, username, db_path, enabled, created_at, last_used, is_default
		 FROM archive_profiles ORDER BY created_at ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanProfiles(rows)
}

// ListEnabled returns only enabled profiles — for the public login page.
func (r *ProfileRepo) ListEnabled(ctx context.Context) ([]ArchiveProfile, error) {
	rows, err := r.pool.QueryContext(ctx,
		`SELECT id, name, username, db_path, enabled, created_at, last_used, is_default
		 FROM archive_profiles WHERE enabled = 1 ORDER BY created_at ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanProfiles(rows)
}

// GetByID fetches a single profile.
func (r *ProfileRepo) GetByID(ctx context.Context, id string) (*ArchiveProfile, error) {
	row := r.pool.QueryRowContext(ctx,
		`SELECT id, name, username, db_path, enabled, created_at, last_used, is_default
		 FROM archive_profiles WHERE id = ?`, id)
	p, err := scanProfile(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return p, err
}

// GetByDBPath fetches a profile by exact db_path (caller should normalise paths).
func (r *ProfileRepo) GetByDBPath(ctx context.Context, dbPath string) (*ArchiveProfile, error) {
	row := r.pool.QueryRowContext(ctx,
		`SELECT id, name, username, db_path, enabled, created_at, last_used, is_default
		 FROM archive_profiles WHERE db_path = ?`, dbPath)
	p, err := scanProfile(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return p, err
}

// GetEnabledStartupDefault returns the enabled profile marked as startup default, if any.
func (r *ProfileRepo) GetEnabledStartupDefault(ctx context.Context) (*ArchiveProfile, error) {
	row := r.pool.QueryRowContext(ctx,
		`SELECT id, name, username, db_path, enabled, created_at, last_used, is_default
		 FROM archive_profiles WHERE is_default = 1 AND enabled = 1 ORDER BY created_at ASC LIMIT 1`)
	p, err := scanProfile(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return p, err
}

// PromoteSoleEnabledArchiveToDefault marks the sole enabled archive as startup default
// when no enabled default is currently set.
func (r *ProfileRepo) PromoteSoleEnabledArchiveToDefault(ctx context.Context) error {
	var nEnabled int
	if err := r.pool.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM archive_profiles WHERE enabled = 1`).Scan(&nEnabled); err != nil {
		return err
	}
	if nEnabled != 1 {
		return nil
	}
	var nDefault int
	if err := r.pool.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM archive_profiles WHERE enabled = 1 AND is_default = 1`).Scan(&nDefault); err != nil {
		return err
	}
	if nDefault >= 1 {
		return nil
	}
	var id string
	if err := r.pool.QueryRowContext(ctx,
		`SELECT id FROM archive_profiles WHERE enabled = 1 LIMIT 1`).Scan(&id); err != nil {
		return err
	}
	return r.SetStartupDefault(ctx, id)
}

// SetStartupDefault marks one profile as the sole startup default (must be enabled).
func (r *ProfileRepo) SetStartupDefault(ctx context.Context, id string) error {
	p, err := r.GetByID(ctx, id)
	if err != nil {
		return err
	}
	if p == nil {
		return fmt.Errorf("profile not found")
	}
	if !p.Enabled {
		return fmt.Errorf("profile is disabled")
	}
	tx, err := r.pool.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `UPDATE archive_profiles SET is_default = 0`); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE archive_profiles SET is_default = 1 WHERE id = ?`, id); err != nil {
		return err
	}
	return tx.Commit()
}

// ClearStartupDefaultForID clears the startup default flag for a single profile.
func (r *ProfileRepo) ClearStartupDefaultForID(ctx context.Context, id string) error {
	_, err := r.pool.ExecContext(ctx, `UPDATE archive_profiles SET is_default = 0 WHERE id = ?`, id)
	return err
}

// Create inserts a new profile and returns it.
func (r *ProfileRepo) Create(ctx context.Context, name, dbPath string) (*ArchiveProfile, error) {
	id := newProfileID()
	_, err := r.pool.ExecContext(ctx,
		`INSERT INTO archive_profiles (id, name, db_path) VALUES (?, ?, ?)`,
		id, name, dbPath)
	if err != nil {
		return nil, err
	}
	return r.GetByID(ctx, id)
}

// UpdateUsername sets the username field for a profile.
func (r *ProfileRepo) UpdateUsername(ctx context.Context, id, username string) error {
	_, err := r.pool.ExecContext(ctx,
		`UPDATE archive_profiles SET username = ? WHERE id = ?`, username, id)
	return err
}

// UpdateName sets the name field for a profile.
func (r *ProfileRepo) UpdateName(ctx context.Context, id, name string) error {
	_, err := r.pool.ExecContext(ctx,
		`UPDATE archive_profiles SET name = ? WHERE id = ?`, name, id)
	return err
}

// SetEnabled enables or disables a profile.
// Disabling clears is_default so a disabled row is never the startup default.
func (r *ProfileRepo) SetEnabled(ctx context.Context, id string, enabled bool) error {
	v := 0
	if enabled {
		v = 1
	}
	if enabled {
		_, err := r.pool.ExecContext(ctx,
			`UPDATE archive_profiles SET enabled = ? WHERE id = ?`, v, id)
		return err
	}
	_, err := r.pool.ExecContext(ctx,
		`UPDATE archive_profiles SET enabled = ?, is_default = 0 WHERE id = ?`, v, id)
	return err
}

// TouchLastUsed updates last_used to now.
func (r *ProfileRepo) TouchLastUsed(ctx context.Context, id string) error {
	_, err := r.pool.ExecContext(ctx,
		`UPDATE archive_profiles SET last_used = datetime('now') WHERE id = ?`, id)
	return err
}

// Delete removes a profile permanently.
func (r *ProfileRepo) Delete(ctx context.Context, id string) error {
	_, err := r.pool.ExecContext(ctx,
		`DELETE FROM archive_profiles WHERE id = ?`, id)
	return err
}

// ── helpers ───────────────────────────────────────────────────────────────────

func newProfileID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return fmt.Sprintf("%x", b)
}

type profileScanner interface {
	Scan(dest ...any) error
}

func scanProfile(s profileScanner) (*ArchiveProfile, error) {
	var p ArchiveProfile
	var enabledInt int
	var createdRaw string
	var lastUsedRaw sql.NullString
	var username sql.NullString
	var isDefaultInt int
	if err := s.Scan(&p.ID, &p.Name, &username, &p.DBPath, &enabledInt, &createdRaw, &lastUsedRaw, &isDefaultInt); err != nil {
		return nil, err
	}
	p.Enabled = enabledInt != 0
	p.IsDefault = isDefaultInt != 0
	if username.Valid {
		p.Username = &username.String
	}
	p.CreatedAt, _ = parseProfileTime(createdRaw)
	if lastUsedRaw.Valid && lastUsedRaw.String != "" {
		t, _ := parseProfileTime(lastUsedRaw.String)
		p.LastUsed = &t
	}
	return &p, nil
}

func scanProfiles(rows *sql.Rows) ([]ArchiveProfile, error) {
	var out []ArchiveProfile
	for rows.Next() {
		p, err := scanProfile(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *p)
	}
	if out == nil {
		out = []ArchiveProfile{}
	}
	return out, rows.Err()
}

func parseProfileTime(s string) (time.Time, error) {
	formats := []string{
		"2006-01-02T15:04:05Z",
		"2006-01-02 15:04:05",
		time.RFC3339,
	}
	for _, f := range formats {
		if t, err := time.Parse(f, s); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("unparseable time: %q", s)
}
