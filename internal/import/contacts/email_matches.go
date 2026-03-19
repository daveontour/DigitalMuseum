package contacts

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

// EmailMatchSet represents a set of emails that are the same person
type EmailMatchSet struct {
	PrimaryName string   `json:"primary_name"`
	Emails      []string `json:"emails"`
}

func buildTransitiveClosure(emailSets []EmailMatchSet) (map[string]string, map[string]string) {
	canonical := make(map[string]string)
	var find func(string) string
	find = func(email string) string {
		if rep, ok := canonical[email]; ok {
			if rep != email {
				root := find(rep)
				canonical[email] = root
				return root
			}
			return rep
		}
		canonical[email] = email
		return email
	}
	union := func(email1, email2 string) {
		root1 := find(email1)
		root2 := find(email2)
		if root1 != root2 {
			if root1 < root2 {
				canonical[root2] = root1
			} else {
				canonical[root1] = root2
			}
		}
	}
	canonicalToPrimaryName := make(map[string]string)
	for _, emailSet := range emailSets {
		if len(emailSet.Emails) == 0 {
			continue
		}
		normalizedEmails := make([]string, len(emailSet.Emails))
		for i, email := range emailSet.Emails {
			normalizedEmails[i] = NormalizeEmailForMatching(email)
		}
		if len(normalizedEmails) > 0 {
			firstEmail := normalizedEmails[0]
			for i := 1; i < len(normalizedEmails); i++ {
				union(firstEmail, normalizedEmails[i])
			}
			canonicalRep := find(firstEmail)
			if emailSet.PrimaryName != "" {
				canonicalToPrimaryName[canonicalRep] = emailSet.PrimaryName
			}
		}
	}
	result := make(map[string]string)
	for _, emailSet := range emailSets {
		for _, email := range emailSet.Emails {
			normalized := NormalizeEmailForMatching(email)
			result[normalized] = find(normalized)
		}
	}
	return result, canonicalToPrimaryName
}

// LoadEmailMatchSets loads email match sets from the email_matches table,
// grouping by name to build sets of emails that belong to the same person.
func LoadEmailMatchSets(ctx context.Context, db *pgxpool.Pool) (map[string]string, map[string]string, error) {
	rows, err := db.Query(ctx, "SELECT primary_name, email FROM email_matches ORDER BY primary_name")
	if err != nil {
		return nil, nil, fmt.Errorf("failed to query email_matches table: %w", err)
	}
	defer rows.Close()

	groupMap := make(map[string][]string)
	for rows.Next() {
		var name, email string
		if err := rows.Scan(&name, &email); err != nil {
			return nil, nil, fmt.Errorf("failed to scan email_matches row: %w", err)
		}
		groupMap[name] = append(groupMap[name], email)
	}
	if err := rows.Err(); err != nil {
		return nil, nil, fmt.Errorf("error iterating email_matches rows: %w", err)
	}

	emailSets := make([]EmailMatchSet, 0, len(groupMap))
	for name, emails := range groupMap {
		emailSets = append(emailSets, EmailMatchSet{PrimaryName: name, Emails: emails})
	}

	canonicalMap, primaryNameMap := buildTransitiveClosure(emailSets)
	return canonicalMap, primaryNameMap, nil
}
