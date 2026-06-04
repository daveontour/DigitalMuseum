package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/daveontour/aimuseum/internal/model"
	"github.com/daveontour/aimuseum/internal/repository"
)

// ErrSubjectContactNotFound is returned when subject_contact_id does not match a contact for the user.
var ErrSubjectContactNotFound = errors.New("subject_contact_id does not match a contact in this archive")

// ErrNoSubjectConfiguration is returned when there is no subject_configuration row for the current user.
var ErrNoSubjectConfiguration = errors.New("subject configuration not found")

// SubjectConfigService handles GET /api/subject-configuration.
type SubjectConfigService struct {
	repo     *repository.SubjectConfigRepo
	appInstr *repository.AppSystemInstructionsRepo
	contacts *repository.ContactRepo
}

// NewSubjectConfigService creates a SubjectConfigService.
func NewSubjectConfigService(repo *repository.SubjectConfigRepo, appInstr *repository.AppSystemInstructionsRepo, contacts *repository.ContactRepo) *SubjectConfigService {
	return &SubjectConfigService{repo: repo, appInstr: appInstr, contacts: contacts}
}

// SubjectConfigUpdateParams mirrors the POST body fields for subject_configuration only.
type SubjectConfigUpdateParams struct {
	SubjectName         string
	Gender              *string
	FamilyName          *string
	OtherNames          *string
	EmailAddresses      *string
	PhoneNumbers        *string
	WhatsAppHandle      *string
	InstagramHandle     *string
	SubjectContactIDSet bool
	SubjectContactID    sql.NullInt64
}

// CreateOrUpdate upserts the singleton subject configuration row.
func (s *SubjectConfigService) CreateOrUpdate(ctx context.Context, p SubjectConfigUpdateParams) (*model.SubjectConfigResponse, error) {
	if p.SubjectContactIDSet && p.SubjectContactID.Valid && s.contacts != nil {
		ok, err := s.contacts.ContactExistsForUser(ctx, p.SubjectContactID.Int64)
		if err != nil {
			return nil, err
		}
		if !ok {
			return nil, ErrSubjectContactNotFound
		}
	}
	cfg, err := s.repo.Upsert(ctx, repository.UpsertSubjectConfigParams{
		SubjectName:         p.SubjectName,
		Gender:              p.Gender,
		FamilyName:          p.FamilyName,
		OtherNames:          p.OtherNames,
		EmailAddresses:      p.EmailAddresses,
		PhoneNumbers:        p.PhoneNumbers,
		WhatsAppHandle:      p.WhatsAppHandle,
		InstagramHandle:     p.InstagramHandle,
		SubjectContactIDSet: p.SubjectContactIDSet,
		SubjectContactID:    p.SubjectContactID,
	})
	if err != nil {
		return nil, err
	}
	return s.mergedResponse(ctx, cfg)
}

// GetConfiguration returns the singleton subject configuration merged with universal system instructions.
func (s *SubjectConfigService) GetConfiguration(ctx context.Context) (*model.SubjectConfigResponse, error) {
	cfg, err := s.repo.GetFirst(ctx)
	if err != nil {
		return nil, err
	}
	if cfg == nil {
		return nil, nil
	}
	return s.mergedResponse(ctx, cfg)
}

func (s *SubjectConfigService) mergedResponse(ctx context.Context, cfg *model.SubjectConfig) (*model.SubjectConfigResponse, error) {
	resp := toSubjectConfigResponse(cfg)
	if s.contacts != nil {
		if cfg.SubjectContactID.Valid {
			if linked, err := s.contacts.GetContact(ctx, cfg.SubjectContactID.Int64); err != nil {
				return nil, err
			} else if linked != nil {
				row := model.OwnerContactSuggestion{ID: linked.ID, Name: linked.Name}
				if linked.Email != nil {
					row.Email = linked.Email
				}
				resp.LinkedOwnerContact = &row
			}
		}
		sug, err := s.contacts.ListOwnerContactSuggestions(ctx, cfg.SubjectName, strPtr(cfg.FamilyName), cfg.OtherNames, cfg.EmailAddresses)
		if err != nil {
			return nil, err
		}
		resp.OwnerContactSuggestions = sug
	} else {
		resp.OwnerContactSuggestions = []model.OwnerContactSuggestion{}
	}
	if s.appInstr == nil {
		return resp, nil
	}
	ins, err := s.appInstr.Get(ctx)
	if err != nil {
		return nil, err
	}
	if ins != nil {
		resp.SystemInstructions = ins.ChatInstructions
		resp.CoreSystemInstructions = ins.CoreInstructions
		resp.QuestionSystemInstructions = ins.QuestionInstructions
	}
	return resp, nil
}

// AppSystemInstructionsUpdate holds the three universal prompt bodies.
type AppSystemInstructionsUpdate struct {
	ChatInstructions     string
	CoreInstructions     string
	QuestionInstructions string
}

// UpdateWritingStyleAIText persists the writing style summary (Markdown/plain text).
func (s *SubjectConfigService) UpdateWritingStyleAIText(ctx context.Context, text string) (*model.SubjectConfigResponse, error) {
	cfg, err := s.repo.GetFirst(ctx)
	if err != nil {
		return nil, err
	}
	if cfg == nil {
		return nil, ErrNoSubjectConfiguration
	}
	if err := s.repo.UpdateWritingStyleAI(ctx, text); err != nil {
		return nil, err
	}
	return s.GetConfiguration(ctx)
}

// UpdatePsychologicalProfileAIText persists the psychological profile (Markdown/plain text).
func (s *SubjectConfigService) UpdatePsychologicalProfileAIText(ctx context.Context, text string) (*model.SubjectConfigResponse, error) {
	cfg, err := s.repo.GetFirst(ctx)
	if err != nil {
		return nil, err
	}
	if cfg == nil {
		return nil, ErrNoSubjectConfiguration
	}
	if err := s.repo.UpdatePsychologicalProfileAI(ctx, text); err != nil {
		return nil, err
	}
	return s.GetConfiguration(ctx)
}

// UpdateAppSystemInstructions replaces the singleton instruction row.
func (s *SubjectConfigService) UpdateAppSystemInstructions(ctx context.Context, p AppSystemInstructionsUpdate) error {
	if s.appInstr == nil {
		return fmt.Errorf("app system instructions repository not configured")
	}
	return s.appInstr.Upsert(ctx, p.ChatInstructions, p.CoreInstructions, p.QuestionInstructions)
}

func strPtr(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

func toSubjectConfigResponse(cfg *model.SubjectConfig) *model.SubjectConfigResponse {
	resp := &model.SubjectConfigResponse{
		ID:                     cfg.ID,
		SubjectName:            cfg.SubjectName,
		Gender:                 cfg.Gender,
		FamilyName:             cfg.FamilyName,
		OtherNames:             cfg.OtherNames,
		EmailAddresses:         cfg.EmailAddresses,
		PhoneNumbers:           cfg.PhoneNumbers,
		WhatsAppHandle:         cfg.WhatsAppHandle,
		InstagramHandle:        cfg.InstagramHandle,
		WritingStyleAI:         cfg.WritingStyleAI,
		PsychologicalProfileAI: cfg.PsychologicalProfileAI,
		CreatedAt:              isoString(ptrTime(cfg.CreatedAt.Time)),
		UpdatedAt:              isoString(ptrTime(cfg.UpdatedAt.Time)),
	}
	if cfg.SubjectContactID.Valid {
		v := cfg.SubjectContactID.Int64
		resp.SubjectContactID = &v
	}
	return resp
}

// ptrTime converts a value time.Time to *time.Time so isoString can accept it.
func ptrTime(t time.Time) *time.Time {
	return &t
}
