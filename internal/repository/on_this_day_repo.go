package repository

import (
	"context"
	"fmt"
	"sort"
	"strconv"

	"github.com/daveontour/aimuseum/internal/model"
)

// On This Day caps per year. Totals are always the full counts.
const (
	onThisDayMaxConversations = 20
	onThisDayMaxEmails        = 20
	onThisDayMaxPhotos        = 30
	onThisDayMaxPosts         = 10
)

// OnThisDay returns everything from years before `year` that happened on month/day, newest year
// first. Like the rest of the timeline, dates are matched on the stored text (substr) rather than
// strftime, so a timezone suffix never shifts an item onto a neighbouring day. Spam and
// promotional emails are left out — they aren't memories.
func (r *TimelineRepo) OnThisDay(ctx context.Context, year, month, day int, src TimelineSources) (*model.OnThisDayResult, error) {
	if year < 1901 || year > 2101 || month < 1 || month > 12 || day < 1 || day > 31 {
		return nil, fmt.Errorf("invalid date %04d-%02d-%02d", year, month, day)
	}
	uid := uidFromCtx(ctx)
	md := fmt.Sprintf("%02d-%02d", month, day)
	beforeYear := fmt.Sprintf("%04d", year)

	out := &model.OnThisDayResult{
		Date:  md,
		Year:  year,
		Years: []model.OnThisDayYear{},
		Limits: map[string]int{
			"conversations": onThisDayMaxConversations,
			"emails":        onThisDayMaxEmails,
			"photos":        onThisDayMaxPhotos,
			"posts":         onThisDayMaxPosts,
		},
		Hidden: []string{},
	}

	byYear := map[int]*model.OnThisDayYear{}
	get := func(y int) *model.OnThisDayYear {
		e := byYear[y]
		if e == nil {
			date := fmt.Sprintf("%04d-%s", y, md)
			e = &model.OnThisDayYear{
				Year:          y,
				YearsAgo:      year - y,
				Conversations: []model.TimelineConversationDay{},
				Emails:        []model.TimelineEmail{},
				Photos:        []model.TimelinePhoto{},
				Posts:         []model.TimelinePost{},
				Totals:        model.TimelineMonth{Month: date},
			}
			byYear[y] = e
		}
		return e
	}

	if !src.Messages {
		out.Hidden = append(out.Hidden, "messages")
	} else {
		q, args := addUIDFilter(`SELECT substr(message_date, 1, 10), COALESCE(chat_session, ''), COUNT(*), MIN(id)
			FROM messages
			WHERE message_date GLOB `+timelineDateGlob+`
			  AND substr(message_date, 6, 5) = ? AND substr(message_date, 1, 4) < ?`, []any{md, beforeYear}, uid)
		q += ` GROUP BY 1, 2 ORDER BY 1 DESC, 3 DESC`
		rows, err := r.pool.QueryContext(ctx, q, args...)
		if err != nil {
			return nil, fmt.Errorf("on this day messages: %w", err)
		}
		for rows.Next() {
			var c model.TimelineConversationDay
			if err := rows.Scan(&c.Date, &c.ChatSession, &c.Count, &c.FirstMessageID); err != nil {
				_ = rows.Close()
				return nil, err
			}
			y, ok := yearOf(c.Date)
			if !ok {
				continue
			}
			e := get(y)
			e.Totals.Messages += c.Count
			if len(e.Conversations) < onThisDayMaxConversations {
				e.Conversations = append(e.Conversations, c)
			}
		}
		_ = rows.Close()
		if err := rows.Err(); err != nil {
			return nil, err
		}
	}

	if !src.Emails {
		out.Hidden = append(out.Hidden, "emails")
	} else {
		q, args := addUIDFilter(`SELECT id, substr(date, 1, 10), substr(date, 12, 5), COALESCE(subject, ''), COALESCE(from_address, '')
			FROM emails
			WHERE COALESCE(user_deleted, 0) = 0 AND COALESCE(is_spam, 0) = 0 AND COALESCE(is_promotional, 0) = 0
			  AND date GLOB `+timelineDateGlob+`
			  AND substr(date, 6, 5) = ? AND substr(date, 1, 4) < ?`, []any{md, beforeYear}, uid)
		q += ` ORDER BY date DESC`
		rows, err := r.pool.QueryContext(ctx, q, args...)
		if err != nil {
			return nil, fmt.Errorf("on this day emails: %w", err)
		}
		for rows.Next() {
			var em model.TimelineEmail
			if err := rows.Scan(&em.ID, &em.Date, &em.Time, &em.Subject, &em.From); err != nil {
				_ = rows.Close()
				return nil, err
			}
			y, ok := yearOf(em.Date)
			if !ok {
				continue
			}
			e := get(y)
			e.Totals.Emails++
			if len(e.Emails) < onThisDayMaxEmails {
				e.Emails = append(e.Emails, em)
			}
		}
		_ = rows.Close()
		if err := rows.Err(); err != nil {
			return nil, err
		}
	}

	{
		q, args := addUIDFilter(`SELECT id, year, COALESCE(title, '')
			FROM media_items
			WHERE media_type LIKE 'image/%' AND month = ? AND day = ? AND year >= 1900 AND year < ?`, []any{month, day, year}, uid)
		q += ` ORDER BY year DESC, id`
		rows, err := r.pool.QueryContext(ctx, q, args...)
		if err != nil {
			return nil, fmt.Errorf("on this day photos: %w", err)
		}
		for rows.Next() {
			var p model.TimelinePhoto
			var y int
			if err := rows.Scan(&p.ID, &y, &p.Title); err != nil {
				_ = rows.Close()
				return nil, err
			}
			p.Day = day
			e := get(y)
			e.Totals.Photos++
			if len(e.Photos) < onThisDayMaxPhotos {
				e.Photos = append(e.Photos, p)
			}
		}
		_ = rows.Close()
		if err := rows.Err(); err != nil {
			return nil, err
		}
	}

	{
		q, args := addUIDFilter(`SELECT id, substr(timestamp, 1, 10), COALESCE(title, ''), COALESCE(post_text, '')
			FROM facebook_posts
			WHERE timestamp GLOB `+timelineDateGlob+`
			  AND substr(timestamp, 6, 5) = ? AND substr(timestamp, 1, 4) < ?`, []any{md, beforeYear}, uid)
		q += ` ORDER BY timestamp DESC`
		rows, err := r.pool.QueryContext(ctx, q, args...)
		if err != nil {
			return nil, fmt.Errorf("on this day posts: %w", err)
		}
		for rows.Next() {
			var p model.TimelinePost
			if err := rows.Scan(&p.ID, &p.Date, &p.Title, &p.Text); err != nil {
				_ = rows.Close()
				return nil, err
			}
			y, ok := yearOf(p.Date)
			if !ok {
				continue
			}
			e := get(y)
			e.Totals.Posts++
			if len(e.Posts) < onThisDayMaxPosts {
				p.Text = truncateRunes(collapseWS(p.Text), 240)
				e.Posts = append(e.Posts, p)
			}
		}
		_ = rows.Close()
		if err := rows.Err(); err != nil {
			return nil, err
		}
	}

	for _, e := range byYear {
		out.Years = append(out.Years, *e)
		out.Total += e.Totals.Messages + e.Totals.Emails + e.Totals.Photos + e.Totals.Posts
	}
	sort.Slice(out.Years, func(i, j int) bool { return out.Years[i].Year > out.Years[j].Year })
	return out, nil
}

// yearOf parses the leading "YYYY" of a stored date, rejecting implausible years.
func yearOf(date string) (int, bool) {
	if len(date) < 4 {
		return 0, false
	}
	y, err := strconv.Atoi(date[:4])
	if err != nil || y < 1900 || y > 2100 {
		return 0, false
	}
	return y, true
}
