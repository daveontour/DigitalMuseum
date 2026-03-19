package contacts

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ReadFromDatabase reads contact records from the database using the given query.
// The query must return a single column with comma-separated email entries.
func ReadFromDatabase(ctx context.Context, db *pgxpool.Pool, query string) ([]InputRecord, error) {
	rows, err := db.Query(ctx, query)
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
func ReadRelationshipsFromDatabase(ctx context.Context, db *pgxpool.Pool, query string) ([]RelationshipRecord, error) {
	rows, err := db.Query(ctx, query)
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
func WriteContactsToDatabase(ctx context.Context, db *pgxpool.Pool, records []FormattedOutputRecord) error {
	tx, err := db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	var subjectIds SubjectIdentifiers
	err = tx.QueryRow(ctx, "SELECT whatsappid, imessageid, smsid, facebookid, instagramid FROM contacts WHERE id = 0").Scan(
		&subjectIds.WhatsAppID, &subjectIds.IMessageID, &subjectIds.SMSID, &subjectIds.FacebookID, &subjectIds.InstagramID)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("read subject identifiers: %w", err)
	}

	_, err = tx.Exec(ctx, "TRUNCATE contacts CASCADE")
	if err != nil {
		return fmt.Errorf("truncate contacts: %w", err)
	}

	for _, r := range records {
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
		_, err = tx.Exec(ctx,
			`INSERT INTO contacts (id, name, alternative_names, email, numemails, numwhatsapp, numimessages, numfacebook, numsms, numinstagram, is_group, total) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)`,
			r.ID, r.PrimaryName, r.AlternativeNames, r.Emails,
			nemails, nw, ni, nf, ns, ninst, r.IsGroupChat, total)
		if err != nil {
			return fmt.Errorf("insert contact id=%d: %w", r.ID, err)
		}
	}

	// Restore subject (id=0) identifiers if we had them before truncate
	if subjectIds.WhatsAppID != nil || subjectIds.IMessageID != nil || subjectIds.SMSID != nil ||
		subjectIds.FacebookID != nil || subjectIds.InstagramID != nil {
		_, err = tx.Exec(ctx,
			`UPDATE contacts SET whatsappid = $1, imessageid = $2, smsid = $3, facebookid = $4, instagramid = $5 WHERE id = 0`,
			subjectIds.WhatsAppID, subjectIds.IMessageID, subjectIds.SMSID, subjectIds.FacebookID, subjectIds.InstagramID)
		if err != nil {
			return fmt.Errorf("restore subject identifiers: %w", err)
		}
	}

	// Reset sequence so future auto-inserts get correct next id
	_, err = tx.Exec(ctx, "SELECT setval(pg_get_serial_sequence('contacts', 'id'), COALESCE((SELECT MAX(id) FROM contacts), 1))")
	if err != nil {
		return fmt.Errorf("reset contacts sequence: %w", err)
	}

	return tx.Commit(ctx)
}

// LoadSubjectIdentifiers loads the subject's (id=0) identifiers from the contacts table.
func LoadSubjectIdentifiers(ctx context.Context, db *pgxpool.Pool) (*SubjectIdentifiers, error) {
	var ids SubjectIdentifiers
	err := db.QueryRow(ctx, "SELECT whatsappid, imessageid, smsid, facebookid, instagramid FROM contacts WHERE id = 0").Scan(
		&ids.WhatsAppID, &ids.IMessageID, &ids.SMSID, &ids.FacebookID, &ids.InstagramID)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
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
func GetDirectionalMessageCounts(ctx context.Context, db *pgxpool.Pool, chatSession, service string, subjectID *string) (DirectionalCounts, error) {
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

	rows, err := db.Query(ctx,
		`SELECT sender_id FROM messages WHERE chat_session = $1 AND service = $2`,
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
