package handler

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	appimporter "github.com/daveontour/digitalmuseum/internal/importer"
	"github.com/emersion/go-imap"
	"github.com/emersion/go-imap/client"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// IMAPHandler handles all /imap/* routes.
type IMAPHandler struct {
	pool *pgxpool.Pool
	job  *appimporter.ImportJob
}

// NewIMAPHandler creates an IMAPHandler.
func NewIMAPHandler(pool *pgxpool.Pool) *IMAPHandler {
	return &IMAPHandler{
		pool: pool,
		job: appimporter.NewImportJob("IMAP import", map[string]any{
			"status":               "idle",
			"status_line":          nil,
			"error_message":        nil,
			"current_folder":       nil,
			"current_folder_index": 0,
			"total_folders":        0,
			"emails_processed":     0,
			"folders":              []string{},
		}),
	}
}

// RegisterRoutes mounts all IMAP routes.
func (h *IMAPHandler) RegisterRoutes(r chi.Router) {
	r.Post("/imap/process", h.StartProcess)
	r.Get("/imap/process/stream", h.StreamProgress)
	r.Post("/imap/process/cancel", h.CancelProcess)
	r.Get("/imap/process/status", h.GetStatus)
	r.Post("/imap/folders", h.GetFolders)
}

// ── Request types ─────────────────────────────────────────────────────────────

type imapConnParams struct {
	Host     string `json:"host"`
	Port     int    `json:"port"`
	Username string `json:"username"`
	Password string `json:"password"`
	UseSSL   bool   `json:"use_ssl"`
}

type imapProcessRequest struct {
	imapConnParams
	Folders    []string `json:"folders"`
	AllFolders bool     `json:"all_folders"`
	NewOnly    bool     `json:"new_only"`
}

type imapFoldersRequest struct {
	imapConnParams
}

// ── Handlers ──────────────────────────────────────────────────────────────────

// StartProcess handles POST /imap/process
func (h *IMAPHandler) StartProcess(w http.ResponseWriter, r *http.Request) {
	if err := h.job.AssertNotRunning(); err != nil {
		writeError(w, http.StatusConflict, err.Error())
		return
	}

	var req imapProcessRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if req.Host == "" || req.Username == "" || req.Password == "" {
		writeError(w, http.StatusBadRequest, "host, username, and password are required")
		return
	}

	// Resolve folder list with a brief connection
	folders, err := imapListFolders(req.imapConnParams)
	if err != nil {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("IMAP connection failed: %s", err))
		return
	}
	if req.AllFolders {
		// keep all
	} else if len(req.Folders) > 0 {
		folders = req.Folders
	} else {
		folders = []string{"INBOX"}
	}
	if len(folders) == 0 {
		writeError(w, http.StatusBadRequest, "no folders found or specified")
		return
	}

	go h.runIMAPImport(req, folders)

	writeJSON(w, map[string]any{
		"message": fmt.Sprintf("IMAP processing started for %d folder(s)", len(folders)),
		"folders": folders,
	})
}

// StreamProgress handles GET /imap/process/stream
func (h *IMAPHandler) StreamProgress(w http.ResponseWriter, r *http.Request) {
	h.job.ServeSSE(w, r)
}

// CancelProcess handles POST /imap/process/cancel
func (h *IMAPHandler) CancelProcess(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, h.job.Cancel())
}

// GetStatus handles GET /imap/process/status
func (h *IMAPHandler) GetStatus(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, h.job.Status())
}

// GetFolders handles POST /imap/folders
func (h *IMAPHandler) GetFolders(w http.ResponseWriter, r *http.Request) {
	var req imapFoldersRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if req.Host == "" || req.Username == "" || req.Password == "" {
		writeError(w, http.StatusBadRequest, "host, username, and password are required")
		return
	}

	folders, err := imapListFolders(req.imapConnParams)
	if err != nil {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("Failed to connect: %s", err))
		return
	}
	writeJSON(w, map[string]any{"folders": folders})
}

// ── IMAP helpers ──────────────────────────────────────────────────────────────

// imapConnect opens an authenticated IMAP connection.
func imapConnect(p imapConnParams) (*client.Client, error) {
	addr := fmt.Sprintf("%s:%d", p.Host, p.Port)
	var (
		c   *client.Client
		err error
	)
	if p.UseSSL {
		c, err = client.DialTLS(addr, &tls.Config{ServerName: p.Host})
	} else {
		c, err = client.Dial(addr)
	}
	if err != nil {
		return nil, fmt.Errorf("dial: %w", err)
	}
	if err := c.Login(p.Username, p.Password); err != nil {
		_ = c.Logout()
		return nil, fmt.Errorf("login: %w", err)
	}
	return c, nil
}

// imapListFolders connects, lists all mailboxes, and disconnects.
func imapListFolders(p imapConnParams) ([]string, error) {
	c, err := imapConnect(p)
	if err != nil {
		return nil, err
	}
	defer func() { _ = c.Logout() }()

	mailboxes := make(chan *imap.MailboxInfo, 64)
	done := make(chan error, 1)
	go func() {
		done <- c.List("", "*", mailboxes)
	}()

	var folders []string
	for m := range mailboxes {
		folders = append(folders, m.Name)
	}
	if err := <-done; err != nil {
		return nil, err
	}
	return folders, nil
}

// ── Background import job ─────────────────────────────────────────────────────

func (h *IMAPHandler) runIMAPImport(req imapProcessRequest, folders []string) {
	h.job.Start()
	h.job.UpdateState(map[string]any{
		"status":               "in_progress",
		"status_line":          "Connecting to IMAP server...",
		"current_folder":       nil,
		"current_folder_index": 0,
		"total_folders":        len(folders),
		"emails_processed":     0,
		"folders":              folders,
	})
	h.job.Broadcast("progress", h.job.GetState())

	defer h.job.Finish()

	c, err := imapConnect(req.imapConnParams)
	if err != nil {
		h.job.UpdateState(map[string]any{
			"status":        "error",
			"error_message": err.Error(),
			"status_line":   err.Error(),
		})
		h.job.Broadcast("error", h.job.GetState())
		return
	}
	defer func() { _ = c.Logout() }()

	totalProcessed := 0

	for idx, folder := range folders {
		if h.job.IsCancelled() {
			h.job.UpdateState(map[string]any{
				"status":      "cancelled",
				"status_line": "Cancelled by user",
			})
			h.job.Broadcast("cancelled", h.job.GetState())
			return
		}

		h.job.UpdateState(map[string]any{
			"current_folder":       folder,
			"current_folder_index": idx + 1,
			"status_line":          fmt.Sprintf("Processing folder: %s (%d/%d)", folder, idx+1, len(folders)),
		})
		h.job.Broadcast("progress", h.job.GetState())

		count, err := h.importFolder(c, folder, req.NewOnly, totalProcessed)
		if err != nil {
			msg := fmt.Sprintf("Error processing folder %s: %s", folder, err)
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
			"status_line":      fmt.Sprintf("Folder %s: %d emails. Total: %d", folder, count, totalProcessed),
		})
		h.job.Broadcast("progress", h.job.GetState())
	}

	h.job.UpdateState(map[string]any{
		"status":      "completed",
		"status_line": fmt.Sprintf("Import completed. %d emails processed.", totalProcessed),
	})
	h.job.Broadcast("completed", h.job.GetState())
}

func (h *IMAPHandler) importFolder(c *client.Client, folder string, newOnly bool, baseCount int) (int, error) {
	mbox, err := c.Select(folder, true /* readonly */)
	if err != nil {
		return 0, fmt.Errorf("select: %w", err)
	}
	if mbox.Messages == 0 {
		return 0, nil
	}

	// Build sequence set
	seqset := new(imap.SeqSet)
	seqset.AddRange(1, mbox.Messages)

	// Fetch envelope + full message (RFC822 includes all headers, needed to parse MIME Content-Type/boundary)
	items := []imap.FetchItem{
		imap.FetchEnvelope,
		imap.FetchFlags,
		imap.FetchRFC822,
	}

	messages := make(chan *imap.Message, 64)
	fetchDone := make(chan error, 1)
	go func() {
		fetchDone <- c.Fetch(seqset, items, messages)
	}()

	count := 0
	for msg := range messages {
		if h.job.IsCancelled() {
			break
		}
		if err := h.storeEmail(context.Background(), folder, msg, newOnly); err != nil {
			// log and continue
			fmt.Printf("[IMAP] warning storing email %d: %s\n", msg.SeqNum, err)
			continue
		}
		count++
		if count%10 == 0 {
			h.job.UpdateState(map[string]any{"emails_processed": baseCount + count})
			h.job.Broadcast("progress", h.job.GetState())
		}
	}
	if err := <-fetchDone; err != nil {
		return count, fmt.Errorf("fetch: %w", err)
	}
	return count, nil
}

func (h *IMAPHandler) storeEmail(ctx context.Context, folder string, msg *imap.Message, newOnly bool) error {
	if msg.Envelope == nil {
		return nil
	}
	env := msg.Envelope
	uid := fmt.Sprintf("%d", msg.SeqNum)

	// Check if already stored (by uid+folder)
	if newOnly {
		var exists bool
		_ = h.pool.QueryRow(ctx,
			`SELECT EXISTS(SELECT 1 FROM emails WHERE uid=$1 AND folder=$2)`,
			uid, folder,
		).Scan(&exists)
		if exists {
			return nil
		}
	}

	from := addrList(env.From)
	to := addrList(env.To)
	cc := addrList(env.Cc)
	bcc := addrList(env.Bcc)

	var rawMsg *string
	var plainText *string
	var snippet *string
	var attachments []imapAttachmentPart

	// Match RFC822 fetch so GetBody finds the stored body (map keys are pointer identity).
	section, secErr := imap.ParseBodySectionName(imap.FetchRFC822)
	if secErr == nil {
		if body := msg.GetBody(section); body != nil {
			raw, rerr := io.ReadAll(body)
			if rerr == nil && len(raw) > 0 {
				parsed := parseIMAPMIMEBody(raw)
				rawMsg, plainText, snippet = imapEmailStoredFields(parsed)
				attachments = parsed.Attachments
			}
		}
	}

	date := env.Date
	if date.IsZero() {
		date = time.Now()
	}

	hasAttach := false
	for _, att := range attachments {
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
		INSERT INTO emails (uid, folder, subject, from_address, to_addresses, cc_addresses, bcc_addresses,
		                    date, raw_message, plain_text, snippet, has_attachments,
		                    user_deleted, is_personal, is_business, is_social, is_promotional,
		                    is_spam, is_important, use_by_ai)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,FALSE,FALSE,FALSE,FALSE,FALSE,FALSE,FALSE,FALSE)
		ON CONFLICT (uid, folder) DO UPDATE
		SET subject=$3, from_address=$4, to_addresses=$5, cc_addresses=$6, bcc_addresses=$7,
		    date=$8, raw_message=$9, plain_text=$10, snippet=$11, has_attachments=$12,
		    updated_at=NOW()
		RETURNING id`,
		uid, folder, env.Subject, from, to, cc, bcc,
		date, rawMsg, plainText, snippet, hasAttach,
	).Scan(&emailID)
	if err != nil {
		return err
	}

	ref := fmt.Sprintf("%d", emailID)
	if _, err = tx.Exec(ctx, `DELETE FROM media_items WHERE source = 'email_attachment' AND source_reference = $1`, ref); err != nil {
		return err
	}

	for _, att := range attachments {
		if len(att.Data) == 0 {
			continue
		}
		title := att.Filename
		if title == "" {
			title = "attachment"
		}
		if len(title) > 1000 {
			title = title[:1000]
		}
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
			) VALUES ($1, $2, $3, 'email_attachment', $4,
				FALSE, FALSE, 5, FALSE, FALSE,
				FALSE, FALSE, FALSE, FALSE, FALSE, FALSE)`,
			blobID, title, mt, ref); err != nil {
			return err
		}
	}

	return tx.Commit(ctx)
}

// addrList formats a slice of IMAP addresses as a comma-separated string.
func addrList(addrs []*imap.Address) string {
	parts := make([]string, 0, len(addrs))
	for _, a := range addrs {
		if a == nil {
			continue
		}
		if a.PersonalName != "" {
			parts = append(parts, fmt.Sprintf("%s <%s@%s>", a.PersonalName, a.MailboxName, a.HostName))
		} else {
			parts = append(parts, fmt.Sprintf("%s@%s", a.MailboxName, a.HostName))
		}
	}
	return strings.Join(parts, ", ")
}
