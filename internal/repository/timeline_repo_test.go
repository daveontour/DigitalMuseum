package repository

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/daveontour/aimuseum/internal/config"
	"github.com/daveontour/aimuseum/internal/database"
)

// newTimelineTestRepo opens a real migrated archive and seeds a few rows across sources,
// storing dates the way the importers do (Go time values → go-sqlite3 TEXT timestamps).
func newTimelineTestRepo(t *testing.T) (*TimelineRepo, context.Context) {
	t.Helper()
	ctx := context.Background()
	db, err := database.New(ctx, config.DatabaseConfig{SQLitePath: filepath.Join(t.TempDir(), "archive.sqlite")})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := database.MigrateSQLite(ctx, db.Std); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	p := db.Std
	tz := time.FixedZone("AEST", 10*3600)
	may2 := time.Date(2014, 5, 2, 23, 30, 0, 0, tz) // late evening local — must stay on May 2
	may9 := time.Date(2014, 5, 9, 9, 0, 0, 0, tz)
	june := time.Date(2014, 6, 1, 12, 0, 0, 0, tz)
	exec := func(q string, args ...any) {
		t.Helper()
		if _, err := p.ExecContext(ctx, q, args...); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
	exec(`INSERT INTO messages (chat_session, message_date, sender_name, text) VALUES (?, ?, ?, ?)`, "Sarah", may2, "Sarah", "Moving to the new house on Saturday")
	exec(`INSERT INTO messages (chat_session, message_date, sender_name, text) VALUES (?, ?, ?, ?)`, "Sarah", may2, "Me", "Great, see you there")
	exec(`INSERT INTO messages (chat_session, message_date, sender_name, text) VALUES (?, ?, ?, ?)`, "Bob", june, "Bob", "hello 100% sure")
	exec(`INSERT INTO messages (chat_session, message_date, sender_name, text) VALUES (?, ?, ?, ?)`, "Zero", time.Time{}, "Zero", "zero date")
	exec(`INSERT INTO emails (uid, folder, subject, from_address, date, plain_text) VALUES ('1', 'INBOX', 'House keys', 'agent@example.com', ?, 'Your keys for the new house are ready')`, may9)
	exec(`INSERT INTO emails (uid, folder, subject, from_address, date, plain_text, user_deleted) VALUES ('2', 'INBOX', 'Deleted house', 'x@example.com', ?, 'house', 1)`, may9)
	exec(`INSERT INTO media_blobs (id) VALUES (1)`)
	exec(`INSERT INTO media_items (media_blob_id, title, media_type, year, month, day, tags) VALUES (1, 'Front of house', 'image/jpeg', 2014, 5, 3, 'house,garden')`)
	exec(`INSERT INTO media_items (media_blob_id, title, media_type, year, month, day) VALUES (1, 'A pdf', 'application/pdf', 2014, 5, 3)`)
	exec(`INSERT INTO facebook_posts (timestamp, title, post_text) VALUES (?, 'Moved!', 'We moved into the new house')`, may9)
	return NewTimelineRepo(p), ctx
}

func TestTimeline_MonthlyCounts(t *testing.T) {
	repo, ctx := newTimelineTestRepo(t)
	months, _, err := repo.MonthlyCounts(ctx, TimelineSources{Messages: true, Emails: true}, TimelineFilter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(months) != 2 {
		t.Fatalf("want 2 months (zero dates ignored), got %+v", months)
	}
	may := months[0]
	if may.Month != "2014-05" || may.Messages != 2 || may.Emails != 1 || may.Photos != 1 || may.Posts != 1 {
		t.Fatalf("unexpected May counts: %+v", may)
	}
	if months[1].Month != "2014-06" || months[1].Messages != 1 {
		t.Fatalf("unexpected June counts: %+v", months[1])
	}

	restricted, _, err := repo.MonthlyCounts(ctx, TimelineSources{}, TimelineFilter{})
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range restricted {
		if m.Messages != 0 || m.Emails != 0 {
			t.Fatalf("restricted session must not see messages/emails: %+v", m)
		}
	}
}

func TestTimeline_MonthDetail(t *testing.T) {
	repo, ctx := newTimelineTestRepo(t)
	d, err := repo.MonthDetail(ctx, 2014, 5, TimelineSources{Messages: true, Emails: true}, TimelineFilter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Conversations) != 1 || d.Conversations[0].Count != 2 || d.Conversations[0].Date != "2014-05-02" || d.Conversations[0].ChatSession != "Sarah" {
		t.Fatalf("conversations = %+v", d.Conversations)
	}
	if len(d.Emails) != 1 || d.Emails[0].Subject != "House keys" || d.Emails[0].Date != "2014-05-09" {
		t.Fatalf("emails = %+v", d.Emails)
	}
	if len(d.Photos) != 1 || d.Photos[0].Day != 3 {
		t.Fatalf("photos = %+v", d.Photos)
	}
	if len(d.Posts) != 1 || d.Totals.Posts != 1 {
		t.Fatalf("posts = %+v", d.Posts)
	}

	hidden, err := repo.MonthDetail(ctx, 2014, 5, TimelineSources{}, TimelineFilter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(hidden.Conversations) != 0 || len(hidden.Emails) != 0 || len(hidden.Hidden) != 2 {
		t.Fatalf("restricted month detail leaked data: %+v", hidden)
	}
	if _, err := repo.MonthDetail(ctx, 2014, 13, TimelineSources{}, TimelineFilter{}); err == nil {
		t.Fatal("invalid month should error")
	}
}

func TestTimeline_Filter(t *testing.T) {
	repo, ctx := newTimelineTestRepo(t)
	p := repo.pool
	exec := func(q string, args ...any) {
		t.Helper()
		if _, err := p.ExecContext(ctx, q, args...); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
	tz := time.FixedZone("AEST", 10*3600)
	may20 := time.Date(2014, 5, 20, 10, 0, 0, 0, tz)
	exec(`INSERT INTO contacts (id, name, alternative_names, email) VALUES (50, 'Sarah Jones', 'Sarah, SJ', 'sarah@example.com')`)
	exec(`INSERT INTO emails (uid, folder, subject, from_address, to_addresses, date, plain_text) VALUES ('3', 'INBOX', 'Dinner', 'me@example.com', 'Sarah <sarah@example.com>', ?, 'See you at dinner')`, may20)
	exec(`INSERT INTO media_items (media_blob_id, title, media_type, year, month, day, tags) VALUES (1, 'Sarah at the beach', 'image/jpeg', 2014, 6, 4, 'Beach, holiday')`)
	exec(`INSERT INTO media_item_faces (media_item_id, bbox_x, bbox_y, bbox_w, bbox_h, embedding_model, contact_id)
		SELECT id, 0, 0, 1, 1, 'test', 50 FROM media_items WHERE title = 'Sarah at the beach'`)
	all := TimelineSources{Messages: true, Emails: true}

	// Person: Sarah's conversation + the email to her in May, her tagged face in June.
	months, excluded, err := repo.MonthlyCounts(ctx, all, TimelineFilter{PersonID: 50})
	if err != nil {
		t.Fatal(err)
	}
	if len(excluded) != 0 || len(months) != 2 {
		t.Fatalf("person months = %+v excluded=%v", months, excluded)
	}
	if m := months[0]; m.Month != "2014-05" || m.Messages != 2 || m.Emails != 1 || m.Photos != 0 || m.Posts != 0 {
		t.Fatalf("person May = %+v", m)
	}
	if m := months[1]; m.Month != "2014-06" || m.Messages != 0 || m.Photos != 1 {
		t.Fatalf("person June = %+v", m)
	}
	d, err := repo.MonthDetail(ctx, 2014, 5, all, TimelineFilter{PersonID: 50})
	if err != nil {
		t.Fatal(err)
	}
	if !d.Filtered || len(d.Emails) != 1 || d.Emails[0].Subject != "Dinner" || len(d.Conversations) != 1 || len(d.Photos) != 0 {
		t.Fatalf("person May detail = %+v", d)
	}
	if _, _, err := repo.MonthlyCounts(ctx, all, TimelineFilter{PersonID: 999}); err != ErrTimelinePersonNotFound {
		t.Fatalf("unknown person err = %v", err)
	}

	// Keywords: every term must match; across sources.
	months, _, err = repo.MonthlyCounts(ctx, all, TimelineFilter{Keywords: ParseTimelineKeywords(`"new house"`)})
	if err != nil {
		t.Fatal(err)
	}
	if len(months) != 1 || months[0].Messages != 1 || months[0].Emails != 1 || months[0].Posts != 1 || months[0].Photos != 0 {
		t.Fatalf("keyword months = %+v", months)
	}
	months, _, _ = repo.MonthlyCounts(ctx, all, TimelineFilter{Keywords: []string{"house", "zebra"}})
	if len(months) != 0 {
		t.Fatalf("all keywords must match, got %+v", months)
	}

	// Tag: exact tag, photos only.
	months, excluded, err = repo.MonthlyCounts(ctx, all, TimelineFilter{Tag: "holiday"})
	if err != nil {
		t.Fatal(err)
	}
	if len(months) != 1 || months[0].Month != "2014-06" || months[0].Photos != 1 || len(excluded) != 3 {
		t.Fatalf("tag months = %+v excluded=%v", months, excluded)
	}
	if months, _, _ = repo.MonthlyCounts(ctx, all, TimelineFilter{Tag: "hol"}); len(months) != 0 {
		t.Fatalf("tag must match whole tags, got %+v", months)
	}
	if months, _, _ = repo.MonthlyCounts(ctx, all, TimelineFilter{Tag: "GARDEN"}); len(months) != 1 || months[0].Month != "2014-05" {
		t.Fatalf("tag match should ignore case, got %+v", months)
	}

	// Combined: person AND tag.
	months, _, _ = repo.MonthlyCounts(ctx, all, TimelineFilter{PersonID: 50, Tag: "garden"})
	if len(months) != 0 {
		t.Fatalf("person+tag should combine with AND, got %+v", months)
	}
}

func TestParseTimelineKeywords(t *testing.T) {
	got := ParseTimelineKeywords(`  new "front  door" a  keys "unbalanced`)
	want := []string{"new", "front door", "keys", "unbalanced"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("got %q want %q", got, want)
	}
	if len(ParseTimelineKeywords("")) != 0 {
		t.Fatal("empty input should give no keywords")
	}
}

func TestQuickSearch(t *testing.T) {
	repo, ctx := newTimelineTestRepo(t)
	res, err := repo.QuickSearch(ctx, "  new   HOUSE ", TimelineSources{Messages: true, Emails: true})
	if err != nil {
		t.Fatal(err)
	}
	if res.Query != "new HOUSE" {
		t.Fatalf("query should be normalised, got %q", res.Query)
	}
	kinds := map[string]int{}
	for _, h := range res.Hits {
		kinds[h.Kind]++
	}
	if kinds["message"] != 1 || kinds["email"] != 1 || kinds["facebook_post"] != 1 {
		t.Fatalf("unexpected hits: %+v", res.Hits)
	}
	for _, h := range res.Hits {
		if h.Kind == "email" && !strings.Contains(h.Snippet, "agent@example.com") {
			t.Errorf("email snippet should include sender: %q", h.Snippet)
		}
	}

	// LIKE wildcards in the query are literal.
	pct, err := repo.QuickSearch(ctx, "100%", TimelineSources{Messages: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(pct.Hits) != 1 {
		t.Fatalf("'100%%' should match exactly one message, got %+v", pct.Hits)
	}
	under, err := repo.QuickSearch(ctx, "h_use", TimelineSources{Messages: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(under.Hits) != 0 {
		t.Fatalf("'_' must not act as a wildcard, got %+v", under.Hits)
	}

	photo, err := repo.QuickSearch(ctx, "garden", TimelineSources{})
	if err != nil {
		t.Fatal(err)
	}
	if len(photo.Hits) != 1 || photo.Hits[0].Kind != "photo" || len(photo.Hidden) != 2 {
		t.Fatalf("photo search = %+v", photo)
	}

	short, _ := repo.QuickSearch(ctx, "a", TimelineSources{Messages: true})
	if len(short.Hits) != 0 {
		t.Fatal("one-character queries should not search")
	}
}

func TestSnippetAround(t *testing.T) {
	text := strings.Repeat("x ", 100) + "the Needle here " + strings.Repeat("y ", 100)
	s := snippetAround(text, "needle", 10)
	if !strings.Contains(s, "Needle") || !strings.HasPrefix(s, "…") || !strings.HasSuffix(s, "…") {
		t.Fatalf("snippet = %q", s)
	}
	if got := snippetAround("short", "missing", 10); got != "short" {
		t.Fatalf("fallback snippet = %q", got)
	}
}
