-- Email queries
-- Used by sqlc to generate internal/db/emails.sql.go
-- Run: cd sqlc && sqlc generate

-- name: GetEmailByID :one
SELECT id, uid, folder, subject, from_address, to_addresses, cc_addresses, bcc_addresses,
       date, raw_message, plain_text, snippet, embedding,
       has_attachments, user_deleted, is_personal, is_business, is_social, is_promotional,
       is_spam, is_important, use_by_ai, created_at, updated_at
FROM emails
WHERE id = $1
  AND user_deleted = FALSE;

-- name: GetEmailAttachmentIDs :many
-- Returns media_item IDs where source is IMAP or Gmail email attachment and source_reference matches
-- the given set of email IDs (passed as text array for VARCHAR source_reference).
SELECT source_reference, id AS media_item_id
FROM media_items
WHERE source IN ('email_attachment', 'gmail_attachment')
  AND source_reference = ANY($1::text[])
ORDER BY id;

-- Dynamic search is handled with a hand-written query builder in email_repo.go
-- because the number of optional filters makes parameterised sqlc queries impractical.
