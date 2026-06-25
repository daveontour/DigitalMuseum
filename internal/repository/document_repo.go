package repository

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/daveontour/aimuseum/internal/model"
)

// DocumentRepo accesses the reference_documents table.
type DocumentRepo struct {
	pool *sql.DB
}

// NewDocumentRepo creates a DocumentRepo.
func NewDocumentRepo(pool *sql.DB) *DocumentRepo {
	return &DocumentRepo{pool: pool}
}

const documentCols = `id, filename, title, description, author, content_type, size,
	tags, categories, notes, available_for_task, include_in_system_prompt, is_private, is_sensitive, is_encrypted,
	created_at, updated_at`

func scanDocument(row interface{ Scan(...any) error }) (*model.ReferenceDocument, error) {
	var d model.ReferenceDocument
	err := row.Scan(
		&d.ID, &d.Filename, &d.Title, &d.Description, &d.Author,
		&d.ContentType, &d.Size, &d.Tags, &d.Categories, &d.Notes,
		&d.AvailableForTask, &d.IncludeInSystemPrompt, &d.IsPrivate, &d.IsSensitive, &d.IsEncrypted,
		&d.CreatedAt, &d.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &d, nil
}

// List returns documents with optional filters. Excludes sensitive records.
func (r *DocumentRepo) List(ctx context.Context, search, category, tag, contentType string, availableForTask *bool) ([]*model.ReferenceDocument, error) {
	uid := uidFromCtx(ctx)
	q := `SELECT ` + documentCols + ` FROM reference_documents WHERE is_sensitive = FALSE`
	var args []any
	var conds []string

	if search != "" {
		args = append(args, "%"+search+"%")
		idx := len(args)
		conds = append(conds, fmt.Sprintf(
			`(filename LIKE ?%d OR title LIKE ?%d OR description LIKE ?%d OR author LIKE ?%d)`,
			idx, idx, idx, idx,
		))
	}
	if category != "" {
		args = append(args, "%"+category+"%")
		conds = append(conds, fmt.Sprintf("categories LIKE ?%d", len(args)))
	}
	if tag != "" {
		args = append(args, "%"+tag+"%")
		conds = append(conds, fmt.Sprintf("tags LIKE ?%d", len(args)))
	}
	if availableForTask != nil {
		args = append(args, *availableForTask)
		conds = append(conds, fmt.Sprintf("available_for_task = ?%d", len(args)))
	}
	if contentType != "" {
		args = append(args, "%"+contentType+"%")
		conds = append(conds, fmt.Sprintf("content_type LIKE ?%d", len(args)))
	}
	if len(conds) > 0 {
		q += " AND " + joinAnd(conds)
	}
	q, args = addUIDFilter(q, args, uid)
	q += " ORDER BY created_at DESC"

	rows, err := r.pool.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("listDocuments: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []*model.ReferenceDocument
	for rows.Next() {
		d, err := scanDocument(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// CountAvailableForAI returns non-sensitive reference documents exposed to the AI: either listed
// via tools (available_for_task, not system-prompt inlined) or inlined in the system prompt.
func (r *DocumentRepo) CountAvailableForAI(ctx context.Context) (int64, error) {
	uid := uidFromCtx(ctx)
	q := `SELECT COUNT(*) FROM reference_documents WHERE is_sensitive = FALSE
	      AND (available_for_task = TRUE OR include_in_system_prompt = TRUE)`
	args := []any{}
	q, args = addUIDFilter(q, args, uid)
	var n int64
	if err := r.pool.QueryRowContext(ctx, q, args...).Scan(&n); err != nil {
		return 0, fmt.Errorf("count reference documents for AI: %w", err)
	}
	return n, nil
}

// ListForSystemPromptInclusion returns rows flagged for system-prompt inlining, with raw data bytes.
// Caller applies session rules (visitor / sensitive) and decrypts when is_encrypted.
func (r *DocumentRepo) ListForSystemPromptInclusion(ctx context.Context) ([]model.ReferenceDocumentPromptBlob, error) {
	uid := uidFromCtx(ctx)
	q := `SELECT id, title, filename, content_type, data, is_encrypted, is_sensitive
		FROM reference_documents WHERE include_in_system_prompt = TRUE`
	args := []any{}
	q, args = addUIDFilter(q, args, uid)
	q += ` ORDER BY id ASC`
	rows, err := r.pool.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("listForSystemPromptInclusion: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []model.ReferenceDocumentPromptBlob
	for rows.Next() {
		var row model.ReferenceDocumentPromptBlob
		if err := rows.Scan(&row.ID, &row.Title, &row.Filename, &row.ContentType, &row.Data, &row.IsEncrypted, &row.IsSensitive); err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

// GetByID returns a document's metadata (no blob data). Returns nil if not found or is_sensitive.
func (r *DocumentRepo) GetByID(ctx context.Context, id int64) (*model.ReferenceDocument, error) {
	uid := uidFromCtx(ctx)
	q := `SELECT ` + documentCols + ` FROM reference_documents WHERE id = ?1 AND is_sensitive = FALSE`
	args := []any{id}
	q, args = addUIDFilter(q, args, uid)
	d, err := scanDocument(r.pool.QueryRowContext(ctx, q, args...))
	if err != nil {
		if isNoRows(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("getDocumentByID %d: %w", id, err)
	}
	return d, nil
}

// GetData returns the raw file bytes and whether the data is encrypted.
func (r *DocumentRepo) GetData(ctx context.Context, id int64) ([]byte, bool, error) {
	uid := uidFromCtx(ctx)
	q := `SELECT data, is_encrypted FROM reference_documents WHERE id = ?1`
	args := []any{id}
	q, args = addUIDFilter(q, args, uid)
	var data []byte
	var isEncrypted bool
	err := r.pool.QueryRowContext(ctx, q, args...).Scan(&data, &isEncrypted)
	if err != nil {
		if isNoRows(err) {
			return nil, false, nil
		}
		return nil, false, err
	}
	return data, isEncrypted, nil
}

// Create inserts a new reference document.
func (r *DocumentRepo) Create(ctx context.Context,
	filename, contentType string, size int64, data []byte,
	title, description, author, tags, categories, notes *string,
	availableForTask, includeInSystemPrompt, isPrivate, isSensitive, isEncrypted bool,
) (*model.ReferenceDocument, error) {
	uid := uidFromCtx(ctx)
	d, err := scanDocument(r.pool.QueryRowContext(ctx,
		`INSERT INTO reference_documents
		 (filename, title, description, author, content_type, size, data,
		  tags, categories, notes, available_for_task, include_in_system_prompt, is_private, is_sensitive, is_encrypted, user_id)
		 VALUES (?1,?2,?3,?4,?5,?6,?7,?8,?9,?10,?11,?12,?13,?14,?15,?16)
		 RETURNING `+documentCols,
		filename, title, description, author, contentType, size, data,
		tags, categories, notes, availableForTask, includeInSystemPrompt, isPrivate, isSensitive, isEncrypted, uidVal(uid),
	))
	if err != nil {
		return nil, fmt.Errorf("createDocument: %w", err)
	}
	return d, nil
}

// Update modifies document metadata fields.
func (r *DocumentRepo) Update(ctx context.Context, id int64,
	title, description, author, tags, categories, notes *string,
	availableForTask *bool,
	includeInSystemPrompt *bool,
) (*model.ReferenceDocument, error) {
	uid := uidFromCtx(ctx)
	q := `UPDATE reference_documents SET
	      title            = COALESCE(?1, title),
	      description      = COALESCE(?2, description),
	      author           = COALESCE(?3, author),
	      tags             = COALESCE(?4, tags),
	      categories       = COALESCE(?5, categories),
	      notes            = COALESCE(?6, notes),
	      available_for_task = COALESCE(?7, available_for_task),
	      include_in_system_prompt = COALESCE(?8, include_in_system_prompt),
	      updated_at       = CURRENT_TIMESTAMP
	      WHERE id = ?9`
	args := []any{title, description, author, tags, categories, notes, availableForTask, includeInSystemPrompt, id}
	q, args = addUIDFilter(q, args, uid)
	q += ` RETURNING ` + documentCols
	d, err := scanDocument(r.pool.QueryRowContext(ctx, q, args...))
	if err != nil {
		if isNoRows(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("updateDocument %d: %w", id, err)
	}
	return d, nil
}

// UpdateData replaces the binary content and encryption state of a document.
func (r *DocumentRepo) UpdateData(ctx context.Context, id int64, data []byte, isEncrypted bool) error {
	uid := uidFromCtx(ctx)
	q := `UPDATE reference_documents SET data=?1, is_encrypted=?2, updated_at=CURRENT_TIMESTAMP WHERE id=?3`
	args := []any{data, isEncrypted, id}
	q, args = addUIDFilter(q, args, uid)
	_, err := r.pool.ExecContext(ctx, q, args...)
	return err
}

// FindIdentityProfile returns the identity_profile.md reference document for the current user,
// or (nil, nil) if none exists. Does not return the binary blob — use GetData separately.
func (r *DocumentRepo) FindIdentityProfile(ctx context.Context) (*model.ReferenceDocument, error) {
	uid := uidFromCtx(ctx)
	q := `SELECT ` + documentCols + ` FROM reference_documents
	      WHERE filename = 'identity_profile.md' AND tags LIKE '%identity-profile%'`
	args := []any{}
	q, args = addUIDFilter(q, args, uid)
	q += ` LIMIT 1`
	d, err := scanDocument(r.pool.QueryRowContext(ctx, q, args...))
	if err != nil {
		if isNoRows(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("findIdentityProfile: %w", err)
	}
	return d, nil
}

// UpdateDocumentContent replaces the binary blob, size, and encryption state of a document.
func (r *DocumentRepo) UpdateDocumentContent(ctx context.Context, id int64, data []byte, size int64, isEncrypted bool) error {
	uid := uidFromCtx(ctx)
	q := `UPDATE reference_documents SET data=?1, size=?2, is_encrypted=?3, updated_at=CURRENT_TIMESTAMP WHERE id=?4`
	args := []any{data, size, isEncrypted, id}
	q, args = addUIDFilter(q, args, uid)
	_, err := r.pool.ExecContext(ctx, q, args...)
	return err
}

// Delete removes a reference document.
func (r *DocumentRepo) Delete(ctx context.Context, id int64) error {
	uid := uidFromCtx(ctx)
	q := `DELETE FROM reference_documents WHERE id = ?1`
	args := []any{id}
	q, args = addUIDFilter(q, args, uid)
	_, err := r.pool.ExecContext(ctx, q, args...)
	return err
}

// ListSensitive returns all rows where is_sensitive=TRUE (metadata only, no data blob).
func (r *DocumentRepo) ListSensitive(ctx context.Context) ([]*model.ReferenceDocument, error) {
	uid := uidFromCtx(ctx)
	q := `SELECT ` + documentCols + ` FROM reference_documents WHERE is_sensitive = TRUE`
	args := []any{}
	q, args = addUIDFilter(q, args, uid)
	q += " ORDER BY created_at DESC"
	rows, err := r.pool.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("listSensitive: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []*model.ReferenceDocument
	for rows.Next() {
		d, err := scanDocument(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// GetSensitiveByID returns a single sensitive record by ID, or nil if not found / not sensitive.
func (r *DocumentRepo) GetSensitiveByID(ctx context.Context, id int64) (*model.ReferenceDocument, error) {
	uid := uidFromCtx(ctx)
	q := `SELECT ` + documentCols + ` FROM reference_documents WHERE id = ?1 AND is_sensitive = TRUE`
	args := []any{id}
	q, args = addUIDFilter(q, args, uid)
	d, err := scanDocument(r.pool.QueryRowContext(ctx, q, args...))
	if err != nil {
		if isNoRows(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("getSensitiveByID %d: %w", id, err)
	}
	return d, nil
}

// ListUnencrypted returns all non-sensitive rows where is_encrypted=FALSE.
func (r *DocumentRepo) ListUnencrypted(ctx context.Context) ([]*model.ReferenceDocument, error) {
	uid := uidFromCtx(ctx)
	q := `SELECT ` + documentCols + ` FROM reference_documents WHERE is_encrypted = FALSE AND is_sensitive = FALSE`
	args := []any{}
	q, args = addUIDFilter(q, args, uid)
	q += " ORDER BY id"
	rows, err := r.pool.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("listUnencrypted: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []*model.ReferenceDocument
	for rows.Next() {
		d, err := scanDocument(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}
