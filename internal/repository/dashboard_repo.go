package repository

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/daveontour/aimuseum/internal/model"
)

// DashboardRepo runs the aggregate queries that power GET /api/dashboard.
type DashboardRepo struct {
	pool *sql.DB
}

// NewDashboardRepo creates a DashboardRepo.
func NewDashboardRepo(pool *sql.DB) *DashboardRepo {
	return &DashboardRepo{pool: pool}
}

// GetStats collects all raw dashboard statistics from the database.
// Queries are run sequentially; correctness is preferred over minimal latency
// for this admin-facing endpoint.
func (r *DashboardRepo) GetStats(ctx context.Context) (*model.DashboardRaw, error) {
	uid := uidFromCtx(ctx)

	// uidCond returns "AND user_id = $N" when uid > 0, with the arg appended.
	// baseArgs is the starting args slice; returns updated args and the condition fragment.
	makeUIDCond := func(baseArgs []any) (string, []any) {
		if uid == 0 {
			return "", baseArgs
		}
		baseArgs = append(baseArgs, uid)
		return fmt.Sprintf(" AND user_id = ?%d", len(baseArgs)), baseArgs
	}

	out := &model.DashboardRaw{
		MessageCounts:      make(map[string]int64),
		MessagesByYear:     make(map[int]int64),
		EmailsByYear:       make(map[int]int64),
		ContactsByCategory: make(map[string]int64),
		ImagesByRegion:     make(map[string]int64),
		EmailsBySource:     make(map[string]int64),
	}

	// ── Messages by service ─────────────────────────────────────────────────
	{
		uidCond, args := makeUIDCond(nil)
		rows, err := r.pool.QueryContext(ctx,
			`SELECT COALESCE(service, 'unknown'), COUNT(id) FROM messages WHERE TRUE`+uidCond+` GROUP BY service`,
			args...)
		if err != nil {
			return nil, fmt.Errorf("messages by service: %w", err)
		}
		for rows.Next() {
			var svc string
			var cnt int64
			if err := rows.Scan(&svc, &cnt); err != nil {
				rows.Close()
				return nil, err
			}
			out.MessageCounts[svc] = cnt
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return nil, fmt.Errorf("messages by service scan: %w", err)
		}
	}

	// ── Messages by year ────────────────────────────────────────────────────
	// strftime: SQLite (TEXT datetimes) and portable; avoids PostgreSQL-only EXTRACT.
	{
		uidCond, args := makeUIDCond(nil)
		rows, err := r.pool.QueryContext(ctx,
			`SELECT CAST(strftime('%Y', message_date) AS INTEGER), COUNT(id)
			 FROM messages
			 WHERE message_date IS NOT NULL`+uidCond+`
			 GROUP BY 1 ORDER BY 1`,
			args...)
		if err != nil {
			return nil, fmt.Errorf("messages by year: %w", err)
		}
		for rows.Next() {
			var yr sql.NullInt64
			var cnt int64
			if err := rows.Scan(&yr, &cnt); err != nil {
				rows.Close()
				return nil, err
			}
			if yr.Valid && yr.Int64 > 0 {
				out.MessagesByYear[int(yr.Int64)] = cnt
			}
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return nil, fmt.Errorf("messages by year scan: %w", err)
		}
	}

	// ── Emails by year ──────────────────────────────────────────────────────
	{
		uidCond, args := makeUIDCond(nil)
		rows, err := r.pool.QueryContext(ctx,
			`SELECT CAST(strftime('%Y', date) AS INTEGER), COUNT(id)
			 FROM emails
			 WHERE date IS NOT NULL`+uidCond+`
			 GROUP BY 1 ORDER BY 1`,
			args...)
		if err != nil {
			return nil, fmt.Errorf("emails by year: %w", err)
		}
		for rows.Next() {
			var yr sql.NullInt64
			var cnt int64
			if err := rows.Scan(&yr, &cnt); err != nil {
				rows.Close()
				return nil, err
			}
			if yr.Valid && yr.Int64 > 0 {
				out.EmailsByYear[int(yr.Int64)] = cnt
			}
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return nil, fmt.Errorf("emails by year scan: %w", err)
		}
	}

	// ── Top 100 senders (unfiltered — service layer removes subject names) ──
	{
		uidCond, args := makeUIDCond(nil)
		rows, err := r.pool.QueryContext(ctx,
			`SELECT sender_name, COUNT(id)
			 FROM messages
			 WHERE sender_name IS NOT NULL AND sender_name <> ''`+uidCond+`
			 GROUP BY sender_name
			 ORDER BY COUNT(id) DESC
			 LIMIT 100`,
			args...)
		if err != nil {
			return nil, fmt.Errorf("top senders: %w", err)
		}
		for rows.Next() {
			var cc model.ContactCount
			if err := rows.Scan(&cc.Name, &cc.Count); err != nil {
				rows.Close()
				return nil, err
			}
			out.TopSenders = append(out.TopSenders, cc)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return nil, fmt.Errorf("top senders scan: %w", err)
		}
	}

	// ── Contacts count ──────────────────────────────────────────────────────
	{
		uidCond, args := makeUIDCond(nil)
		if err := r.pool.QueryRowContext(ctx,
			`SELECT COUNT(id) FROM contacts WHERE TRUE`+uidCond, args...,
		).Scan(&out.ContactsCount); err != nil {
			return nil, fmt.Errorf("contacts count: %w", err)
		}
	}

	// ── Contacts by category ────────────────────────────────────────────────
	{
		uidCond, args := makeUIDCond(nil)
		rows, err := r.pool.QueryContext(ctx,
			`SELECT COALESCE(NULLIF(TRIM(rel_type), ''), 'unknown'), COUNT(id)
			 FROM contacts
			 WHERE TRUE`+uidCond+`
			 GROUP BY 1`,
			args...)
		if err != nil {
			return nil, fmt.Errorf("contacts by category: %w", err)
		}
		for rows.Next() {
			var cat string
			var cnt int64
			if err := rows.Scan(&cat, &cnt); err != nil {
				rows.Close()
				return nil, err
			}
			out.ContactsByCategory[cat] = cnt
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return nil, fmt.Errorf("contacts by category scan: %w", err)
		}
	}

	// ── Image counts ────────────────────────────────────────────────────────
	{
		uidCond, args := makeUIDCond(nil)
		if err := r.pool.QueryRowContext(ctx,
			`SELECT COUNT(id) FROM media_items WHERE media_type LIKE 'image/%'`+uidCond, args...,
		).Scan(&out.TotalImages); err != nil {
			return nil, fmt.Errorf("total images: %w", err)
		}
	}
	{
		uidCond, args := makeUIDCond(nil)
		// Count all filesystem-sourced rows (browser upload and directory import). Do not require
		// media_type LIKE 'image/%' — uploads often store application/octet-stream until sniffed.
		if err := r.pool.QueryRowContext(ctx,
			`SELECT COUNT(id) FROM media_items WHERE source = 'filesystem'`+uidCond, args...,
		).Scan(&out.FilesystemImagesCount); err != nil {
			return nil, fmt.Errorf("filesystem images: %w", err)
		}
	}
	{
		uidCond, args := makeUIDCond(nil)
		if err := r.pool.QueryRowContext(ctx,
			`SELECT COUNT(id) FROM media_items WHERE source = 'filesystem' AND is_referenced = FALSE`+uidCond, args...,
		).Scan(&out.FilesystemImagesEmbeddedCount); err != nil {
			return nil, fmt.Errorf("filesystem embedded images: %w", err)
		}
	}
	{
		uidCond, args := makeUIDCond(nil)
		if err := r.pool.QueryRowContext(ctx,
			`SELECT COUNT(id) FROM media_items WHERE source = 'filesystem' AND is_referenced = TRUE`+uidCond, args...,
		).Scan(&out.FilesystemImagesReferencedCount); err != nil {
			return nil, fmt.Errorf("filesystem referenced images: %w", err)
		}
	}
	{
		uidCond, args := makeUIDCond(nil)
		if err := r.pool.QueryRowContext(ctx,
			`SELECT COUNT(id) FROM media_items WHERE media_type LIKE 'image/%' AND is_referenced = FALSE`+uidCond, args...,
		).Scan(&out.ImportedImages); err != nil {
			return nil, fmt.Errorf("imported images: %w", err)
		}
	}
	{
		uidCond, args := makeUIDCond(nil)
		if err := r.pool.QueryRowContext(ctx,
			`SELECT COUNT(id) FROM media_items WHERE media_type LIKE 'image/%' AND is_referenced = TRUE`+uidCond, args...,
		).Scan(&out.ReferenceImages); err != nil {
			return nil, fmt.Errorf("reference images: %w", err)
		}
	}
	{
		// Must use qualified alias — both media_items and media_blobs have user_id
		var uidCond string
		var args []any
		if uid > 0 {
			args = append(args, uid)
			uidCond = fmt.Sprintf(" AND mi.user_id = ?%d", len(args))
		}
		if err := r.pool.QueryRowContext(ctx,
			`SELECT COUNT(mi.id)
			 FROM media_items mi
			 JOIN media_blobs mb ON mb.id = mi.media_blob_id
			 WHERE mi.media_type LIKE 'image/%'
			   AND mb.thumbnail_data IS NOT NULL`+uidCond, args...,
		).Scan(&out.ThumbnailCount); err != nil {
			return nil, fmt.Errorf("thumbnail count: %w", err)
		}
	}

	// ── Images by region ────────────────────────────────────────────────────
	{
		uidCond, args := makeUIDCond(nil)
		rows, err := r.pool.QueryContext(ctx,
			`SELECT COALESCE(NULLIF(TRIM(region), ''), 'Unknown'), COUNT(id)
			 FROM media_items
			 WHERE media_type LIKE 'image/%'`+uidCond+`
			 GROUP BY 1
			 ORDER BY COUNT(id) DESC`,
			args...)
		if err != nil {
			return nil, fmt.Errorf("images by region: %w", err)
		}
		for rows.Next() {
			var reg string
			var cnt int64
			if err := rows.Scan(&reg, &cnt); err != nil {
				rows.Close()
				return nil, err
			}
			out.ImagesByRegion[reg] = cnt
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return nil, fmt.Errorf("images by region scan: %w", err)
		}
	}

	// ── Simple scalar counts ────────────────────────────────────────────────
	{
		uidCond, args := makeUIDCond(nil)
		scalars := []struct {
			dest  *int64
			query string
			label string
		}{
			{&out.FacebookAlbumsCount, `SELECT COUNT(id) FROM facebook_albums WHERE TRUE` + uidCond, "facebook_albums"},
			{&out.FacebookPostsCount, `SELECT COUNT(id) FROM facebook_posts WHERE TRUE` + uidCond, "facebook_posts"},
			{&out.LocationsCount, `SELECT COUNT(id) FROM locations WHERE TRUE` + uidCond, "locations"},
			{&out.PlacesCount, `SELECT COUNT(id) FROM places WHERE TRUE` + uidCond, "places"},
			{&out.EmailsCount, `SELECT COUNT(id) FROM emails WHERE TRUE` + uidCond, "emails"},
			{&out.ArtefactsCount, `SELECT COUNT(id) FROM artefacts WHERE TRUE` + uidCond, "artefacts"},
			{&out.ReferenceDocsCount, `SELECT COUNT(id) FROM reference_documents WHERE TRUE` + uidCond, "reference_docs"},
			{&out.ReferenceDocsEnabled, `SELECT COUNT(id) FROM reference_documents WHERE available_for_task = TRUE` + uidCond, "reference_docs_enabled"},
			{&out.CompleteProfilesCount, `SELECT COUNT(id) FROM complete_profiles WHERE NOT generation_pending` + uidCond, "complete_profiles"},
		}
		for _, s := range scalars {
			if err := r.pool.QueryRowContext(ctx, s.query, args...).Scan(s.dest); err != nil {
				return nil, fmt.Errorf("%s count: %w", s.label, err)
			}
		}
	}

	// ── Emails by import source (gmail, IMAP hostname, legacy, …) ─────────────
	{
		uidCond, args := makeUIDCond(nil)
		rows, err := r.pool.QueryContext(ctx,
			`SELECT COALESCE(NULLIF(TRIM(source), ''), 'legacy'), COUNT(id) FROM emails WHERE TRUE`+uidCond+` GROUP BY 1`,
			args...)
		if err != nil {
			return nil, fmt.Errorf("emails by source: %w", err)
		}
		for rows.Next() {
			var src string
			var cnt int64
			if err := rows.Scan(&src, &cnt); err != nil {
				rows.Close()
				return nil, err
			}
			out.EmailsBySource[src] = cnt
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return nil, fmt.Errorf("emails by source scan: %w", err)
		}
	}

	return out, nil
}

// GetImportModalStats collects only the aggregate counts needed by the Import & Manage Data modal.
func (r *DashboardRepo) GetImportModalStats(ctx context.Context) (*model.ImportModalStatsResponse, error) {
	uid := uidFromCtx(ctx)

	makeUIDCond := func(baseArgs []any) (string, []any) {
		if uid == 0 {
			return "", baseArgs
		}
		baseArgs = append(baseArgs, uid)
		return fmt.Sprintf(" AND user_id = ?%d", len(baseArgs)), baseArgs
	}

	out := &model.ImportModalStatsResponse{
		MessageCounts:  make(map[string]int64),
		EmailsBySource: make(map[string]int64),
	}

	// Messages by service (WhatsApp, Instagram, iMessage, …)
	{
		uidCond, args := makeUIDCond(nil)
		rows, err := r.pool.QueryContext(ctx,
			`SELECT COALESCE(service, 'unknown'), COUNT(id) FROM messages WHERE TRUE`+uidCond+` GROUP BY service`,
			args...)
		if err != nil {
			return nil, fmt.Errorf("messages by service: %w", err)
		}
		for rows.Next() {
			var svc string
			var cnt int64
			if err := rows.Scan(&svc, &cnt); err != nil {
				rows.Close()
				return nil, err
			}
			out.MessageCounts[svc] = cnt
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return nil, fmt.Errorf("messages by service scan: %w", err)
		}
	}

	// Contacts count
	{
		uidCond, args := makeUIDCond(nil)
		if err := r.pool.QueryRowContext(ctx,
			`SELECT COUNT(id) FROM contacts WHERE TRUE`+uidCond, args...,
		).Scan(&out.ContactsCount); err != nil {
			return nil, fmt.Errorf("contacts count: %w", err)
		}
	}

	// Image counts
	{
		uidCond, args := makeUIDCond(nil)
		if err := r.pool.QueryRowContext(ctx,
			`SELECT COUNT(id) FROM media_items WHERE media_type LIKE 'image/%'`+uidCond, args...,
		).Scan(&out.TotalImages); err != nil {
			return nil, fmt.Errorf("total images: %w", err)
		}
	}
	{
		uidCond, args := makeUIDCond(nil)
		if err := r.pool.QueryRowContext(ctx,
			`SELECT COUNT(id) FROM media_items WHERE source = 'filesystem' AND is_referenced = FALSE`+uidCond, args...,
		).Scan(&out.FilesystemImagesEmbeddedCount); err != nil {
			return nil, fmt.Errorf("filesystem embedded images: %w", err)
		}
	}
	{
		uidCond, args := makeUIDCond(nil)
		if err := r.pool.QueryRowContext(ctx,
			`SELECT COUNT(id) FROM media_items WHERE source = 'filesystem' AND is_referenced = TRUE`+uidCond, args...,
		).Scan(&out.FilesystemImagesReferencedCount); err != nil {
			return nil, fmt.Errorf("filesystem referenced images: %w", err)
		}
	}
	{
		uidCond, args := makeUIDCond(nil)
		if err := r.pool.QueryRowContext(ctx,
			`SELECT COUNT(id) FROM media_items WHERE media_type LIKE 'image/%' AND is_referenced = FALSE`+uidCond, args...,
		).Scan(&out.ImportedImages); err != nil {
			return nil, fmt.Errorf("imported images: %w", err)
		}
	}
	{
		uidCond, args := makeUIDCond(nil)
		if err := r.pool.QueryRowContext(ctx,
			`SELECT COUNT(id) FROM media_items WHERE media_type LIKE 'image/%' AND is_referenced = TRUE`+uidCond, args...,
		).Scan(&out.ReferenceImages); err != nil {
			return nil, fmt.Errorf("reference images: %w", err)
		}
	}
	{
		var uidCond string
		var args []any
		if uid > 0 {
			args = append(args, uid)
			uidCond = fmt.Sprintf(" AND mi.user_id = ?%d", len(args))
		}
		if err := r.pool.QueryRowContext(ctx,
			`SELECT COUNT(mi.id)
			 FROM media_items mi
			 JOIN media_blobs mb ON mb.id = mi.media_blob_id
			 WHERE mi.media_type LIKE 'image/%'
			   AND mb.thumbnail_data IS NOT NULL`+uidCond, args...,
		).Scan(&out.ThumbnailCount); err != nil {
			return nil, fmt.Errorf("thumbnail count: %w", err)
		}
	}

	// Facebook / locations / reference docs
	{
		uidCond, args := makeUIDCond(nil)
		scalars := []struct {
			dest  *int64
			query string
			label string
		}{
			{&out.FacebookAlbumsCount, `SELECT COUNT(id) FROM facebook_albums WHERE TRUE` + uidCond, "facebook_albums"},
			{&out.FacebookPostsCount, `SELECT COUNT(id) FROM facebook_posts WHERE TRUE` + uidCond, "facebook_posts"},
			{&out.LocationsCount, `SELECT COUNT(id) FROM locations WHERE TRUE` + uidCond, "locations"},
			{&out.ReferenceDocsCount, `SELECT COUNT(id) FROM reference_documents WHERE TRUE` + uidCond, "reference_docs"},
		}
		for _, s := range scalars {
			if err := r.pool.QueryRowContext(ctx, s.query, args...).Scan(s.dest); err != nil {
				return nil, fmt.Errorf("%s count: %w", s.label, err)
			}
		}
	}

	// Emails by import source (gmail, IMAP hostname, legacy, …)
	{
		uidCond, args := makeUIDCond(nil)
		rows, err := r.pool.QueryContext(ctx,
			`SELECT COALESCE(NULLIF(TRIM(source), ''), 'legacy'), COUNT(id) FROM emails WHERE TRUE`+uidCond+` GROUP BY 1`,
			args...)
		if err != nil {
			return nil, fmt.Errorf("emails by source: %w", err)
		}
		for rows.Next() {
			var src string
			var cnt int64
			if err := rows.Scan(&src, &cnt); err != nil {
				rows.Close()
				return nil, err
			}
			out.EmailsBySource[src] = cnt
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return nil, fmt.Errorf("emails by source scan: %w", err)
		}
	}

	return out, nil
}

// GetSubjectContactNames returns the names of contacts that should be treated as
// the subject person (contacts WHERE is_subject=TRUE plus the contact with id=0).
// Names are returned as-is (lowercasing is done in the service layer).
func (r *DashboardRepo) GetSubjectContactNames(ctx context.Context) ([]string, error) {
	uid := uidFromCtx(ctx)
	q := `SELECT name FROM contacts WHERE (is_subject = TRUE OR id = 0) AND name IS NOT NULL`
	args := []any{}
	q, args = addUIDFilter(q, args, uid)
	rows, err := r.pool.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("subject contact names: %w", err)
	}
	defer rows.Close()
	var names []string
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			return nil, err
		}
		if strings.TrimSpace(n) != "" {
			names = append(names, n)
		}
	}
	return names, rows.Err()
}

// HasCompleteProfileForNames returns true if any row in complete_profiles has a
// non-empty profile whose lowercased name matches one of the provided names.
// namesLower must already be lower-cased.
func (r *DashboardRepo) HasCompleteProfileForNames(ctx context.Context, namesLower []string) (bool, error) {
	if len(namesLower) == 0 {
		return false, nil
	}
	uid := uidFromCtx(ctx)
	ph := make([]string, len(namesLower))
	args := make([]any, 0, len(namesLower)+1)
	for i, n := range namesLower {
		ph[i] = "?"
		args = append(args, n)
	}
	q := `SELECT profile FROM complete_profiles
	      WHERE LOWER(name) IN (` + strings.Join(ph, ",") + `) AND profile IS NOT NULL AND NOT generation_pending`
	q, args = addUIDFilter(q, args, uid)
	q += ` LIMIT 1`
	var profile *string
	err := r.pool.QueryRowContext(ctx, q, args...).Scan(&profile)
	if err != nil {
		if isNoRows(err) {
			return false, nil
		}
		return false, fmt.Errorf("complete profile check: %w", err)
	}
	return profile != nil && strings.TrimSpace(*profile) != "", nil
}
