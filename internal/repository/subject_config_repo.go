package repository

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/daveontour/aimuseum/internal/model"
)

// SubjectConfigRepo reads the singleton subject_configuration table.
type SubjectConfigRepo struct {
	pool *sql.DB
}

// NewSubjectConfigRepo creates a SubjectConfigRepo.
func NewSubjectConfigRepo(pool *sql.DB) *SubjectConfigRepo {
	return &SubjectConfigRepo{pool: pool}
}

// UpdateWritingStyleAI sets the writing_style_ai field on the first config row.
func (r *SubjectConfigRepo) UpdateWritingStyleAI(ctx context.Context, summary string) error {
	uid := uidFromCtx(ctx)
	q := `UPDATE subject_configuration SET writing_style_ai = ?, updated_at = CURRENT_TIMESTAMP
	      WHERE id = (SELECT id FROM subject_configuration WHERE TRUE`
	args := []any{summary}
	if uid > 0 {
		args = append(args, uid)
		q += " AND user_id = ?"
	}
	q += " LIMIT 1)"
	_, err := r.pool.ExecContext(ctx, q, args...)
	return err
}

// UpdatePsychologicalProfileAI sets the psychological_profile_ai field on the first config row.
func (r *SubjectConfigRepo) UpdatePsychologicalProfileAI(ctx context.Context, profile string) error {
	uid := uidFromCtx(ctx)
	q := `UPDATE subject_configuration SET psychological_profile_ai = ?, updated_at = CURRENT_TIMESTAMP
	      WHERE id = (SELECT id FROM subject_configuration WHERE TRUE`
	args := []any{profile}
	if uid > 0 {
		args = append(args, uid)
		q += " AND user_id = ?"
	}
	q += " LIMIT 1)"
	_, err := r.pool.ExecContext(ctx, q, args...)
	return err
}

// UpsertParams holds the fields the caller wants to write.
type UpsertSubjectConfigParams struct {
	SubjectName         string
	Gender              *string // defaults to "Male" if nil
	FamilyName          *string
	OtherNames          *string
	EmailAddresses      *string
	PhoneNumbers        *string
	WhatsAppHandle      *string
	InstagramHandle     *string
	SubjectContactIDSet bool
	SubjectContactID    sql.NullInt64
}

// Upsert creates the subject configuration row if it doesn't exist, or updates
// the existing singleton row in-place.
func (r *SubjectConfigRepo) Upsert(ctx context.Context, p UpsertSubjectConfigParams) (*model.SubjectConfig, error) {
	uid := uidFromCtx(ctx)
	gender := "Male"
	if p.Gender != nil && *p.Gender != "" {
		gender = *p.Gender
	}

	// Check whether a row already exists (scoped by uid when uid > 0; include legacy NULL user_id row).
	q := `SELECT id FROM subject_configuration WHERE TRUE`
	args := []any{}
	if uid > 0 {
		args = append(args, uid)
		q += " AND (user_id = ? OR user_id IS NULL)"
	}
	q += " ORDER BY CASE WHEN user_id IS NULL THEN 1 ELSE 0 END, id ASC LIMIT 1"
	var id int64
	err := r.pool.QueryRowContext(ctx, q, args...).Scan(&id)
	noRow := isNoRows(err)
	if err != nil && !noRow {
		return nil, fmt.Errorf("upsert check: %w", err)
	}

	if noRow {
		var insSubjContact any
		if p.SubjectContactIDSet {
			if p.SubjectContactID.Valid {
				insSubjContact = p.SubjectContactID.Int64
			} else {
				insSubjContact = nil
			}
		} else {
			insSubjContact = nil
		}
		err = r.pool.QueryRowContext(ctx, `
			INSERT INTO subject_configuration
				(subject_name, gender,
				 family_name, other_names, email_addresses, phone_numbers,
				 whatsapp_handle, instagram_handle, subject_contact_id, user_id)
			VALUES (?,?,?,?,?,?,?,?,?,?)
			RETURNING id`,
			p.SubjectName, gender,
			p.FamilyName, p.OtherNames, p.EmailAddresses,
			p.PhoneNumbers, p.WhatsAppHandle, p.InstagramHandle, insSubjContact, uidVal(uid),
		).Scan(&id)
		if err != nil {
			return nil, fmt.Errorf("upsert insert: %w", err)
		}
	} else {
		// Always update the required fields; only overwrite optional fields when provided.
		set := []string{"subject_name = ?", "gender = ?", "updated_at = CURRENT_TIMESTAMP"}
		args := []any{p.SubjectName, gender}
		if uid > 0 {
			set = append(set, "user_id = ?")
			args = append(args, uid)
		}

		addOpt := func(col string, v *string) {
			if v != nil {
				set = append(set, col+" = ?")
				args = append(args, *v)
			}
		}
		addOpt("family_name", p.FamilyName)
		addOpt("other_names", p.OtherNames)
		addOpt("email_addresses", p.EmailAddresses)
		addOpt("phone_numbers", p.PhoneNumbers)
		addOpt("whatsapp_handle", p.WhatsAppHandle)
		addOpt("instagram_handle", p.InstagramHandle)
		if p.SubjectContactIDSet {
			if p.SubjectContactID.Valid {
				set = append(set, "subject_contact_id = ?")
				args = append(args, p.SubjectContactID.Int64)
			} else {
				set = append(set, "subject_contact_id = NULL")
			}
		}

		args = append(args, id)
		_, err = r.pool.ExecContext(ctx,
			fmt.Sprintf(`UPDATE subject_configuration SET %s WHERE id = ?`, strings.Join(set, ", ")),
			args...)
		if err != nil {
			return nil, fmt.Errorf("upsert update: %w", err)
		}
	}

	return r.GetFirst(ctx)
}

// GetFirst returns the first (and only) subject configuration row.
// Returns nil, nil if no row exists yet.
func (r *SubjectConfigRepo) GetFirst(ctx context.Context) (*model.SubjectConfig, error) {
	uid := uidFromCtx(ctx)
	q := `
		SELECT id, subject_name, gender, family_name, other_names,
		       email_addresses, phone_numbers, whatsapp_handle, instagram_handle,
		       subject_contact_id,
		       writing_style_ai, psychological_profile_ai,
		       created_at, updated_at
		FROM subject_configuration
		WHERE TRUE`
	args := []any{}
	if uid > 0 {
		args = append(args, uid)
		q += " AND (user_id = ? OR user_id IS NULL)"
	}
	q += " ORDER BY CASE WHEN user_id IS NULL THEN 1 ELSE 0 END, id ASC LIMIT 1"

	row := r.pool.QueryRowContext(ctx, q, args...)

	cfg := &model.SubjectConfig{}
	err := row.Scan(
		&cfg.ID, &cfg.SubjectName, &cfg.Gender, &cfg.FamilyName, &cfg.OtherNames,
		&cfg.EmailAddresses, &cfg.PhoneNumbers, &cfg.WhatsAppHandle, &cfg.InstagramHandle,
		&cfg.SubjectContactID,
		&cfg.WritingStyleAI, &cfg.PsychologicalProfileAI,
		&cfg.CreatedAt, &cfg.UpdatedAt,
	)
	if err != nil {
		if isNoRows(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("getFirst subject_configuration: %w", err)
	}
	return cfg, nil
}

// GetContactID returns subject_contact_id for the current user's subject_configuration row.
// Returns (0, nil) when there is no row or subject_contact_id is unset.
func (r *SubjectConfigRepo) GetContactID(ctx context.Context) (int64, error) {
	uid := uidFromCtx(ctx)
	q := `SELECT subject_contact_id FROM subject_configuration WHERE TRUE`
	args := []any{}
	if uid > 0 {
		args = append(args, uid)
		q += " AND (user_id = ? OR user_id IS NULL)"
	}
	q += " ORDER BY CASE WHEN user_id IS NULL THEN 1 ELSE 0 END, id ASC LIMIT 1"
	var nid sql.NullInt64
	err := r.pool.QueryRowContext(ctx, q, args...).Scan(&nid)
	if err != nil {
		if isNoRows(err) {
			return 0, nil
		}
		return 0, fmt.Errorf("getContactID: %w", err)
	}
	if !nid.Valid {
		return 0, nil
	}
	return nid.Int64, nil
}

// FindUserIDBySubjectName returns (userID, found, error) for the archive whose
// subject_name matches case-insensitively.
// found=false means no row matched. found=true with userID=0 means the row exists
// but has no user_id (legacy single-tenant row).
func (r *SubjectConfigRepo) FindUserIDBySubjectName(ctx context.Context, name string) (int64, bool, error) {
	var uid sql.NullInt64
	err := r.pool.QueryRowContext(ctx,
		`SELECT user_id FROM subject_configuration WHERE LOWER(subject_name) = LOWER(?) LIMIT 1`,
		name).Scan(&uid)
	if isNoRows(err) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, fmt.Errorf("findUserIDBySubjectName: %w", err)
	}
	if !uid.Valid {
		return 0, true, nil // single-tenant/legacy row
	}
	return uid.Int64, true, nil
}
