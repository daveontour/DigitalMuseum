package repository

import (
	"context"
	"database/sql"
	"fmt"
	"math/rand"
	"strconv"
	"strings"
	"testing"

	_ "github.com/mattn/go-sqlite3"

	"github.com/daveontour/aimuseum/internal/appctx"
)

func TestBuildOptions(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	for i := 0; i < 50; i++ {
		opts, idx, ok := buildOptions("Alice", []string{"alice", "Bob", "Carol", "Dan", "Bob", " "}, rng)
		if !ok {
			t.Fatal("expected options")
		}
		if len(opts) != 4 || opts[idx] != "Alice" {
			t.Fatalf("bad options %v idx %d", opts, idx)
		}
		seen := map[string]bool{}
		for _, o := range opts {
			k := strings.ToLower(o)
			if seen[k] {
				t.Fatalf("duplicate option in %v", opts)
			}
			seen[k] = true
		}
	}
	if _, _, ok := buildOptions("Alice", []string{"Bob", "ALICE", "Carol"}, rng); ok {
		t.Fatal("only 2 real distractors — should fail")
	}
}

func TestYearOptions(t *testing.T) {
	rng := rand.New(rand.NewSource(2))
	for _, tc := range []struct {
		correct int
		pool    []int
	}{
		{2010, []int{2010}},                          // no other years → nearby years
		{2010, []int{1990, 2008, 2010, 2012, 2015}},  // 1990 is too far away
		{1900, nil},                                  // can't go below 1900
	} {
		opts, idx := yearOptions(tc.correct, tc.pool, rng)
		if len(opts) != 4 || opts[idx] != strconv.Itoa(tc.correct) {
			t.Fatalf("correct %d: bad options %v idx %d", tc.correct, opts, idx)
		}
		prev := 0
		for _, o := range opts {
			y, _ := strconv.Atoi(o)
			if y <= prev {
				t.Fatalf("options not strictly ascending: %v", opts)
			}
			if y < 1900 || y < tc.correct-12 || y > tc.correct+12 {
				t.Fatalf("implausible distractor %d in %v", y, opts)
			}
			prev = y
		}
	}
}

func TestEmailDisplayName(t *testing.T) {
	for in, want := range map[string]string{
		`"Jane Smith" <jane@example.com>`: "Jane Smith",
		`Jane Smith <jane@example.com>`:   "Jane Smith",
		`jane@example.com`:                "jane@example.com",
		`<jane@example.com>`:              "jane@example.com",
		``:                                "",
	} {
		if got := emailDisplayName(in); got != want {
			t.Errorf("emailDisplayName(%q) = %q, want %q", in, got, want)
		}
	}
}

func setupMemoriesTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite3", "file:memories?mode=memory&cache=shared")
	if err != nil {
		t.Skipf("sqlite3 unavailable: %v", err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })
	_, err = db.Exec(`
		CREATE TABLE media_items (id INTEGER PRIMARY KEY, title TEXT, media_type TEXT, year INTEGER, month INTEGER, day INTEGER, region TEXT, user_id INTEGER);
		CREATE TABLE messages (id INTEGER PRIMARY KEY, chat_session TEXT, message_date TEXT, is_group_chat BOOLEAN NOT NULL DEFAULT 0,
			type TEXT, sender_name TEXT, text TEXT, user_id INTEGER);
		CREATE TABLE emails (id INTEGER PRIMARY KEY, subject TEXT, from_address TEXT, date TEXT, plain_text TEXT, snippet TEXT,
			user_deleted BOOLEAN NOT NULL DEFAULT 0, is_spam BOOLEAN NOT NULL DEFAULT 0, is_promotional BOOLEAN NOT NULL DEFAULT 0, user_id INTEGER);
		CREATE TABLE facebook_posts (id INTEGER PRIMARY KEY, timestamp TEXT, title TEXT, post_text TEXT, user_id INTEGER);
		CREATE TABLE contacts (id INTEGER PRIMARY KEY, name TEXT, is_group BOOLEAN DEFAULT 0, total INTEGER DEFAULT 0, user_id INTEGER);
		CREATE TABLE media_item_faces (id INTEGER PRIMARY KEY, media_item_id INTEGER, bbox_w REAL, contact_id INTEGER, ignored BOOLEAN NOT NULL DEFAULT 0, user_id INTEGER);
		CREATE TABLE subject_configuration (id INTEGER PRIMARY KEY, subject_name TEXT, user_id INTEGER);
	`)
	if err != nil {
		t.Fatalf("schema: %v", err)
	}
	return db
}

func seedMemories(t *testing.T, db *sql.DB) {
	t.Helper()
	exec := func(q string, args ...any) {
		if _, err := db.Exec(q, args...); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
	regions := []string{"Paris", "Kyoto", "Perth", "Oslo", "Lima"}
	people := []string{"Alice", "Bob", "Carol", "Dan", "Erin"}
	id := 0
	for y := 2010; y <= 2020; y++ {
		for i, r := range regions {
			id++
			exec(`INSERT INTO media_items (id, title, media_type, year, month, day, region, user_id) VALUES (?, ?, 'image/jpeg', ?, 10, 16, ?, 2)`,
				id, fmt.Sprintf("photo %d", id), y, r)
			exec(`INSERT INTO media_item_faces (media_item_id, bbox_w, contact_id, user_id) VALUES (?, 0.2, ?, 2)`, id, i+1)
		}
		for i, p := range people {
			for k := 0; k <= i; k++ { // Erin is busiest every year, no ties
				exec(`INSERT INTO messages (chat_session, message_date, type, sender_name, text, user_id) VALUES (?, ?, 'Incoming', ?, ?, 2)`,
					p, fmt.Sprintf("%d-10-16 09:%02d:00", y, k), p, fmt.Sprintf("See you at the lake on Saturday, bring snacks please %d", k))
			}
			for k := 0; k < 10; k++ {
				exec(`INSERT INTO messages (chat_session, message_date, type, sender_name, text, user_id) VALUES ('Erin', ?, 'Outgoing', 'Me', 'ok sounds great see you there then', 2)`,
					fmt.Sprintf("%d-03-01 10:00:00", y))
			}
		}
		exec(`INSERT INTO facebook_posts (timestamp, title, post_text, user_id) VALUES (?, 'Post', 'What a beautiful day for a long walk by the river', 2)`, fmt.Sprintf("%d-10-16 12:00:00", y))
	}
	for i, p := range people {
		exec(`INSERT INTO contacts (id, name, total, user_id) VALUES (?, ?, ?, 2)`, i+1, p, 100-i)
		exec(`INSERT INTO emails (subject, from_address, date, snippet, user_id) VALUES ('Weekend plans', ?, '2015-10-16 08:00:00', 'Are we still on for the weekend?', 2)`,
			fmt.Sprintf(`"%s Smith" <%s@example.com>`, p, strings.ToLower(p)))
	}
	// Spam email and another user's data must never show up.
	exec(`INSERT INTO emails (subject, from_address, date, snippet, is_spam, user_id) VALUES ('WIN', 'spam@x.com', '2015-10-16 08:00:00', 'win', 1, 2)`)
	exec(`INSERT INTO media_items (title, media_type, year, month, day, region, user_id) VALUES ('other', 'image/jpeg', 2012, 10, 16, 'Elsewhere', 3)`)
	exec(`INSERT INTO subject_configuration (subject_name, user_id) VALUES ('Dave', 2)`)
}

func TestTriviaGenerate(t *testing.T) {
	db := setupMemoriesTestDB(t)
	seedMemories(t, db)
	ctx := context.WithValue(context.Background(), appctx.ContextKeyUserID, int64(2))
	repo := NewTriviaRepo(db)

	res, err := repo.Generate(ctx, TriviaOptions{Count: 14, Messages: true, Emails: true, Contacts: true}, rand.New(rand.NewSource(7)))
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if len(res.Questions) != 14 {
		t.Fatalf("got %d questions, want 14 (available %v)", len(res.Questions), res.Available)
	}
	types := map[string]int{}
	for _, q := range res.Questions {
		types[q.Type]++
		if len(q.Options) != 4 || q.CorrectIndex < 0 || q.CorrectIndex > 3 || q.Question == "" || q.Explanation == "" {
			t.Fatalf("malformed question %+v", q)
		}
		for _, o := range q.Options {
			if o == "Elsewhere" || o == "spam@x.com" || o == "Me" {
				t.Fatalf("leaked option %q in %+v", o, q)
			}
		}
		if q.Type == TriviaTopContactYear && q.Options[q.CorrectIndex] != "Erin" {
			t.Fatalf("top contact should be Erin: %+v", q)
		}
		if q.Type == TriviaMessageSender && !strings.Contains(q.Question, "you") {
			t.Fatalf("owner phrasing expected: %q", q.Question)
		}
	}
	if len(types) < 6 {
		t.Fatalf("expected a mix of question types, got %v", types)
	}

	// Visitor without messages/emails/contacts: only photo + post questions, third-person.
	res, err = repo.Generate(ctx, TriviaOptions{Count: 10, SubjectName: repo.TriviaSubjectName(ctx)}, rand.New(rand.NewSource(8)))
	if err != nil {
		t.Fatalf("Generate (visitor): %v", err)
	}
	for _, q := range res.Questions {
		switch q.Type {
		case TriviaPhotoYear, TriviaPhotoPlace, TriviaPostYear:
		default:
			t.Fatalf("restricted session got %s question", q.Type)
		}
		if q.Type == TriviaPostYear && !strings.Contains(q.Question, "Dave") {
			t.Fatalf("visitor phrasing expected: %q", q.Question)
		}
	}
}

func TestOnThisDay(t *testing.T) {
	db := setupMemoriesTestDB(t)
	seedMemories(t, db)
	ctx := context.WithValue(context.Background(), appctx.ContextKeyUserID, int64(2))
	repo := NewTimelineRepo(db)

	res, err := repo.OnThisDay(ctx, 2020, 10, 16, TimelineSources{Messages: true, Emails: true})
	if err != nil {
		t.Fatalf("OnThisDay: %v", err)
	}
	if len(res.Years) != 10 || res.Years[0].Year != 2019 || res.Years[0].YearsAgo != 1 {
		t.Fatalf("want 2010–2019 newest first, got %d years starting %+v", len(res.Years), res.Years[0])
	}
	for _, y := range res.Years {
		if y.Year == 2020 {
			t.Fatal("current year must be excluded")
		}
		if y.Totals.Photos != 5 || len(y.Photos) != 5 {
			t.Fatalf("%d: want 5 photos (other user's excluded), got %d", y.Year, y.Totals.Photos)
		}
		if y.Totals.Messages != 15 || len(y.Conversations) != 5 {
			t.Fatalf("%d: want 15 messages in 5 conversations, got %d/%d", y.Year, y.Totals.Messages, len(y.Conversations))
		}
		if y.Year == 2015 && y.Totals.Emails != 5 {
			t.Fatalf("2015: want 5 emails (spam excluded), got %d", y.Totals.Emails)
		}
	}

	res, err = repo.OnThisDay(ctx, 2020, 10, 16, TimelineSources{})
	if err != nil {
		t.Fatalf("OnThisDay (restricted): %v", err)
	}
	if len(res.Hidden) != 2 || res.Years[0].Totals.Messages != 0 || res.Years[0].Totals.Emails != 0 {
		t.Fatalf("restricted session should not see messages/emails: %+v", res)
	}

	res, err = repo.OnThisDay(ctx, 2020, 3, 2, TimelineSources{Messages: true, Emails: true})
	if err != nil || len(res.Years) != 0 || res.Total != 0 {
		t.Fatalf("empty day: %+v, %v", res, err)
	}
}
