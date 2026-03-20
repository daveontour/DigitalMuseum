package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"

	appcrypto "github.com/daveontour/digitalmuseum/internal/crypto"
	"github.com/jackc/pgx/v5/pgxpool"
)

// RAMMasterGetter returns the session master password when the browser has unlocked the keyring.
type RAMMasterGetter func() (password string, ok bool)

// NewToolExecutor creates a ToolExecutor backed by the provided pool.
// getRAM returns the sensitive keyring password for this HTTP request's session; used to decrypt encrypted reference documents for AI tools.
func NewToolExecutor(pool *pgxpool.Pool, subjectName, tavilyKey, pepper string, getRAM RAMMasterGetter) ToolExecutor {
	if getRAM == nil {
		getRAM = func() (string, bool) { return "", false }
	}
	return func(ctx context.Context, name string, args map[string]any) (map[string]any, error) {
		switch name {
		case "get_current_time":
			return map[string]any{
				"current_time": time.Now().UTC().Format(time.RFC3339),
				"timezone":     "UTC",
			}, nil
		case "get_imessages_by_chat_session":
			chatSession, _ := args["chat_session"].(string)
			return getMessagesByChatSession(ctx, pool, chatSession)
		case "get_emails_by_contact":
			n, _ := args["name"].(string)
			return getEmailsByContact(ctx, pool, n)
		case "get_subject_writing_examples":
			return getSubjectWritingExamples(ctx, pool, subjectName)
		case "search_tavily":
			query, _ := args["query"].(string)
			return searchTavily(tavilyKey, query)
		case "get_all_messages_by_contact":
			n, _ := args["name"].(string)
			return getAllMessagesByContact(ctx, pool, n)
		case "get_unique_tags_count":
			return getUniqueTagsCount(ctx, pool)
		case "search_facebook_albums":
			keyword, _ := args["keyword"].(string)
			return searchFacebookAlbums(ctx, pool, keyword)
		case "search_facebook_posts":
			desc, _ := args["description"].(string)
			return searchFacebookPosts(ctx, pool, desc)
		case "get_all_facebook_posts":
			return getAllFacebookPosts(ctx, pool)
		case "get_user_interests":
			return getUserInterests(ctx, pool)
		case "get_available_reference_documents":
			return getAvailableReferenceDocuments(ctx, pool)
		case "get_reference_document":
			idsRaw, _ := args["document_ids"].([]any)
			var ids []int64
			for _, v := range idsRaw {
				switch x := v.(type) {
				case float64:
					ids = append(ids, int64(x))
				case int64:
					ids = append(ids, x)
				}
			}
			return getReferenceDocuments(ctx, pool, ids, pepper, getRAM)
		default:
			return nil, fmt.Errorf("unknown tool: %s", name)
		}
	}
}

// tool to get the title, description, id and tags of all reference documents
func getAvailableReferenceDocuments(ctx context.Context, pool *pgxpool.Pool) (map[string]any, error) {
	rows, err := pool.Query(ctx, `SELECT id, title, description, tags FROM reference_documents WHERE available_for_task = TRUE`)
	if err != nil {
		return map[string]any{"error": err.Error(), "documents": []any{}}, nil
	}
	defer rows.Close()
	var documents []map[string]any

	for rows.Next() {
		var id int64
		var title, description, tags *string
		if err := rows.Scan(&id, &title, &description, &tags); err != nil {
			continue
		}
		documents = append(documents, map[string]any{
			"id":          id,
			"title":       title,
			"description": description,
			"tags":        tags,
		})
	}
	if documents == nil {
		documents = []map[string]any{}
	}
	return map[string]any{"documents": documents}, nil
}

func getMessagesByChatSession(ctx context.Context, pool *pgxpool.Pool, chatSession string) (map[string]any, error) {
	rows, err := pool.Query(ctx,
		`SELECT id, message_date, sender_name, sender_id, type, text, service, subject
		 FROM messages WHERE chat_session ILIKE $1 ORDER BY message_date ASC`,
		"%"+chatSession+"%")
	if err != nil {
		return map[string]any{"error": err.Error(), "chat_session": chatSession, "message_count": 0, "messages": []any{}}, nil
	}
	defer rows.Close()
	var msgs []map[string]any
	for rows.Next() {
		var id int64
		var msgDate *time.Time
		var senderName, senderID, typ, text, service, subject *string
		if err := rows.Scan(&id, &msgDate, &senderName, &senderID, &typ, &text, &service, &subject); err != nil {
			continue
		}
		m := map[string]any{
			"id":           id,
			"message_date": nil,
			"sender_name":  strVal(senderName, "Unknown"),
			"sender_id":    strVal(senderID, ""),
			"type":         strVal(typ, ""),
			"text":         strVal(text, ""),
			"service":      strVal(service, ""),
			"subject":      strVal(subject, ""),
		}
		if msgDate != nil {
			m["message_date"] = msgDate.Format(time.RFC3339)
		}
		msgs = append(msgs, m)
	}
	if msgs == nil {
		msgs = []map[string]any{}
	}
	return map[string]any{
		"chat_session":  chatSession,
		"message_count": len(msgs),
		"messages":      msgs,
	}, nil
}

func getEmailsByContact(ctx context.Context, pool *pgxpool.Pool, name string) (map[string]any, error) {
	pattern := "%" + name + "%"
	rows, err := pool.Query(ctx,
		`SELECT id, date, from_address, to_addresses, subject, plain_text, snippet, has_attachments
		 FROM emails WHERE from_address ILIKE $1 OR to_addresses ILIKE $1 ORDER BY date ASC`,
		pattern)
	if err != nil {
		return map[string]any{"error": err.Error(), "contact_name": name, "email_count": 0, "emails": []any{}}, nil
	}
	defer rows.Close()
	var emails []map[string]any
	for rows.Next() {
		var id int64
		var date *time.Time
		var from, to, subject, plainText, snippet *string
		var hasAttachments bool
		if err := rows.Scan(&id, &date, &from, &to, &subject, &plainText, &snippet, &hasAttachments); err != nil {
			continue
		}
		text := ""
		if plainText != nil && *plainText != "" {
			text = *plainText
		} else if snippet != nil {
			text = *snippet
		}
		e := map[string]any{
			"id":              id,
			"date":            nil,
			"from_address":    strVal(from, ""),
			"to_addresses":    strVal(to, ""),
			"subject":         strVal(subject, ""),
			"plain_text":      text,
			"has_attachments": hasAttachments,
		}
		if date != nil {
			e["date"] = date.Format(time.RFC3339)
		}
		emails = append(emails, e)
	}
	if emails == nil {
		emails = []map[string]any{}
	}
	return map[string]any{
		"contact_name": name,
		"email_count":  len(emails),
		"emails":       emails,
	}, nil
}

func getSubjectWritingExamples(ctx context.Context, pool *pgxpool.Pool, subjectName string) (map[string]any, error) {
	rows, err := pool.Query(ctx,
		`SELECT id, message_date, sender_name, sender_id, type, text, service, subject
		 FROM messages WHERE sender_id = $1 AND type = 'text' ORDER BY RANDOM() LIMIT 200`,
		subjectName)
	if err != nil {
		return map[string]any{"error": err.Error()}, nil
	}
	defer rows.Close()
	var msgs []map[string]any
	for rows.Next() {
		var id int64
		var msgDate *time.Time
		var senderName, senderID, typ, text, service, subject *string
		if err := rows.Scan(&id, &msgDate, &senderName, &senderID, &typ, &text, &service, &subject); err != nil {
			continue
		}
		m := map[string]any{
			"id":           id,
			"message_date": nil,
			"sender_name":  strVal(senderName, "Unknown"),
			"sender_id":    strVal(senderID, ""),
			"type":         strVal(typ, ""),
			"text":         strVal(text, ""),
			"service":      strVal(service, ""),
			"subject":      strVal(subject, ""),
		}
		if msgDate != nil {
			m["message_date"] = msgDate.Format(time.RFC3339)
		}
		msgs = append(msgs, m)
	}
	if msgs == nil {
		msgs = []map[string]any{}
	}
	return map[string]any{"subject_writing_examples": msgs}, nil
}

func getAllMessagesByContact(ctx context.Context, pool *pgxpool.Pool, name string) (map[string]any, error) {
	messages, _ := getMessagesByChatSession(ctx, pool, name)
	emails, _ := getEmailsByContact(ctx, pool, name)
	messagesJSON, _ := json.Marshal(messages)
	emailsJSON, _ := json.Marshal(emails)
	return map[string]any{
		"contact_name":        name,
		"relationshipSummary": fmt.Sprintf("Messages: %s\nEmails: %s", string(messagesJSON), string(emailsJSON)),
	}, nil
}

func getUniqueTagsCount(ctx context.Context, pool *pgxpool.Pool) (map[string]any, error) {
	mediaTags := map[string]struct{}{}
	rows, err := pool.Query(ctx, `SELECT tags FROM media_items WHERE tags IS NOT NULL AND tags != ''`)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var raw string
			if err := rows.Scan(&raw); err == nil {
				for _, t := range strings.Split(raw, ",") {
					if s := strings.TrimSpace(t); s != "" {
						mediaTags[s] = struct{}{}
					}
				}
			}
		}
	}
	artefactTags := map[string]struct{}{}
	rows2, err := pool.Query(ctx, `SELECT tags FROM artefacts WHERE tags IS NOT NULL AND tags != ''`)
	if err == nil {
		defer rows2.Close()
		for rows2.Next() {
			var raw string
			if err := rows2.Scan(&raw); err == nil {
				for _, t := range strings.Split(raw, ",") {
					if s := strings.TrimSpace(t); s != "" {
						artefactTags[s] = struct{}{}
					}
				}
			}
		}
	}
	combined := map[string]struct{}{}
	for k := range mediaTags {
		combined[k] = struct{}{}
	}
	for k := range artefactTags {
		combined[k] = struct{}{}
	}
	return map[string]any{
		"media_items_unique_tag_count": len(mediaTags),
		"media_items_tags":             sortedKeys(mediaTags),
		"artefacts_unique_tag_count":   len(artefactTags),
		"artefacts_tags":               sortedKeys(artefactTags),
		"combined_unique_tag_count":    len(combined),
		"combined_tags":                sortedKeys(combined),
	}, nil
}

func searchFacebookAlbums(ctx context.Context, pool *pgxpool.Pool, keyword string) (map[string]any, error) {
	pattern := "%" + keyword + "%"
	rows, err := pool.Query(ctx,
		`SELECT id, name, description, cover_photo_uri, last_modified_timestamp
		 FROM facebook_albums WHERE name ILIKE $1 OR description ILIKE $1 ORDER BY name ASC`,
		pattern)
	if err != nil {
		return map[string]any{"error": err.Error(), "albums": []any{}, "count": 0}, nil
	}
	defer rows.Close()
	var albums []map[string]any
	for rows.Next() {
		var id int64
		var name, desc, coverURI *string
		var lastMod *time.Time
		if err := rows.Scan(&id, &name, &desc, &coverURI, &lastMod); err != nil {
			continue
		}
		a := map[string]any{
			"id":              id,
			"name":            strVal(name, ""),
			"description":     strVal(desc, ""),
			"cover_photo_uri": strVal(coverURI, ""),
			"last_modified":   nil,
		}
		if lastMod != nil {
			a["last_modified"] = lastMod.Format(time.RFC3339)
		}
		albums = append(albums, a)
	}
	if albums == nil {
		albums = []map[string]any{}
	}
	return map[string]any{"keyword": keyword, "count": len(albums), "albums": albums}, nil
}

func searchFacebookPosts(ctx context.Context, pool *pgxpool.Pool, description string) (map[string]any, error) {
	pattern := "%" + description + "%"
	rows, err := pool.Query(ctx,
		`SELECT id, title, post_text FROM facebook_posts WHERE post_text ILIKE $1 ORDER BY timestamp DESC`,
		pattern)
	if err != nil {
		return map[string]any{"error": err.Error(), "posts": []any{}, "count": 0}, nil
	}
	defer rows.Close()
	var posts []map[string]any
	for rows.Next() {
		var id int64
		var title, postText *string
		if err := rows.Scan(&id, &title, &postText); err != nil {
			continue
		}
		posts = append(posts, map[string]any{
			"id":          id,
			"title":       strVal(title, ""),
			"description": strVal(postText, ""),
		})
	}
	if posts == nil {
		posts = []map[string]any{}
	}
	return map[string]any{"description": description, "count": len(posts), "posts": posts}, nil
}

func getAllFacebookPosts(ctx context.Context, pool *pgxpool.Pool) (map[string]any, error) {
	rows, err := pool.Query(ctx,
		`SELECT id, timestamp, title, post_text, external_url, post_type FROM facebook_posts ORDER BY timestamp DESC`)
	if err != nil {
		return map[string]any{"error": err.Error(), "posts": []any{}, "count": 0}, nil
	}
	defer rows.Close()
	var posts []map[string]any
	for rows.Next() {
		var id int64
		var ts *time.Time
		var title, postText, extURL, postType *string
		if err := rows.Scan(&id, &ts, &title, &postText, &extURL, &postType); err != nil {
			continue
		}
		p := map[string]any{
			"id":           id,
			"timestamp":    nil,
			"title":        strVal(title, ""),
			"description":  strVal(postText, ""),
			"external_url": strVal(extURL, ""),
			"post_type":    strVal(postType, ""),
		}
		if ts != nil {
			p["timestamp"] = ts.Format(time.RFC3339)
		}
		posts = append(posts, p)
	}
	if posts == nil {
		posts = []map[string]any{}
	}
	return map[string]any{"count": len(posts), "posts": posts}, nil
}

func getUserInterests(ctx context.Context, pool *pgxpool.Pool) (map[string]any, error) {
	rows, err := pool.Query(ctx, `SELECT name FROM interests ORDER BY name`)
	if err != nil {
		return map[string]any{"error": err.Error(), "interests": []any{}}, nil
	}
	defer rows.Close()
	var interests []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err == nil && name != "" {
			interests = append(interests, name)
		}
	}
	if interests == nil {
		interests = []string{}
	}
	return map[string]any{"interests": interests, "count": len(interests)}, nil
}

func getReferenceDocuments(ctx context.Context, pool *pgxpool.Pool, ids []int64, pepper string, getRAM RAMMasterGetter) (map[string]any, error) {
	masterPassword, haveMaster := "", false
	if getRAM != nil {
		masterPassword, haveMaster = getRAM()
	}
	var results []map[string]any
	for _, id := range ids {
		var title, filename, contentType *string
		var data []byte
		var isEncrypted bool
		err := pool.QueryRow(ctx,
			`SELECT title, filename, content_type, data, is_encrypted FROM reference_documents WHERE id = $1 AND is_sensitive = FALSE`, id,
		).Scan(&title, &filename, &contentType, &data, &isEncrypted)
		if err != nil {
			results = append(results, map[string]any{"id": id, "error": "not found"})
			continue
		}
		ct := ""
		if contentType != nil {
			ct = *contentType
		}
		displayTitle := strVal(title, strVal(filename, ""))
		if isEncrypted {
			if !haveMaster || masterPassword == "" {
				results = append(results, map[string]any{"id": id, "title": displayTitle, "content": "[encrypted — master key not unlocked in this session]"})
				continue
			}
			plain, err := appcrypto.DecryptDocumentData(ctx, pool, masterPassword, data, pepper)
			if err != nil || len(plain) == 0 {
				results = append(results, map[string]any{"id": id, "title": displayTitle, "content": "[encrypted — decryption failed]"})
				continue
			}
			data = plain
		}
		if ct == "application/pdf" {
			results = append(results, map[string]any{
				"id":      id,
				"title":   displayTitle,
				"content": "[PDF document — not renderable as text]",
			})
		} else {
			results = append(results, map[string]any{
				"id":      id,
				"title":   displayTitle,
				"content": string(data),
			})
		}
	}
	if results == nil {
		results = []map[string]any{}
	}
	return map[string]any{"documents": results}, nil
}

func searchTavily(tavilyKey, query string) (map[string]any, error) {
	if tavilyKey == "" {
		return map[string]any{"error": "Tavily search not configured (TAVILY_API_KEY missing)"}, nil
	}
	body, _ := json.Marshal(map[string]any{
		"api_key":      tavilyKey,
		"query":        query,
		"search_depth": "advanced",
		"max_results":  5,
	})
	resp, err := http.Post("https://api.tavily.com/search", "application/json", bytes.NewReader(body))
	if err != nil {
		return map[string]any{"error": err.Error(), "query": query}, nil
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	var result map[string]any
	if err := json.Unmarshal(b, &result); err != nil {
		return map[string]any{"error": "invalid response", "query": query}, nil
	}
	return result, nil
}

// strVal dereferences a *string or returns def.
func strVal(s *string, def string) string {
	if s != nil {
		return *s
	}
	return def
}

// sortedKeys returns the keys of a set map in sorted order.
func sortedKeys(m map[string]struct{}) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
