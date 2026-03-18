package repository

import (
	"context"
	"fmt"
	"strings"

	"github.com/daveontour/digitalmuseum/internal/model"
	"github.com/jackc/pgx/v5/pgxpool"
)

// DashboardRepo runs the aggregate queries that power GET /api/dashboard.
type DashboardRepo struct {
	pool *pgxpool.Pool
}

// NewDashboardRepo creates a DashboardRepo.
func NewDashboardRepo(pool *pgxpool.Pool) *DashboardRepo {
	return &DashboardRepo{pool: pool}
}

// GetStats collects all raw dashboard statistics from the database.
// Queries are run sequentially; correctness is preferred over minimal latency
// for this admin-facing endpoint.
func (r *DashboardRepo) GetStats(ctx context.Context) (*model.DashboardRaw, error) {
	out := &model.DashboardRaw{
		MessageCounts:      make(map[string]int64),
		MessagesByYear:     make(map[int]int64),
		EmailsByYear:       make(map[int]int64),
		ContactsByCategory: make(map[string]int64),
		ImagesByRegion:     make(map[string]int64),
	}

	// ── Messages by service ─────────────────────────────────────────────────
	rows, err := r.pool.Query(ctx,
		`SELECT COALESCE(service, 'unknown'), COUNT(id) FROM messages GROUP BY service`)
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

	// ── Messages by year ────────────────────────────────────────────────────
	rows, err = r.pool.Query(ctx,
		`SELECT EXTRACT(year FROM message_date)::int, COUNT(id)
		 FROM messages
		 WHERE message_date IS NOT NULL
		 GROUP BY 1 ORDER BY 1`)
	if err != nil {
		return nil, fmt.Errorf("messages by year: %w", err)
	}
	for rows.Next() {
		var yr int
		var cnt int64
		if err := rows.Scan(&yr, &cnt); err != nil {
			rows.Close()
			return nil, err
		}
		out.MessagesByYear[yr] = cnt
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("messages by year scan: %w", err)
	}

	// ── Emails by year ──────────────────────────────────────────────────────
	rows, err = r.pool.Query(ctx,
		`SELECT EXTRACT(year FROM date)::int, COUNT(id)
		 FROM emails
		 WHERE date IS NOT NULL
		 GROUP BY 1 ORDER BY 1`)
	if err != nil {
		return nil, fmt.Errorf("emails by year: %w", err)
	}
	for rows.Next() {
		var yr int
		var cnt int64
		if err := rows.Scan(&yr, &cnt); err != nil {
			rows.Close()
			return nil, err
		}
		out.EmailsByYear[yr] = cnt
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("emails by year scan: %w", err)
	}

	// ── Top 100 senders (unfiltered — service layer removes subject names) ──
	rows, err = r.pool.Query(ctx,
		`SELECT sender_name, COUNT(id)
		 FROM messages
		 WHERE sender_name IS NOT NULL AND sender_name <> ''
		 GROUP BY sender_name
		 ORDER BY COUNT(id) DESC
		 LIMIT 100`)
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

	// ── Contacts count ──────────────────────────────────────────────────────
	if err := r.pool.QueryRow(ctx, `SELECT COUNT(id) FROM contacts`).Scan(&out.ContactsCount); err != nil {
		return nil, fmt.Errorf("contacts count: %w", err)
	}

	// ── Contacts by category ────────────────────────────────────────────────
	rows, err = r.pool.Query(ctx,
		`SELECT COALESCE(NULLIF(TRIM(rel_type), ''), 'unknown'), COUNT(id)
		 FROM contacts
		 GROUP BY 1`)
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

	// ── Image counts ────────────────────────────────────────────────────────
	if err := r.pool.QueryRow(ctx,
		`SELECT COUNT(id) FROM media_items WHERE media_type LIKE 'image/%'`,
	).Scan(&out.TotalImages); err != nil {
		return nil, fmt.Errorf("total images: %w", err)
	}
	if err := r.pool.QueryRow(ctx,
		`SELECT COUNT(id) FROM media_items WHERE media_type LIKE 'image/%' AND is_referenced = FALSE`,
	).Scan(&out.ImportedImages); err != nil {
		return nil, fmt.Errorf("imported images: %w", err)
	}
	if err := r.pool.QueryRow(ctx,
		`SELECT COUNT(id) FROM media_items WHERE media_type LIKE 'image/%' AND is_referenced = TRUE`,
	).Scan(&out.ReferenceImages); err != nil {
		return nil, fmt.Errorf("reference images: %w", err)
	}
	if err := r.pool.QueryRow(ctx,
		`SELECT COUNT(mi.id)
		 FROM media_items mi
		 JOIN media_blobs mb ON mb.id = mi.media_blob_id
		 WHERE mi.media_type LIKE 'image/%'
		   AND mb.thumbnail_data IS NOT NULL`,
	).Scan(&out.ThumbnailCount); err != nil {
		return nil, fmt.Errorf("thumbnail count: %w", err)
	}

	// ── Images by region ────────────────────────────────────────────────────
	rows, err = r.pool.Query(ctx,
		`SELECT COALESCE(NULLIF(TRIM(region), ''), 'Unknown'), COUNT(id)
		 FROM media_items
		 WHERE media_type LIKE 'image/%'
		 GROUP BY 1
		 ORDER BY COUNT(id) DESC`)
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

	// ── Simple scalar counts ────────────────────────────────────────────────
	scalars := []struct {
		dest  *int64
		query string
		label string
	}{
		{&out.FacebookAlbumsCount, `SELECT COUNT(id) FROM facebook_albums`, "facebook_albums"},
		{&out.FacebookPostsCount, `SELECT COUNT(id) FROM facebook_posts`, "facebook_posts"},
		{&out.LocationsCount, `SELECT COUNT(id) FROM locations`, "locations"},
		{&out.PlacesCount, `SELECT COUNT(id) FROM places`, "places"},
		{&out.EmailsCount, `SELECT COUNT(id) FROM emails`, "emails"},
		{&out.ArtefactsCount, `SELECT COUNT(id) FROM artefacts`, "artefacts"},
		{&out.ReferenceDocsCount, `SELECT COUNT(id) FROM reference_documents`, "reference_docs"},
		{&out.ReferenceDocsEnabled, `SELECT COUNT(id) FROM reference_documents WHERE available_for_task = TRUE`, "reference_docs_enabled"},
		{&out.CompleteProfilesCount, `SELECT COUNT(id) FROM complete_profiles`, "complete_profiles"},
	}
	for _, s := range scalars {
		if err := r.pool.QueryRow(ctx, s.query).Scan(s.dest); err != nil {
			return nil, fmt.Errorf("%s count: %w", s.label, err)
		}
	}

	return out, nil
}

// GetSubjectContactNames returns the names of contacts that should be treated as
// the subject person (contacts WHERE is_subject=TRUE plus the contact with id=0).
// Names are returned as-is (lowercasing is done in the service layer).
func (r *DashboardRepo) GetSubjectContactNames(ctx context.Context) ([]string, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT name FROM contacts WHERE (is_subject = TRUE OR id = 0) AND name IS NOT NULL`)
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
	var profile *string
	err := r.pool.QueryRow(ctx,
		`SELECT profile FROM complete_profiles
		 WHERE LOWER(name) = ANY($1) AND profile IS NOT NULL
		 LIMIT 1`, namesLower,
	).Scan(&profile)
	if err != nil {
		if isNoRows(err) {
			return false, nil
		}
		return false, fmt.Errorf("complete profile check: %w", err)
	}
	return profile != nil && strings.TrimSpace(*profile) != "", nil
}
