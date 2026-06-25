package repository

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/daveontour/aimuseum/internal/model"
	"github.com/daveontour/aimuseum/internal/sqlutil"
)

// ContactRepo accesses contacts and related tables.
type ContactRepo struct {
	pool *sql.DB
}

// NewContactRepo creates a ContactRepo.
func NewContactRepo(pool *sql.DB) *ContactRepo {
	return &ContactRepo{pool: pool}
}

// ── Contacts ──────────────────────────────────────────────────────────────────

const allowedContactOrderCols = "id name email numemails numsms numwhatsapp numimessages numinstagram numfacebook"

// excludeNameLooksLikePhoneOnlySQL matches the intent of PostgreSQL's `name !~ '^[0-9\s+]+$'`
// without regexp operators: after stripping digits and phone punctuation, the name must still
// have positive length. Built programmatically so nested replace() parentheses stay balanced.
func excludeNameLooksLikePhoneOnlySQL() string {
	const col = "name"
	chars := []string{"0", "1", "2", "3", "4", "5", "6", "7", "8", "9", " ", "+", "(", ")", "-"}
	expr := "coalesce(" + col + ",'')"
	for _, ch := range chars {
		expr = "replace(" + expr + ",'" + ch + "','')"
	}
	return "(" + col + " IS NULL OR " + col + " = '' OR length(trim(" + expr + ")) > 0)"
}

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
	uid := uidFromCtx(ctx)
	const cols = `id, name, email, numemails, facebookid, numfacebook, whatsappid,
		numwhatsapp, imessageid, numimessages, smsid, numsms, instagramid, numinstagram`

	var args []any
	var conds []string

	if p.Name != "" {
		args = append(args, "%"+p.Name+"%")
		conds = append(conds, "name LIKE ?")
	}
	if p.Email != "" {
		args = append(args, "%"+p.Email+"%")
		conds = append(conds, "email LIKE ?")
	}
	if p.Search != "" {
		pat := "%" + p.Search + "%"
		args = append(args, pat, pat)
		conds = append(conds, "(name LIKE ? OR email LIKE ?)")
	}
	if p.IsSubject != nil {
		args = append(args, *p.IsSubject)
		conds = append(conds, "is_subject = ?")
	}
	if p.IsGroup != nil {
		args = append(args, *p.IsGroup)
		conds = append(conds, "is_group = ?")
	}
	if p.HasMessages != nil && *p.HasMessages {
		conds = append(conds, "(COALESCE(numemails,0)+COALESCE(numfacebook,0)+COALESCE(numwhatsapp,0)+COALESCE(numsms,0)+COALESCE(numimessages,0)+COALESCE(numinstagram,0)) > 0")
	}
	if p.EmailContainsAt != nil && *p.EmailContainsAt {
		conds = append(conds, "email LIKE '%@%'")
	}
	if p.ExcludePhoneNums != nil && *p.ExcludePhoneNums {
		conds = append(conds, excludeNameLooksLikePhoneOnlySQL())
	}

	if uid > 0 {
		args = append(args, uid)
		conds = append(conds, "user_id = ?")
	}

	where := ""
	if len(conds) > 0 {
		where = " WHERE " + joinAnd(conds)
	}

	sql := "SELECT COUNT(*) FROM contacts" + where

	// Count
	var total int
	if err := r.pool.QueryRowContext(ctx, sql, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("contactListCount: %w", err)
	}

	// Validate order
	col := "name"
	if p.OrderBy != "" && strings.Contains(allowedContactOrderCols, p.OrderBy) {
		col = p.OrderBy
	}
	dir := "ASC"
	if strings.ToLower(p.Order) == "desc" {
		dir = "DESC"
	}
	q := fmt.Sprintf("SELECT %s FROM contacts%s ORDER BY %s %s", cols, where, col, dir)
	if p.Limit > 0 {
		args = append(args, p.Limit)
		q += " LIMIT ?"
	}
	if p.Offset > 0 {
		args = append(args, p.Offset)
		q += " OFFSET ?"
	}

	rows, err := r.pool.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("contactList: %w", err)
	}
	defer func() { _ = rows.Close() }()

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
	uid := uidFromCtx(ctx)
	q := `SELECT id, name FROM contacts
	      WHERE (` + excludeNameLooksLikePhoneOnlySQL() + `)
	        AND (COALESCE(numemails,0)+COALESCE(numfacebook,0)+COALESCE(numwhatsapp,0)+COALESCE(numsms,0)+COALESCE(numimessages,0)+COALESCE(numinstagram,0)) > 0`
	args := []any{}
	q, args = addUIDFilter(q, args, uid)
	q += " ORDER BY name"
	rows, err := r.pool.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
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

// ContactExistsForUser reports whether a contact row exists for the current user.
func (r *ContactRepo) ContactExistsForUser(ctx context.Context, contactID int64) (bool, error) {
	if contactID < 0 {
		return false, nil
	}
	uid := uidFromCtx(ctx)
	q := `SELECT 1 FROM contacts WHERE id = ?`
	args := []any{contactID}
	if contactID == 0 {
		q += " LIMIT 1"
	} else {
		q, args = addUIDFilterNullableGlobal(q, args, uid)
		q += " LIMIT 1"
	}
	var one int
	err := r.pool.QueryRowContext(ctx, q, args...).Scan(&one)
	if err != nil {
		if isNoRows(err) {
			return false, nil
		}
		return false, fmt.Errorf("contactExistsForUser: %w", err)
	}
	return true, nil
}

// GetContact loads one contact by id for the current user (id, name, email, alternative_names).
// Returns (nil, nil) when not found or id < 0.
func (r *ContactRepo) GetContact(ctx context.Context, id int64) (*model.ContactDetail, error) {
	if id < 0 {
		return nil, nil
	}
	uid := uidFromCtx(ctx)
	q := `SELECT id, name, email, alternative_names FROM contacts WHERE id = ?`
	args := []any{id}
	if id == 0 {
		// Archive subject row (id=0) is shared per archive.
	} else {
		q, args = addUIDFilterNullableGlobal(q, args, uid)
	}
	var (
		out      model.ContactDetail
		em       sql.NullString
		altNames sql.NullString
	)
	err := r.pool.QueryRowContext(ctx, q, args...).Scan(&out.ID, &out.Name, &em, &altNames)
	if err != nil {
		if isNoRows(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("getContact: %w", err)
	}
	if em.Valid {
		s := em.String
		out.Email = &s
	}
	if altNames.Valid {
		s := altNames.String
		out.AlternativeNames = &s
	}
	return &out, nil
}

const maxOwnerContactTokens = 32

func ownerContactMatchTokens(subjectName, familyName string, otherNames, emails *string) []string {
	seen := make(map[string]struct{})
	var out []string
	add := func(s string) {
		t := strings.ToLower(strings.TrimSpace(s))
		if len(t) < 2 {
			return
		}
		if _, ok := seen[t]; ok {
			return
		}
		seen[t] = struct{}{}
		out = append(out, t)
	}
	add(subjectName)
	add(familyName)
	full := strings.TrimSpace(strings.Join([]string{strings.TrimSpace(subjectName), strings.TrimSpace(familyName)}, " "))
	add(full)
	if otherNames != nil {
		for _, p := range strings.Split(*otherNames, ",") {
			add(p)
			for _, w := range strings.Fields(p) {
				add(w)
			}
		}
	}
	if emails != nil {
		for _, p := range strings.Split(*emails, ",") {
			add(p)
			if i := strings.IndexByte(p, '@'); i > 0 {
				add(p[:i])
			}
		}
	}
	if len(out) > maxOwnerContactTokens {
		out = out[:maxOwnerContactTokens]
	}
	return out
}

// ListOwnerContactSuggestions returns contacts whose name or email likely match the subject profile.
func (r *ContactRepo) ListOwnerContactSuggestions(ctx context.Context, subjectName, familyName string, otherNames, emails *string) ([]model.OwnerContactSuggestion, error) {
	tokens := ownerContactMatchTokens(subjectName, familyName, otherNames, emails)
	if len(tokens) == 0 {
		return []model.OwnerContactSuggestion{}, nil
	}
	var orConds []string
	var args []any
	for _, tok := range tokens {
		pat := "%" + tok + "%"
		orConds = append(orConds, "(LOWER(COALESCE(name,'')) LIKE ? OR LOWER(COALESCE(email,'')) LIKE ?)")
		args = append(args, pat, pat)
	}
	q := `SELECT DISTINCT id, name, email FROM contacts WHERE (` + strings.Join(orConds, " OR ") + `)`
	q, args = addUIDFilterNullableGlobal(q, args, uidFromCtx(ctx))
	q += ` ORDER BY name COLLATE NOCASE LIMIT 200`

	rows, err := r.pool.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("listOwnerContactSuggestions: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []model.OwnerContactSuggestion
	for rows.Next() {
		var id int64
		var name string
		var em sql.NullString
		if err := rows.Scan(&id, &name, &em); err != nil {
			return nil, err
		}
		row := model.OwnerContactSuggestion{ID: id, Name: name}
		if em.Valid {
			s := em.String
			row.Email = &s
		}
		if row.Email != nil && strings.Contains(*row.Email, "\\") {
			continue
		}
		out = append(out, row)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if out == nil {
		out = []model.OwnerContactSuggestion{}
	}
	return out, nil
}

// GetByName returns the first contact matching name (for classification update).
func (r *ContactRepo) GetByName(ctx context.Context, name string) (*struct {
	ID      int64
	RelType *string
}, error) {
	uid := uidFromCtx(ctx)
	q := `SELECT id, rel_type FROM contacts WHERE name = ?1`
	args := []any{name}
	q, args = addUIDFilter(q, args, uid)
	q += " LIMIT 1"
	var c struct {
		ID      int64
		RelType *string
	}
	err := r.pool.QueryRowContext(ctx, q, args...).
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
	uid := uidFromCtx(ctx)
	q := `UPDATE contacts SET rel_type=?1, updated_at=CURRENT_TIMESTAMP WHERE id=?2`
	args := []any{relType, id}
	q, args = addUIDFilter(q, args, uid)
	_, err := r.pool.ExecContext(ctx, q, args...)
	return err
}

// Delete removes a contact. Returns false if not found.
func (r *ContactRepo) Delete(ctx context.Context, id int64) (bool, error) {
	uid := uidFromCtx(ctx)
	q := `DELETE FROM contacts WHERE id = ?1`
	args := []any{id}
	q, args = addUIDFilter(q, args, uid)
	tag, err := r.pool.ExecContext(ctx, q, args...)
	if err != nil {
		return false, err
	}
	return rowsAffectedOrZero(tag) > 0, nil
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
	uid := uidFromCtx(ctx)
	// Validate types
	var validT []string
	for _, t := range types {
		if validRelTypes[t] {
			validT = append(validT, t)
		}
	}
	if len(validT) == 0 {
		validT = []string{"friend", "acquaintance", "unknown"}
	}
	typeClause, typeArgs, _ := sqlutil.StringIN("rel_type", validT, 1)

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

	args := typeArgs
	uidCondSubject := ""
	uidCondContacts := ""
	if uid > 0 {
		args = append(args, uid)
		uidCondSubject = fmt.Sprintf(" AND user_id = ?%d", len(args))
		args = append(args, uid)
		uidCondContacts = fmt.Sprintf(" AND user_id = ?%d", len(args))
	}

	// Always include the archive subject (id = 0); apply max_nodes only to other contacts.
	q := fmt.Sprintf(`
		SELECT id, name, rel_type, numemails, numimessages, numfacebook, numwhatsapp, numsms, numinstagram,
		       (%[1]s) AS total
		FROM contacts
		WHERE id = 0%[2]s
		UNION ALL
		SELECT id, name, rel_type, numemails, numimessages, numfacebook, numwhatsapp, numsms, numinstagram,
		       (%[1]s) AS total
		FROM (
		    SELECT id, name, rel_type, numemails, numimessages, numfacebook, numwhatsapp, numsms, numinstagram,
		           (%[1]s) AS total
		    FROM contacts
		    WHERE id != 0
		      AND (%[3]s)
		      AND (%[4]s)
		      AND ((%[1]s) > 3)
		      %[5]s
		    ORDER BY total DESC
		    LIMIT %[6]d
		)`, sumClause, uidCondSubject, typeClause, sourceClause, uidCondContacts, maxNodes)

	rows, err := r.pool.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("getRelationshipGraph: %w", err)
	}
	defer func() { _ = rows.Close() }()

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
	uid := uidFromCtx(ctx)
	q := `SELECT id, primary_name, email, created_at, updated_at FROM email_matches WHERE TRUE`
	args := []any{}
	if primaryName != "" {
		args = append(args, "%"+primaryName+"%")
		q += fmt.Sprintf(" AND primary_name LIKE ?%d", len(args))
	}
	q, args = addUIDFilterNullableGlobal(q, args, uid)
	q += " ORDER BY primary_name, email"
	rows, err := r.pool.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
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
	uid := uidFromCtx(ctx)
	q := `SELECT id, primary_name, email, created_at, updated_at FROM email_matches WHERE id=?1`
	args := []any{id}
	q, args = addUIDFilterNullableGlobal(q, args, uid)
	m, err := scanEmailMatch(r.pool.QueryRowContext(ctx, q, args...))
	if err != nil {
		if isNoRows(err) {
			return nil, nil
		}
		return nil, err
	}
	return m, nil
}

func (r *ContactRepo) EmailMatchExists(ctx context.Context, primaryName, email string) (bool, error) {
	uid := uidFromCtx(ctx)
	q := `SELECT COUNT(*) FROM email_matches WHERE primary_name=?1 AND email=?2`
	args := []any{primaryName, email}
	q, args = addUIDFilterNullableGlobal(q, args, uid)
	var n int
	err := r.pool.QueryRowContext(ctx, q, args...).Scan(&n)
	return n > 0, err
}

func (r *ContactRepo) CreateEmailMatch(ctx context.Context, primaryName, email string) (*model.EmailMatch, error) {
	uid := uidFromCtx(ctx)
	m, err := scanEmailMatch(r.pool.QueryRowContext(ctx,
		`INSERT INTO email_matches (primary_name, email, user_id) VALUES (?1,?2,?3)
		 RETURNING id, primary_name, email, created_at, updated_at`, primaryName, email, uidVal(uid)))
	if err != nil {
		return nil, fmt.Errorf("createEmailMatch: %w", err)
	}
	return m, nil
}

func (r *ContactRepo) UpdateEmailMatch(ctx context.Context, id int64, primaryName, email *string) (*model.EmailMatch, error) {
	uid := uidFromCtx(ctx)
	q := `UPDATE email_matches SET
	      primary_name = COALESCE(?1, primary_name),
	      email        = COALESCE(?2, email),
	      updated_at   = CURRENT_TIMESTAMP
	      WHERE id=?3`
	args := []any{primaryName, email, id}
	q, args = addUIDFilter(q, args, uid)
	q += ` RETURNING id, primary_name, email, created_at, updated_at`
	m, err := scanEmailMatch(r.pool.QueryRowContext(ctx, q, args...))
	if err != nil {
		if isNoRows(err) {
			return nil, nil
		}
		return nil, err
	}
	return m, nil
}

func (r *ContactRepo) DeleteEmailMatch(ctx context.Context, id int64) (bool, error) {
	uid := uidFromCtx(ctx)
	q := `DELETE FROM email_matches WHERE id=?1`
	args := []any{id}
	q, args = addUIDFilter(q, args, uid)
	tag, err := r.pool.ExecContext(ctx, q, args...)
	if err != nil {
		return false, err
	}
	return rowsAffectedOrZero(tag) > 0, nil
}

// ── Email exclusions ──────────────────────────────────────────────────────────

func scanEmailExclusion(row interface{ Scan(...any) error }) (*model.EmailExclusion, error) {
	var e model.EmailExclusion
	err := row.Scan(&e.ID, &e.Email, &e.Name, &e.NameEmail, &e.CreatedAt, &e.UpdatedAt)
	return &e, err
}

func (r *ContactRepo) ListEmailExclusions(ctx context.Context, search string, nameEmail *bool) ([]*model.EmailExclusion, error) {
	uid := uidFromCtx(ctx)
	var args []any
	var conds []string
	if search != "" {
		args = append(args, "%"+search+"%")
		idx := len(args)
		conds = append(conds, fmt.Sprintf("(email LIKE ?%d OR name LIKE ?%d)", idx, idx))
	}
	if nameEmail != nil {
		args = append(args, *nameEmail)
		conds = append(conds, fmt.Sprintf("name_email = ?%d", len(args)))
	}
	q := `SELECT id, email, name, name_email, created_at, updated_at FROM email_exclusions WHERE TRUE`
	if len(conds) > 0 {
		q += " AND " + joinAnd(conds)
	}
	q, args = addUIDFilterNullableGlobal(q, args, uid)
	q += " ORDER BY email, name"
	rows, err := r.pool.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
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
	uid := uidFromCtx(ctx)
	q := `SELECT id, email, name, name_email, created_at, updated_at FROM email_exclusions WHERE id=?1`
	args := []any{id}
	q, args = addUIDFilterNullableGlobal(q, args, uid)
	e, err := scanEmailExclusion(r.pool.QueryRowContext(ctx, q, args...))
	if err != nil {
		if isNoRows(err) {
			return nil, nil
		}
		return nil, err
	}
	return e, nil
}

func (r *ContactRepo) ExclusionExists(ctx context.Context, email, name string, nameEmail bool) (bool, error) {
	uid := uidFromCtx(ctx)
	q := `SELECT COUNT(*) FROM email_exclusions WHERE email=?1 AND name=?2 AND name_email=?3`
	args := []any{email, name, nameEmail}
	q, args = addUIDFilterNullableGlobal(q, args, uid)
	var n int
	err := r.pool.QueryRowContext(ctx, q, args...).Scan(&n)
	return n > 0, err
}

func (r *ContactRepo) CreateEmailExclusion(ctx context.Context, email, name string, nameEmail bool) (*model.EmailExclusion, error) {
	uid := uidFromCtx(ctx)
	e, err := scanEmailExclusion(r.pool.QueryRowContext(ctx,
		`INSERT INTO email_exclusions (email, name, name_email, user_id) VALUES (?1,?2,?3,?4)
		 RETURNING id, email, name, name_email, created_at, updated_at`, email, name, nameEmail, uidVal(uid)))
	if err != nil {
		return nil, fmt.Errorf("createEmailExclusion: %w", err)
	}
	return e, nil
}

func (r *ContactRepo) UpdateEmailExclusion(ctx context.Context, id int64, email, name *string, nameEmail *bool) (*model.EmailExclusion, error) {
	uid := uidFromCtx(ctx)
	q := `UPDATE email_exclusions SET
	      email      = COALESCE(?1, email),
	      name       = COALESCE(?2, name),
	      name_email = COALESCE(?3, name_email),
	      updated_at = CURRENT_TIMESTAMP
	      WHERE id=?4`
	args := []any{email, name, nameEmail, id}
	q, args = addUIDFilter(q, args, uid)
	q += ` RETURNING id, email, name, name_email, created_at, updated_at`
	e, err := scanEmailExclusion(r.pool.QueryRowContext(ctx, q, args...))
	if err != nil {
		if isNoRows(err) {
			return nil, nil
		}
		return nil, err
	}
	return e, nil
}

func (r *ContactRepo) DeleteEmailExclusion(ctx context.Context, id int64) (bool, error) {
	uid := uidFromCtx(ctx)
	q := `DELETE FROM email_exclusions WHERE id=?1`
	args := []any{id}
	q, args = addUIDFilter(q, args, uid)
	tag, err := r.pool.ExecContext(ctx, q, args...)
	if err != nil {
		return false, err
	}
	return rowsAffectedOrZero(tag) > 0, nil
}

// ── Email classifications ──────────────────────────────────────────────────────

func scanEmailClassification(row interface{ Scan(...any) error }) (*model.EmailClassification, error) {
	var c model.EmailClassification
	err := row.Scan(&c.ID, &c.Name, &c.Classification, &c.CreatedAt, &c.UpdatedAt)
	return &c, err
}

func (r *ContactRepo) ListEmailClassifications(ctx context.Context, name, classification string) ([]*model.EmailClassification, error) {
	uid := uidFromCtx(ctx)
	var args []any
	var conds []string
	if name != "" {
		args = append(args, "%"+name+"%")
		conds = append(conds, fmt.Sprintf("name LIKE ?%d", len(args)))
	}
	if classification != "" && validRelTypes[classification] {
		args = append(args, classification)
		conds = append(conds, fmt.Sprintf("classification = ?%d", len(args)))
	}
	q := `SELECT id, name, classification, created_at, updated_at FROM email_classifications WHERE TRUE`
	if len(conds) > 0 {
		q += " AND " + joinAnd(conds)
	}
	q, args = addUIDFilterNullableGlobal(q, args, uid)
	q += " ORDER BY classification, name"
	rows, err := r.pool.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
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
	uid := uidFromCtx(ctx)
	q := `SELECT id, name, classification, created_at, updated_at FROM email_classifications WHERE id=?1`
	args := []any{id}
	q, args = addUIDFilterNullableGlobal(q, args, uid)
	c, err := scanEmailClassification(r.pool.QueryRowContext(ctx, q, args...))
	if err != nil {
		if isNoRows(err) {
			return nil, nil
		}
		return nil, err
	}
	return c, nil
}

func (r *ContactRepo) ClassificationExists(ctx context.Context, name, classification string) (bool, error) {
	uid := uidFromCtx(ctx)
	q := `SELECT COUNT(*) FROM email_classifications WHERE name=?1 AND classification=?2`
	args := []any{name, classification}
	q, args = addUIDFilterNullableGlobal(q, args, uid)
	var n int
	err := r.pool.QueryRowContext(ctx, q, args...).Scan(&n)
	return n > 0, err
}

func (r *ContactRepo) CreateEmailClassification(ctx context.Context, name, classification string) (*model.EmailClassification, error) {
	uid := uidFromCtx(ctx)
	c, err := scanEmailClassification(r.pool.QueryRowContext(ctx,
		`INSERT INTO email_classifications (name, classification, user_id) VALUES (?1,?2,?3)
		 RETURNING id, name, classification, created_at, updated_at`, name, classification, uidVal(uid)))
	if err != nil {
		return nil, fmt.Errorf("createEmailClassification: %w", err)
	}
	return c, nil
}

func (r *ContactRepo) UpdateEmailClassification(ctx context.Context, id int64, name, classification *string) (*model.EmailClassification, error) {
	uid := uidFromCtx(ctx)
	q := `UPDATE email_classifications SET
	      name           = COALESCE(?1, name),
	      classification = COALESCE(?2, classification),
	      updated_at     = CURRENT_TIMESTAMP
	      WHERE id=?3`
	args := []any{name, classification, id}
	q, args = addUIDFilter(q, args, uid)
	q += ` RETURNING id, name, classification, created_at, updated_at`
	c, err := scanEmailClassification(r.pool.QueryRowContext(ctx, q, args...))
	if err != nil {
		if isNoRows(err) {
			return nil, nil
		}
		return nil, err
	}
	return c, nil
}

func (r *ContactRepo) DeleteEmailClassification(ctx context.Context, id int64) (bool, error) {
	uid := uidFromCtx(ctx)
	q := `DELETE FROM email_classifications WHERE id=?1`
	args := []any{id}
	q, args = addUIDFilter(q, args, uid)
	tag, err := r.pool.ExecContext(ctx, q, args...)
	if err != nil {
		return false, err
	}
	return rowsAffectedOrZero(tag) > 0, nil
}

// GetClassificationByNameLower returns a classification row matching name (case-insensitive).
func (r *ContactRepo) GetClassificationByNameLower(ctx context.Context, name string) (*model.EmailClassification, error) {
	uid := uidFromCtx(ctx)
	q := `SELECT id, name, classification, created_at, updated_at FROM email_classifications WHERE LOWER(name)=LOWER(?1)`
	args := []any{name}
	q, args = addUIDFilterNullableGlobal(q, args, uid)
	q += " LIMIT 1"
	c, err := scanEmailClassification(r.pool.QueryRowContext(ctx, q, args...))
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
	uid := uidFromCtx(ctx)
	q := `UPDATE contacts SET rel_type=?1, updated_at=CURRENT_TIMESTAMP
	      WHERE id != 0 AND (
	          LOWER(name) = LOWER(?2)
	          OR LOWER(alternative_names) LIKE '%' || LOWER(?2) || '%'
	      )`
	args := []any{classification, name}
	q, args = addUIDFilter(q, args, uid)
	_, err := r.pool.ExecContext(ctx, q, args...)
	return err
}
