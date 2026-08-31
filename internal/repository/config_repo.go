package repository

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"strings"

	"github.com/daveontour/aimuseum/internal/model"
	"github.com/daveontour/aimuseum/internal/sqlutil"
)

// ConfigRepo accesses the app_configuration table.
type ConfigRepo struct {
	pool *sql.DB
}

// NewConfigRepo creates a ConfigRepo.
func NewConfigRepo(pool *sql.DB) *ConfigRepo {
	return &ConfigRepo{pool: pool}
}

// KnownKey describes a well-known configuration key.
type KnownKey struct {
	Key         string
	EnvDefault  *string
	IsMandatory bool
	Description string
}

// knownKeys mirrors Python's KNOWN_KEYS list.
var knownKeys = func() []KnownKey {
	str := func(s string) *string { return &s }
	return []KnownKey{
		// OpenRouter, Tavily, and RunPod API keys are configured only via
		// Configuration → API Keys in the running app — not as env-seeded
		// app_configuration rows.
		{"ELEVENLABS_API_KEY", nil, false, "ElevenLabs API key (speech / voice integrations)"},
		{"PAGE_TITLE", str("Digital Museum of SUBJECT_NAME"), false, "Browser page title"},
		{"ATTACHMENT_ALLOWED_TYPES", str(""), false, "Comma-separated allowed attachment MIME/ext types"},
		{"ATTACHMENT_MIN_SIZE", str("0"), false, "Minimum attachment size in bytes"},
		{"FILESYSTEM_IMPORT_EXCLUDE_PATTERNS", str(""), false, "Comma-separated directory exclusion patterns"},
	}
}()

const configCols = `id, key, value, is_mandatory, description, created_at, updated_at`

func scanConfig(row interface{ Scan(...any) error }) (*model.AppConfiguration, error) {
	var c model.AppConfiguration
	err := row.Scan(&c.ID, &c.Key, &c.Value, &c.IsMandatory, &c.Description, &c.CreatedAt, &c.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &c, nil
}

// List returns all configuration rows ordered by key.
func (r *ConfigRepo) List(ctx context.Context) ([]*model.AppConfiguration, error) {
	uid := uidFromCtx(ctx)
	q := `SELECT ` + configCols + ` FROM app_configuration WHERE TRUE`
	args := []any{}
	q, args = addUIDFilter(q, args, uid)
	q += " ORDER BY key"
	rows, err := r.pool.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("listConfig: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []*model.AppConfiguration
	for rows.Next() {
		c, err := scanConfig(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// GetValueByKey returns the value for a configuration key, or nil if not set.
func (r *ConfigRepo) GetValueByKey(ctx context.Context, key string) (*string, error) {
	uid := uidFromCtx(ctx)
	q := `SELECT value FROM app_configuration WHERE key = ?1`
	args := []any{key}
	q, args = addUIDFilter(q, args, uid)
	var val sql.NullString
	err := r.pool.QueryRowContext(ctx, q, args...).Scan(&val)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("getValueByKey %s: %w", key, err)
	}
	if !val.Valid || strings.TrimSpace(val.String) == "" {
		return nil, nil
	}
	s := val.String
	return &s, nil
}

// Upsert creates or updates a configuration key. Returns the row.
func (r *ConfigRepo) Upsert(ctx context.Context, key string, value *string, isMandatory *bool, description *string) (*model.AppConfiguration, error) {
	uid := uidFromCtx(ctx)
	// Look up known-key metadata for defaults
	var defMandatory bool
	var defDesc *string
	for _, k := range knownKeys {
		if k.Key == key {
			defMandatory = k.IsMandatory
			if k.Description != "" {
				d := k.Description
				defDesc = &d
			}
			break
		}
	}
	if isMandatory == nil {
		isMandatory = &defMandatory
	}
	if description == nil {
		description = defDesc
	}

	// app_configuration uses partial unique indexes (WHERE user_id IS NULL / IS NOT NULL).
	// SQLite requires the conflict target to include a matching WHERE clause.
	conflictClause := `ON CONFLICT (key) DO UPDATE SET`
	if sqlutil.IsSQLite(ctx, r.pool) {
		if uid > 0 {
			conflictClause = `ON CONFLICT (key, user_id) WHERE user_id IS NOT NULL DO UPDATE SET`
		} else {
			conflictClause = `ON CONFLICT (key) WHERE user_id IS NULL DO UPDATE SET`
		}
	}
	c, err := scanConfig(r.pool.QueryRowContext(ctx,
		`INSERT INTO app_configuration (key, value, is_mandatory, description, user_id)
		 VALUES (?1, ?2, ?3, ?4, ?5)
		 `+conflictClause+`
		   value        = EXCLUDED.value,
		   is_mandatory = COALESCE(EXCLUDED.is_mandatory, app_configuration.is_mandatory),
		   description  = COALESCE(EXCLUDED.description, app_configuration.description),
		   updated_at   = CURRENT_TIMESTAMP
		 RETURNING `+configCols,
		key, value, isMandatory, description, uidVal(uid),
	))
	if err != nil {
		return nil, fmt.Errorf("upsertConfig %s: %w", key, err)
	}
	return c, nil
}

// Delete removes a configuration key. Returns false if not found.
func (r *ConfigRepo) Delete(ctx context.Context, key string) (bool, error) {
	uid := uidFromCtx(ctx)
	q := `DELETE FROM app_configuration WHERE key = ?1`
	args := []any{key}
	q, args = addUIDFilter(q, args, uid)
	tag, err := r.pool.ExecContext(ctx, q, args...)
	if err != nil {
		return false, err
	}
	return rowsAffectedOrZero(tag) > 0, nil
}

// SeedFromEnv inserts KNOWN_KEYS not yet in the DB, using env values or documented defaults.
// Returns number of new rows inserted.
func (r *ConfigRepo) SeedFromEnv(ctx context.Context) (int, error) {
	uid := uidFromCtx(ctx)
	// Load existing keys
	q := `SELECT key, description FROM app_configuration WHERE TRUE`
	args := []any{}
	q, args = addUIDFilter(q, args, uid)
	rows, err := r.pool.QueryContext(ctx, q, args...)
	if err != nil {
		return 0, err
	}
	type existing struct{ desc *string }
	have := map[string]existing{}
	for rows.Next() {
		var k string
		var desc *string
		if err := rows.Scan(&k, &desc); err != nil {
			_ = rows.Close()
			return 0, err
		}
		have[k] = existing{desc}
	}
	_ = rows.Close()
	if err := rows.Err(); err != nil {
		return 0, err
	}

	inserted := 0
	for _, kk := range knownKeys {
		if ex, found := have[kk.Key]; found {
			// Backfill description if missing
			if ex.desc == nil && kk.Description != "" {
				uq := `UPDATE app_configuration SET description=?1 WHERE key=?2 AND description IS NULL`
				uargs := []any{kk.Description, kk.Key}
				uq, uargs = addUIDFilter(uq, uargs, uid)
				_, err := r.pool.ExecContext(ctx, uq, uargs...)
				if err != nil {
					return inserted, err
				}
			}
			continue
		}
		// New key — use env value if set, else documented default
		var value *string
		if v := os.Getenv(kk.Key); v != "" {
			value = &v
		} else {
			value = kk.EnvDefault
		}
		desc := kk.Description
		doNothing := `ON CONFLICT (key) DO NOTHING`
		if sqlutil.IsSQLite(ctx, r.pool) {
			doNothing = `ON CONFLICT (key) WHERE user_id IS NULL DO NOTHING`
		}
		_, err := r.pool.ExecContext(ctx,
			`INSERT INTO app_configuration (key, value, is_mandatory, description, user_id)
			 VALUES (?1, ?2, ?3, ?4, ?5)
			 `+doNothing,
			kk.Key, value, kk.IsMandatory, desc, uidVal(uid))
		if err != nil {
			return inserted, fmt.Errorf("seedFromEnv %s: %w", kk.Key, err)
		}
		inserted++
	}
	return inserted, nil
}
