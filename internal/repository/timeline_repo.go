package repository

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/daveontour/aimuseum/internal/model"
)

// TimelineRepo reads cross-source, date-ordered views of the archive (timeline + quick search).
//
// Dates are stored as TEXT starting "YYYY-MM-DD" (whatever the rest of the format — see
// sqlutil.ParseSQLiteDatetime), so months/days are taken with substr() rather than strftime(),
// which would shift values with a timezone suffix into UTC. Rows whose date doesn't start with a
// plausible year are ignored (e.g. Go's zero time "0001-01-01").
type TimelineRepo struct {
	pool *sql.DB
}

// NewTimelineRepo creates a TimelineRepo.
func NewTimelineRepo(pool *sql.DB) *TimelineRepo {
	return &TimelineRepo{pool: pool}
}

// TimelineSources says which sources the caller may see.
type TimelineSources struct {
	Messages bool
	Emails   bool
}

const (
	timelineDateGlob     = `'[12][0-9][0-9][0-9]-[01][0-9]*'`
	timelineMinMonth     = "1900-01"
	timelineMaxMonth     = "2100-12"
	timelineMaxConvDays  = 500
	timelineMaxEmails    = 300
	timelineMaxPhotos    = 300
	timelineMaxPosts     = 200
	quickSearchPerKind   = 8
	quickSearchSnippet   = 70
	quickSearchMaxQuery  = 200
	quickSearchMinLength = 2
)

// MonthlyCounts returns per-month counts for every month that has anything, oldest first.
func (r *TimelineRepo) MonthlyCounts(ctx context.Context, src TimelineSources) ([]model.TimelineMonth, error) {
	uid := uidFromCtx(ctx)
	byMonth := map[string]*model.TimelineMonth{}
	get := func(m string) *model.TimelineMonth {
		t := byMonth[m]
		if t == nil {
			t = &model.TimelineMonth{Month: m}
			byMonth[m] = t
		}
		return t
	}
	collect := func(q string, args []any, set func(*model.TimelineMonth, int64)) error {
		rows, err := r.pool.QueryContext(ctx, q, args...)
		if err != nil {
			return err
		}
		defer func() { _ = rows.Close() }()
		for rows.Next() {
			var m sql.NullString
			var n int64
			if err := rows.Scan(&m, &n); err != nil {
				return err
			}
			if !m.Valid || m.String < timelineMinMonth || m.String > timelineMaxMonth {
				continue
			}
			set(get(m.String), n)
		}
		return rows.Err()
	}

	if src.Messages {
		q, args := addUIDFilter(`SELECT substr(message_date, 1, 7), COUNT(*) FROM messages
			WHERE message_date GLOB `+timelineDateGlob, nil, uid)
		if err := collect(q+` GROUP BY 1`, args, func(t *model.TimelineMonth, n int64) { t.Messages = n }); err != nil {
			return nil, fmt.Errorf("timeline messages: %w", err)
		}
	}
	if src.Emails {
		q, args := addUIDFilter(`SELECT substr(date, 1, 7), COUNT(*) FROM emails
			WHERE COALESCE(user_deleted, 0) = 0 AND date GLOB `+timelineDateGlob, nil, uid)
		if err := collect(q+` GROUP BY 1`, args, func(t *model.TimelineMonth, n int64) { t.Emails = n }); err != nil {
			return nil, fmt.Errorf("timeline emails: %w", err)
		}
	}
	{
		q, args := addUIDFilter(`SELECT printf('%04d-%02d', year, month), COUNT(*) FROM media_items
			WHERE media_type LIKE 'image/%' AND year > 0 AND month BETWEEN 1 AND 12`, nil, uid)
		if err := collect(q+` GROUP BY year, month`, args, func(t *model.TimelineMonth, n int64) { t.Photos = n }); err != nil {
			return nil, fmt.Errorf("timeline photos: %w", err)
		}
	}
	{
		q, args := addUIDFilter(`SELECT substr(timestamp, 1, 7), COUNT(*) FROM facebook_posts
			WHERE timestamp GLOB `+timelineDateGlob, nil, uid)
		if err := collect(q+` GROUP BY 1`, args, func(t *model.TimelineMonth, n int64) { t.Posts = n }); err != nil {
			return nil, fmt.Errorf("timeline posts: %w", err)
		}
	}

	out := make([]model.TimelineMonth, 0, len(byMonth))
	for _, t := range byMonth {
		out = append(out, *t)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Month < out[j].Month })
	return out, nil
}

func monthBounds(year, month int) (lo, hi string) {
	lo = fmt.Sprintf("%04d-%02d", year, month)
	ny, nm := year, month+1
	if nm > 12 {
		ny, nm = year+1, 1
	}
	hi = fmt.Sprintf("%04d-%02d", ny, nm)
	return lo, hi
}

// MonthDetail returns the items for one month, grouped for display.
func (r *TimelineRepo) MonthDetail(ctx context.Context, year, month int, src TimelineSources) (*model.TimelineMonthDetail, error) {
	if year < 1900 || year > 2100 || month < 1 || month > 12 {
		return nil, fmt.Errorf("invalid month %04d-%02d", year, month)
	}
	uid := uidFromCtx(ctx)
	lo, hi := monthBounds(year, month)
	out := &model.TimelineMonthDetail{
		Month:         lo,
		Conversations: []model.TimelineConversationDay{},
		Emails:        []model.TimelineEmail{},
		Photos:        []model.TimelinePhoto{},
		Posts:         []model.TimelinePost{},
		Totals:        model.TimelineMonth{Month: lo},
		Limits: map[string]int{
			"conversations": timelineMaxConvDays,
			"emails":        timelineMaxEmails,
			"photos":        timelineMaxPhotos,
			"posts":         timelineMaxPosts,
		},
		Hidden: []string{},
	}

	if src.Messages {
		q, args := addUIDFilter(`SELECT substr(message_date, 1, 10), COALESCE(chat_session, ''), COUNT(*), MIN(id)
			FROM messages WHERE message_date >= ? AND message_date < ?`, []any{lo, hi}, uid)
		q += ` GROUP BY 1, 2 ORDER BY 1, 3 DESC LIMIT ?`
		args = append(args, timelineMaxConvDays)
		rows, err := r.pool.QueryContext(ctx, q, args...)
		if err != nil {
			return nil, fmt.Errorf("timeline month messages: %w", err)
		}
		for rows.Next() {
			var c model.TimelineConversationDay
			if err := rows.Scan(&c.Date, &c.ChatSession, &c.Count, &c.FirstMessageID); err != nil {
				_ = rows.Close()
				return nil, err
			}
			out.Conversations = append(out.Conversations, c)
		}
		_ = rows.Close()
		if err := rows.Err(); err != nil {
			return nil, err
		}
		cq, cargs := addUIDFilter(`SELECT COUNT(*) FROM messages WHERE message_date >= ? AND message_date < ?`, []any{lo, hi}, uid)
		if err := r.pool.QueryRowContext(ctx, cq, cargs...).Scan(&out.Totals.Messages); err != nil {
			return nil, err
		}
	} else {
		out.Hidden = append(out.Hidden, "messages")
	}

	if src.Emails {
		q, args := addUIDFilter(`SELECT id, substr(date, 1, 10), substr(date, 12, 5), COALESCE(subject, ''), COALESCE(from_address, '')
			FROM emails WHERE COALESCE(user_deleted, 0) = 0 AND date >= ? AND date < ?`, []any{lo, hi}, uid)
		q += ` ORDER BY date LIMIT ?`
		args = append(args, timelineMaxEmails)
		rows, err := r.pool.QueryContext(ctx, q, args...)
		if err != nil {
			return nil, fmt.Errorf("timeline month emails: %w", err)
		}
		for rows.Next() {
			var e model.TimelineEmail
			if err := rows.Scan(&e.ID, &e.Date, &e.Time, &e.Subject, &e.From); err != nil {
				_ = rows.Close()
				return nil, err
			}
			out.Emails = append(out.Emails, e)
		}
		_ = rows.Close()
		if err := rows.Err(); err != nil {
			return nil, err
		}
		cq, cargs := addUIDFilter(`SELECT COUNT(*) FROM emails WHERE COALESCE(user_deleted, 0) = 0 AND date >= ? AND date < ?`, []any{lo, hi}, uid)
		if err := r.pool.QueryRowContext(ctx, cq, cargs...).Scan(&out.Totals.Emails); err != nil {
			return nil, err
		}
	} else {
		out.Hidden = append(out.Hidden, "emails")
	}

	{
		q, args := addUIDFilter(`SELECT id, COALESCE(day, 0), COALESCE(title, '')
			FROM media_items WHERE media_type LIKE 'image/%' AND year = ? AND month = ?`, []any{year, month}, uid)
		q += ` ORDER BY COALESCE(day, 0), id LIMIT ?`
		args = append(args, timelineMaxPhotos)
		rows, err := r.pool.QueryContext(ctx, q, args...)
		if err != nil {
			return nil, fmt.Errorf("timeline month photos: %w", err)
		}
		for rows.Next() {
			var p model.TimelinePhoto
			if err := rows.Scan(&p.ID, &p.Day, &p.Title); err != nil {
				_ = rows.Close()
				return nil, err
			}
			out.Photos = append(out.Photos, p)
		}
		_ = rows.Close()
		if err := rows.Err(); err != nil {
			return nil, err
		}
		cq, cargs := addUIDFilter(`SELECT COUNT(*) FROM media_items WHERE media_type LIKE 'image/%' AND year = ? AND month = ?`, []any{year, month}, uid)
		if err := r.pool.QueryRowContext(ctx, cq, cargs...).Scan(&out.Totals.Photos); err != nil {
			return nil, err
		}
	}

	{
		q, args := addUIDFilter(`SELECT id, substr(timestamp, 1, 10), COALESCE(title, ''), COALESCE(post_text, '')
			FROM facebook_posts WHERE timestamp >= ? AND timestamp < ?`, []any{lo, hi}, uid)
		q += ` ORDER BY timestamp LIMIT ?`
		args = append(args, timelineMaxPosts)
		rows, err := r.pool.QueryContext(ctx, q, args...)
		if err != nil {
			return nil, fmt.Errorf("timeline month posts: %w", err)
		}
		for rows.Next() {
			var p model.TimelinePost
			if err := rows.Scan(&p.ID, &p.Date, &p.Title, &p.Text); err != nil {
				_ = rows.Close()
				return nil, err
			}
			p.Text = truncateRunes(collapseWS(p.Text), 240)
			out.Posts = append(out.Posts, p)
		}
		_ = rows.Close()
		if err := rows.Err(); err != nil {
			return nil, err
		}
		cq, cargs := addUIDFilter(`SELECT COUNT(*) FROM facebook_posts WHERE timestamp >= ? AND timestamp < ?`, []any{lo, hi}, uid)
		if err := r.pool.QueryRowContext(ctx, cq, cargs...).Scan(&out.Totals.Posts); err != nil {
			return nil, err
		}
	}
	return out, nil
}

// escapeLike escapes %, _ and \ for use with LIKE ... ESCAPE '\'.
func escapeLike(s string) string {
	r := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)
	return r.Replace(s)
}

// NormalizeQuickSearchQuery trims and bounds a query; ok=false when it's too short to search.
func NormalizeQuickSearchQuery(q string) (string, bool) {
	q = collapseWS(q)
	if utf8.RuneCountInString(q) > quickSearchMaxQuery {
		q = string([]rune(q)[:quickSearchMaxQuery])
	}
	return q, utf8.RuneCountInString(q) >= quickSearchMinLength
}

// QuickSearch runs a keyword (substring) search across sources, newest first, a few hits per kind.
func (r *TimelineRepo) QuickSearch(ctx context.Context, query string, src TimelineSources) (*model.QuickSearchResult, error) {
	out := &model.QuickSearchResult{Query: query, Hits: []model.QuickSearchHit{}, Hidden: []string{}}
	q, ok := NormalizeQuickSearchQuery(query)
	if !ok {
		return out, nil
	}
	out.Query = q
	uid := uidFromCtx(ctx)
	pat := "%" + escapeLike(q) + "%"

	scan := func(sqlText string, args []any, build func(rows *sql.Rows) (model.QuickSearchHit, error)) error {
		rows, err := r.pool.QueryContext(ctx, sqlText, args...)
		if err != nil {
			return err
		}
		defer func() { _ = rows.Close() }()
		for rows.Next() {
			h, err := build(rows)
			if err != nil {
				return err
			}
			out.Hits = append(out.Hits, h)
		}
		return rows.Err()
	}

	if src.Messages {
		sq, args := addUIDFilter(`SELECT id, COALESCE(chat_session, ''), COALESCE(sender_name, ''), COALESCE(text, ''), COALESCE(substr(message_date, 1, 10), '')
			FROM messages WHERE text LIKE ? ESCAPE '\'`, []any{pat}, uid)
		sq += ` ORDER BY message_date DESC LIMIT ?`
		args = append(args, quickSearchPerKind)
		err := scan(sq, args, func(rows *sql.Rows) (model.QuickSearchHit, error) {
			var h model.QuickSearchHit
			var session, sender, text string
			if err := rows.Scan(&h.ID, &session, &sender, &text, &h.Date); err != nil {
				return h, err
			}
			h.Kind = "message"
			h.Title = firstNonBlank(session, sender, "Message")
			if sender != "" && sender != session && sender != "Unknown" {
				h.Title += " · " + sender
			}
			h.Snippet = snippetAround(text, q, quickSearchSnippet)
			return h, nil
		})
		if err != nil {
			return nil, fmt.Errorf("quick search messages: %w", err)
		}
	} else {
		out.Hidden = append(out.Hidden, "messages")
	}

	if src.Emails {
		sq, args := addUIDFilter(`SELECT id, COALESCE(subject, ''), COALESCE(from_address, ''), COALESCE(plain_text, ''), COALESCE(substr(date, 1, 10), '')
			FROM emails WHERE COALESCE(user_deleted, 0) = 0
			  AND (subject LIKE ? ESCAPE '\' OR from_address LIKE ? ESCAPE '\' OR plain_text LIKE ? ESCAPE '\')`, []any{pat, pat, pat}, uid)
		sq += ` ORDER BY date DESC LIMIT ?`
		args = append(args, quickSearchPerKind)
		err := scan(sq, args, func(rows *sql.Rows) (model.QuickSearchHit, error) {
			var h model.QuickSearchHit
			var subject, from, body string
			if err := rows.Scan(&h.ID, &subject, &from, &body, &h.Date); err != nil {
				return h, err
			}
			h.Kind = "email"
			h.Title = firstNonBlank(subject, "(no subject)")
			snip := snippetAround(body, q, quickSearchSnippet)
			if !containsFold(body, q) {
				snip = ""
			}
			h.Snippet = strings.TrimSpace(strings.Join(nonEmpty(from, snip), " — "))
			return h, nil
		})
		if err != nil {
			return nil, fmt.Errorf("quick search emails: %w", err)
		}
	} else {
		out.Hidden = append(out.Hidden, "emails")
	}

	{
		sq, args := addUIDFilter(`SELECT id, COALESCE(title, ''), COALESCE(description, ''), COALESCE(tags, ''), COALESCE(year, 0), COALESCE(month, 0)
			FROM media_items WHERE media_type LIKE 'image/%'
			  AND (title LIKE ? ESCAPE '\' OR description LIKE ? ESCAPE '\' OR tags LIKE ? ESCAPE '\')`, []any{pat, pat, pat}, uid)
		sq += ` ORDER BY COALESCE(year, 0) DESC, COALESCE(month, 0) DESC, id DESC LIMIT ?`
		args = append(args, quickSearchPerKind)
		err := scan(sq, args, func(rows *sql.Rows) (model.QuickSearchHit, error) {
			var h model.QuickSearchHit
			var title, desc, tags string
			var year, month int
			if err := rows.Scan(&h.ID, &title, &desc, &tags, &year, &month); err != nil {
				return h, err
			}
			h.Kind = "photo"
			h.Title = firstNonBlank(title, "Photo")
			if containsFold(desc, q) {
				h.Snippet = snippetAround(desc, q, quickSearchSnippet)
			} else if containsFold(tags, q) {
				h.Snippet = "Tags: " + truncateRunes(collapseWS(tags), 2*quickSearchSnippet)
			}
			if year > 0 && month > 0 {
				h.Date = fmt.Sprintf("%04d-%02d", year, month)
			} else if year > 0 {
				h.Date = fmt.Sprintf("%04d", year)
			}
			return h, nil
		})
		if err != nil {
			return nil, fmt.Errorf("quick search photos: %w", err)
		}
	}

	{
		sq, args := addUIDFilter(`SELECT id, COALESCE(title, ''), COALESCE(post_text, ''), COALESCE(substr(timestamp, 1, 10), '')
			FROM facebook_posts WHERE (title LIKE ? ESCAPE '\' OR post_text LIKE ? ESCAPE '\')`, []any{pat, pat}, uid)
		sq += ` ORDER BY timestamp DESC LIMIT ?`
		args = append(args, quickSearchPerKind)
		err := scan(sq, args, func(rows *sql.Rows) (model.QuickSearchHit, error) {
			var h model.QuickSearchHit
			var title, text string
			if err := rows.Scan(&h.ID, &title, &text, &h.Date); err != nil {
				return h, err
			}
			h.Kind = "facebook_post"
			h.Title = firstNonBlank(title, "Facebook post")
			h.Snippet = snippetAround(text, q, quickSearchSnippet)
			return h, nil
		})
		if err != nil {
			return nil, fmt.Errorf("quick search posts: %w", err)
		}
	}
	return out, nil
}

// ── small text helpers ──────────────────────────────────────────────────────

func collapseWS(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

func truncateRunes(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	r := []rune(s)
	return strings.TrimSpace(string(r[:n])) + "…"
}

func firstNonBlank(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

func nonEmpty(vals ...string) []string {
	out := make([]string, 0, len(vals))
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			out = append(out, strings.TrimSpace(v))
		}
	}
	return out
}

func containsFold(s, sub string) bool {
	return strings.Contains(strings.ToLower(s), strings.ToLower(sub))
}

// snippetAround returns about `radius` runes either side of the first case-insensitive match of
// q in text (whitespace collapsed), with ellipses where text was cut. Falls back to the start.
func snippetAround(text, q string, radius int) string {
	t := []rune(collapseWS(text))
	if len(t) == 0 {
		return ""
	}
	lowerT := []rune(strings.ToLower(string(t)))
	lowerQ := []rune(strings.ToLower(q))
	idx := -1
	if len(lowerT) == len(t) { // ToLower kept rune count — index positions line up
		idx = runeIndex(lowerT, lowerQ)
	}
	if idx < 0 {
		return truncateRunes(string(t), 2*radius)
	}
	start := idx - radius
	if start < 0 {
		start = 0
	}
	end := idx + len(lowerQ) + radius
	if end > len(t) {
		end = len(t)
	}
	s := strings.TrimSpace(string(t[start:end]))
	if start > 0 {
		s = "…" + s
	}
	if end < len(t) {
		s += "…"
	}
	return s
}

func runeIndex(hay, needle []rune) int {
	if len(needle) == 0 || len(needle) > len(hay) {
		return -1
	}
outer:
	for i := 0; i+len(needle) <= len(hay); i++ {
		for j := range needle {
			if hay[i+j] != needle[j] {
				continue outer
			}
		}
		return i
	}
	return -1
}
