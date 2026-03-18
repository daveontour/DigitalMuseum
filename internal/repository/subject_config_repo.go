package repository

import (
	"context"
	"fmt"
	"strings"

	"github.com/daveontour/digitalmuseum/internal/model"
	"github.com/jackc/pgx/v5/pgxpool"
)

// SubjectConfigRepo reads the singleton subject_configuration table.
type SubjectConfigRepo struct {
	pool *pgxpool.Pool
}

// NewSubjectConfigRepo creates a SubjectConfigRepo.
func NewSubjectConfigRepo(pool *pgxpool.Pool) *SubjectConfigRepo {
	return &SubjectConfigRepo{pool: pool}
}

// UpdateWritingStyleAI sets the writing_style_ai field on the first config row.
func (r *SubjectConfigRepo) UpdateWritingStyleAI(ctx context.Context, summary string) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE subject_configuration SET writing_style_ai = $1, updated_at = NOW()
		 WHERE id = (SELECT id FROM subject_configuration LIMIT 1)`, summary)
	return err
}

// UpdatePsychologicalProfileAI sets the psychological_profile_ai field on the first config row.
func (r *SubjectConfigRepo) UpdatePsychologicalProfileAI(ctx context.Context, profile string) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE subject_configuration SET psychological_profile_ai = $1, updated_at = NOW()
		 WHERE id = (SELECT id FROM subject_configuration LIMIT 1)`, profile)
	return err
}

// UpsertParams holds the fields the caller wants to write.
type UpsertSubjectConfigParams struct {
	SubjectName            string
	SystemInstructions     string
	CoreSystemInstructions *string
	Gender                 *string // defaults to "Male" if nil
	FamilyName             *string
	OtherNames             *string
	EmailAddresses         *string
	PhoneNumbers           *string
	WhatsAppHandle         *string
	InstagramHandle        *string
}

// Upsert creates the subject configuration row if it doesn't exist, or updates
// the existing singleton row in-place.
func (r *SubjectConfigRepo) Upsert(ctx context.Context, p UpsertSubjectConfigParams) (*model.SubjectConfig, error) {
	gender := "Male"
	if p.Gender != nil && *p.Gender != "" {
		gender = *p.Gender
	}

	// Check whether a row already exists.
	var id int64
	err := r.pool.QueryRow(ctx, `SELECT id FROM subject_configuration LIMIT 1`).Scan(&id)
	noRow := isNoRows(err)
	if err != nil && !noRow {
		return nil, fmt.Errorf("Upsert check: %w", err)
	}

	if noRow {
		core := ""
		if p.CoreSystemInstructions != nil {
			core = *p.CoreSystemInstructions
		}
		err = r.pool.QueryRow(ctx, `
			INSERT INTO subject_configuration
				(subject_name, system_instructions, gender, core_system_instructions,
				 family_name, other_names, email_addresses, phone_numbers,
				 whatsapp_handle, instagram_handle)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)
			RETURNING id`,
			p.SubjectName, p.SystemInstructions, gender, core,
			p.FamilyName, p.OtherNames, p.EmailAddresses,
			p.PhoneNumbers, p.WhatsAppHandle, p.InstagramHandle,
		).Scan(&id)
		if err != nil {
			return nil, fmt.Errorf("Upsert insert: %w", err)
		}
	} else {
		// Always update the required fields; only overwrite optional fields when provided.
		set := []string{"subject_name = $1", "system_instructions = $2", "gender = $3", "updated_at = NOW()"}
		args := []any{p.SubjectName, p.SystemInstructions, gender}
		idx := 4

		addOpt := func(col string, v *string) {
			if v != nil {
				set = append(set, fmt.Sprintf("%s = $%d", col, idx))
				args = append(args, *v)
				idx++
			}
		}
		addOpt("core_system_instructions", p.CoreSystemInstructions)
		addOpt("family_name", p.FamilyName)
		addOpt("other_names", p.OtherNames)
		addOpt("email_addresses", p.EmailAddresses)
		addOpt("phone_numbers", p.PhoneNumbers)
		addOpt("whatsapp_handle", p.WhatsAppHandle)
		addOpt("instagram_handle", p.InstagramHandle)

		args = append(args, id)
		_, err = r.pool.Exec(ctx,
			fmt.Sprintf(`UPDATE subject_configuration SET %s WHERE id = $%d`, strings.Join(set, ", "), idx),
			args...)
		if err != nil {
			return nil, fmt.Errorf("Upsert update: %w", err)
		}
	}

	return r.GetFirst(ctx)
}

// GetFirst returns the first (and only) subject configuration row.
// Returns nil, nil if no row exists yet.
func (r *SubjectConfigRepo) GetFirst(ctx context.Context) (*model.SubjectConfig, error) {
	row := r.pool.QueryRow(ctx, `
		SELECT id, subject_name, gender, family_name, other_names,
		       email_addresses, phone_numbers, whatsapp_handle, instagram_handle,
		       writing_style_ai, psychological_profile_ai,
		       system_instructions, core_system_instructions,
		       created_at, updated_at
		FROM subject_configuration
		LIMIT 1`)

	cfg := &model.SubjectConfig{}
	err := row.Scan(
		&cfg.ID, &cfg.SubjectName, &cfg.Gender, &cfg.FamilyName, &cfg.OtherNames,
		&cfg.EmailAddresses, &cfg.PhoneNumbers, &cfg.WhatsAppHandle, &cfg.InstagramHandle,
		&cfg.WritingStyleAI, &cfg.PsychologicalProfileAI,
		&cfg.SystemInstructions, &cfg.CoreSystemInstructions,
		&cfg.CreatedAt, &cfg.UpdatedAt,
	)
	if err != nil {
		if isNoRows(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("GetFirst subject_configuration: %w", err)
	}
	return cfg, nil
}
