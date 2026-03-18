package repository

import (
	"context"
	"fmt"

	"github.com/daveontour/digitalmuseum/internal/model"
	"github.com/jackc/pgx/v5/pgxpool"
)

// DocumentRepo accesses the reference_documents table.
type DocumentRepo struct {
	pool *pgxpool.Pool
}

// NewDocumentRepo creates a DocumentRepo.
func NewDocumentRepo(pool *pgxpool.Pool) *DocumentRepo {
	return &DocumentRepo{pool: pool}
}

const documentCols = `id, filename, title, description, author, content_type, size,
	tags, categories, notes, available_for_task, is_private, is_sensitive, is_encrypted,
	created_at, updated_at`

func scanDocument(row interface{ Scan(...any) error }) (*model.ReferenceDocument, error) {
	var d model.ReferenceDocument
	err := row.Scan(
		&d.ID, &d.Filename, &d.Title, &d.Description, &d.Author,
		&d.ContentType, &d.Size, &d.Tags, &d.Categories, &d.Notes,
		&d.AvailableForTask, &d.IsPrivate, &d.IsSensitive, &d.IsEncrypted,
		&d.CreatedAt, &d.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &d, nil
}

// List returns documents with optional filters. Excludes sensitive records.
func (r *DocumentRepo) List(ctx context.Context, search, category, tag, contentType string, availableForTask *bool) ([]*model.ReferenceDocument, error) {
	q := `SELECT ` + documentCols + ` FROM reference_documents WHERE is_sensitive = FALSE`
	var args []any
	var conds []string

	if search != "" {
		args = append(args, "%"+search+"%")
		idx := len(args)
		conds = append(conds, fmt.Sprintf(
			`(filename ILIKE $%d OR title ILIKE $%d OR description ILIKE $%d OR author ILIKE $%d)`,
			idx, idx, idx, idx,
		))
	}
	if category != "" {
		args = append(args, "%"+category+"%")
		conds = append(conds, fmt.Sprintf("categories ILIKE $%d", len(args)))
	}
	if tag != "" {
		args = append(args, "%"+tag+"%")
		conds = append(conds, fmt.Sprintf("tags ILIKE $%d", len(args)))
	}
	if availableForTask != nil {
		args = append(args, *availableForTask)
		conds = append(conds, fmt.Sprintf("available_for_task = $%d", len(args)))
	}
	if contentType != "" {
		args = append(args, "%"+contentType+"%")
		conds = append(conds, fmt.Sprintf("content_type ILIKE $%d", len(args)))
	}
	if len(conds) > 0 {
		q += " AND " + joinAnd(conds)
	}
	q += " ORDER BY created_at DESC"

	rows, err := r.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("ListDocuments: %w", err)
	}
	defer rows.Close()

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

// GetByID returns a document's metadata (no blob data). Returns nil if not found or is_sensitive.
func (r *DocumentRepo) GetByID(ctx context.Context, id int64) (*model.ReferenceDocument, error) {
	d, err := scanDocument(r.pool.QueryRow(ctx,
		`SELECT `+documentCols+` FROM reference_documents WHERE id = $1 AND is_sensitive = FALSE`, id))
	if err != nil {
		if isNoRows(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("GetDocumentByID %d: %w", id, err)
	}
	return d, nil
}

// GetData returns the raw file bytes and whether the data is encrypted.
func (r *DocumentRepo) GetData(ctx context.Context, id int64) ([]byte, bool, error) {
	var data []byte
	var isEncrypted bool
	err := r.pool.QueryRow(ctx,
		`SELECT data, is_encrypted FROM reference_documents WHERE id = $1`, id,
	).Scan(&data, &isEncrypted)
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
	availableForTask, isPrivate, isSensitive, isEncrypted bool,
) (*model.ReferenceDocument, error) {
	d, err := scanDocument(r.pool.QueryRow(ctx,
		`INSERT INTO reference_documents
		 (filename, title, description, author, content_type, size, data,
		  tags, categories, notes, available_for_task, is_private, is_sensitive, is_encrypted)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14)
		 RETURNING `+documentCols,
		filename, title, description, author, contentType, size, data,
		tags, categories, notes, availableForTask, isPrivate, isSensitive, isEncrypted,
	))
	if err != nil {
		return nil, fmt.Errorf("CreateDocument: %w", err)
	}
	return d, nil
}

// Update modifies document metadata fields.
func (r *DocumentRepo) Update(ctx context.Context, id int64,
	title, description, author, tags, categories, notes *string,
	availableForTask *bool,
) (*model.ReferenceDocument, error) {
	d, err := scanDocument(r.pool.QueryRow(ctx,
		`UPDATE reference_documents SET
		 title            = COALESCE($1, title),
		 description      = COALESCE($2, description),
		 author           = COALESCE($3, author),
		 tags             = COALESCE($4, tags),
		 categories       = COALESCE($5, categories),
		 notes            = COALESCE($6, notes),
		 available_for_task = COALESCE($7, available_for_task),
		 updated_at       = NOW()
		 WHERE id = $8
		 RETURNING `+documentCols,
		title, description, author, tags, categories, notes, availableForTask, id,
	))
	if err != nil {
		if isNoRows(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("UpdateDocument %d: %w", id, err)
	}
	return d, nil
}

// UpdateData replaces the binary content and encryption state of a document.
func (r *DocumentRepo) UpdateData(ctx context.Context, id int64, data []byte, isEncrypted bool) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE reference_documents SET data=$1, is_encrypted=$2, updated_at=NOW() WHERE id=$3`,
		data, isEncrypted, id)
	return err
}

// Delete removes a reference document.
func (r *DocumentRepo) Delete(ctx context.Context, id int64) error {
	_, err := r.pool.Exec(ctx, `DELETE FROM reference_documents WHERE id = $1`, id)
	return err
}

// ListSensitive returns all rows where is_sensitive=TRUE (metadata only, no data blob).
func (r *DocumentRepo) ListSensitive(ctx context.Context) ([]*model.ReferenceDocument, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT `+documentCols+` FROM reference_documents WHERE is_sensitive = TRUE ORDER BY created_at DESC`)
	if err != nil {
		return nil, fmt.Errorf("ListSensitive: %w", err)
	}
	defer rows.Close()
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
	d, err := scanDocument(r.pool.QueryRow(ctx,
		`SELECT `+documentCols+` FROM reference_documents WHERE id = $1 AND is_sensitive = TRUE`, id))
	if err != nil {
		if isNoRows(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("GetSensitiveByID %d: %w", id, err)
	}
	return d, nil
}

// ListUnencrypted returns all non-sensitive rows where is_encrypted=FALSE.
func (r *DocumentRepo) ListUnencrypted(ctx context.Context) ([]*model.ReferenceDocument, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT `+documentCols+` FROM reference_documents WHERE is_encrypted = FALSE AND is_sensitive = FALSE ORDER BY id`)
	if err != nil {
		return nil, fmt.Errorf("ListUnencrypted: %w", err)
	}
	defer rows.Close()
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
