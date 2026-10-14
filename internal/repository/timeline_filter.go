package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"
)

// TimelineFilter optionally narrows the timeline. The zero value means "no filter"; set fields
// combine with AND.
//
//   - PersonID: a contacts row. Messages match on conversation / sender name / sender id
//     (contact name, alternative names, email and messenger ids); emails on from/to/cc
//     (email addresses and names); photos on a face linked to that contact (People in Photos);
//     Facebook posts on a mention of the name in the title or text.
//   - Keywords: every term must appear (substring, case-insensitive) in the item's text.
//   - Tag: an exact photo tag. Only photos have tags, so a tag filter leaves the other sources out.
type TimelineFilter struct {
	PersonID int64
	Keywords []string
	Tag      string
}

// ErrTimelinePersonNotFound is returned when PersonID doesn't name a contact in this archive.
var ErrTimelinePersonNotFound = errors.New("person not found")

const (
	timelineMaxKeywords     = 8
	timelineMinKeywordRunes = 2
	timelineMinNameRunes    = 3 // shorter names/ids would match far too much as substrings
)

// Active reports whether any filter is set.
func (f TimelineFilter) Active() bool {
	return f.PersonID > 0 || len(f.Keywords) > 0 || f.Tag != ""
}

// ParseTimelineKeywords splits a keyword box into terms: whitespace-separated words, with
// "double-quoted phrases" kept together. Terms shorter than two characters are dropped, and at
// most eight are kept.
func ParseTimelineKeywords(s string) []string {
	s, _ = NormalizeQuickSearchQuery(s)
	var out []string
	add := func(t string) {
		t = collapseWS(t)
		if utf8.RuneCountInString(t) >= timelineMinKeywordRunes && len(out) < timelineMaxKeywords {
			out = append(out, t)
		}
	}
	for {
		s = strings.TrimSpace(s)
		if s == "" {
			break
		}
		if s[0] == '"' {
			if end := strings.IndexByte(s[1:], '"'); end >= 0 {
				add(s[1 : end+1])
				s = s[end+2:]
				continue
			}
			s = s[1:] // unbalanced quote — treat the rest as plain words
			continue
		}
		end := strings.IndexAny(s, " \"")
		if end < 0 {
			add(s)
			break
		}
		add(s[:end])
		s = s[end:]
	}
	return out
}

// timelineCond is one source's extra WHERE text (starting " AND ...") and its args.
// skip means the active filter can't apply to that source, so it's left out entirely.
type timelineCond struct {
	sql  string
	args []any
	skip bool
}

// where appends the condition and then the user scope to a query ending in a WHERE clause.
func (c timelineCond) where(q string, args []any, uid int64) (string, []any) {
	return addUIDFilter(q+c.sql, append(args, c.args...), uid)
}

type timelineConds struct {
	messages, emails, photos, posts timelineCond
	excluded                        []string // sources the filter leaves out ("messages", …)
}

type timelinePerson struct {
	names  []string // name + alternative names (long enough to match on)
	ids    []string // messenger ids and email addresses, for messages.sender_id / chat_session
	emails []string
}

func splitList(s string) []string {
	var out []string
	for _, p := range strings.FieldsFunc(s, func(r rune) bool { return r == ',' || r == ';' }) {
		if p = strings.TrimSpace(p); utf8.RuneCountInString(p) >= timelineMinNameRunes {
			out = append(out, p)
		}
	}
	return out
}

func (r *TimelineRepo) loadTimelinePerson(ctx context.Context, id int64) (*timelinePerson, error) {
	q, args := addUIDFilter(`SELECT COALESCE(name, ''), COALESCE(alternative_names, ''), COALESCE(email, ''),
		COALESCE(facebookid, ''), COALESCE(whatsappid, ''), COALESCE(imessageid, ''), COALESCE(smsid, ''), COALESCE(instagramid, '')
		FROM contacts WHERE id = ?`, []any{id}, uidFromCtx(ctx))
	var name, alt, email, fb, wa, im, sms, ig string
	err := r.pool.QueryRowContext(ctx, q, args...).Scan(&name, &alt, &email, &fb, &wa, &im, &sms, &ig)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrTimelinePersonNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("timeline person: %w", err)
	}
	p := &timelinePerson{}
	seen := map[string]bool{}
	addTo := func(dst *[]string, vals ...string) {
		for _, v := range vals {
			k := strings.ToLower(v)
			if !seen[k] {
				seen[k] = true
				*dst = append(*dst, v)
			}
		}
	}
	addTo(&p.names, splitList(name)...)
	addTo(&p.names, splitList(alt)...)
	p.emails = splitList(email)
	for _, s := range []string{fb, wa, im, sms, ig} {
		addTo(&p.ids, splitList(s)...)
	}
	addTo(&p.ids, p.emails...)
	return p, nil
}

// likeAny builds "(colA LIKE ? OR colB LIKE ? …)" over every column × value, or "0" when
// there's nothing to match (so the source counts as zero rather than unfiltered).
func likeAny(cols []string, vals []string) (string, []any) {
	var parts []string
	var args []any
	for _, v := range vals {
		pat := "%" + escapeLike(v) + "%"
		for _, c := range cols {
			parts = append(parts, c+` LIKE ? ESCAPE '\'`)
			args = append(args, pat)
		}
	}
	if len(parts) == 0 {
		return "0", nil
	}
	return "(" + strings.Join(parts, " OR ") + ")", args
}

// likeAll requires every keyword to appear in at least one of cols.
func likeAll(cols []string, keywords []string) (string, []any) {
	var sqlText string
	var args []any
	for _, k := range keywords {
		s, a := likeAny(cols, []string{k})
		sqlText += " AND " + s
		args = append(args, a...)
	}
	return sqlText, args
}

// timelineConditions resolves a filter into per-source WHERE fragments.
func (r *TimelineRepo) timelineConditions(ctx context.Context, f TimelineFilter) (*timelineConds, error) {
	c := &timelineConds{excluded: []string{}}
	if !f.Active() {
		return c, nil
	}
	var person *timelinePerson
	if f.PersonID > 0 {
		p, err := r.loadTimelinePerson(ctx, f.PersonID)
		if err != nil {
			return nil, err
		}
		person = p
	}

	add := func(dst *timelineCond, s string, a []any) {
		dst.sql += s
		dst.args = append(dst.args, a...)
	}

	if person != nil {
		s, a := likeAny([]string{"chat_session", "sender_name"}, person.names)
		s2, a2 := likeAny([]string{"sender_id", "chat_session"}, person.ids)
		add(&c.messages, " AND ("+s+" OR "+s2+")", append(a, a2...))

		s, a = likeAny([]string{"from_address", "to_addresses", "cc_addresses"}, append(append([]string{}, person.emails...), person.names...))
		add(&c.emails, " AND "+s, a)

		add(&c.photos, ` AND EXISTS (SELECT 1 FROM media_item_faces mif
			WHERE mif.media_item_id = media_items.id AND mif.contact_id = ? AND mif.ignored = FALSE)`, []any{f.PersonID})

		s, a = likeAny([]string{"title", "post_text"}, person.names)
		add(&c.posts, " AND "+s, a)
	}

	if len(f.Keywords) > 0 {
		s, a := likeAll([]string{"text"}, f.Keywords)
		add(&c.messages, s, a)
		s, a = likeAll([]string{"subject", "from_address", "plain_text"}, f.Keywords)
		add(&c.emails, s, a)
		s, a = likeAll([]string{"title", "description", "tags"}, f.Keywords)
		add(&c.photos, s, a)
		s, a = likeAll([]string{"title", "post_text"}, f.Keywords)
		add(&c.posts, s, a)
	}

	if f.Tag != "" {
		// Exact tag within the comma-separated list, ignoring spaces around commas (LIKE ignores ASCII case).
		add(&c.photos, ` AND (',' || REPLACE(REPLACE(COALESCE(tags, ''), ', ', ','), ' ,', ',') || ',') LIKE ? ESCAPE '\'`,
			[]any{"%," + escapeLike(f.Tag) + ",%"})
		c.messages.skip, c.emails.skip, c.posts.skip = true, true, true
	}

	for _, s := range []struct {
		name string
		cond timelineCond
	}{{"messages", c.messages}, {"emails", c.emails}, {"photos", c.photos}, {"posts", c.posts}} {
		if s.cond.skip {
			c.excluded = append(c.excluded, s.name)
		}
	}
	return c, nil
}
