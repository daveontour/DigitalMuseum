package repository

import (
	"context"
	"database/sql"
	"errors"
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
				_ = rows.Close()
				return nil, err
			}
			out.MessageCounts[svc] = cnt
		}
		_ = rows.Close()
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
				_ = rows.Close()
				return nil, err
			}
			if yr.Valid && yr.Int64 > 0 {
				out.MessagesByYear[int(yr.Int64)] = cnt
			}
		}
		_ = rows.Close()
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
				_ = rows.Close()
				return nil, err
			}
			if yr.Valid && yr.Int64 > 0 {
				out.EmailsByYear[int(yr.Int64)] = cnt
			}
		}
		_ = rows.Close()
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
				_ = rows.Close()
				return nil, err
			}
			out.TopSenders = append(out.TopSenders, cc)
		}
		_ = rows.Close()
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
				_ = rows.Close()
				return nil, err
			}
			out.ContactsByCategory[cat] = cnt
		}
		_ = rows.Close()
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
				_ = rows.Close()
				return nil, err
			}
			out.ImagesByRegion[reg] = cnt
		}
		_ = rows.Close()
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
				_ = rows.Close()
				return nil, err
			}
			out.EmailsBySource[src] = cnt
		}
		_ = rows.Close()
		if err := rows.Err(); err != nil {
			return nil, fmt.Errorf("emails by source scan: %w", err)
		}
	}

	return out, nil
}

// GetImportModalStats collects aggregate counts for the import/maintenance modals.
// Embedding/searchable progress is intentionally not included here — it is expensive
// (see GetEmbeddingProgress) and is fetched separately so callers can render these
// cheap counts immediately.
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
				_ = rows.Close()
				return nil, err
			}
			out.MessageCounts[svc] = cnt
		}
		_ = rows.Close()
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
	{
		uidCond, args := makeUIDCond(nil)
		if err := r.pool.QueryRowContext(ctx,
			`SELECT COUNT(id) FROM media_items
			 WHERE media_type LIKE 'image/%'
			   AND latitude IS NOT NULL AND longitude IS NOT NULL`+uidCond, args...,
		).Scan(&out.GpsImagesCount); err != nil {
			return nil, fmt.Errorf("gps images count: %w", err)
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
				_ = rows.Close()
				return nil, err
			}
			out.EmailsBySource[src] = cnt
		}
		_ = rows.Close()
		if err := rows.Err(); err != nil {
			return nil, fmt.Errorf("emails by source scan: %w", err)
		}
	}

	return out, nil
}

// embeddingProgressSourceDef defines one embedding-progress source's "missing
// embedding" predicate. Mirrors the exact predicates used by the backfill jobs
// themselves (see runEmailEmbeddingBackfill / runMessageContextEmbeddingBackfill in
// importer_handler.go, runFacebookPostEmbeddingBackfill / runFacebookAlbumEmbeddingBackfill
// and ListMediaItemsForTagEmbeddingBackfill in image_handler.go / image_repo.go).
type embeddingProgressSourceDef struct {
	key          string
	totalQuery   string
	pendingExtra string
}

// EmbeddingProgressSourceKeys lists the valid `source` values for GetEmbeddingProgressForSource,
// in display order.
var EmbeddingProgressSourceKeys = []string{
	"message_context_embeddings",
	"email_embeddings",
	"facebook_post_text_embeddings",
	"facebook_album_description_embeddings",
	"image_tag_embeddings",
}

var embeddingProgressSources = map[string]embeddingProgressSourceDef{
	"message_context_embeddings": {
		key:          "message_context_embeddings",
		totalQuery:   `SELECT COUNT(id) FROM messages WHERE text IS NOT NULL`,
		pendingExtra: ` AND NOT EXISTS (SELECT 1 FROM message_embeddings me WHERE me.rowid = messages.id)`,
	},
	"email_embeddings": {
		key:          "email_embeddings",
		totalQuery:   `SELECT COUNT(id) FROM emails WHERE user_deleted = FALSE`,
		pendingExtra: ` AND NOT EXISTS (SELECT 1 FROM email_embeddings ee WHERE ee.rowid = emails.id)`,
	},
	"facebook_post_text_embeddings": {
		key:          "facebook_post_text_embeddings",
		totalQuery:   `SELECT COUNT(id) FROM facebook_posts WHERE TRIM(COALESCE(post_text, '')) != ''`,
		pendingExtra: ` AND NOT EXISTS (SELECT 1 FROM facebook_post_text_embeddings e WHERE e.rowid = facebook_posts.id)`,
	},
	"facebook_album_description_embeddings": {
		key:          "facebook_album_description_embeddings",
		totalQuery:   `SELECT COUNT(id) FROM facebook_albums WHERE TRIM(COALESCE(description, '')) != ''`,
		pendingExtra: ` AND NOT EXISTS (SELECT 1 FROM facebook_album_description_embeddings e WHERE e.rowid = facebook_albums.id)`,
	},
	"image_tag_embeddings": {
		key:          "image_tag_embeddings",
		totalQuery:   `SELECT COUNT(id) FROM media_items WHERE tags IS NOT NULL AND TRIM(tags) != ''`,
		pendingExtra: ` AND require_classification = TRUE`,
	},
}

// ErrUnknownEmbeddingSource is returned by GetEmbeddingProgressForSource when the
// requested key isn't one of EmbeddingProgressSourceKeys.
var ErrUnknownEmbeddingSource = errors.New("unknown embedding progress source")

// GetEmbeddingProgressForSource reports how much content of one source still needs an
// AI embedding to be searchable.
//
// This is expensive: the embedding tables are sqlite-vec vec0 virtual tables, and a
// correlated NOT EXISTS predicate evaluated per row of the base table (messages,
// emails, ...) against a vec0 table is effectively a per-row scan. Callers should
// fetch each source independently (rather than in one batched call) so a single
// slow or failing source doesn't hold up the others, and fetch separately from
// GetImportModalStats so cheap counts can render first.
func (r *DashboardRepo) GetEmbeddingProgressForSource(ctx context.Context, key string) (model.EmbeddingProgressEntry, error) {
	s, ok := embeddingProgressSources[key]
	if !ok {
		return model.EmbeddingProgressEntry{}, ErrUnknownEmbeddingSource
	}

	uid := uidFromCtx(ctx)
	makeUIDCond := func(baseArgs []any) (string, []any) {
		if uid == 0 {
			return "", baseArgs
		}
		baseArgs = append(baseArgs, uid)
		return fmt.Sprintf(" AND user_id = ?%d", len(baseArgs)), baseArgs
	}

	var entry model.EmbeddingProgressEntry
	totalCond, totalArgs := makeUIDCond(nil)
	if err := r.pool.QueryRowContext(ctx, s.totalQuery+totalCond, totalArgs...).Scan(&entry.Total); err != nil {
		return model.EmbeddingProgressEntry{}, fmt.Errorf("%s total: %w", s.key, err)
	}
	pendingCond, pendingArgs := makeUIDCond(nil)
	if err := r.pool.QueryRowContext(ctx, s.totalQuery+pendingCond+s.pendingExtra, pendingArgs...).Scan(&entry.Pending); err != nil {
		return model.EmbeddingProgressEntry{}, fmt.Errorf("%s pending: %w", s.key, err)
	}
	return entry, nil
}

// GetArchiveDataInventory returns entry counts per archive data type for conversational AI context.
func (r *DashboardRepo) GetArchiveDataInventory(ctx context.Context) (*model.ArchiveDataInventory, error) {
	stats, err := r.GetImportModalStats(ctx)
	if err != nil {
		return nil, err
	}

	uid := uidFromCtx(ctx)
	makeUIDCond := func(baseArgs []any) (string, []any) {
		if uid == 0 {
			return "", baseArgs
		}
		baseArgs = append(baseArgs, uid)
		return fmt.Sprintf(" AND user_id = ?%d", len(baseArgs)), baseArgs
	}

	var emailsTotal, places, artefacts int64
	{
		uidCond, args := makeUIDCond(nil)
		scalars := []struct {
			dest  *int64
			query string
			label string
		}{
			{&emailsTotal, `SELECT COUNT(id) FROM emails WHERE TRUE` + uidCond, "emails_total"},
			{&places, `SELECT COUNT(id) FROM places WHERE TRUE` + uidCond, "places"},
			{&artefacts, `SELECT COUNT(id) FROM artefacts WHERE TRUE` + uidCond, "artefacts"},
		}
		for _, s := range scalars {
			if err := r.pool.QueryRowContext(ctx, s.query, args...).Scan(s.dest); err != nil {
				return nil, fmt.Errorf("%s count: %w", s.label, err)
			}
		}
	}

	messagesByService := stats.MessageCounts
	if messagesByService == nil {
		messagesByService = make(map[string]int64)
	}
	emailsBySource := stats.EmailsBySource
	if emailsBySource == nil {
		emailsBySource = make(map[string]int64)
	}

	return &model.ArchiveDataInventory{
		MessagesByService:  messagesByService,
		EmailsTotal:        emailsTotal,
		EmailsBySource:     emailsBySource,
		ImagesTotal:        stats.TotalImages,
		ImagesInDatabase:   stats.ImportedImages,
		ImagesLinkedOnDisk: stats.FilesystemImagesReferencedCount,
		FacebookAlbums:     stats.FacebookAlbumsCount,
		FacebookPosts:      stats.FacebookPostsCount,
		Locations:          stats.LocationsCount,
		Places:             places,
		Contacts:           stats.ContactsCount,
		Artefacts:          artefacts,
		ReferenceDocuments: stats.ReferenceDocsCount,
	}, nil
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
	defer func() { _ = rows.Close() }()
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
