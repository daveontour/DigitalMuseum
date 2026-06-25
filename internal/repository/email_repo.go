// Package repository contains pgx query implementations.
package repository

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"strings"

	"github.com/daveontour/aimuseum/internal/model"
	"github.com/daveontour/aimuseum/internal/sqlutil"
)

// EmailRepo runs queries against the emails and related tables.
type EmailRepo struct {
	pool *sql.DB
}

// NewEmailRepo creates an EmailRepo backed by the given pool.
func NewEmailRepo(pool *sql.DB) *EmailRepo {
	return &EmailRepo{pool: pool}
}

// GetByID returns a non-deleted email by primary key, or nil if not found.
func (r *EmailRepo) GetByID(ctx context.Context, id int64) (*model.Email, error) {
	uid := uidFromCtx(ctx)
	q := `
		SELECT id, uid, folder, subject, from_address, to_addresses, cc_addresses, bcc_addresses,
		       date, raw_message, plain_text, snippet, embedding,
		       has_attachments, user_deleted, is_personal, is_business, is_social, is_promotional,
		       is_spam, is_important, use_by_ai, source, created_at, updated_at
		FROM emails
		WHERE id = ?1
		  AND user_deleted = FALSE`
	args := []any{id}
	q, args = addUIDFilter(q, args, uid)

	row := r.pool.QueryRowContext(ctx, q, args...)

	e := &model.Email{}
	err := row.Scan(
		&e.ID, &e.UID, &e.Folder, &e.Subject, &e.FromAddress, &e.ToAddresses,
		&e.CCAddresses, &e.BCCAddresses, &e.Date, &e.RawMessage, &e.PlainText,
		&e.Snippet, &e.Embedding, &e.HasAttachments, &e.UserDeleted,
		&e.IsPersonal, &e.IsBusiness, &e.IsSocial, &e.IsPromotional,
		&e.IsSpam, &e.IsImportant, &e.UseByAI, &e.Source, &e.CreatedAt, &e.UpdatedAt,
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
	uid := uidFromCtx(ctx)
	isSQL := sqlutil.IsSQLite(ctx, r.pool)
	var (
		conds []string
		args  []any
		n     = 1
	)

	add := func(cond string, val any) {
		conds = append(conds, strings.ReplaceAll(cond, "?", fmt.Sprintf("?%d", n)))
		args = append(args, val)
		n++
	}

	// addLIKE appends a single-parameter OR clause across multiple columns.
	// All fields share the same ?n placeholder so only one arg is added.
	addLIKE := func(fields []string, val string) {
		parts := make([]string, len(fields))
		for i, f := range fields {
			parts[i] = fmt.Sprintf("%s LIKE ?%d", f, n)
		}
		conds = append(conds, "("+strings.Join(parts, " OR ")+")")
		args = append(args, "%"+val+"%")
		n++
	}

	if p.FromAddress != nil {
		add("from_address LIKE ?", "%"+*p.FromAddress+"%")
	}
	if p.ToAddress != nil {
		add("to_addresses LIKE ?", "%"+*p.ToAddress+"%")
	}
	if p.Month != nil {
		if isSQL {
			add("CAST(strftime('%m', date) AS INTEGER) = ?", *p.Month)
		} else {
			add("EXTRACT(month FROM date) = ?", *p.Month)
		}
	}
	if p.Year != nil {
		if isSQL {
			add("CAST(strftime('%Y', date) AS INTEGER) = ?", *p.Year)
		} else {
			add("EXTRACT(year FROM date) = ?", *p.Year)
		}
	}
	if p.Subject != nil {
		addLIKE([]string{"subject", "snippet", "folder"}, *p.Subject)
	}
	if p.ToFrom != nil {
		parts := splitTrim(*p.ToFrom, ',')
		if len(parts) > 0 {
			var orParts []string
			for _, addr := range parts {
				orParts = append(orParts,
					fmt.Sprintf("(to_addresses LIKE ?%d OR from_address LIKE ?%d)", n, n+1),
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
	if p.SourceFilter != nil {
		sf := strings.TrimSpace(*p.SourceFilter)
		if sf != "" {
			switch strings.ToLower(sf) {
			case "gmail":
				add("source = ?", "gmail")
			case "imap":
				conds = append(conds, "(source IS NULL OR source <> 'gmail')")
			default:
				add("source = ?", sf)
			}
		}
	}
	// Always exclude soft-deleted rows
	conds = append(conds, "user_deleted = FALSE")

	if uid > 0 {
		args = append(args, uid)
		conds = append(conds, fmt.Sprintf("user_id = ?%d", len(args)))
	}

	sql := `SELECT id, uid, folder, subject, from_address, to_addresses, cc_addresses, bcc_addresses,
			       date, raw_message, plain_text, snippet, embedding,
			       has_attachments, user_deleted, is_personal, is_business, is_social, is_promotional,
			       is_spam, is_important, use_by_ai, source, created_at, updated_at
			FROM emails`
	if len(conds) > 0 {
		sql += " WHERE " + strings.Join(conds, " AND ")
	}
	sql += " ORDER BY date DESC"

	rows, err := r.pool.QueryContext(ctx, sql, args...)
	if err != nil {
		return nil, fmt.Errorf("email Search: %w", err)
	}
	defer func() { _ = rows.Close() }()
	return scanEmails(rows)
}

// GetByLabels returns non-deleted emails whose folder column matches any of the given labels.
// The folder column can be a comma-separated list of labels (e.g. "INBOX,IMPORTANT").
func (r *EmailRepo) GetByLabels(ctx context.Context, labels []string) ([]*model.Email, error) {
	if len(labels) == 0 {
		return nil, nil
	}

	uid := uidFromCtx(ctx)

	// Build OR conditions: exact match OR starts with "label," OR contains ",label," OR ends with ",label"
	var conds []string
	var args []any
	n := 1
	for _, label := range labels {
		conds = append(conds,
			fmt.Sprintf("(folder = ?%d OR folder LIKE ?%d OR folder LIKE ?%d OR folder LIKE ?%d)",
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
			       is_spam, is_important, use_by_ai, source, created_at, updated_at
			FROM emails
			WHERE (` + strings.Join(conds, " OR ") + `) AND user_deleted = FALSE`
	sql, args = addUIDFilter(sql, args, uid)

	rows, err := r.pool.QueryContext(ctx, sql, args...)
	if err != nil {
		return nil, fmt.Errorf("email GetByLabels: %w", err)
	}
	defer func() { _ = rows.Close() }()
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

	inCond, inArgs, _ := sqlutil.StringIN("source_reference", idStrs, 1)
	q := fmt.Sprintf(`
		SELECT source_reference, id
		FROM media_items
		WHERE source IN ('email_attachment', 'gmail_attachment')
		  AND %s
		ORDER BY id`, inCond)
	rows, err := r.pool.QueryContext(ctx, q, inArgs...)
	if err != nil {
		return nil, fmt.Errorf("getAttachmentIDsForEmails: %w", err)
	}
	defer func() { _ = rows.Close() }()

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
	uid := uidFromCtx(ctx)
	var sets []string
	var args []any
	n := 1

	addSet := func(col string, val any) {
		sets = append(sets, fmt.Sprintf("%s = ?%d", col, n))
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
		q := `SELECT EXISTS(SELECT 1 FROM emails WHERE id = ?1 AND user_deleted = FALSE`
		args2 := []any{id}
		q, args2 = addUIDFilter(q, args2, uid)
		q += ")"
		var exists bool
		err := r.pool.QueryRowContext(ctx, q, args2...).Scan(&exists)
		return exists, err
	}

	args = append(args, id)
	q := fmt.Sprintf(
		"UPDATE emails SET %s WHERE id = ?%d AND user_deleted = FALSE",
		strings.Join(sets, ", "), n,
	)
	n++
	args2 := args
	q, args2 = addUIDFilter(q, args2, uid)
	tag, err := r.pool.ExecContext(ctx, q, args2...)
	if err != nil {
		return false, fmt.Errorf("email Update %d: %w", id, err)
	}
	return rowsAffectedOrZero(tag) > 0, nil
}

// SoftDelete nullifies message content and marks the email as deleted.
// Returns false, nil when the email does not exist.
func (r *EmailRepo) SoftDelete(ctx context.Context, id int64) (bool, error) {
	uid := uidFromCtx(ctx)
	tx, err := r.pool.BeginTx(ctx, nil)
	if err != nil {
		return false, fmt.Errorf("email SoftDelete %d: %w", id, err)
	}
	defer func() { _ = tx.Rollback() }()

	q := `
		UPDATE emails
		SET raw_message      = NULL,
		    plain_text        = NULL,
		    snippet           = NULL,
		    embedding         = NULL,
		    has_attachments   = FALSE,
		    user_deleted      = TRUE
		WHERE id = ?1 AND user_deleted = FALSE`
	args := []any{id}
	q, args = addUIDFilter(q, args, uid)
	tag, err := tx.ExecContext(ctx, q, args...)
	if err != nil {
		return false, fmt.Errorf("email SoftDelete %d: %w", id, err)
	}
	if rowsAffectedOrZero(tag) == 0 {
		return false, nil
	}
	ref := fmt.Sprintf("%d", id)
	if err := sqlutil.DeleteEmailAttachmentMediaBySourceRefsTx(ctx, tx, []string{ref}); err != nil {
		return false, fmt.Errorf("email SoftDelete %d: attachment media: %w", id, err)
	}
	if err := tx.Commit(); err != nil {
		return false, fmt.Errorf("email SoftDelete %d: commit: %w", id, err)
	}
	return true, nil
}

// BulkSoftDelete soft-deletes a batch of emails by ID.
// Returns the count of rows actually deleted.
func (r *EmailRepo) BulkSoftDelete(ctx context.Context, ids []int64) (int64, error) {
	if len(ids) == 0 {
		return 0, nil
	}
	uid := uidFromCtx(ctx)
	tx, err := r.pool.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("email BulkSoftDelete: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	idCond, idArgs, nextArg := sqlutil.Int64IN("id", ids, 1)
	q := fmt.Sprintf(`
		UPDATE emails
		SET raw_message      = NULL,
		    plain_text        = NULL,
		    snippet           = NULL,
		    embedding         = NULL,
		    has_attachments   = FALSE,
		    user_deleted      = TRUE
		WHERE %s
		  AND user_deleted = FALSE`, idCond)
	args := idArgs
	if uid > 0 {
		args = append(args, uid)
		q += fmt.Sprintf(" AND user_id = ?%d", nextArg)
	}
	tag, err := tx.ExecContext(ctx, q, args...)
	if err != nil {
		return 0, fmt.Errorf("email BulkSoftDelete: %w", err)
	}
	refs := make([]string, len(ids))
	for i, id := range ids {
		refs[i] = fmt.Sprintf("%d", id)
	}
	if err := sqlutil.DeleteEmailAttachmentMediaBySourceRefsTx(ctx, tx, refs); err != nil {
		return 0, fmt.Errorf("email BulkSoftDelete attachment media: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("email BulkSoftDelete: commit: %w", err)
	}
	return rowsAffectedOrZero(tag), nil
}

// GetThreadEmails returns non-deleted emails where from_address or to_addresses
// contain the given participant address, ordered by date ASC.
func (r *EmailRepo) GetThreadEmails(ctx context.Context, participant string) ([]*model.Email, error) {
	uid := uidFromCtx(ctx)
	q := `
		SELECT id, uid, folder, subject, from_address, to_addresses, cc_addresses, bcc_addresses,
		       date, raw_message, plain_text, snippet, embedding,
		       has_attachments, user_deleted, is_personal, is_business, is_social, is_promotional,
		       is_spam, is_important, use_by_ai, source, created_at, updated_at
		FROM emails
		WHERE (from_address LIKE ?1 OR to_addresses LIKE ?1)
		  AND user_deleted = FALSE`
	args := []any{"%" + participant + "%"}
	q, args = addUIDFilter(q, args, uid)
	q += " ORDER BY date ASC"
	rows, err := r.pool.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("getThreadEmails: %w", err)
	}
	defer func() { _ = rows.Close() }()
	return scanEmails(rows)
}

// ListFolders returns distinct folder/label names stored across all emails.
// Folder is stored as comma-joined label names, so this unnests and deduplicates them.
func (r *EmailRepo) ListFolders(ctx context.Context) ([]string, error) {
	uid := uidFromCtx(ctx)

	if sqlutil.IsSQLite(ctx, r.pool) {
		// SQLite has no unnest/string_to_array; fetch raw folder strings and split in Go.
		q := `SELECT DISTINCT folder FROM emails WHERE user_deleted = FALSE AND folder IS NOT NULL AND folder != ''`
		args := []any{}
		q, args = addUIDFilter(q, args, uid)
		rows, err := r.pool.QueryContext(ctx, q, args...)
		if err != nil {
			return nil, fmt.Errorf("listFolders: %w", err)
		}
		defer func() { _ = rows.Close() }()
		seen := make(map[string]struct{})
		var folders []string
		for rows.Next() {
			var f string
			if err := rows.Scan(&f); err != nil {
				return nil, err
			}
			for _, part := range strings.Split(f, ",") {
				part = strings.TrimSpace(part)
				if part == "" {
					continue
				}
				if _, ok := seen[part]; !ok {
					seen[part] = struct{}{}
					folders = append(folders, part)
				}
			}
		}
		if err := rows.Err(); err != nil {
			return nil, err
		}
		sort.Strings(folders)
		return folders, nil
	}

	q := `
		SELECT DISTINCT unnest(string_to_array(folder, ',')) AS f
		FROM emails
		WHERE user_deleted = FALSE`
	args := []any{}
	q, args = addUIDFilter(q, args, uid)
	q += " ORDER BY f"
	rows, err := r.pool.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("listFolders: %w", err)
	}
	defer func() { _ = rows.Close() }()
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
			&e.IsSpam, &e.IsImportant, &e.UseByAI, &e.Source, &e.CreatedAt, &e.UpdatedAt,
		); err != nil {
			return nil, err
		}
		emails = append(emails, e)
	}
	return emails, rows.Err()
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
