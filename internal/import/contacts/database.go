package contacts

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/daveontour/aimuseum/internal/appctx"
	"github.com/daveontour/aimuseum/internal/sqlutil"
)

// activeEmailsWhere returns a WHERE clause that excludes soft-deleted emails and scopes by user_id when set in ctx.
func activeEmailsWhere(ctx context.Context) (string, []any) {
	where := "user_deleted = FALSE"
	var args []any
	if uid := appctx.UserIDFromCtx(ctx); uid > 0 {
		where += " AND user_id = ?"
		args = append(args, uid)
	}
	return where, args
}

// activeEmailsUnionQuery returns a UNION query over from/to addresses for non-deleted emails.
func activeEmailsUnionQuery(ctx context.Context) (string, []any) {
	w, args := activeEmailsWhere(ctx)
	q := fmt.Sprintf(
		`SELECT from_address FROM emails WHERE %s UNION SELECT to_addresses FROM emails WHERE %s`,
		w, w,
	)
	if len(args) == 0 {
		return q, nil
	}
	return q, append(append([]any{}, args...), args...)
}

// socialMediaCountsQuery counts messages per chat_session/service, scoped to the current user when ctx carries user_id.
func socialMediaCountsQuery(ctx context.Context) (string, []any) {
	q := `
SELECT
    chat_session,
    is_group_chat,
    COUNT(CASE WHEN service = 'WhatsApp' THEN 1 END) AS number_of_whatsapp,
    COUNT(CASE WHEN service = 'iMessage' THEN 1 END) AS number_of_imessage,
    COUNT(CASE WHEN service = 'Facebook Messenger' THEN 1 END) AS number_of_facebook,
    COUNT(CASE WHEN service = 'SMS' THEN 1 END) AS number_of_sms,
    COUNT(CASE WHEN service = 'Instagram' THEN 1 END) AS number_of_insta,
    COUNT(CASE WHEN service LIKE '%' THEN 1 END) AS total
FROM
    messages`
	var args []any
	if uid := appctx.UserIDFromCtx(ctx); uid > 0 {
		q += `
WHERE
    user_id = ?`
		args = append(args, uid)
	}
	q += `
GROUP BY
    chat_session, is_group_chat
ORDER BY
    is_group_chat, total DESC`
	return q, args
}

// ReadFromDatabase reads contact records from the database using the given query.
// The query must return a single column with comma-separated email entries.
func ReadFromDatabase(ctx context.Context, db *sql.DB, query string, args ...any) ([]InputRecord, error) {
	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to execute query: %w", err)
	}
	defer rows.Close()

	var records []InputRecord
	emailMap := make(map[string][]string)

	for rows.Next() {
		var field *string
		if err := rows.Scan(&field); err != nil {
			return nil, fmt.Errorf("failed to scan row: %w", err)
		}
		if field == nil || *field == "" {
			continue
		}
		entries := strings.Split(*field, ",")
		for _, entry := range entries {
			email, name := ParseEmailEntry(entry)
			if email == "" {
				continue
			}
			if name == "" {
				name = email
			}
			if isExcluded(name, email) {
				continue
			}
			emailMap[email] = append(emailMap[email], name)
		}
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating rows: %w", err)
	}

	for email, names := range emailMap {
		records = append(records, InputRecord{Email: email, Names: names})
	}
	return records, nil
}

// ReadRelationshipsFromDatabase reads relationship records (from, to) from the database
func ReadRelationshipsFromDatabase(ctx context.Context, db *sql.DB, query string) ([]RelationshipRecord, error) {
	rows, err := db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to execute query: %w", err)
	}
	defer rows.Close()

	var relationships []RelationshipRecord
	for rows.Next() {
		var from, to *string
		if err := rows.Scan(&from, &to); err != nil {
			return nil, fmt.Errorf("failed to scan row: %w", err)
		}
		if from == nil || *from == "" || to == nil || *to == "" {
			continue
		}
		fromEmail, _ := ParseEmailEntry(*from)
		if fromEmail == "" {
			fromEmail = strings.ToLower(strings.TrimSpace(*from))
		}
		toAddresses := strings.Split(*to, ",")
		for _, toAddr := range toAddresses {
			toEmail, _ := ParseEmailEntry(toAddr)
			if toEmail == "" {
				toEmail = strings.ToLower(strings.TrimSpace(toAddr))
			}
			if toEmail != "" {
				relationships = append(relationships, RelationshipRecord{From: fromEmail, To: toEmail})
			}
		}
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating rows: %w", err)
	}
	return relationships, nil
}

// SubjectIdentifiers holds the subject's (id=0) identifiers for directional message queries.
type SubjectIdentifiers struct {
	WhatsAppID  *string
	IMessageID  *string
	SMSID       *string
	FacebookID  *string
	InstagramID *string
}

// WriteContactsToDatabase writes formatted contact records to the contacts table.
// Maps: id->id, primary_name->name, alternative_names->alternative_names, emails->email.
// Truncates the contacts table (and dependent relationships) before inserting.
// Preserves and restores subject (id=0) identifiers (whatsappid, imessageid, smsid, facebookid, instagramid).
func WriteContactsToDatabase(ctx context.Context, db *sql.DB, records []FormattedOutputRecord, ownerUserID int64) error {
	totalRecords := len(records)
	fmt.Fprintf(os.Stderr, "Starting contacts transaction (%d records)\n", totalRecords)
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback()
	fmt.Fprintf(os.Stderr, "Contacts transaction begun\n")

	var subjectIds SubjectIdentifiers
	err = tx.QueryRowContext(ctx, "SELECT whatsappid, imessageid, smsid, facebookid, instagramid FROM contacts WHERE id = 0").Scan(
		&subjectIds.WhatsAppID, &subjectIds.IMessageID, &subjectIds.SMSID, &subjectIds.FacebookID, &subjectIds.InstagramID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("read subject identifiers: %w", err)
	}
	fmt.Fprintf(os.Stderr, "Subject identifiers loaded\n")

	if sqlutil.IsSQLite(ctx, db) {
		// SQLite has no TRUNCATE; FKs from relationships ON DELETE CASCADE clear dependent rows.
		_, err = tx.ExecContext(ctx, "DELETE FROM contacts")
		fmt.Fprintf(os.Stderr, "SQLite contacts table cleared\n")
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error clearing SQLite contacts table: %v\n", err)
		}
	} else {
		_, err = tx.ExecContext(ctx, "TRUNCATE contacts CASCADE")
		fmt.Fprintf(os.Stderr, "PostgreSQL contacts table cleared\n")
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error clearing PostgreSQL contacts table: %v\n", err)
		}
	}
	if err != nil {
		return fmt.Errorf("truncate contacts: %w", err)
	}
	fmt.Fprintf(os.Stderr, "Contacts table cleared, inserting records\n")

	const progressInterval = 1000
	for i, r := range records {
		select {
		case <-ctx.Done():
			return fmt.Errorf("write contacts cancelled: %w", ctx.Err())
		default:
		}
		nemails := r.NumEmails
		nw, ni, nf, ns, ninst := r.NumWhatsApp, r.NumIMessage, r.NumFacebook, r.NumSMS, r.NumInstagram
		if nemails < 0 {
			nemails = 0
		}
		if nw < 0 {
			nw = 0
		}
		if ni < 0 {
			ni = 0
		}
		if nf < 0 {
			nf = 0
		}
		if ns < 0 {
			ns = 0
		}
		if ninst < 0 {
			ninst = 0
		}
		total := nemails + nw + ni + nf + ns + ninst
		var userIDArg any
		if ownerUserID > 0 {
			userIDArg = ownerUserID
		}
		_, err = tx.ExecContext(ctx,
			`INSERT INTO contacts (id, name, alternative_names, email, numemails, numwhatsapp, numimessages, numfacebook, numsms, numinstagram, is_group, total, user_id) VALUES (?1, ?2, ?3, ?4, ?5, ?6, ?7, ?8, ?9, ?10, ?11, ?12, ?13)`,
			r.ID, r.PrimaryName, r.AlternativeNames, r.Emails,
			nemails, nw, ni, nf, ns, ninst, r.IsGroupChat, total, userIDArg)
		if err != nil {
			return fmt.Errorf("insert contact id=%d: %w", r.ID, err)
		}
		if (i+1)%progressInterval == 0 || i+1 == totalRecords {
			fmt.Fprintf(os.Stderr, "Inserted %d/%d contacts records\n", i+1, totalRecords)
		}
	}

	// Restore subject (id=0) identifiers if we had them before truncate
	if subjectIds.WhatsAppID != nil || subjectIds.IMessageID != nil || subjectIds.SMSID != nil ||
		subjectIds.FacebookID != nil || subjectIds.InstagramID != nil {
		_, err = tx.ExecContext(ctx,
			`UPDATE contacts SET whatsappid = ?1, imessageid = ?2, smsid = ?3, facebookid = ?4, instagramid = ?5 WHERE id = 0`,
			subjectIds.WhatsAppID, subjectIds.IMessageID, subjectIds.SMSID, subjectIds.FacebookID, subjectIds.InstagramID)
		if err != nil {
			return fmt.Errorf("restore subject identifiers: %w", err)
		}
	}

	// Reset sequence so future auto-inserts get correct next id
	if sqlutil.IsSQLite(ctx, db) {
		err = resetSQLiteContactsSequence(ctx, tx)
	} else {
		_, err = tx.ExecContext(ctx, "SELECT setval(pg_get_serial_sequence('contacts', 'id'), COALESCE((SELECT MAX(id) FROM contacts), 1))")
	}
	if err != nil {
		return fmt.Errorf("reset contacts sequence: %w", err)
	}

	fmt.Fprintf(os.Stderr, "Committing contacts transaction\n")
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit contacts transaction: %w", err)
	}
	fmt.Fprintf(os.Stderr, "Contacts transaction committed\n")
	return nil
}

// ownerContactLink captures subject_configuration.subject_contact_id before contacts are rebuilt.
// Deleting contacts triggers ON DELETE SET NULL on subject_contact_id; restore after rewrite.
type ownerContactLink struct {
	active      bool
	configRowID int64
	contactID   int64
	name        string
	email       string
}

func loadOwnerContactLink(ctx context.Context, db *sql.DB) (ownerContactLink, error) {
	if db == nil {
		return ownerContactLink{}, nil
	}
	uid := appctx.UserIDFromCtx(ctx)
	q := `
		SELECT sc.id, sc.subject_contact_id, COALESCE(c.name, ''), COALESCE(c.email, '')
		FROM subject_configuration sc
		LEFT JOIN contacts c ON c.id = sc.subject_contact_id
		WHERE sc.subject_contact_id IS NOT NULL`
	args := []any{}
	if uid > 0 {
		q += ` AND (sc.user_id = ? OR sc.user_id IS NULL)`
		args = append(args, uid)
	}
	q += ` ORDER BY CASE WHEN sc.user_id IS NULL THEN 1 ELSE 0 END, sc.id ASC LIMIT 1`

	var link ownerContactLink
	var contactID sql.NullInt64
	err := db.QueryRowContext(ctx, q, args...).Scan(&link.configRowID, &contactID, &link.name, &link.email)
	if errors.Is(err, sql.ErrNoRows) {
		return ownerContactLink{}, nil
	}
	if err != nil {
		return ownerContactLink{}, fmt.Errorf("load owner contact link: %w", err)
	}
	if !contactID.Valid {
		return ownerContactLink{}, nil
	}
	link.active = true
	link.contactID = contactID.Int64
	return link, nil
}

func resolveContactIDAfterRebuild(ctx context.Context, db *sql.DB, link ownerContactLink) (int64, bool) {
	if !link.active {
		return 0, false
	}
	if link.contactID == 0 {
		var one int
		if err := db.QueryRowContext(ctx, `SELECT 1 FROM contacts WHERE id = 0 LIMIT 1`).Scan(&one); err == nil {
			return 0, true
		}
	} else {
		var one int
		if err := db.QueryRowContext(ctx, `SELECT 1 FROM contacts WHERE id = ? LIMIT 1`, link.contactID).Scan(&one); err == nil {
			return link.contactID, true
		}
	}
	name := strings.TrimSpace(link.name)
	if name != "" {
		var id int64
		if err := db.QueryRowContext(ctx,
			`SELECT id FROM contacts WHERE LOWER(TRIM(name)) = LOWER(TRIM(?)) ORDER BY id LIMIT 1`,
			name).Scan(&id); err == nil {
			return id, true
		}
	}
	for _, part := range strings.Split(link.email, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		email, _ := ParseEmailEntry(part)
		if email == "" {
			email = strings.ToLower(part)
		}
		norm := NormalizeEmailForMatching(email)
		if norm == "" {
			continue
		}
		var id int64
		pat := "%" + norm + "%"
		if err := db.QueryRowContext(ctx,
			`SELECT id FROM contacts WHERE LOWER(COALESCE(email, '')) LIKE ? ORDER BY id LIMIT 1`,
			pat).Scan(&id); err == nil {
			return id, true
		}
	}
	return 0, false
}

func restoreOwnerContactLink(ctx context.Context, db *sql.DB, link ownerContactLink) error {
	if db == nil || !link.active {
		return nil
	}
	newID, ok := resolveContactIDAfterRebuild(ctx, db, link)
	if !ok {
		fmt.Fprintf(os.Stderr, "warning: could not remap archive owner contact %q after contacts rebuild\n", link.name)
		return nil
	}
	_, err := db.ExecContext(ctx,
		`UPDATE subject_configuration SET subject_contact_id = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`,
		newID, link.configRowID)
	if err != nil {
		return fmt.Errorf("restore owner contact link: %w", err)
	}
	fmt.Fprintf(os.Stderr, "Restored archive owner contact link (contact id=%d)\n", newID)
	return nil
}

func resetSQLiteContactsSequence(ctx context.Context, tx *sql.Tx) error {
	var maxID sql.NullInt64
	if err := tx.QueryRowContext(ctx, "SELECT MAX(id) FROM contacts").Scan(&maxID); err != nil {
		return err
	}
	n := int64(1)
	if maxID.Valid && maxID.Int64 > 0 {
		n = maxID.Int64
	}
	if _, err := tx.ExecContext(ctx, "DELETE FROM sqlite_sequence WHERE name = 'contacts'"); err != nil {
		return err
	}
	_, err := tx.ExecContext(ctx, "INSERT INTO sqlite_sequence (name, seq) VALUES ('contacts', ?)", n)
	return err
}

// LoadSubjectIdentifiers loads the subject's (id=0) identifiers from the contacts table.
func LoadSubjectIdentifiers(ctx context.Context, db *sql.DB) (*SubjectIdentifiers, error) {
	var ids SubjectIdentifiers
	err := db.QueryRowContext(ctx, "SELECT whatsappid, imessageid, smsid, facebookid, instagramid FROM contacts WHERE id = 0").Scan(
		&ids.WhatsAppID, &ids.IMessageID, &ids.SMSID, &ids.FacebookID, &ids.InstagramID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("load subject identifiers: %w", err)
	}
	return &ids, nil
}

// normalizeForMatch normalizes an identifier for matching (strip spaces, +, leading zeros).
func normalizeForMatch(s string) string {
	s = strings.TrimSpace(s)
	s = strings.ReplaceAll(s, " ", "")
	s = strings.ReplaceAll(s, "+", "")
	s = strings.ReplaceAll(s, "-", "")
	for len(s) > 1 && s[0] == '0' {
		s = s[1:]
	}
	return strings.ToLower(s)
}

// DirectionalCounts holds message counts by direction for a (chat_session, service) pair.
type DirectionalCounts struct {
	FromSubject int64
	FromContact int64
}

// GetDirectionalMessageCounts returns message counts by direction for the given chat_session and service.
// subjectIdentifiers: comma-separated values for the subject's identifier(s) for this service.
func GetDirectionalMessageCounts(ctx context.Context, db *sql.DB, chatSession, service string, subjectID *string) (DirectionalCounts, error) {
	serviceVal := service
	switch service {
	case "whatsapp":
		serviceVal = "WhatsApp"
	case "imessage":
		serviceVal = "iMessage"
	case "facebook":
		serviceVal = "Facebook Messenger"
	case "sms":
		serviceVal = "SMS"
	case "instagram":
		serviceVal = "Instagram"
	default:
		return DirectionalCounts{}, fmt.Errorf("unknown service: %s", service)
	}

	rows, err := db.QueryContext(ctx,
		`SELECT sender_id FROM messages WHERE chat_session = ?1 AND service = ?2`,
		chatSession, serviceVal)
	if err != nil {
		return DirectionalCounts{}, fmt.Errorf("query messages: %w", err)
	}
	defer rows.Close()

	var subjectNormSet map[string]struct{}
	if subjectID != nil && *subjectID != "" {
		for _, part := range strings.Split(*subjectID, ",") {
			norm := normalizeForMatch(strings.TrimSpace(part))
			if norm != "" {
				if subjectNormSet == nil {
					subjectNormSet = make(map[string]struct{})
				}
				subjectNormSet[norm] = struct{}{}
			}
		}
	}

	var fromSubject, fromContact int64
	for rows.Next() {
		var senderID *string
		if err := rows.Scan(&senderID); err != nil {
			return DirectionalCounts{}, fmt.Errorf("scan sender_id: %w", err)
		}
		if senderID == nil || *senderID == "" {
			continue
		}
		senderNorm := normalizeForMatch(*senderID)
		if subjectNormSet != nil {
			if _, ok := subjectNormSet[senderNorm]; ok {
				fromSubject++
				continue
			}
		}
		fromContact++
	}
	if err := rows.Err(); err != nil {
		return DirectionalCounts{}, fmt.Errorf("iterate rows: %w", err)
	}
	// When no subject identifiers, split total evenly as fallback
	if len(subjectNormSet) == 0 {
		total := fromSubject + fromContact
		fromSubject = total / 2
		fromContact = total - fromSubject
	}
	return DirectionalCounts{FromSubject: fromSubject, FromContact: fromContact}, nil
}
