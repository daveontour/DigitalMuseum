// Package repository contains pgx query implementations.
package repository

import (
	"context"
	"fmt"
	"strings"

	"github.com/daveontour/digitalmuseum/internal/model"
	"github.com/jackc/pgx/v5/pgxpool"
)

// EmailRepo runs queries against the emails and related tables.
type EmailRepo struct {
	pool *pgxpool.Pool
}

// NewEmailRepo creates an EmailRepo backed by the given pool.
func NewEmailRepo(pool *pgxpool.Pool) *EmailRepo {
	return &EmailRepo{pool: pool}
}

// GetByID returns a non-deleted email by primary key, or nil if not found.
func (r *EmailRepo) GetByID(ctx context.Context, id int64) (*model.Email, error) {
	row := r.pool.QueryRow(ctx, `
		SELECT id, uid, folder, subject, from_address, to_addresses, cc_addresses, bcc_addresses,
		       date, raw_message, plain_text, snippet, embedding,
		       has_attachments, user_deleted, is_personal, is_business, is_social, is_promotional,
		       is_spam, is_important, use_by_ai, created_at, updated_at
		FROM emails
		WHERE id = $1
		  AND user_deleted = FALSE`, id)

	e := &model.Email{}
	err := row.Scan(
		&e.ID, &e.UID, &e.Folder, &e.Subject, &e.FromAddress, &e.ToAddresses,
		&e.CCAddresses, &e.BCCAddresses, &e.Date, &e.RawMessage, &e.PlainText,
		&e.Snippet, &e.Embedding, &e.HasAttachments, &e.UserDeleted,
		&e.IsPersonal, &e.IsBusiness, &e.IsSocial, &e.IsPromotional,
		&e.IsSpam, &e.IsImportant, &e.UseByAI, &e.CreatedAt, &e.UpdatedAt,
	)
	if err != nil {
		if isNoRows(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("email GetByID %d: %w", id, err)
	}
	return e, nil
}

// Search returns emails matching the given optional filters, ordered by date DESC.
// All filters are AND-combined; within to_from the addresses are OR-combined.
func (r *EmailRepo) Search(ctx context.Context, p model.EmailSearchParams) ([]*model.Email, error) {
	var (
		conds []string
		args  []any
		n     = 1
	)

	add := func(cond string, val any) {
		conds = append(conds, strings.ReplaceAll(cond, "?", fmt.Sprintf("$%d", n)))
		args = append(args, val)
		n++
	}

	if p.FromAddress != nil {
		add("from_address ILIKE ?", "%"+*p.FromAddress+"%")
	}
	if p.ToAddress != nil {
		add("to_addresses ILIKE ?", "%"+*p.ToAddress+"%")
	}
	if p.Month != nil {
		add("EXTRACT(month FROM date) = ?", *p.Month)
	}
	if p.Year != nil {
		add("EXTRACT(year FROM date) = ?", *p.Year)
	}
	if p.Subject != nil {
		add("(subject ILIKE ? OR snippet ILIKE ? OR folder ILIKE ?)", "%"+*p.Subject+"%")
	}
	if p.ToFrom != nil {
		parts := splitTrim(*p.ToFrom, ',')
		if len(parts) > 0 {
			var orParts []string
			for _, addr := range parts {
				orParts = append(orParts,
					fmt.Sprintf("(to_addresses ILIKE $%d OR from_address ILIKE $%d)", n, n+1),
				)
				args = append(args, "%"+addr+"%", "%"+addr+"%")
				n += 2
			}
			conds = append(conds, "("+strings.Join(orParts, " OR ")+")")
		}
	}
	if p.HasAttachments != nil {
		add("has_attachments = ?", *p.HasAttachments)
	}
	// Always exclude soft-deleted rows
	conds = append(conds, "user_deleted = FALSE")

	sql := `SELECT id, uid, folder, subject, from_address, to_addresses, cc_addresses, bcc_addresses,
			       date, raw_message, plain_text, snippet, embedding,
			       has_attachments, user_deleted, is_personal, is_business, is_social, is_promotional,
			       is_spam, is_important, use_by_ai, created_at, updated_at
			FROM emails`
	if len(conds) > 0 {
		sql += " WHERE " + strings.Join(conds, " AND ")
	}
	sql += " ORDER BY date DESC"

	rows, err := r.pool.Query(ctx, sql, args...)
	if err != nil {
		return nil, fmt.Errorf("email Search: %w", err)
	}
	defer rows.Close()
	return scanEmails(rows)
}

// GetByLabels returns non-deleted emails whose folder column matches any of the given labels.
// The folder column can be a comma-separated list of labels (e.g. "INBOX,IMPORTANT").
func (r *EmailRepo) GetByLabels(ctx context.Context, labels []string) ([]*model.Email, error) {
	if len(labels) == 0 {
		return nil, nil
	}

	// Build OR conditions: exact match OR starts with "label," OR contains ",label," OR ends with ",label"
	var conds []string
	var args []any
	n := 1
	for _, label := range labels {
		conds = append(conds,
			fmt.Sprintf("(folder = $%d OR folder LIKE $%d OR folder LIKE $%d OR folder LIKE $%d)",
				n, n+1, n+2, n+3),
		)
		args = append(args,
			label,
			label+",%",
			"%, "+label+",%",
			"%, "+label,
		)
		n += 4
	}

	sql := `SELECT id, uid, folder, subject, from_address, to_addresses, cc_addresses, bcc_addresses,
			       date, raw_message, plain_text, snippet, embedding,
			       has_attachments, user_deleted, is_personal, is_business, is_social, is_promotional,
			       is_spam, is_important, use_by_ai, created_at, updated_at
			FROM emails
			WHERE (` + strings.Join(conds, " OR ") + `) AND user_deleted = FALSE`

	rows, err := r.pool.Query(ctx, sql, args...)
	if err != nil {
		return nil, fmt.Errorf("email GetByLabels: %w", err)
	}
	defer rows.Close()
	return scanEmails(rows)
}

// GetAttachmentIDsForEmails returns a map of emailID → []mediaItemID for all given email IDs.
// Uses a single query to avoid N+1 round trips.
func (r *EmailRepo) GetAttachmentIDsForEmails(ctx context.Context, emailIDs []int64) (map[int64][]int64, error) {
	result := make(map[int64][]int64, len(emailIDs))
	if len(emailIDs) == 0 {
		return result, nil
	}

	// source_reference is VARCHAR storing the string form of the email ID
	idStrs := make([]string, len(emailIDs))
	for i, id := range emailIDs {
		idStrs[i] = fmt.Sprintf("%d", id)
		result[id] = []int64{} // ensure key exists even if no attachments
	}

	rows, err := r.pool.Query(ctx, `
		SELECT source_reference, id
		FROM media_items
		WHERE source IN ('email_attachment', 'gmail_attachment')
		  AND source_reference = ANY($1::text[])
		ORDER BY id`, idStrs)
	if err != nil {
		return nil, fmt.Errorf("GetAttachmentIDsForEmails: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var ref string
		var mediaID int64
		if err := rows.Scan(&ref, &mediaID); err != nil {
			return nil, err
		}
		// parse back to int64
		var emailID int64
		if _, err := fmt.Sscanf(ref, "%d", &emailID); err == nil {
			result[emailID] = append(result[emailID], mediaID)
		}
	}
	return result, rows.Err()
}

// Update modifies the flag columns of an existing email.
// Returns false, nil when the email does not exist (not found or already deleted).
func (r *EmailRepo) Update(ctx context.Context, id int64, isPersonal, isBusiness, isImportant, useByAI *bool) (bool, error) {
	var sets []string
	var args []any
	n := 1

	addSet := func(col string, val any) {
		sets = append(sets, fmt.Sprintf("%s = $%d", col, n))
		args = append(args, val)
		n++
	}

	if isPersonal != nil {
		addSet("is_personal", *isPersonal)
	}
	if isBusiness != nil {
		addSet("is_business", *isBusiness)
	}
	if isImportant != nil {
		addSet("is_important", *isImportant)
	}
	if useByAI != nil {
		addSet("use_by_ai", *useByAI)
	}
	if len(sets) == 0 {
		// nothing to update — check existence
		var exists bool
		err := r.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM emails WHERE id = $1 AND user_deleted = FALSE)`, id).Scan(&exists)
		return exists, err
	}

	args = append(args, id)
	sql := fmt.Sprintf(
		"UPDATE emails SET %s WHERE id = $%d AND user_deleted = FALSE",
		strings.Join(sets, ", "), n,
	)
	tag, err := r.pool.Exec(ctx, sql, args...)
	if err != nil {
		return false, fmt.Errorf("email Update %d: %w", id, err)
	}
	return tag.RowsAffected() > 0, nil
}

// SoftDelete nullifies message content and marks the email as deleted.
// Returns false, nil when the email does not exist.
func (r *EmailRepo) SoftDelete(ctx context.Context, id int64) (bool, error) {
	tag, err := r.pool.Exec(ctx, `
		UPDATE emails
		SET raw_message      = NULL,
		    plain_text        = NULL,
		    snippet           = NULL,
		    embedding         = NULL,
		    has_attachments   = FALSE,
		    user_deleted      = TRUE
		WHERE id = $1 AND user_deleted = FALSE`, id)
	if err != nil {
		return false, fmt.Errorf("email SoftDelete %d: %w", id, err)
	}
	if tag.RowsAffected() == 0 {
		return false, nil
	}
	// Remove IMAP/Gmail email attachment media_items for this email; delete blobs no longer referenced.
	ref := fmt.Sprintf("%d", id)
	_, _ = r.pool.Exec(ctx, `
		WITH deleted AS (
			DELETE FROM media_items
			WHERE source IN ('email_attachment', 'gmail_attachment') AND source_reference = $1
			RETURNING media_blob_id
		)
		DELETE FROM media_blobs b
		WHERE b.id IN (SELECT DISTINCT media_blob_id FROM deleted WHERE media_blob_id IS NOT NULL)
		  AND NOT EXISTS (SELECT 1 FROM media_items m WHERE m.media_blob_id = b.id)`, ref)
	return true, nil
}

// BulkSoftDelete soft-deletes a batch of emails by ID.
// Returns the count of rows actually deleted.
func (r *EmailRepo) BulkSoftDelete(ctx context.Context, ids []int64) (int64, error) {
	if len(ids) == 0 {
		return 0, nil
	}
	tag, err := r.pool.Exec(ctx, `
		UPDATE emails
		SET raw_message      = NULL,
		    plain_text        = NULL,
		    snippet           = NULL,
		    embedding         = NULL,
		    has_attachments   = FALSE,
		    user_deleted      = TRUE
		WHERE id = ANY($1::bigint[])
		  AND user_deleted = FALSE`, ids)
	if err != nil {
		return 0, fmt.Errorf("email BulkSoftDelete: %w", err)
	}
	refs := make([]string, len(ids))
	for i, id := range ids {
		refs[i] = fmt.Sprintf("%d", id)
	}
	_, _ = r.pool.Exec(ctx, `
		WITH deleted AS (
			DELETE FROM media_items
			WHERE source IN ('email_attachment', 'gmail_attachment') AND source_reference = ANY($1::text[])
			RETURNING media_blob_id
		)
		DELETE FROM media_blobs b
		WHERE b.id IN (SELECT DISTINCT media_blob_id FROM deleted WHERE media_blob_id IS NOT NULL)
		  AND NOT EXISTS (SELECT 1 FROM media_items m WHERE m.media_blob_id = b.id)`, refs)
	return tag.RowsAffected(), nil
}

// GetThreadEmails returns non-deleted emails where from_address or to_addresses
// contain the given participant address, ordered by date ASC.
func (r *EmailRepo) GetThreadEmails(ctx context.Context, participant string) ([]*model.Email, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, uid, folder, subject, from_address, to_addresses, cc_addresses, bcc_addresses,
		       date, raw_message, plain_text, snippet, embedding,
		       has_attachments, user_deleted, is_personal, is_business, is_social, is_promotional,
		       is_spam, is_important, use_by_ai, created_at, updated_at
		FROM emails
		WHERE (from_address ILIKE $1 OR to_addresses ILIKE $1)
		  AND user_deleted = FALSE
		ORDER BY date ASC`, "%"+participant+"%")
	if err != nil {
		return nil, fmt.Errorf("GetThreadEmails: %w", err)
	}
	defer rows.Close()
	return scanEmails(rows)
}

// ListFolders returns distinct folder/label names stored across all emails.
// Folder is stored as comma-joined label names, so this unnests and deduplicates them.
func (r *EmailRepo) ListFolders(ctx context.Context) ([]string, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT DISTINCT unnest(string_to_array(folder, ',')) AS f
		FROM emails
		WHERE user_deleted = FALSE
		ORDER BY f`)
	if err != nil {
		return nil, fmt.Errorf("ListFolders: %w", err)
	}
	defer rows.Close()
	var folders []string
	for rows.Next() {
		var f string
		if err := rows.Scan(&f); err != nil {
			return nil, err
		}
		if f != "" {
			folders = append(folders, f)
		}
	}
	return folders, rows.Err()
}

// ── helpers ───────────────────────────────────────────────────────────────────

// scanEmails collects rows into a slice of Email pointers.
func scanEmails(rows interface {
	Next() bool
	Scan(dest ...any) error
	Err() error
}) ([]*model.Email, error) {
	var emails []*model.Email
	for rows.Next() {
		e := &model.Email{}
		if err := rows.Scan(
			&e.ID, &e.UID, &e.Folder, &e.Subject, &e.FromAddress, &e.ToAddresses,
			&e.CCAddresses, &e.BCCAddresses, &e.Date, &e.RawMessage, &e.PlainText,
			&e.Snippet, &e.Embedding, &e.HasAttachments, &e.UserDeleted,
			&e.IsPersonal, &e.IsBusiness, &e.IsSocial, &e.IsPromotional,
			&e.IsSpam, &e.IsImportant, &e.UseByAI, &e.CreatedAt, &e.UpdatedAt,
		); err != nil {
			return nil, err
		}
		emails = append(emails, e)
	}
	return emails, rows.Err()
}

// isNoRows returns true for pgx "no rows in result set" error.
func isNoRows(err error) bool {
	return err != nil && err.Error() == "no rows in result set"
}

// splitTrim splits s by sep and trims whitespace from each element, omitting blanks.
func splitTrim(s string, sep rune) []string {
	parts := strings.FieldsFunc(s, func(r rune) bool { return r == sep })
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if t := strings.TrimSpace(p); t != "" {
			out = append(out, t)
		}
	}
	return out
}
