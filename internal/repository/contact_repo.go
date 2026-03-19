package repository

import (
	"context"
	"fmt"
	"strings"

	"github.com/daveontour/digitalmuseum/internal/model"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ContactRepo accesses contacts and related tables.
type ContactRepo struct {
	pool *pgxpool.Pool
}

// NewContactRepo creates a ContactRepo.
func NewContactRepo(pool *pgxpool.Pool) *ContactRepo {
	return &ContactRepo{pool: pool}
}

// ── Contacts ──────────────────────────────────────────────────────────────────

const allowedContactOrderCols = "id name email numemails numsms numwhatsapp numimessages numinstagram numfacebook"

// ContactListParams holds filter/sort/page parameters for listing contacts.
type ContactListParams struct {
	Name             string
	Email            string
	Search           string
	IsSubject        *bool
	IsGroup          *bool
	HasMessages      *bool
	EmailContainsAt  *bool
	ExcludePhoneNums *bool
	Limit            int
	Offset           int
	OrderBy          string
	Order            string
}

// ListShort returns contacts with short response fields.
func (r *ContactRepo) ListShort(ctx context.Context, p ContactListParams) ([]*model.Contact, int, error) {
	const cols = `id, name, email, numemails, facebookid, numfacebook, whatsappid,
		numwhatsapp, imessageid, numimessages, smsid, numsms, instagramid, numinstagram`

	var args []any
	var conds []string

	if p.Name != "" {
		args = append(args, "%"+p.Name+"%")
		conds = append(conds, fmt.Sprintf("name ILIKE $%d", len(args)))
	}
	if p.Email != "" {
		args = append(args, "%"+p.Email+"%")
		conds = append(conds, fmt.Sprintf("email ILIKE $%d", len(args)))
	}
	if p.Search != "" {
		args = append(args, "%"+p.Search+"%")
		idx := len(args)
		conds = append(conds, fmt.Sprintf("(name ILIKE $%d OR email ILIKE $%d)", idx, idx))
	}
	if p.IsSubject != nil {
		args = append(args, *p.IsSubject)
		conds = append(conds, fmt.Sprintf("is_subject = $%d", len(args)))
	}
	if p.IsGroup != nil {
		args = append(args, *p.IsGroup)
		conds = append(conds, fmt.Sprintf("is_group = $%d", len(args)))
	}
	if p.HasMessages != nil && *p.HasMessages {
		conds = append(conds, "(COALESCE(numemails,0)+COALESCE(numfacebook,0)+COALESCE(numwhatsapp,0)+COALESCE(numsms,0)+COALESCE(numimessages,0)+COALESCE(numinstagram,0)) > 0")
	}
	if p.EmailContainsAt != nil && *p.EmailContainsAt {
		conds = append(conds, "email ILIKE '%@%'")
	}
	if p.ExcludePhoneNums != nil && *p.ExcludePhoneNums {
		conds = append(conds, `(name IS NULL OR name = '' OR name !~ '^[0-9\s+]+$')`)
	}

	where := ""
	if len(conds) > 0 {
		where = " WHERE " + joinAnd(conds)
	}

	// Count
	var total int
	if err := r.pool.QueryRow(ctx, "SELECT COUNT(*) FROM contacts"+where, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("ContactListCount: %w", err)
	}

	// Validate order
	col := "name"
	if p.OrderBy != "" && strings.Contains(allowedContactOrderCols, p.OrderBy) {
		col = p.OrderBy
	}
	dir := "ASC"
	if strings.ToLower(p.Order) == "desc" {
		dir = "DESC NULLS LAST"
	}
	q := fmt.Sprintf("SELECT %s FROM contacts%s ORDER BY %s %s", cols, where, col, dir)
	if p.Limit > 0 {
		args = append(args, p.Limit)
		q += fmt.Sprintf(" LIMIT $%d", len(args))
	}
	if p.Offset > 0 {
		args = append(args, p.Offset)
		q += fmt.Sprintf(" OFFSET $%d", len(args))
	}

	rows, err := r.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("ContactList: %w", err)
	}
	defer rows.Close()

	var out []*model.Contact
	for rows.Next() {
		var c model.Contact
		if err := rows.Scan(&c.ID, &c.Name, &c.Email, &c.NumEmails,
			&c.FacebookID, &c.NumFacebook, &c.WhatsAppID, &c.NumWhatsApp,
			&c.IMessageID, &c.NumIMessages, &c.SMSID, &c.NumSMS,
			&c.InstagramID, &c.NumInstagram); err != nil {
			return nil, 0, err
		}
		out = append(out, &c)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}

	return out, total, nil
}

// ListNames returns all contacts as (id, name) pairs for the light endpoint.
func (r *ContactRepo) ListNames(ctx context.Context) ([]struct {
	ID   int64
	Name string
}, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT id, name FROM contacts
		 WHERE (name IS NULL OR name = '' OR name !~ '^[0-9\s+]+$')
		   AND (COALESCE(numemails,0)+COALESCE(numfacebook,0)+COALESCE(numwhatsapp,0)+COALESCE(numsms,0)+COALESCE(numimessages,0)+COALESCE(numinstagram,0)) > 0
		 ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []struct {
		ID   int64
		Name string
	}
	for rows.Next() {
		var item struct {
			ID   int64
			Name string
		}
		if err := rows.Scan(&item.ID, &item.Name); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

// GetByName returns the first contact matching name (for classification update).
func (r *ContactRepo) GetByName(ctx context.Context, name string) (*struct {
	ID      int64
	RelType *string
}, error) {
	var c struct {
		ID      int64
		RelType *string
	}
	err := r.pool.QueryRow(ctx, `SELECT id, rel_type FROM contacts WHERE name = $1 LIMIT 1`, name).
		Scan(&c.ID, &c.RelType)
	if err != nil {
		if isNoRows(err) {
			return nil, nil
		}
		return nil, err
	}
	return &c, nil
}

// UpdateRelType sets rel_type for a contact by ID.
func (r *ContactRepo) UpdateRelType(ctx context.Context, id int64, relType string) error {
	_, err := r.pool.Exec(ctx, `UPDATE contacts SET rel_type=$1, updated_at=NOW() WHERE id=$2`, relType, id)
	return err
}

// Delete removes a contact. Returns false if not found.
func (r *ContactRepo) Delete(ctx context.Context, id int64) (bool, error) {
	tag, err := r.pool.Exec(ctx, `DELETE FROM contacts WHERE id = $1`, id)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() > 0, nil
}

// BulkDelete removes multiple contacts. Returns lists of deleted and skipped IDs.
func (r *ContactRepo) BulkDelete(ctx context.Context, ids []int64) (deleted, skipped []int64, err error) {
	for _, id := range ids {
		if id == 0 {
			skipped = append(skipped, id)
			continue
		}
		ok, e := r.Delete(ctx, id)
		if e != nil {
			return deleted, skipped, e
		}
		if ok {
			deleted = append(deleted, id)
		} else {
			skipped = append(skipped, id)
		}
	}
	return deleted, skipped, nil
}

// ── Relationship graph ────────────────────────────────────────────────────────

var validRelTypes = map[string]bool{
	"friend": true, "family": true, "colleague": true, "acquaintance": true,
	"business": true, "social": true, "promotional": true, "spam": true,
	"important": true, "unknown": true,
}

var validSources = map[string]string{
	"email":        "COALESCE(numemails,0)",
	"facebook":     "COALESCE(numfacebook,0)",
	"whatsapp":     "COALESCE(numwhatsapp,0)",
	"sms-imessage": "COALESCE(numsms,0) + COALESCE(numimessages,0)",
	"instagram":    "COALESCE(numinstagram,0)",
}

var sourceContactCond = map[string]string{
	"email":        "numemails > 0",
	"facebook":     "numfacebook > 0",
	"whatsapp":     "numwhatsapp > 0",
	"sms-imessage": "(numsms > 0 OR numimessages > 0)",
	"instagram":    "numinstagram > 0",
}

// GetRelationshipGraph returns nodes for the relationship graph.
func (r *ContactRepo) GetRelationshipGraph(ctx context.Context, types, sources []string, maxNodes int) ([]*model.ContactGraph, error) {
	// Validate types
	var validT []string
	for _, t := range types {
		if validRelTypes[t] {
			validT = append(validT, "'"+t+"'")
		}
	}
	typeClause := "rel_type IN ('friend', 'acquaintance', 'unknown')"
	if len(validT) > 0 {
		typeClause = "rel_type IN (" + strings.Join(validT, ",") + ")"
	}

	// Validate sources
	var srcConds []string
	var sumParts []string
	for _, s := range sources {
		if cond, ok := sourceContactCond[s]; ok {
			srcConds = append(srcConds, cond)
		}
		if expr, ok := validSources[s]; ok {
			sumParts = append(sumParts, expr)
		}
	}
	sourceClause := "numwhatsapp > 0 OR numemails > 0 OR numimessages > 0 OR numsms > 0 OR numfacebook > 0 OR numinstagram > 0"
	sumClause := "COALESCE(numemails,0)+COALESCE(numfacebook,0)+COALESCE(numwhatsapp,0)+COALESCE(numsms,0)+COALESCE(numimessages,0)+COALESCE(numinstagram,0)"
	if len(srcConds) > 0 {
		sourceClause = strings.Join(srcConds, " OR ")
		sumClause = strings.Join(sumParts, " + ")
	}

	if maxNodes < 1 {
		maxNodes = 1
	}
	if maxNodes > 1000 {
		maxNodes = 1000
	}

	q := fmt.Sprintf(`
		SELECT id, name, rel_type, numemails, numimessages, numfacebook, numwhatsapp, numsms, numinstagram,
		       (%s) AS total
		FROM contacts
		WHERE (id = 0 OR (
		    %s
		    AND (%s)
		    AND ((%s) > 3)
		))
		ORDER BY total DESC
		LIMIT %d`, sumClause, typeClause, sourceClause, sumClause, maxNodes)

	rows, err := r.pool.Query(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("GetRelationshipGraph: %w", err)
	}
	defer rows.Close()

	var out []*model.ContactGraph
	for rows.Next() {
		var c model.ContactGraph
		if err := rows.Scan(&c.ID, &c.Name, &c.RelType,
			&c.NumEmails, &c.NumIMessages, &c.NumFacebook,
			&c.NumWhatsApp, &c.NumSMS, &c.NumInstagram, &c.Total); err != nil {
			return nil, err
		}
		out = append(out, &c)
	}
	return out, rows.Err()
}

// ── Email matches ─────────────────────────────────────────────────────────────

func scanEmailMatch(row interface{ Scan(...any) error }) (*model.EmailMatch, error) {
	var m model.EmailMatch
	err := row.Scan(&m.ID, &m.PrimaryName, &m.Email, &m.CreatedAt, &m.UpdatedAt)
	return &m, err
}

// ListEmailMatches returns all email matches with optional primary_name filter.
func (r *ContactRepo) ListEmailMatches(ctx context.Context, primaryName string) ([]*model.EmailMatch, error) {
	q := `SELECT id, primary_name, email, created_at, updated_at FROM email_matches`
	var args []any
	if primaryName != "" {
		args = append(args, "%"+primaryName+"%")
		q += " WHERE primary_name ILIKE $1"
	}
	q += " ORDER BY primary_name, email"
	rows, err := r.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*model.EmailMatch
	for rows.Next() {
		m, err := scanEmailMatch(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

func (r *ContactRepo) GetEmailMatchByID(ctx context.Context, id int64) (*model.EmailMatch, error) {
	m, err := scanEmailMatch(r.pool.QueryRow(ctx,
		`SELECT id, primary_name, email, created_at, updated_at FROM email_matches WHERE id=$1`, id))
	if err != nil {
		if isNoRows(err) {
			return nil, nil
		}
		return nil, err
	}
	return m, nil
}

func (r *ContactRepo) EmailMatchExists(ctx context.Context, primaryName, email string) (bool, error) {
	var n int
	err := r.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM email_matches WHERE primary_name=$1 AND email=$2`, primaryName, email).Scan(&n)
	return n > 0, err
}

func (r *ContactRepo) CreateEmailMatch(ctx context.Context, primaryName, email string) (*model.EmailMatch, error) {
	m, err := scanEmailMatch(r.pool.QueryRow(ctx,
		`INSERT INTO email_matches (primary_name, email) VALUES ($1,$2)
		 RETURNING id, primary_name, email, created_at, updated_at`, primaryName, email))
	if err != nil {
		return nil, fmt.Errorf("CreateEmailMatch: %w", err)
	}
	return m, nil
}

func (r *ContactRepo) UpdateEmailMatch(ctx context.Context, id int64, primaryName, email *string) (*model.EmailMatch, error) {
	m, err := scanEmailMatch(r.pool.QueryRow(ctx,
		`UPDATE email_matches SET
		 primary_name = COALESCE($1, primary_name),
		 email        = COALESCE($2, email),
		 updated_at   = NOW()
		 WHERE id=$3
		 RETURNING id, primary_name, email, created_at, updated_at`,
		primaryName, email, id))
	if err != nil {
		if isNoRows(err) {
			return nil, nil
		}
		return nil, err
	}
	return m, nil
}

func (r *ContactRepo) DeleteEmailMatch(ctx context.Context, id int64) (bool, error) {
	tag, err := r.pool.Exec(ctx, `DELETE FROM email_matches WHERE id=$1`, id)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() > 0, nil
}

// ── Email exclusions ──────────────────────────────────────────────────────────

func scanEmailExclusion(row interface{ Scan(...any) error }) (*model.EmailExclusion, error) {
	var e model.EmailExclusion
	err := row.Scan(&e.ID, &e.Email, &e.Name, &e.NameEmail, &e.CreatedAt, &e.UpdatedAt)
	return &e, err
}

func (r *ContactRepo) ListEmailExclusions(ctx context.Context, search string, nameEmail *bool) ([]*model.EmailExclusion, error) {
	var args []any
	var conds []string
	if search != "" {
		args = append(args, "%"+search+"%")
		idx := len(args)
		conds = append(conds, fmt.Sprintf("(email ILIKE $%d OR name ILIKE $%d)", idx, idx))
	}
	if nameEmail != nil {
		args = append(args, *nameEmail)
		conds = append(conds, fmt.Sprintf("name_email = $%d", len(args)))
	}
	q := `SELECT id, email, name, name_email, created_at, updated_at FROM email_exclusions`
	if len(conds) > 0 {
		q += " WHERE " + joinAnd(conds)
	}
	q += " ORDER BY email, name"
	rows, err := r.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*model.EmailExclusion
	for rows.Next() {
		e, err := scanEmailExclusion(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

func (r *ContactRepo) GetEmailExclusionByID(ctx context.Context, id int64) (*model.EmailExclusion, error) {
	e, err := scanEmailExclusion(r.pool.QueryRow(ctx,
		`SELECT id, email, name, name_email, created_at, updated_at FROM email_exclusions WHERE id=$1`, id))
	if err != nil {
		if isNoRows(err) {
			return nil, nil
		}
		return nil, err
	}
	return e, nil
}

func (r *ContactRepo) ExclusionExists(ctx context.Context, email, name string, nameEmail bool) (bool, error) {
	var n int
	err := r.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM email_exclusions WHERE email=$1 AND name=$2 AND name_email=$3`,
		email, name, nameEmail).Scan(&n)
	return n > 0, err
}

func (r *ContactRepo) CreateEmailExclusion(ctx context.Context, email, name string, nameEmail bool) (*model.EmailExclusion, error) {
	e, err := scanEmailExclusion(r.pool.QueryRow(ctx,
		`INSERT INTO email_exclusions (email, name, name_email) VALUES ($1,$2,$3)
		 RETURNING id, email, name, name_email, created_at, updated_at`, email, name, nameEmail))
	if err != nil {
		return nil, fmt.Errorf("CreateEmailExclusion: %w", err)
	}
	return e, nil
}

func (r *ContactRepo) UpdateEmailExclusion(ctx context.Context, id int64, email, name *string, nameEmail *bool) (*model.EmailExclusion, error) {
	e, err := scanEmailExclusion(r.pool.QueryRow(ctx,
		`UPDATE email_exclusions SET
		 email      = COALESCE($1, email),
		 name       = COALESCE($2, name),
		 name_email = COALESCE($3, name_email),
		 updated_at = NOW()
		 WHERE id=$4
		 RETURNING id, email, name, name_email, created_at, updated_at`,
		email, name, nameEmail, id))
	if err != nil {
		if isNoRows(err) {
			return nil, nil
		}
		return nil, err
	}
	return e, nil
}

func (r *ContactRepo) DeleteEmailExclusion(ctx context.Context, id int64) (bool, error) {
	tag, err := r.pool.Exec(ctx, `DELETE FROM email_exclusions WHERE id=$1`, id)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() > 0, nil
}

// ── Email classifications ──────────────────────────────────────────────────────

func scanEmailClassification(row interface{ Scan(...any) error }) (*model.EmailClassification, error) {
	var c model.EmailClassification
	err := row.Scan(&c.ID, &c.Name, &c.Classification, &c.CreatedAt, &c.UpdatedAt)
	return &c, err
}

func (r *ContactRepo) ListEmailClassifications(ctx context.Context, name, classification string) ([]*model.EmailClassification, error) {
	var args []any
	var conds []string
	if name != "" {
		args = append(args, "%"+name+"%")
		conds = append(conds, fmt.Sprintf("name ILIKE $%d", len(args)))
	}
	if classification != "" && validRelTypes[classification] {
		args = append(args, classification)
		conds = append(conds, fmt.Sprintf("classification = $%d", len(args)))
	}
	q := `SELECT id, name, classification, created_at, updated_at FROM email_classifications`
	if len(conds) > 0 {
		q += " WHERE " + joinAnd(conds)
	}
	q += " ORDER BY classification, name"
	rows, err := r.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*model.EmailClassification
	for rows.Next() {
		c, err := scanEmailClassification(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (r *ContactRepo) GetEmailClassificationByID(ctx context.Context, id int64) (*model.EmailClassification, error) {
	c, err := scanEmailClassification(r.pool.QueryRow(ctx,
		`SELECT id, name, classification, created_at, updated_at FROM email_classifications WHERE id=$1`, id))
	if err != nil {
		if isNoRows(err) {
			return nil, nil
		}
		return nil, err
	}
	return c, nil
}

func (r *ContactRepo) ClassificationExists(ctx context.Context, name, classification string) (bool, error) {
	var n int
	err := r.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM email_classifications WHERE name=$1 AND classification=$2`, name, classification).Scan(&n)
	return n > 0, err
}

func (r *ContactRepo) CreateEmailClassification(ctx context.Context, name, classification string) (*model.EmailClassification, error) {
	c, err := scanEmailClassification(r.pool.QueryRow(ctx,
		`INSERT INTO email_classifications (name, classification) VALUES ($1,$2)
		 RETURNING id, name, classification, created_at, updated_at`, name, classification))
	if err != nil {
		return nil, fmt.Errorf("CreateEmailClassification: %w", err)
	}
	return c, nil
}

func (r *ContactRepo) UpdateEmailClassification(ctx context.Context, id int64, name, classification *string) (*model.EmailClassification, error) {
	c, err := scanEmailClassification(r.pool.QueryRow(ctx,
		`UPDATE email_classifications SET
		 name           = COALESCE($1, name),
		 classification = COALESCE($2, classification),
		 updated_at     = NOW()
		 WHERE id=$3
		 RETURNING id, name, classification, created_at, updated_at`,
		name, classification, id))
	if err != nil {
		if isNoRows(err) {
			return nil, nil
		}
		return nil, err
	}
	return c, nil
}

func (r *ContactRepo) DeleteEmailClassification(ctx context.Context, id int64) (bool, error) {
	tag, err := r.pool.Exec(ctx, `DELETE FROM email_classifications WHERE id=$1`, id)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() > 0, nil
}

// GetClassificationByNameLower returns a classification row matching name (case-insensitive).
func (r *ContactRepo) GetClassificationByNameLower(ctx context.Context, name string) (*model.EmailClassification, error) {
	c, err := scanEmailClassification(r.pool.QueryRow(ctx,
		`SELECT id, name, classification, created_at, updated_at FROM email_classifications WHERE LOWER(name)=LOWER($1) LIMIT 1`, name))
	if err != nil {
		if isNoRows(err) {
			return nil, nil
		}
		return nil, err
	}
	return c, nil
}

// ApplyClassificationToContacts updates rel_type for all contacts matching the given name.
func (r *ContactRepo) ApplyClassificationToContacts(ctx context.Context, name, classification string) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE contacts SET rel_type=$1, updated_at=NOW()
		 WHERE id != 0 AND (
		     LOWER(name) = LOWER($2)
		     OR LOWER(alternative_names) LIKE '%' || LOWER($2) || '%'
		 )`,
		classification, name)
	return err
}
