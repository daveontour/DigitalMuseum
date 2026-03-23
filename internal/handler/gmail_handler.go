package handler

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"sort"
	"time"

	appgmail "github.com/daveontour/digitalmuseum/internal/gmail"
	appimporter "github.com/daveontour/digitalmuseum/internal/importer"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/oauth2"
)

// gmailAttachmentSource is media_items.source for attachments imported via Gmail API (distinct from IMAP).
const gmailAttachmentSource = "gmail_attachment"

// GmailHandler handles all /gmail/* routes.
type GmailHandler struct {
	pool     *pgxpool.Pool
	oauthCfg *oauth2.Config
	job      *appimporter.ImportJob
}

// NewGmailHandler creates a GmailHandler.
func NewGmailHandler(pool *pgxpool.Pool, clientID, clientSecret, redirectURL string) *GmailHandler {
	return &GmailHandler{
		pool:     pool,
		oauthCfg: appgmail.OAuthConfig(clientID, clientSecret, redirectURL),
		job: appimporter.NewImportJob("Gmail import", map[string]any{
			"status":               "idle",
			"status_line":          nil,
			"error_message":        nil,
			"current_label":        nil,
			"current_label_index":  0,
			"total_labels":         0,
			"emails_processed":     0,
			"labels":               []string{},
		}),
	}
}

// RegisterRoutes mounts all Gmail routes.
func (h *GmailHandler) RegisterRoutes(r chi.Router) {
	// OAuth management
	r.Get("/gmail/auth/start", h.AuthStart)
	r.Get("/gmail/auth/callback", h.AuthCallback)
	r.Get("/gmail/auth/status", h.AuthStatus)
	r.Delete("/gmail/auth", h.AuthRevoke)

	// Label listing
	r.Get("/gmail/labels", h.GetLabels)

	// Import job (mirrors /imap/process pattern)
	r.Post("/gmail/process", h.StartProcess)
	r.Get("/gmail/process/stream", h.StreamProgress)
	r.Post("/gmail/process/cancel", h.CancelProcess)
	r.Get("/gmail/process/status", h.GetStatus)
}

// ── OAuth handlers ────────────────────────────────────────────────────────────

// AuthStart handles GET /gmail/auth/start
// Generates a CSRF state, saves it, and redirects to Google's consent page.
func (h *GmailHandler) AuthStart(w http.ResponseWriter, r *http.Request) {
	if h.oauthCfg.ClientID == "" || h.oauthCfg.ClientSecret == "" {
		writeError(w, http.StatusServiceUnavailable,
			"Gmail OAuth is not configured — set GMAIL_CLIENT_ID and GMAIL_CLIENT_SECRET")
		return
	}
	stateBytes := make([]byte, 16)
	if _, err := rand.Read(stateBytes); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to generate state")
		return
	}
	state := hex.EncodeToString(stateBytes)

	if err := appgmail.SaveState(r.Context(), h.pool, state); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to save state")
		return
	}

	url := h.oauthCfg.AuthCodeURL(state, oauth2.AccessTypeOffline)
	http.Redirect(w, r, url, http.StatusFound)
}

// AuthCallback handles GET /gmail/auth/callback
// Validates the state, exchanges the code for a token, and saves it.
func (h *GmailHandler) AuthCallback(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	storedState, err := appgmail.LoadState(ctx, h.pool)
	if err != nil || storedState == "" {
		writeError(w, http.StatusBadRequest, "invalid or missing OAuth state")
		return
	}
	if r.URL.Query().Get("state") != storedState {
		writeError(w, http.StatusBadRequest, "state mismatch")
		return
	}
	_ = appgmail.DeleteState(ctx, h.pool)

	code := r.URL.Query().Get("code")
	if code == "" {
		writeError(w, http.StatusBadRequest, "missing code parameter")
		return
	}

	tok, err := h.oauthCfg.Exchange(ctx, code)
	if err != nil {
		writeError(w, http.StatusInternalServerError,
			fmt.Sprintf("token exchange failed: %s", err))
		return
	}

	if err := appgmail.SaveToken(ctx, h.pool, tok); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to save token")
		return
	}

	http.Redirect(w, r, "/", http.StatusFound)
}

// AuthStatus handles GET /gmail/auth/status
func (h *GmailHandler) AuthStatus(w http.ResponseWriter, r *http.Request) {
	tok, err := appgmail.LoadToken(r.Context(), h.pool)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if tok == nil {
		writeJSON(w, map[string]any{"authenticated": false})
		return
	}
	resp := map[string]any{"authenticated": true}
	if !tok.Expiry.IsZero() {
		resp["expiry"] = tok.Expiry.UTC().Format(time.RFC3339)
	}
	writeJSON(w, resp)
}

// AuthRevoke handles DELETE /gmail/auth
func (h *GmailHandler) AuthRevoke(w http.ResponseWriter, r *http.Request) {
	if err := appgmail.DeleteToken(r.Context(), h.pool); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, map[string]any{"message": "token removed"})
}

// ── Label listing ─────────────────────────────────────────────────────────────

type labelItem struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// GetLabels handles GET /gmail/labels
// Returns [{id, name}] sorted by name so the UI can display names but submit IDs.
func (h *GmailHandler) GetLabels(w http.ResponseWriter, r *http.Request) {
	tok, err := appgmail.LoadToken(r.Context(), h.pool)
	if err != nil || tok == nil {
		writeError(w, http.StatusUnauthorized, "not authenticated with Gmail")
		return
	}

	client, err := appgmail.NewClient(r.Context(), h.oauthCfg, tok)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	labelMap, err := client.ListLabels(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	labels := make([]labelItem, 0, len(labelMap))
	for id, name := range labelMap {
		labels = append(labels, labelItem{ID: id, Name: name})
	}
	sort.Slice(labels, func(i, j int) bool { return labels[i].Name < labels[j].Name })
	writeJSON(w, map[string]any{"labels": labels})
}

// ── Import job ────────────────────────────────────────────────────────────────

type gmailProcessRequest struct {
	LabelIDs      []string `json:"label_ids"`
	AllLabels     bool     `json:"all_labels"`
	ExcludeLabels []string `json:"exclude_labels"`
	NewOnly       bool     `json:"new_only"`
}

// StartProcess handles POST /gmail/process
func (h *GmailHandler) StartProcess(w http.ResponseWriter, r *http.Request) {
	if err := h.job.AssertNotRunning(); err != nil {
		writeError(w, http.StatusConflict, err.Error())
		return
	}

	tok, err := appgmail.LoadToken(r.Context(), h.pool)
	if err != nil || tok == nil {
		writeError(w, http.StatusUnauthorized, "not authenticated with Gmail")
		return
	}

	var req gmailProcessRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	// Resolve label IDs to process
	client, err := appgmail.NewClient(r.Context(), h.oauthCfg, tok)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	labelMap, err := client.ListLabels(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError,
			fmt.Sprintf("could not list labels: %s", err))
		return
	}

	var labelIDs []string
	if req.AllLabels {
		for id := range labelMap {
			labelIDs = append(labelIDs, id)
		}
	} else if len(req.LabelIDs) > 0 {
		labelIDs = req.LabelIDs
	} else {
		// Default to INBOX
		for id, name := range labelMap {
			if name == "INBOX" {
				labelIDs = []string{id}
				break
			}
		}
		if len(labelIDs) == 0 {
			labelIDs = []string{"INBOX"}
		}
	}

	if len(req.ExcludeLabels) > 0 {
		filtered := make([]string, 0, len(labelIDs))
		for _, id := range labelIDs {
			name := labelMap[id]
			excluded := false
			for _, pattern := range req.ExcludeLabels {
				re, err := regexp.Compile(pattern)
				if err != nil {
					continue
				}
				if re.MatchString(name) || re.MatchString(id) {
					excluded = true
					break
				}
			}
			if !excluded {
				filtered = append(filtered, id)
			}
		}
		labelIDs = filtered
	}

	go h.runGmailImport(tok, labelIDs, labelMap, req.NewOnly)

	writeJSON(w, map[string]any{
		"message":   fmt.Sprintf("Gmail import started for %d label(s)", len(labelIDs)),
		"label_ids": labelIDs,
	})
}

// StreamProgress handles GET /gmail/process/stream
func (h *GmailHandler) StreamProgress(w http.ResponseWriter, r *http.Request) {
	h.job.ServeSSE(w, r)
}

// CancelProcess handles POST /gmail/process/cancel
func (h *GmailHandler) CancelProcess(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, h.job.Cancel())
}

// GetStatus handles GET /gmail/process/status
func (h *GmailHandler) GetStatus(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, h.job.Status())
}

// ── Background import goroutine ───────────────────────────────────────────────

func (h *GmailHandler) runGmailImport(tok *oauth2.Token, labelIDs []string, labelMap map[string]string, newOnly bool) {
	h.job.Start()
	defer h.job.Finish()

	// Build display names for the label IDs being processed.
	labelNames := make([]string, 0, len(labelIDs))
	for _, id := range labelIDs {
		if name, ok := labelMap[id]; ok {
			labelNames = append(labelNames, name)
		} else {
			labelNames = append(labelNames, id)
		}
	}

	h.job.UpdateState(map[string]any{
		"status":               "in_progress",
		"status_line":          "Connecting to Gmail...",
		"current_label":        nil,
		"current_label_index":  0,
		"total_labels":         len(labelIDs),
		"emails_processed":     0,
		"labels":               labelNames,
		"error_message":        nil,
	})
	h.job.Broadcast("progress", h.job.GetState())

	ctx := context.Background()

	client, err := appgmail.NewClient(ctx, h.oauthCfg, tok)
	if err != nil {
		h.job.UpdateState(map[string]any{
			"status":        "error",
			"error_message": err.Error(),
			"status_line":   err.Error(),
		})
		h.job.Broadcast("error", h.job.GetState())
		return
	}

	// Build existing-UID set for newOnly filtering.
	existingUIDs := map[string]struct{}{}
	if newOnly {
		rows, err := h.pool.Query(ctx, `SELECT uid FROM emails`)
		if err == nil {
			defer rows.Close()
			for rows.Next() {
				var uid string
				if rows.Scan(&uid) == nil {
					existingUIDs[uid] = struct{}{}
				}
			}
		}
	}

	totalProcessed := 0

	for idx, labelID := range labelIDs {
		if h.job.IsCancelled() {
			h.job.UpdateState(map[string]any{
				"status":      "cancelled",
				"status_line": "Cancelled by user",
			})
			h.job.Broadcast("cancelled", h.job.GetState())
			return
		}

		labelName := labelNames[idx]
		h.job.UpdateState(map[string]any{
			"current_label":        labelName,
			"current_label_index":  idx + 1,
			"status_line":          fmt.Sprintf("Processing label: %s (%d/%d)", labelName, idx+1, len(labelIDs)),
		})
		h.job.Broadcast("progress", h.job.GetState())

		count, err := h.importLabel(ctx, client, []string{labelID}, labelName, newOnly, existingUIDs, &totalProcessed)
		if err != nil {
			msg := fmt.Sprintf("Error processing label %s: %s", labelName, err)
			h.job.UpdateState(map[string]any{
				"status":        "error",
				"error_message": msg,
				"status_line":   msg,
			})
			h.job.Broadcast("error", h.job.GetState())
			return
		}

		totalProcessed += count
		h.job.UpdateState(map[string]any{
			"emails_processed": totalProcessed,
			"status_line":      fmt.Sprintf("Label %s: %d emails. Total: %d", labelName, count, totalProcessed),
		})
		h.job.Broadcast("progress", h.job.GetState())
	}

	h.job.UpdateState(map[string]any{
		"status":      "completed",
		"status_line": fmt.Sprintf("Import completed. %d emails processed.", totalProcessed),
	})
	runThumbnailsAfterImportIfIdle(h.pool)
	h.job.Broadcast("completed", h.job.GetState())
}

func (h *GmailHandler) importLabel(
	ctx context.Context,
	client *appgmail.Client,
	labelIDs []string,
	labelName string,
	newOnly bool,
	existingUIDs map[string]struct{},
	totalProcessed *int,
) (int, error) {
	progressFn := func(fetched, estimated int) {
		if fetched%10 == 0 {
			h.job.UpdateState(map[string]any{
				"emails_processed": *totalProcessed + fetched,
				"status_line": fmt.Sprintf("Fetching %s: %d/%d messages", labelName, fetched, estimated),
			})
			h.job.Broadcast("progress", h.job.GetState())
		}
	}

	messages, err := client.FetchMessages(ctx, labelIDs, newOnly, existingUIDs, progressFn)
	if err != nil {
		return 0, err
	}

	count := 0
	for _, msg := range messages {
		if h.job.IsCancelled() {
			break
		}
		if err := h.storeGmailEmail(ctx, msg); err != nil {
			fmt.Printf("[Gmail] warning storing email %s: %s\n", msg.UID, err)
			continue
		}
		count++
		existingUIDs[msg.UID] = struct{}{} // avoid re-inserting same message from another label
	}
	return count, nil
}

func (h *GmailHandler) storeGmailEmail(ctx context.Context, msg *appgmail.Message) error {
	date := msg.Date
	if date.IsZero() {
		date = time.Now()
	}

	// emails.uid / folder are VARCHAR(255); Gmail folder is comma-joined label names and often exceeds 255.
	uid := truncateUTF8Runes(ensureUTF8String(msg.UID), 255)
	folder := truncateUTF8Runes(ensureUTF8String(msg.Folder), 255)
	subject := truncateUTF8Runes(ensureUTF8String(msg.Subject), 1000)
	fromAddr := truncateUTF8Runes(ensureUTF8String(msg.FromAddress), 500)
	toAddr := ensureUTF8String(msg.ToAddress)

	var plainText *string
	if msg.BodyText != "" {
		t := ensureUTF8String(msg.BodyText)
		plainText = &t
	}

	var rawMessage *string
	if msg.BodyHTML != "" {
		r := ensureUTF8String(msg.BodyHTML)
		rawMessage = &r
	} else if msg.BodyText != "" {
		r := ensureUTF8String(msg.BodyText)
		rawMessage = &r
	}

	var snippet *string
	if msg.Snippet != "" {
		s := ensureUTF8String(truncateUTF8Runes(msg.Snippet, 200))
		snippet = &s
	}

	hasAttach := false
	for _, att := range msg.Attachments {
		if len(att.Data) > 0 {
			hasAttach = true
			break
		}
	}

	tx, err := h.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	var emailID int64
	err = tx.QueryRow(ctx, `
		INSERT INTO emails (uid, folder, subject, from_address, to_addresses,
		                    date, raw_message, plain_text, snippet, has_attachments,
		                    user_deleted, is_personal, is_business, is_social, is_promotional,
		                    is_spam, is_important, use_by_ai)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,FALSE,FALSE,FALSE,FALSE,FALSE,FALSE,FALSE,TRUE)
		ON CONFLICT (uid, folder) DO UPDATE SET
			subject=$3, from_address=$4, to_addresses=$5,
			date=$6, raw_message=$7, plain_text=$8, snippet=$9,
			has_attachments=$10,
			updated_at=NOW()
		RETURNING id`,
		uid, folder, subject, fromAddr, toAddr,
		date, rawMessage, plainText, snippet, hasAttach,
	).Scan(&emailID)
	if err != nil {
		return err
	}

	ref := fmt.Sprintf("%d", emailID)
	// Remove prior Gmail rows and any legacy rows wrongly tagged email_attachment for this email (same emails.id).
	if _, err = tx.Exec(ctx, `DELETE FROM media_items WHERE source_reference = $1 AND source IN ($2, $3)`, ref, gmailAttachmentSource, "email_attachment"); err != nil {
		return err
	}

	for _, att := range msg.Attachments {
		if len(att.Data) == 0 {
			continue
		}
		title := ensureUTF8String(att.Filename)
		if title == "" {
			title = "attachment"
		}
		title = truncateUTF8Runes(title, 1000)
		mt := att.MediaType
		if len(mt) > 255 {
			mt = mt[:255]
		}
		var blobID int64
		if err = tx.QueryRow(ctx, `INSERT INTO media_blobs (image_data, thumbnail_data) VALUES ($1, NULL) RETURNING id`, att.Data).Scan(&blobID); err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, `
			INSERT INTO media_items (
				media_blob_id, title, media_type, source, source_reference,
				processed, available_for_task, rating, has_gps, is_referenced,
				is_personal, is_business, is_social, is_promotional, is_spam, is_important
			) VALUES ($1, $2, $3, $4, $5,
				FALSE, FALSE, 5, FALSE, FALSE,
				FALSE, FALSE, FALSE, FALSE, FALSE, FALSE)`,
			blobID, title, mt, gmailAttachmentSource, ref); err != nil {
			return err
		}
	}

	return tx.Commit(ctx)
}
