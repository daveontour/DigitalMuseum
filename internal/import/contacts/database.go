package contacts

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/daveontour/aimuseum/internal/appctx"
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
	defer func() { _ = rows.Close() }()

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
	defer func() { _ = rows.Close() }()

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

// existingContactRow is one row loaded from contacts before an extraction
// write, used to match freshly computed records against already-present
// contacts instead of blindly overwriting the table.
type existingContactRow struct {
	ID               int64
	Name             string
	AlternativeNames string
	Email            string
	Used             bool
}

// splitTrimmed splits a comma-joined string into trimmed, non-empty parts.
func splitTrimmed(s string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// mergeCommaSets unions two comma-joined lists, case-insensitively deduped,
// preserving each kept entry's original casing and returning them sorted.
func mergeCommaSets(existing, newVal string) string {
	seen := map[string]struct{}{}
	var out []string
	add := func(s string) {
		key := strings.ToLower(s)
		if _, ok := seen[key]; ok {
			return
		}
		seen[key] = struct{}{}
		out = append(out, s)
	}
	for _, s := range splitTrimmed(existing) {
		add(s)
	}
	for _, s := range splitTrimmed(newVal) {
		add(s)
	}
	sort.Strings(out)
	return strings.Join(out, ", ")
}

// matchExistingContact finds the best still-unused existing contact for a
// freshly computed record, in priority order: exact normalized email, exact
// case-insensitive name (either direction between primary/alternative
// names), then fuzzy name similarity (same scorer/threshold runMerge uses
// to cluster raw records in the first place). Returns nil when nothing
// clears the bar, so the caller inserts a new contact instead of guessing.
func matchExistingContact(r FormattedOutputRecord, existing []*existingContactRow) *existingContactRow {
	for _, e := range splitTrimmed(r.Emails) {
		norm := NormalizeEmailForMatching(e)
		if norm == "" {
			continue
		}
		for _, ex := range existing {
			if ex.Used {
				continue
			}
			for _, exEmail := range splitTrimmed(ex.Email) {
				if NormalizeEmailForMatching(exEmail) == norm {
					return ex
				}
			}
		}
	}

	primaryLower := strings.ToLower(strings.TrimSpace(r.PrimaryName))
	altLowerSet := map[string]struct{}{}
	for _, a := range splitTrimmed(r.AlternativeNames) {
		altLowerSet[strings.ToLower(a)] = struct{}{}
	}
	for _, ex := range existing {
		if ex.Used {
			continue
		}
		exNameLower := strings.ToLower(strings.TrimSpace(ex.Name))
		if exNameLower != "" && exNameLower == primaryLower {
			return ex
		}
		if _, ok := altLowerSet[exNameLower]; ok {
			return ex
		}
		for _, exAlt := range splitTrimmed(ex.AlternativeNames) {
			if strings.ToLower(exAlt) == primaryLower {
				return ex
			}
		}
	}

	rNorm := normalizeName(r.PrimaryName)
	if rNorm == "" {
		return nil
	}
	var best *existingContactRow
	bestScore := 0.0
	for _, ex := range existing {
		if ex.Used {
			continue
		}
		if score := fuzzySimilarity(rNorm, normalizeName(ex.Name)); score > bestScore {
			bestScore = score
			best = ex
		}
	}
	if best != nil && bestScore >= FuzzyMergeThreshold {
		return best
	}
	return nil
}

func clampNonNegative(n int64) int64 {
	if n < 0 {
		return 0
	}
	return n
}

// WriteContactsToDatabase upserts formatted contact records into the
// contacts table instead of rebuilding it from scratch: each record is
// matched against already-present contacts (see matchExistingContact) and,
// on a match, only extraction-derived columns (message counts, merged
// alternative_names/email, is_group, total) are updated — name, rel_type,
// description, use_by_ai and the whatsappid/imessageid/smsid/facebookid/
// instagramid columns are left exactly as they were, since they're either
// user-edited (a manual rename, a rel_type/description set from the
// Contacts UI) or set by a different mechanism entirely (rel_type is also
// separately reapplied by ApplyClassificationsToContacts after this
// returns). Unmatched existing contacts — including ones created purely to
// name a face, which have no message history to ever match against — are
// never touched or deleted, so ids stay stable and every FK that points at
// a contact (media_item_faces.contact_id, face_clusters.contact_id,
// relationships, subject_configuration.subject_contact_id) survives a rerun.
func WriteContactsToDatabase(ctx context.Context, db *sql.DB, records []FormattedOutputRecord, ownerUserID int64) error {
	totalRecords := len(records)
	fmt.Fprintf(os.Stderr, "Starting contacts transaction (%d records)\n", totalRecords)
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	rows, err := tx.QueryContext(ctx, `SELECT id, COALESCE(name, ''), COALESCE(alternative_names, ''), COALESCE(email, '') FROM contacts`)
	if err != nil {
		return fmt.Errorf("load existing contacts: %w", err)
	}
	var existing []*existingContactRow
	for rows.Next() {
		ec := &existingContactRow{}
		if err := rows.Scan(&ec.ID, &ec.Name, &ec.AlternativeNames, &ec.Email); err != nil {
			_ = rows.Close()
			return fmt.Errorf("scan existing contact: %w", err)
		}
		existing = append(existing, ec)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return fmt.Errorf("iterate existing contacts: %w", err)
	}
	_ = rows.Close()
	fmt.Fprintf(os.Stderr, "Loaded %d existing contacts for matching\n", len(existing))

	existingByID := make(map[int64]*existingContactRow, len(existing))
	for _, ec := range existing {
		existingByID[ec.ID] = ec
	}

	var userIDArg any
	if ownerUserID > 0 {
		userIDArg = ownerUserID
	}

	const progressInterval = 1000
	matched, inserted := 0, 0
	for i, r := range records {
		select {
		case <-ctx.Done():
			return fmt.Errorf("write contacts cancelled: %w", ctx.Err())
		default:
		}

		nemails := clampNonNegative(r.NumEmails)
		nw := clampNonNegative(r.NumWhatsApp)
		ni := clampNonNegative(r.NumIMessage)
		nf := clampNonNegative(r.NumFacebook)
		ns := clampNonNegative(r.NumSMS)
		ninst := clampNonNegative(r.NumInstagram)
		total := nemails + nw + ni + nf + ns + ninst

		// r.ID == 0 is formatOutput's exclusive marker for the one record
		// matching the archive subject's configured name (see its
		// assignedZero bookkeeping) — check the reserved id=0 row first so
		// the subject's contact is preferred over an email/name/fuzzy
		// match that might otherwise fire for the same person.
		isSubject := r.ID == 0
		var match *existingContactRow
		if isSubject {
			if ec, ok := existingByID[0]; ok && !ec.Used {
				match = ec
			}
		}
		if match == nil {
			match = matchExistingContact(r, existing)
		}

		if match != nil {
			match.Used = true
			mergedAlt := mergeCommaSets(match.AlternativeNames, r.AlternativeNames)
			mergedEmail := mergeCommaSets(match.Email, r.Emails)
			_, err = tx.ExecContext(ctx, `UPDATE contacts SET
				alternative_names = ?1, email = ?2,
				numemails = ?3, numwhatsapp = ?4, numimessages = ?5, numfacebook = ?6, numsms = ?7, numinstagram = ?8,
				is_group = ?9, total = ?10, updated_at = CURRENT_TIMESTAMP
				WHERE id = ?11`,
				mergedAlt, mergedEmail,
				nemails, nw, ni, nf, ns, ninst, r.IsGroupChat, total, match.ID)
			if err != nil {
				return fmt.Errorf("update contact id=%d: %w", match.ID, err)
			}
			matched++
		} else if isSubject {
			_, err = tx.ExecContext(ctx,
				`INSERT INTO contacts (id, name, alternative_names, email, numemails, numwhatsapp, numimessages, numfacebook, numsms, numinstagram, is_group, total, user_id) VALUES (0, ?1, ?2, ?3, ?4, ?5, ?6, ?7, ?8, ?9, ?10, ?11, ?12)`,
				r.PrimaryName, r.AlternativeNames, r.Emails,
				nemails, nw, ni, nf, ns, ninst, r.IsGroupChat, total, userIDArg)
			if err != nil {
				return fmt.Errorf("insert subject contact %q: %w", r.PrimaryName, err)
			}
			inserted++
		} else {
			_, err = tx.ExecContext(ctx,
				`INSERT INTO contacts (name, alternative_names, email, numemails, numwhatsapp, numimessages, numfacebook, numsms, numinstagram, is_group, total, user_id) VALUES (?1, ?2, ?3, ?4, ?5, ?6, ?7, ?8, ?9, ?10, ?11, ?12)`,
				r.PrimaryName, r.AlternativeNames, r.Emails,
				nemails, nw, ni, nf, ns, ninst, r.IsGroupChat, total, userIDArg)
			if err != nil {
				return fmt.Errorf("insert contact %q: %w", r.PrimaryName, err)
			}
			inserted++
		}

		if (i+1)%progressInterval == 0 || i+1 == totalRecords {
			fmt.Fprintf(os.Stderr, "Processed %d/%d contacts records (%d matched, %d new)\n", i+1, totalRecords, matched, inserted)
		}
	}

	fmt.Fprintf(os.Stderr, "Committing contacts transaction\n")
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit contacts transaction: %w", err)
	}
	fmt.Fprintf(os.Stderr, "Contacts transaction committed (%d matched, %d new)\n", matched, inserted)
	return nil
}
