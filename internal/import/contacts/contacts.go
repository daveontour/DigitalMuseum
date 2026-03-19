package contacts

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

// RunContactsNormalise runs the contact normalisation process
func RunContactsNormalise(ctx context.Context, opts RunOptions) error {
	progress := func(msg string) {
		if opts.ProgressFunc != nil {
			opts.ProgressFunc(msg)
		} else {
			fmt.Fprintln(os.Stderr, msg)
		}
	}

	if err := LoadExclusionsFromDB(ctx, opts.ContactsDB); err != nil {
		return fmt.Errorf("load exclusions from db: %w", err)
	}

	var emailMatchMap map[string]string
	var emailPrimaryNameMap map[string]string
	if opts.ContactsDB != nil {
		var err error
		emailMatchMap, emailPrimaryNameMap, err = LoadEmailMatchSets(ctx, opts.ContactsDB)
		if err != nil {
			return fmt.Errorf("load email matches: %w", err)
		}
	}

	var records []InputRecord
	var err error

	if opts.ContactsDB == nil {
		return fmt.Errorf("Database required for CONTACTS_QUERY mode")
	}
	progress("Reading contacts from database")
	records, err = ReadFromDatabase(ctx, opts.ContactsDB, "SELECT from_address FROM emails UNION SELECT to_addresses FROM emails")
	if err != nil {
		return fmt.Errorf("read from database: %w", err)
	}
	progress(fmt.Sprintf("Read %d contacts from database", len(records)))

	progress("Merging contacts")
	groups := runMerge(records, emailMatchMap, emailPrimaryNameMap, opts.Workers)
	progress(fmt.Sprintf("Found %d groups", len(groups)))
	formattedOutput := formatOutput(ctx, opts.ContactsDB, groups)

	if opts.ContactsDB != nil {
		progress("Running social media query")
		smRecords, err := runSocialMediaQuery(ctx, opts.ContactsDB)
		if err != nil {
			return fmt.Errorf("social media query: %w", err)
		}
		progress(fmt.Sprintf("Found %d social media records", len(smRecords)))
		for _, r := range smRecords {

			// for each contact, if chat_session matches primary name or any email, add the social media counts
			if r.ChatSession == nil {
				continue
			}
			csNorm := strings.TrimSpace(strings.ToLower(*r.ChatSession))
			matched := false
			for i := range formattedOutput {
				f := &formattedOutput[i]
				if strings.TrimSpace(strings.ToLower(f.PrimaryName)) == csNorm {
					matched = true
					f.NumWhatsApp += r.NumWhatsApp
					f.NumIMessage += r.NumIMessage
					f.NumFacebook += r.NumFacebook
					f.NumSMS += r.NumSMS
					f.NumInstagram += r.NumInstagram
					break
				}
				for _, email := range strings.Split(f.Emails, ",") {
					if strings.TrimSpace(strings.ToLower(email)) == csNorm {
						matched = true
						f.NumWhatsApp += r.NumWhatsApp
						f.NumIMessage += r.NumIMessage
						f.NumFacebook += r.NumFacebook
						f.NumSMS += r.NumSMS
						f.NumInstagram += r.NumInstagram
						break
					}
				}
				if matched {
					break
				}
			}
			if !matched {
				maxID := 0
				for _, f := range formattedOutput {
					if f.ID > maxID {
						maxID = f.ID
					}
				}
				formattedOutput = append(formattedOutput, FormattedOutputRecord{
					ID:           maxID + 1,
					PrimaryName:  *r.ChatSession,
					Emails:       *r.ChatSession,
					NumWhatsApp:  r.NumWhatsApp,
					NumIMessage:  r.NumIMessage,
					NumFacebook:  r.NumFacebook,
					NumSMS:       r.NumSMS,
					NumInstagram: r.NumInstagram,
					IsGroupChat:  r.IsGroupChat,
				})
			}
		}
	}

	progress("Computing email message counts per contact")
	if err := computeEmailMessageCounts(ctx, opts.ContactsDB, formattedOutput); err != nil {
		return fmt.Errorf("compute email counts: %w", err)
	}

	progress("Truncating contacts table")
	if err := TruncateContactsTable(ctx, opts.ContactsDB); err != nil {
		return fmt.Errorf("truncate contacts table: %w", err)
	}
	if opts.ContactsDB == nil {
		return fmt.Errorf("database required for write")
	}
	progress("Writing contacts and classifications to database")
	if err := writeContactsAndClassifications(ctx, opts.ContactsDB, opts.ClassificationsFile, formattedOutput); err != nil {
		return err
	}
	if opts.RelationshipQuery != "" {
		progress("Finding relationships")
		emailMap := CreateEmailMap(formattedOutput)
		findRelationships(ctx, emailMap, opts.RelationshipQuery, true, opts.ContactsDB)
	}

	return nil
}

// writeContactsAndClassifications writes contacts to DB and applies classifications
func writeContactsAndClassifications(ctx context.Context, db *pgxpool.Pool, classificationsFile string, formattedOutput []FormattedOutputRecord) error {
	if db == nil {
		return fmt.Errorf("database required for write")
	}
	fmt.Fprintf(os.Stderr, "Writing %d contacts records to database\n", len(formattedOutput))
	if err := WriteContactsToDatabase(ctx, db, formattedOutput); err != nil {
		return err
	}
	classifications, err := LoadEmailClassifications(ctx, db)
	if err != nil {
		return fmt.Errorf("load classifications: %w", err)
	}
	if err := ApplyClassificationsToContacts(ctx, db, classifications); err != nil {
		return fmt.Errorf("apply classifications: %w", err)
	}
	fmt.Fprintf(os.Stderr, "Records written to database\n")
	return nil
}

func TruncateContactsTable(ctx context.Context, db *pgxpool.Pool) error {
	_, err := db.Exec(ctx, "TRUNCATE contacts CASCADE")
	if err != nil {
		return fmt.Errorf("truncate contacts table: %w", err)
	}
	return nil
}

// createSocialMediaRelationships creates directional relationships for each contact with non-zero social media counts.
func createSocialMediaRelationships(ctx context.Context, db *pgxpool.Pool, formattedOutput []FormattedOutputRecord, socialMediaRecords []SocialMediaRecord) error {
	subjectIds, err := LoadSubjectIdentifiers(ctx, db)
	if err != nil {
		return fmt.Errorf("load subject identifiers: %w", err)
	}

	// Aggregate by (fromID, toID, type) since one contact can have multiple chat_sessions per service
	type key struct {
		fromID int
		toID   int
		typ    string
	}
	agg := make(map[key]int)

	for _, r := range socialMediaRecords {
		if r.ChatSession == nil {
			continue
		}
		csNorm := strings.TrimSpace(strings.ToLower(*r.ChatSession))
		var contactID *int
		for _, f := range formattedOutput {
			if strings.TrimSpace(strings.ToLower(f.PrimaryName)) == csNorm {
				contactID = &f.ID
				break
			}
			for _, email := range strings.Split(f.Emails, ",") {
				if strings.TrimSpace(strings.ToLower(email)) == csNorm {
					contactID = &f.ID
					break
				}
			}
			if contactID != nil {
				break
			}
		}
		if contactID == nil {
			continue
		}
		cid := *contactID

		services := []struct {
			name string
			subj *string
			cnt  int64
		}{
			{"whatsapp", subjectIds.WhatsAppID, r.NumWhatsApp},
			{"imessage", subjectIds.IMessageID, r.NumIMessage},
			{"facebook", subjectIds.FacebookID, r.NumFacebook},
			{"sms", subjectIds.SMSID, r.NumSMS},
			{"instagram", subjectIds.InstagramID, r.NumInstagram},
		}
		for _, svc := range services {
			if svc.cnt <= 0 {
				continue
			}
			dc, err := GetDirectionalMessageCounts(ctx, db, *r.ChatSession, svc.name, svc.subj)
			if err != nil {
				return fmt.Errorf("directional counts %s %q: %w", svc.name, *r.ChatSession, err)
			}
			if dc.FromSubject > 0 {
				k := key{0, cid, svc.name}
				agg[k] += int(dc.FromSubject)
			}
			if dc.FromContact > 0 {
				k := key{cid, 0, svc.name}
				agg[k] += int(dc.FromContact)
			}
		}
	}

	var items []relationshipItem
	for k, count := range agg {
		items = append(items, relationshipItem{FromID: k.fromID, ToID: k.toID, Count: count, Type: k.typ})
	}
	if len(items) > 0 {
		if err := writeSocialMediaRelationshipsToDatabase(ctx, db, items); err != nil {
			return err
		}
		fmt.Fprintf(os.Stderr, "Wrote %d social media relationships\n", len(items))
	}
	return nil
}

// computeEmailMessageCounts populates records[i].NumEmails with the count of email
// messages (rows in the `emails` table) that match any email address for that contact.
//
// A single email row counts at most once per contact, even if multiple from/to
// addresses in that row map to the same contact.
func computeEmailMessageCounts(ctx context.Context, db *pgxpool.Pool, records []FormattedOutputRecord) error {
	if db == nil || len(records) == 0 {
		return nil
	}

	// normalized email address -> contact indexes (into `records`)
	emailNormToRecordIdx := make(map[string][]int)
	// normalized display name -> contact indexes (into `records`)
	nameNormToRecordIdx := make(map[string][]int)
	for i := range records {
		if pn := normalizeName(records[i].PrimaryName); pn != "" {
			nameNormToRecordIdx[pn] = append(nameNormToRecordIdx[pn], i)
		}

		parts := strings.Split(records[i].Emails, ",")
		seenForRecord := make(map[string]struct{})
		for _, part := range parts {
			part = strings.TrimSpace(part)
			if part == "" {
				continue
			}
			email, _ := ParseEmailEntry(part)
			if email == "" {
				email = strings.ToLower(part)
			}
			if email == "" {
				continue
			}
			norm := NormalizeEmailForMatching(email)
			if norm == "" {
				continue
			}
			if _, ok := seenForRecord[norm]; ok {
				continue
			}
			seenForRecord[norm] = struct{}{}
			emailNormToRecordIdx[norm] = append(emailNormToRecordIdx[norm], i)
		}
	}
	if len(emailNormToRecordIdx) == 0 {
		return nil
	}

	rows, err := db.Query(ctx, `SELECT from_address, to_addresses FROM emails`)
	if err != nil {
		return fmt.Errorf("query emails for counts: %w", err)
	}
	defer rows.Close()

	var fromAddr sql.NullString
	var toAddrs sql.NullString
	for rows.Next() {
		if err := rows.Scan(&fromAddr, &toAddrs); err != nil {
			return fmt.Errorf("scan emails row: %w", err)
		}
		// contact index -> already-counted for this email row
		matched := make(map[int]struct{})

		if fromAddr.Valid && fromAddr.String != "" {
			email, name := ParseEmailEntry(fromAddr.String)
			if email != "" {
				if idxs, ok := emailNormToRecordIdx[NormalizeEmailForMatching(email)]; ok {
					for _, idx := range idxs {
						matched[idx] = struct{}{}
					}
				}
			}
			if name != "" {
				if idxs, ok := nameNormToRecordIdx[normalizeName(name)]; ok {
					for _, idx := range idxs {
						matched[idx] = struct{}{}
					}
				}
			}
		}

		if toAddrs.Valid && toAddrs.String != "" {
			for _, part := range strings.Split(toAddrs.String, ",") {
				part = strings.TrimSpace(part)
				if part == "" {
					continue
				}
				email, name := ParseEmailEntry(part)
				if email == "" {
					email = strings.ToLower(part)
				}
				if email != "" {
					norm := NormalizeEmailForMatching(email)
					if idxs, ok := emailNormToRecordIdx[norm]; ok {
						for _, idx := range idxs {
							matched[idx] = struct{}{}
						}
					}
				}
				if name != "" {
					if idxs, ok := nameNormToRecordIdx[normalizeName(name)]; ok {
						for _, idx := range idxs {
							matched[idx] = struct{}{}
						}
					}
				}
			}
		}

		for idx := range matched {
			records[idx].NumEmails++
		}
	}

	return rows.Err()
}

const socialMediaQuery = `
SELECT
    chat_session,
    is_group_chat,
    COUNT(CASE WHEN service = 'WhatsApp' THEN 1 END) AS number_of_whatsapp,
    COUNT(CASE WHEN service = 'iMessage' THEN 1 END) AS number_of_imessage,
    COUNT(CASE WHEN service = 'Facebook Messenger' THEN 1 END) AS number_of_facebook,
    COUNT(CASE WHEN service = 'SMS' THEN 1 END) AS number_of_sms,
    COUNT(CASE WHEN service = 'Instagram' THEN 1 END) AS number_of_insta,
    COUNT(CASE WHEN service ILIKE '%' THEN 1 END) AS total
FROM
    messages
GROUP BY
    chat_session, is_group_chat
ORDER BY
    is_group_chat, total DESC;
`

func runSocialMediaQuery(ctx context.Context, db *pgxpool.Pool) ([]SocialMediaRecord, error) {
	rows, err := db.Query(ctx, socialMediaQuery)
	if err != nil {
		return nil, fmt.Errorf("execute query: %w", err)
	}
	defer rows.Close()

	var records []SocialMediaRecord
	for rows.Next() {
		var r SocialMediaRecord
		if err := rows.Scan(&r.ChatSession, &r.IsGroupChat, &r.NumWhatsApp, &r.NumIMessage, &r.NumFacebook, &r.NumSMS, &r.NumInstagram, &r.Total); err != nil {
			return nil, fmt.Errorf("scan row: %w", err)
		}
		records = append(records, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate rows: %w", err)
	}
	return records, nil
}

func runMerge(records []InputRecord, emailMatchMap, emailPrimaryNameMap map[string]string, workers int) []Group {
	var groups []Group
	newGroup := func() int {
		groups = append(groups, Group{
			Names:         map[string]struct{}{},
			Emails:        map[string]struct{}{},
			Normalized:    map[string]struct{}{},
			NameFrequency: map[string]int{},
		})
		return len(groups) - 1
	}

	for _, rec := range records {
		var normalized []string
		for _, n := range rec.Names {
			if isEmailAddress(n) {
				continue
			}
			if norm := normalizeName(n); norm != "" {
				normalized = append(normalized, norm)
			}
		}

		recEmailNorm := NormalizeEmailForMatching(rec.Email)
		bestGroup, bestScore := -1, 0.0

		if emailMatchMap != nil {
			if canonicalEmail, ok := emailMatchMap[recEmailNorm]; ok {
				for gi, g := range groups {
					for existingEmail := range g.Emails {
						existingEmailNorm := NormalizeEmailForMatching(existingEmail)
						if existingCanonical, exists := emailMatchMap[existingEmailNorm]; exists {
							if existingCanonical == canonicalEmail {
								bestGroup = gi
								bestScore = 1.0
								break
							}
						}
					}
					if bestScore >= 1.0 {
						break
					}
				}
			}
		}

		if bestScore < 1.0 {
			for gi, g := range groups {
				for existingEmail := range g.Emails {
					if NormalizeEmailForMatching(existingEmail) == recEmailNorm {
						bestGroup = gi
						bestScore = 1.0
						break
					}
				}
				if bestScore >= 1.0 {
					break
				}
			}
		}

		if bestScore < 1.0 {
			taskCh := make(chan similarityTask)
			resultCh := make(chan similarityResult)
			taskCount := 0
			var tasks []similarityTask
			for gi, g := range groups {
				for _, n1 := range normalized {
					for n2 := range g.Normalized {
						tasks = append(tasks, similarityTask{gi, n1, n2})
						taskCount++
					}
				}
			}

			for i := 0; i < workers; i++ {
				go similarityWorker(taskCh, resultCh)
			}
			go func() {
				for _, task := range tasks {
					taskCh <- task
				}
				close(taskCh)
			}()

			bestGroup, bestScore = -1, 0.0
			for i := 0; i < taskCount; i++ {
				res := <-resultCh
				if res.Score > bestScore {
					bestScore = res.Score
					bestGroup = res.GroupID
				}
			}
		}

		var gid int
		if bestScore >= FuzzyMergeThreshold {
			gid = bestGroup
		} else {
			gid = newGroup()
		}

		g := &groups[gid]
		g.Emails[rec.Email] = struct{}{}

		if bestScore >= 1.0 && emailMatchMap != nil && emailPrimaryNameMap != nil {
			if canonicalEmail, ok := emailMatchMap[recEmailNorm]; ok {
				if primaryName, hasPrimaryName := emailPrimaryNameMap[canonicalEmail]; hasPrimaryName && primaryName != "" {
					g.Names[primaryName] = struct{}{}
					norm := normalizeName(primaryName)
					if norm != "" {
						g.Normalized[norm] = struct{}{}
					}
					g.NameFrequency[primaryName] += 1000
				}
			}
		}

		for _, name := range rec.Names {
			norm := normalizeName(name)
			if norm == "" {
				continue
			}
			g.Names[name] = struct{}{}
			g.Normalized[norm] = struct{}{}
			g.NameFrequency[name]++
		}

		if bestScore > 0 {
			g.MergeScores = append(g.MergeScores, bestScore)
		}
	}

	return groups
}

func formatOutput(ctx context.Context, db *pgxpool.Pool, groups []Group) []FormattedOutputRecord {
	var formattedOutput []FormattedOutputRecord
	index := 1
	assignedZero := false

	var subjectName string
	var familyNamePtr *string
	err := db.QueryRow(ctx, "SELECT subject_name, family_name FROM subject_configuration LIMIT 1").Scan(&subjectName, &familyNamePtr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error getting subject configuration: %v\n", err)
		return formattedOutput
	}
	if subjectName == "" {
		subjectName = "Unknown"
	}
	familyName := "Unknown"
	if familyNamePtr != nil && *familyNamePtr != "" {
		familyName = *familyNamePtr
	}

	for _, g := range groups {
		primary := choosePrimaryName(g.NameFrequency)
		var emails []string
		for e := range g.Emails {
			emails = append(emails, e)
		}
		sort.Strings(emails)
		var altNames []string
		for n := range g.Names {
			if n != primary && !isEmailAddress(n) {
				altNames = append(altNames, n)
			}
		}

		sort.Strings(altNames)
		id := index
		if (primary == subjectName+" "+familyName) && !assignedZero {
			id = 0
			assignedZero = true
		} else {
			index++
		}
		formattedOutput = append(formattedOutput, FormattedOutputRecord{
			ID:               id,
			PrimaryName:      primary,
			AlternativeNames: strings.Join(altNames, ", "),
			Emails:           strings.Join(emails, ", "),
		})
	}
	sort.Slice(formattedOutput, func(i, j int) bool {
		return formattedOutput[i].ID < formattedOutput[j].ID
	})
	return formattedOutput
}

func findRelationships(ctx context.Context, emailMap map[string]int, query string, writeToDB bool, db *pgxpool.Pool) {

	emailToContact := make(map[string]struct {
		Name string
		ID   int
	})
	if db != nil {
		rows, err := db.Query(ctx, "SELECT id, name, email FROM contacts")
		if err != nil {
			fmt.Fprintf(os.Stderr, "error reading contacts table: %v\n", err)
		} else {
			defer rows.Close()
			for rows.Next() {
				var id int
				var name, emails string
				if err := rows.Scan(&id, &name, &emails); err != nil {
					fmt.Fprintf(os.Stderr, "error scanning contact row: %v\n", err)
					continue
				}
				// emails is a comma-separated string
				emailList := strings.Split(emails, ",")
				for _, e := range emailList {
					address := strings.TrimSpace(e)
					if address == "" {
						continue
					}
					emailToContact[address] = struct {
						Name string
						ID   int
					}{Name: name, ID: id}
				}
			}
		}
	}

	relationships, err := ReadRelationshipsFromDatabase(ctx, db, query)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error reading relationship query: %v\n", err)
		return
	}

	type combo struct {
		FromName string
		FromID   int
		ToName   string
		ToID     int
	}

	uniqueCombinations := make(map[combo]int)
	for _, r := range relationships {
		combo := combo{
			FromName: r.From,
			FromID:   0,
			ToName:   r.To,
			ToID:     0,
		}

		if contact, ok := emailToContact[r.From]; ok && contact.Name != "" {
			combo.FromID = contact.ID
		}
		if contact, ok := emailToContact[r.To]; ok && contact.Name != "" {
			combo.ToID = contact.ID
		}

		if combo.FromID != 0 && combo.ToID != 0 {
			continue
		}
		uniqueCombinations[combo]++
	}

	fmt.Fprintf(os.Stderr, "Found %d unique relationship combinations\n", len(uniqueCombinations))
}

// relationshipItem holds source/target IDs and count for DB insert
type relationshipItem struct {
	FromID int
	ToID   int
	Count  int
	Type   string // empty means "email" for backward compatibility
}

const relationshipBatchSize = 1000

// writeSocialMediaRelationshipsToDatabase inserts social media relationships with type per service.
// Deletes existing relationships of the given types for (source_id, target_id) pairs before inserting to avoid duplicates.
func writeSocialMediaRelationshipsToDatabase(ctx context.Context, db *pgxpool.Pool, items []relationshipItem) error {
	if len(items) == 0 {
		return nil
	}
	tx, err := db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback(ctx)
	// Delete existing social media relationships for the (source_id, target_id, type) combinations we're about to insert
	seen := make(map[string]struct{})
	for _, item := range items {
		if item.Type == "" {
			continue
		}
		key := fmt.Sprintf("%d|%d|%s", item.FromID, item.ToID, item.Type)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		_, err := tx.Exec(ctx, `DELETE FROM relationships WHERE source_id = $1 AND target_id = $2 AND type = $3`,
			item.FromID, item.ToID, item.Type)
		if err != nil {
			return fmt.Errorf("delete existing relationship: %w", err)
		}
	}
	for i := 0; i < len(items); i += relationshipBatchSize {
		end := i + relationshipBatchSize
		if end > len(items) {
			end = len(items)
		}
		batch := items[i:end]
		var placeholders []string
		var args []interface{}
		pos := 1
		for _, item := range batch {
			if item.Type == "" || item.Count <= 0 {
				continue
			}
			placeholders = append(placeholders, fmt.Sprintf("($%d, $%d, $%d, $%d, $%d, $%d, $%d)", pos, pos+1, pos+2, pos+3, pos+4, pos+5, pos+6))
			args = append(args, item.FromID, item.ToID, item.Type, item.Count, true, false, false)
			pos += 7
		}
		if len(placeholders) == 0 {
			continue
		}
		query := "INSERT INTO relationships (source_id, target_id, type, strength, is_active, is_personal, is_deleted) VALUES " + strings.Join(placeholders, ", ")
		_, err := tx.Exec(ctx, query, args...)
		if err != nil {
			return fmt.Errorf("batch insert social media relationships: %w", err)
		}
	}
	return tx.Commit(ctx)
}

// FindRelationshipsFromDB finds relationships from database and prints to stderr
func FindRelationshipsFromDB(ctx context.Context, db *pgxpool.Pool, query string, emailMap map[string]int) {
	findRelationships(ctx, emailMap, query, false, db)
}

// FindAndWriteRelationships finds relationships from database and writes them to the relationships table
func FindAndWriteRelationships(ctx context.Context, db *pgxpool.Pool, query string, emailMap map[string]int) {
	findRelationships(ctx, emailMap, query, true, db)
}
