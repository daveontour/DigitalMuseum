package contacts

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

// EmailClassifications maps rel_type values to lists of contact names
type EmailClassifications map[string][]string

var relTypeKeys = []string{
	"friend", "family", "colleague", "acquaintance",
	"business", "social", "promotional", "spam", "important",
}

// LoadEmailClassifications loads classifications from the email_classifications table.
// Keys are rel_type values (friend, family, colleague, etc.).
func LoadEmailClassifications(ctx context.Context, db *pgxpool.Pool) (EmailClassifications, error) {
	rows, err := db.Query(ctx, "SELECT classification, name FROM email_classifications ORDER BY classification")
	if err != nil {
		return nil, fmt.Errorf("query email_classifications: %w", err)
	}
	defer rows.Close()

	result := make(EmailClassifications)
	for rows.Next() {
		var classification, name string
		if err := rows.Scan(&classification, &name); err != nil {
			return nil, fmt.Errorf("scan email_classifications row: %w", err)
		}
		result[classification] = append(result[classification], name)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate email_classifications rows: %w", err)
	}

	for _, key := range relTypeKeys {
		if _, ok := result[key]; !ok {
			result[key] = nil
		}
	}
	return result, nil
}

// ApplyClassificationsToContacts updates contacts by name using the classification lists.
// Matches against contact name and alternative_names (comma-separated). Case-insensitive.
func ApplyClassificationsToContacts(ctx context.Context, db *pgxpool.Pool, classifications EmailClassifications) error {
	if len(classifications) == 0 {
		return nil
	}

	// Build normalized name set for each rel_type
	relTypeToNames := make(map[string]map[string]struct{})
	for relType, names := range classifications {
		if len(names) == 0 {
			continue
		}
		m := make(map[string]struct{})
		for _, n := range names {
			norm := strings.ToLower(strings.TrimSpace(n))
			if norm != "" {
				m[norm] = struct{}{}
			}
		}
		if len(m) > 0 {
			relTypeToNames[relType] = m
		}
	}
	if len(relTypeToNames) == 0 {
		return nil
	}

	// Fetch all contacts
	rows, err := db.Query(ctx, "SELECT id, name, alternative_names FROM contacts")
	if err != nil {
		return fmt.Errorf("query contacts: %w", err)
	}
	defer rows.Close()

	type contact struct {
		id               int
		name             string
		alternativeNames *string
	}

	var contacts []contact
	for rows.Next() {
		var c contact
		if err := rows.Scan(&c.id, &c.name, &c.alternativeNames); err != nil {
			return fmt.Errorf("scan contact: %w", err)
		}
		contacts = append(contacts, c)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate contacts: %w", err)
	}

	// For each rel_type, collect contact IDs that match
	updates := make(map[string][]int)
	for relType, nameSet := range relTypeToNames {
		for _, c := range contacts {
			names := []string{strings.TrimSpace(c.name)}
			if c.alternativeNames != nil && *c.alternativeNames != "" {
				for _, alt := range strings.Split(*c.alternativeNames, ",") {
					alt = strings.TrimSpace(alt)
					if alt != "" {
						names = append(names, alt)
					}
				}
			}
			for _, n := range names {
				if _, ok := nameSet[strings.ToLower(n)]; ok {
					if c.id != 0 {
						updates[relType] = append(updates[relType], c.id)
					}
					break
				}
			}
		}
	}

	// Reset all contacts to unknown
	_, err = db.Exec(ctx, "UPDATE contacts SET rel_type = 'unknown' WHERE id != 0")
	if err != nil {
		return fmt.Errorf("update contacts: %w", err)
	}

	// Apply each rel_type to matching contacts
	for _, relType := range relTypeKeys {
		ids := updates[relType]
		if len(ids) == 0 {
			continue
		}
		placeholders := make([]string, len(ids))
		args := make([]interface{}, len(ids)+1)
		args[0] = relType
		for i, id := range ids {
			placeholders[i] = fmt.Sprintf("$%d", i+2)
			args[i+1] = id
		}
		query := fmt.Sprintf("UPDATE contacts SET rel_type = $1 WHERE id IN (%s)", strings.Join(placeholders, ","))
		_, err = db.Exec(ctx, query, args...)
		if err != nil {
			return fmt.Errorf("update rel_type=%s: %w", relType, err)
		}
	}

	// Ensure subject (id=0) always stays unknown
	_, err = db.Exec(ctx, "UPDATE contacts SET rel_type = 'unknown' WHERE id = 0")
	if err != nil {
		return fmt.Errorf("reset subject rel_type: %w", err)
	}

	return nil
}
