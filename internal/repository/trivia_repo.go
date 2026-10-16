package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math/rand"
	"net/mail"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/daveontour/aimuseum/internal/model"
)

// TriviaRepo builds the "Memory Challenge" — multiple-choice questions generated straight from
// archive data (who is this face, what year was this photo, who sent this message, …).
//
// Unlike the login-page quiz (service.GenerateQuiz) there is no LLM involved: every correct
// answer is a fact read from the database and every wrong answer is another real value from the
// same archive (another contact, another year that has photos, another region), so questions are
// instant, free, offline and can't be hallucinated.
type TriviaRepo struct {
	pool *sql.DB
}

// NewTriviaRepo creates a TriviaRepo.
func NewTriviaRepo(pool *sql.DB) *TriviaRepo {
	return &TriviaRepo{pool: pool}
}

// TriviaOptions controls one generated game.
type TriviaOptions struct {
	Count int
	// Sources the session may see (visitor keys can be restricted).
	Messages bool
	Emails   bool
	Contacts bool // face questions reveal contact names
	// SubjectName, when set, phrases questions in the third person ("Who sent Dave this
	// message?") — used for visitors. Empty means the owner is playing ("Who sent you…").
	SubjectName string
	// Types restricts generation to these question types (empty = all available).
	Types []string
}

// Question types.
const (
	TriviaFaceWho        = "face_who"
	TriviaPhotoYear      = "photo_year"
	TriviaPhotoPlace     = "photo_place"
	TriviaMessageSender  = "message_sender"
	TriviaEmailSender    = "email_sender"
	TriviaPostYear       = "post_year"
	TriviaTopContactYear = "top_contact_year"
)

const (
	triviaMaxCount       = 20
	triviaDefaultCount   = 10
	triviaPickAttempts   = 6 // random probes per question before giving up on that type
	triviaQuoteMaxRunes  = 220
	triviaMinQuoteRunes  = 25
	triviaDistractorPool = 40
)

var triviaMonthNames = [...]string{"January", "February", "March", "April", "May", "June",
	"July", "August", "September", "October", "November", "December"}

// triviaGame holds per-request state: the RNG and lazily loaded answer pools.
type triviaGame struct {
	r   *TriviaRepo
	ctx context.Context
	uid int64
	rng *rand.Rand
	opt TriviaOptions

	photoYears    []int
	postYears     []int
	regions       []string
	senders       []string
	emailSenders  []string
	faceNames     []string
	contactNames  []string
	topByYear     map[int][]string // year → 1:1 conversations, busiest first (ties removed)
	loadedPools   map[string]bool
	usedRecordIDs map[string]bool // "kind:id" — never ask about the same record twice
	usedTopYears  map[int]bool
}

// Generate builds up to opt.Count questions, mixing types round-robin so a game isn't ten
// "what year" questions in a row. Types without enough data are skipped; Available reports
// which types could be produced at all.
func (r *TriviaRepo) Generate(ctx context.Context, opt TriviaOptions, rng *rand.Rand) (*model.TriviaResult, error) {
	if opt.Count <= 0 {
		opt.Count = triviaDefaultCount
	}
	if opt.Count > triviaMaxCount {
		opt.Count = triviaMaxCount
	}
	if rng == nil {
		rng = rand.New(rand.NewSource(time.Now().UnixNano()))
	}
	g := &triviaGame{
		r: r, ctx: ctx, uid: uidFromCtx(ctx), rng: rng, opt: opt,
		loadedPools:   map[string]bool{},
		usedRecordIDs: map[string]bool{},
		usedTopYears:  map[int]bool{},
	}

	gens := g.generators()
	out := &model.TriviaResult{Questions: []model.TriviaQuestion{}, Available: []string{}}
	if len(gens) == 0 {
		return out, nil
	}

	// Round-robin over a shuffled type order; drop a type once it fails to produce a question.
	order := make([]string, 0, len(gens))
	for t := range gens {
		order = append(order, t)
	}
	sort.Strings(order) // deterministic base order before shuffling (map order is random)
	rng.Shuffle(len(order), func(i, j int) { order[i], order[j] = order[j], order[i] })

	produced := map[string]bool{}
	active := order
	for len(out.Questions) < opt.Count && len(active) > 0 {
		next := active[:0:0]
		for _, t := range active {
			if len(out.Questions) >= opt.Count {
				next = append(next, t)
				continue
			}
			q, err := gens[t]()
			if err != nil {
				if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
					return nil, err
				}
				return nil, fmt.Errorf("trivia %s: %w", t, err)
			}
			if q == nil {
				continue // not enough data for this type (any more)
			}
			q.Type = t
			produced[t] = true
			out.Questions = append(out.Questions, *q)
			next = append(next, t)
		}
		active = next
	}
	rng.Shuffle(len(out.Questions), func(i, j int) { out.Questions[i], out.Questions[j] = out.Questions[j], out.Questions[i] })
	for _, t := range order {
		if produced[t] {
			out.Available = append(out.Available, t)
		}
	}
	sort.Strings(out.Available)
	return out, nil
}

func (g *triviaGame) generators() map[string]func() (*model.TriviaQuestion, error) {
	all := map[string]func() (*model.TriviaQuestion, error){
		TriviaPhotoYear:  g.photoYear,
		TriviaPhotoPlace: g.photoPlace,
		TriviaPostYear:   g.postYear,
	}
	if g.opt.Contacts {
		all[TriviaFaceWho] = g.faceWho
	}
	if g.opt.Messages {
		all[TriviaMessageSender] = g.messageSender
		all[TriviaTopContactYear] = g.topContactYear
	}
	if g.opt.Emails {
		all[TriviaEmailSender] = g.emailSender
	}
	if len(g.opt.Types) == 0 {
		return all
	}
	want := map[string]bool{}
	for _, t := range g.opt.Types {
		want[strings.TrimSpace(t)] = true
	}
	for t := range all {
		if !want[t] {
			delete(all, t)
		}
	}
	return all
}

// ── phrasing ────────────────────────────────────────────────────────────────

// you returns "you" for the owner or the subject's name for visitors.
func (g *triviaGame) you() string {
	if g.opt.SubjectName != "" {
		return g.opt.SubjectName
	}
	return "you"
}

// your returns "your" or "<Name>'s".
func (g *triviaGame) your() string {
	if g.opt.SubjectName != "" {
		return g.opt.SubjectName + "'s"
	}
	return "your"
}

// ── answer pools (loaded once per game, on first use) ──────────────────────

func (g *triviaGame) loadStrings(key string, dst *[]string, q string, args []any) error {
	if g.loadedPools[key] {
		return nil
	}
	g.loadedPools[key] = true
	rows, err := g.r.pool.QueryContext(g.ctx, q, args...)
	if err != nil {
		return err
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var s sql.NullString
		var n int64
		if err := rows.Scan(&s, &n); err != nil {
			return err
		}
		if v := strings.TrimSpace(s.String); v != "" {
			*dst = append(*dst, v)
		}
	}
	return rows.Err()
}

func (g *triviaGame) loadYears(key string, dst *[]int, q string, args []any) error {
	if g.loadedPools[key] {
		return nil
	}
	g.loadedPools[key] = true
	rows, err := g.r.pool.QueryContext(g.ctx, q, args...)
	if err != nil {
		return err
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var s sql.NullString
		if err := rows.Scan(&s); err != nil {
			return err
		}
		if y, ok := yearOf(s.String); ok {
			*dst = append(*dst, y)
		}
	}
	return rows.Err()
}

func (g *triviaGame) loadPhotoYears() error {
	q, args := addUIDFilter(`SELECT DISTINCT printf('%04d', year) FROM media_items
		WHERE media_type LIKE 'image/%' AND year BETWEEN 1900 AND 2100`, nil, g.uid)
	return g.loadYears("photo_years", &g.photoYears, q, args)
}

func (g *triviaGame) loadPostYears() error {
	q, args := addUIDFilter(`SELECT DISTINCT substr(timestamp, 1, 4) FROM facebook_posts
		WHERE timestamp GLOB `+timelineDateGlob, nil, g.uid)
	return g.loadYears("post_years", &g.postYears, q, args)
}

func (g *triviaGame) loadRegions() error {
	q, args := addUIDFilter(`SELECT region, COUNT(*) FROM media_items
		WHERE media_type LIKE 'image/%' AND TRIM(COALESCE(region, '')) <> ''`, nil, g.uid)
	q += fmt.Sprintf(` GROUP BY region ORDER BY 2 DESC LIMIT %d`, triviaDistractorPool)
	return g.loadStrings("regions", &g.regions, q, args)
}

func (g *triviaGame) loadSenders() error {
	q, args := addUIDFilter(`SELECT sender_name, COUNT(*) FROM messages
		WHERE COALESCE(type, '') <> 'Outgoing' AND TRIM(COALESCE(sender_name, '')) NOT IN ('', 'Unknown')`, nil, g.uid)
	q += fmt.Sprintf(` GROUP BY sender_name ORDER BY 2 DESC LIMIT %d`, triviaDistractorPool)
	return g.loadStrings("senders", &g.senders, q, args)
}

func (g *triviaGame) loadEmailSenders() error {
	if g.loadedPools["email_senders"] {
		return nil
	}
	var raw []string
	q, args := addUIDFilter(`SELECT from_address, COUNT(*) FROM emails
		WHERE COALESCE(user_deleted, 0) = 0 AND COALESCE(is_spam, 0) = 0 AND COALESCE(is_promotional, 0) = 0
		  AND TRIM(COALESCE(from_address, '')) <> ''`, nil, g.uid)
	q += fmt.Sprintf(` GROUP BY from_address ORDER BY 2 DESC LIMIT %d`, 2*triviaDistractorPool)
	if err := g.loadStrings("email_senders", &raw, q, args); err != nil {
		return err
	}
	g.emailSenders = dedupeFold(mapStrings(raw, emailDisplayName))
	return nil
}

func (g *triviaGame) loadFaceNames() error {
	q, args := addUIDFilterQualified(`SELECT c.name, COUNT(*) FROM media_item_faces f
		JOIN contacts c ON c.id = f.contact_id
		WHERE COALESCE(f.ignored, 0) = 0 AND TRIM(COALESCE(c.name, '')) <> ''`, nil, g.uid, "f")
	q += fmt.Sprintf(` GROUP BY c.id ORDER BY 2 DESC LIMIT %d`, triviaDistractorPool)
	return g.loadStrings("face_names", &g.faceNames, q, args)
}

func (g *triviaGame) loadContactNames() error {
	q, args := addUIDFilter(`SELECT name, COALESCE(total, 0) FROM contacts
		WHERE COALESCE(is_group, 0) = 0 AND TRIM(COALESCE(name, '')) <> ''`, nil, g.uid)
	q += fmt.Sprintf(` ORDER BY COALESCE(total, 0) DESC LIMIT %d`, triviaDistractorPool)
	return g.loadStrings("contact_names", &g.contactNames, q, args)
}

// loadTopByYear finds, per year, the 1:1 conversations ranked by message count. A year is only
// kept when the busiest conversation clearly beats the runner-up (no ties) and there are at
// least four candidates to choose from.
func (g *triviaGame) loadTopByYear() error {
	if g.loadedPools["top_by_year"] {
		return nil
	}
	g.loadedPools["top_by_year"] = true
	g.topByYear = map[int][]string{}
	q, args := addUIDFilter(`SELECT substr(message_date, 1, 4), chat_session, COUNT(*) FROM messages
		WHERE COALESCE(is_group_chat, 0) = 0 AND message_date GLOB `+timelineDateGlob+`
		  AND TRIM(COALESCE(chat_session, '')) <> ''`, nil, g.uid)
	q += ` GROUP BY 1, 2`
	rows, err := g.r.pool.QueryContext(g.ctx, q, args...)
	if err != nil {
		return err
	}
	type entry struct {
		name string
		n    int64
	}
	per := map[int][]entry{}
	for rows.Next() {
		var ys, name string
		var n int64
		if err := rows.Scan(&ys, &name, &n); err != nil {
			_ = rows.Close()
			return err
		}
		if y, ok := yearOf(ys); ok {
			per[y] = append(per[y], entry{strings.TrimSpace(name), n})
		}
	}
	_ = rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	for y, list := range per {
		if len(list) < 4 {
			continue
		}
		sort.Slice(list, func(i, j int) bool { return list[i].n > list[j].n })
		if list[0].n == list[1].n || list[0].n < 10 {
			continue
		}
		names := make([]string, 0, 10)
		for i := 0; i < len(list) && i < 10; i++ {
			names = append(names, list[i].name)
		}
		g.topByYear[y] = names
	}
	return nil
}

// ── random row picking ─────────────────────────────────────────────────────

// randomRow scans one random row matching `where` without ORDER BY RANDOM() (a full-table sort
// on a large messages table): it jumps to a random id between the table's bounds and takes the
// first match at or after it, wrapping around to before it. idCol/uidCol may be qualified when
// `from` is a join. Returns false when nothing matches.
func (g *triviaGame) randomRow(boundsTable, from, idCol, uidCol, cols, where string, args []any, dest ...any) (bool, error) {
	var lo, hi sql.NullInt64
	if err := g.r.pool.QueryRowContext(g.ctx, `SELECT MIN(id), MAX(id) FROM `+boundsTable).Scan(&lo, &hi); err != nil {
		return false, err
	}
	if !lo.Valid || !hi.Valid || hi.Int64 < lo.Int64 {
		return false, nil
	}
	pivot := lo.Int64 + g.rng.Int63n(hi.Int64-lo.Int64+1)
	for _, dir := range []struct{ cmp, order string }{{">=", "ASC"}, {"<", "DESC"}} {
		q := fmt.Sprintf(`SELECT %s FROM %s WHERE (%s) AND %s %s ?`, cols, from, where, idCol, dir.cmp)
		a := append(append([]any{}, args...), pivot)
		if g.uid != 0 {
			q += fmt.Sprintf(` AND %s = ?`, uidCol)
			a = append(a, g.uid)
		}
		q += fmt.Sprintf(` ORDER BY %s %s LIMIT 1`, idCol, dir.order)
		err := g.r.pool.QueryRowContext(g.ctx, q, a...).Scan(dest...)
		if err == nil {
			return true, nil
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return false, err
		}
	}
	return false, nil
}

// claim marks a record as used; false when it was already asked about in this game.
func (g *triviaGame) claim(kind string, id int64) bool {
	k := fmt.Sprintf("%s:%d", kind, id)
	if g.usedRecordIDs[k] {
		return false
	}
	g.usedRecordIDs[k] = true
	return true
}

// ── question generators (nil, nil = can't make one of this type) ───────────

func (g *triviaGame) faceWho() (*model.TriviaQuestion, error) {
	if err := g.loadFaceNames(); err != nil {
		return nil, err
	}
	if len(g.faceNames) == 0 {
		return nil, nil
	}
	if err := g.loadContactNames(); err != nil {
		return nil, err
	}
	pool := dedupeFold(append(append([]string{}, g.faceNames...), g.contactNames...))
	for i := 0; i < triviaPickAttempts; i++ {
		var faceID, mediaID int64
		var name string
		var year, month sql.NullInt64
		ok, err := g.randomRow("media_item_faces",
			`media_item_faces f JOIN contacts c ON c.id = f.contact_id LEFT JOIN media_items m ON m.id = f.media_item_id`,
			"f.id", "f.user_id",
			"f.id, f.media_item_id, c.name, m.year, m.month",
			`f.contact_id IS NOT NULL AND COALESCE(f.ignored, 0) = 0 AND f.bbox_w >= 0.03 AND TRIM(COALESCE(c.name, '')) <> ''`,
			nil, &faceID, &mediaID, &name, &year, &month)
		if err != nil || !ok {
			return nil, err
		}
		if !g.claim("face", faceID) || !g.claim("photo", mediaID) {
			continue
		}
		opts, idx, ok := buildOptions(strings.TrimSpace(name), pool, g.rng)
		if !ok {
			return nil, nil
		}
		expl := fmt.Sprintf("That's %s", strings.TrimSpace(name))
		if d := monthYear(year, month); d != "" {
			expl += ", in a photo from " + d
		}
		return &model.TriviaQuestion{
			Question:     "Who is this?",
			ImageURL:     fmt.Sprintf("/api/faces/%d/crop", faceID),
			Options:      opts,
			CorrectIndex: idx,
			Explanation:  expl + ".",
			Source:       &model.TriviaSource{Kind: "photo", ID: mediaID},
		}, nil
	}
	return nil, nil
}

func (g *triviaGame) photoYear() (*model.TriviaQuestion, error) {
	if err := g.loadPhotoYears(); err != nil {
		return nil, err
	}
	if len(g.photoYears) == 0 {
		return nil, nil
	}
	for i := 0; i < triviaPickAttempts; i++ {
		var id int64
		var year, month sql.NullInt64
		var title, region string
		ok, err := g.randomRow("media_items", "media_items", "id", "user_id",
			"id, year, month, COALESCE(title, ''), COALESCE(region, '')",
			`media_type LIKE 'image/%' AND year BETWEEN 1900 AND 2100`,
			nil, &id, &year, &month, &title, &region)
		if err != nil || !ok {
			return nil, err
		}
		if !g.claim("photo", id) {
			continue
		}
		opts, idx := yearOptions(int(year.Int64), g.photoYears, g.rng)
		expl := "Taken in " + monthYear(year, month)
		if r := strings.TrimSpace(region); r != "" {
			expl += " — " + r
		}
		return &model.TriviaQuestion{
			Question:     "What year was this photo taken?",
			ImageURL:     fmt.Sprintf("/images/%d?type=metadata&preview=true", id),
			Options:      opts,
			CorrectIndex: idx,
			Explanation:  expl + ".",
			Source:       &model.TriviaSource{Kind: "photo", ID: id},
		}, nil
	}
	return nil, nil
}

func (g *triviaGame) photoPlace() (*model.TriviaQuestion, error) {
	if err := g.loadRegions(); err != nil {
		return nil, err
	}
	if len(g.regions) < 4 {
		return nil, nil
	}
	for i := 0; i < triviaPickAttempts; i++ {
		var id int64
		var year, month sql.NullInt64
		var region string
		ok, err := g.randomRow("media_items", "media_items", "id", "user_id",
			"id, year, month, region",
			`media_type LIKE 'image/%' AND TRIM(COALESCE(region, '')) <> ''`,
			nil, &id, &year, &month, &region)
		if err != nil || !ok {
			return nil, err
		}
		if !g.claim("photo", id) {
			continue
		}
		opts, idx, ok := buildOptions(strings.TrimSpace(region), g.regions, g.rng)
		if !ok {
			return nil, nil
		}
		expl := "This was taken in " + strings.TrimSpace(region)
		if d := monthYear(year, month); d != "" {
			expl += ", " + d
		}
		return &model.TriviaQuestion{
			Question:     "Where was this photo taken?",
			ImageURL:     fmt.Sprintf("/images/%d?type=metadata&preview=true", id),
			Options:      opts,
			CorrectIndex: idx,
			Explanation:  expl + ".",
			Source:       &model.TriviaSource{Kind: "photo", ID: id},
		}, nil
	}
	return nil, nil
}

func (g *triviaGame) messageSender() (*model.TriviaQuestion, error) {
	if err := g.loadSenders(); err != nil {
		return nil, err
	}
	if len(g.senders) < 4 {
		return nil, nil
	}
	for i := 0; i < triviaPickAttempts; i++ {
		var id int64
		var sender, text, date, session string
		ok, err := g.randomRow("messages", "messages", "id", "user_id",
			"id, sender_name, text, COALESCE(substr(message_date, 1, 10), ''), COALESCE(chat_session, '')",
			`COALESCE(type, '') <> 'Outgoing' AND TRIM(COALESCE(sender_name, '')) NOT IN ('', 'Unknown')
			 AND length(text) BETWEEN ? AND ? AND text NOT LIKE 'http%'`,
			[]any{triviaMinQuoteRunes, 3 * triviaQuoteMaxRunes}, &id, &sender, &text, &date, &session)
		if err != nil || !ok {
			return nil, err
		}
		if !g.claim("message", id) {
			continue
		}
		sender = strings.TrimSpace(sender)
		quote := truncateRunes(collapseWS(text), triviaQuoteMaxRunes)
		if utf8.RuneCountInString(quote) < triviaMinQuoteRunes || containsFold(quote, sender) {
			continue // too short, or gives the answer away
		}
		opts, idx, ok := buildOptions(sender, g.senders, g.rng)
		if !ok {
			return nil, nil
		}
		expl := sender + " sent this"
		if d := longDate(date); d != "" {
			expl += " on " + d
		}
		if s := strings.TrimSpace(session); s != "" && !strings.EqualFold(s, sender) {
			expl += " in “" + s + "”"
		}
		return &model.TriviaQuestion{
			Question:     fmt.Sprintf("Who sent %s this message?", g.you()),
			Quote:        quote,
			Options:      opts,
			CorrectIndex: idx,
			Explanation:  expl + ".",
			Source:       &model.TriviaSource{Kind: "message", ID: id},
		}, nil
	}
	return nil, nil
}

func (g *triviaGame) emailSender() (*model.TriviaQuestion, error) {
	if err := g.loadEmailSenders(); err != nil {
		return nil, err
	}
	if len(g.emailSenders) < 4 {
		return nil, nil
	}
	for i := 0; i < triviaPickAttempts; i++ {
		var id int64
		var from, subject, body, date string
		ok, err := g.randomRow("emails", "emails", "id", "user_id",
			"id, from_address, COALESCE(subject, ''), COALESCE(NULLIF(snippet, ''), plain_text, ''), COALESCE(substr(date, 1, 10), '')",
			`COALESCE(user_deleted, 0) = 0 AND COALESCE(is_spam, 0) = 0 AND COALESCE(is_promotional, 0) = 0
			 AND TRIM(COALESCE(from_address, '')) <> '' AND TRIM(COALESCE(subject, '')) <> ''`,
			nil, &id, &from, &subject, &body, &date)
		if err != nil || !ok {
			return nil, err
		}
		if !g.claim("email", id) {
			continue
		}
		name := emailDisplayName(from)
		if name == "" {
			continue
		}
		quote := "Subject: " + truncateRunes(collapseWS(subject), 120)
		if b := truncateRunes(collapseWS(body), 140); b != "" && !containsFold(b, name) {
			quote += "\n" + b
		}
		if containsFold(subject, name) {
			continue
		}
		opts, idx, ok := buildOptions(name, g.emailSenders, g.rng)
		if !ok {
			return nil, nil
		}
		expl := name + " sent this email"
		if d := longDate(date); d != "" {
			expl += " on " + d
		}
		return &model.TriviaQuestion{
			Question:     "Who sent this email?",
			Quote:        quote,
			Options:      opts,
			CorrectIndex: idx,
			Explanation:  expl + ".",
			Source:       &model.TriviaSource{Kind: "email", ID: id},
		}, nil
	}
	return nil, nil
}

func (g *triviaGame) postYear() (*model.TriviaQuestion, error) {
	if err := g.loadPostYears(); err != nil {
		return nil, err
	}
	if len(g.postYears) == 0 {
		return nil, nil
	}
	for i := 0; i < triviaPickAttempts; i++ {
		var id int64
		var text, date string
		ok, err := g.randomRow("facebook_posts", "facebook_posts", "id", "user_id",
			"id, post_text, substr(timestamp, 1, 10)",
			`timestamp GLOB `+timelineDateGlob+` AND length(post_text) BETWEEN ? AND ?`,
			[]any{triviaMinQuoteRunes, 4 * triviaQuoteMaxRunes}, &id, &text, &date)
		if err != nil || !ok {
			return nil, err
		}
		if !g.claim("facebook_post", id) {
			continue
		}
		y, ok := yearOf(date)
		if !ok {
			continue
		}
		opts, idx := yearOptions(y, g.postYears, g.rng)
		return &model.TriviaQuestion{
			Question:     fmt.Sprintf("When did %s post this on Facebook?", g.you()),
			Quote:        truncateRunes(collapseWS(text), triviaQuoteMaxRunes),
			Options:      opts,
			CorrectIndex: idx,
			Explanation:  "Posted on " + longDate(date) + ".",
			Source:       &model.TriviaSource{Kind: "facebook_post", ID: id},
		}, nil
	}
	return nil, nil
}

func (g *triviaGame) topContactYear() (*model.TriviaQuestion, error) {
	if err := g.loadTopByYear(); err != nil {
		return nil, err
	}
	years := make([]int, 0, len(g.topByYear))
	for y := range g.topByYear {
		if !g.usedTopYears[y] {
			years = append(years, y)
		}
	}
	if len(years) == 0 {
		return nil, nil
	}
	sort.Ints(years)
	y := years[g.rng.Intn(len(years))]
	g.usedTopYears[y] = true
	names := g.topByYear[y]
	correct := names[0]
	opts, idx, ok := buildOptions(correct, names[1:], g.rng)
	if !ok {
		return nil, nil
	}
	return &model.TriviaQuestion{
		Question:     fmt.Sprintf("Who did %s message the most in %d?", g.you(), y),
		Options:      opts,
		CorrectIndex: idx,
		Explanation:  fmt.Sprintf("%s was %s most-messaged one-to-one conversation in %d.", correct, g.your(), y),
	}, nil
}

// ── option helpers ─────────────────────────────────────────────────────────

// buildOptions returns 4 options — correct plus 3 distinct distractors from pool (compared
// case-insensitively) — in random order, and the index of the correct one.
func buildOptions(correct string, pool []string, rng *rand.Rand) ([]string, int, bool) {
	correct = strings.TrimSpace(correct)
	if correct == "" {
		return nil, 0, false
	}
	seen := map[string]bool{strings.ToLower(correct): true}
	cands := make([]string, 0, len(pool))
	for _, p := range pool {
		p = strings.TrimSpace(p)
		k := strings.ToLower(p)
		if p == "" || seen[k] {
			continue
		}
		seen[k] = true
		cands = append(cands, p)
	}
	if len(cands) < 3 {
		return nil, 0, false
	}
	rng.Shuffle(len(cands), func(i, j int) { cands[i], cands[j] = cands[j], cands[i] })
	opts := append([]string{}, cands[:3]...)
	idx := rng.Intn(4)
	opts = append(opts[:idx], append([]string{correct}, opts[idx:]...)...)
	return opts, idx, true
}

// yearOptions returns 4 ascending years including correct. Distractors prefer other years the
// archive actually has data for (within ±8 years, so it's a real memory test rather than "which
// of these is plausible"), topped up with nearby years — never in the future.
func yearOptions(correct int, pool []int, rng *rand.Rand) ([]string, int) {
	maxYear := time.Now().Year()
	seen := map[int]bool{correct: true}
	var near []int
	for _, y := range pool {
		if !seen[y] && y >= correct-8 && y <= correct+8 && y <= maxYear {
			seen[y] = true
			near = append(near, y)
		}
	}
	rng.Shuffle(len(near), func(i, j int) { near[i], near[j] = near[j], near[i] })
	picked := near
	if len(picked) > 3 {
		picked = picked[:3]
	}
	for d := 1; len(picked) < 3 && d <= 12; d++ {
		for _, y := range []int{correct - d, correct + d} {
			if len(picked) < 3 && !seen[y] && y >= 1900 && y <= maxYear {
				seen[y] = true
				picked = append(picked, y)
			}
		}
	}
	years := append(picked, correct)
	sort.Ints(years)
	opts := make([]string, len(years))
	idx := 0
	for i, y := range years {
		opts[i] = fmt.Sprintf("%d", y)
		if y == correct {
			idx = i
		}
	}
	return opts, idx
}

// emailDisplayName turns `"Jane Smith" <jane@x.com>` into "Jane Smith", falling back to the
// address itself when there's no display name.
func emailDisplayName(from string) string {
	from = strings.TrimSpace(from)
	if from == "" {
		return ""
	}
	if a, err := mail.ParseAddress(from); err == nil {
		if n := strings.TrimSpace(a.Name); n != "" {
			return n
		}
		return a.Address
	}
	if i := strings.Index(from, "<"); i > 0 {
		return strings.Trim(strings.TrimSpace(from[:i]), `"'`)
	}
	return strings.Trim(from, `<>"' `)
}

func mapStrings(in []string, f func(string) string) []string {
	out := make([]string, 0, len(in))
	for _, s := range in {
		out = append(out, f(s))
	}
	return out
}

func dedupeFold(in []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(in))
	for _, s := range in {
		s = strings.TrimSpace(s)
		k := strings.ToLower(s)
		if s == "" || seen[k] {
			continue
		}
		seen[k] = true
		out = append(out, s)
	}
	return out
}

func monthYear(year, month sql.NullInt64) string {
	if !year.Valid || year.Int64 < 1900 {
		return ""
	}
	if month.Valid && month.Int64 >= 1 && month.Int64 <= 12 {
		return fmt.Sprintf("%s %d", triviaMonthNames[month.Int64-1], year.Int64)
	}
	return fmt.Sprintf("%d", year.Int64)
}

// longDate formats "YYYY-MM-DD" as "3 March 2014"; returns "" for anything unparseable.
func longDate(s string) string {
	t, err := time.Parse("2006-01-02", strings.TrimSpace(s))
	if err != nil {
		return ""
	}
	return fmt.Sprintf("%d %s %d", t.Day(), triviaMonthNames[t.Month()-1], t.Year())
}

// TriviaSubjectName returns the archive subject's configured name ("" when unset).
func (r *TriviaRepo) TriviaSubjectName(ctx context.Context) string {
	q, args := addUIDFilter(`SELECT COALESCE(subject_name, '') FROM subject_configuration WHERE 1 = 1`, nil, uidFromCtx(ctx))
	var name string
	if err := r.pool.QueryRowContext(ctx, q+` ORDER BY id LIMIT 1`, args...).Scan(&name); err != nil {
		return ""
	}
	return strings.TrimSpace(name)
}
