package model

import (
	"strings"

	"github.com/daveontour/aimuseum/internal/sqlutil"
)

// Interview is a row from the interviews table.
type Interview struct {
	ID            int64   `json:"id"`
	Title         string  `json:"title"`
	Style         string  `json:"style"`
	Purpose       string  `json:"purpose"`
	PurposeDetail string  `json:"purpose_detail,omitempty"`
	State         string  `json:"state"`
	Provider      string  `json:"provider,omitempty"`
	Writeup       *string `json:"writeup,omitempty"`
	// HasWriteup is true when a non-empty writeup exists (set on list and detail responses).
	HasWriteup bool               `json:"has_writeup"`
	CreatedAt  sqlutil.DBTime     `json:"created_at"`
	UpdatedAt  sqlutil.DBTime     `json:"updated_at"`
	LastTurnAt sqlutil.NullDBTime `json:"last_turn_at,omitempty"`
	TurnCount  int                `json:"turn_count,omitempty"`
}

// SetHasWriteupFromWriteup sets HasWriteup from the Writeup field (for detail loads).
func (iv *Interview) SetHasWriteupFromWriteup() {
	if iv == nil {
		return
	}
	if iv.Writeup == nil || strings.TrimSpace(*iv.Writeup) == "" {
		iv.HasWriteup = false
		return
	}
	iv.HasWriteup = true
}

// InterviewTurn is a row from the interview_turns table.
type InterviewTurn struct {
	ID          int64          `json:"id"`
	InterviewID int64          `json:"interview_id"`
	Question    string         `json:"question"`
	Answer      *string        `json:"answer,omitempty"`
	TurnNumber  int            `json:"turn_number"`
	CreatedAt   sqlutil.DBTime `json:"created_at"`
}

// StartInterviewRequest is the JSON body for POST /interview/start.
type StartInterviewRequest struct {
	Style         string `json:"style"`
	Purpose       string `json:"purpose"`
	PurposeDetail string `json:"purpose_detail"`
	Provider      string `json:"provider"`
}

// InterviewTurnRequest is the JSON body for POST /interview/turn.
type InterviewTurnRequest struct {
	InterviewID int64  `json:"interview_id"`
	Answer      string `json:"answer"`
}

// InterviewTurnResponse is the JSON response for interview turn endpoints.
type InterviewTurnResponse struct {
	InterviewID    int64  `json:"interview_id"`
	Question       string `json:"question"`
	TurnNumber     int    `json:"turn_number"`
	InterviewState string `json:"interview_state"`
}

// InterviewActionRequest is the JSON body for POST /interview/pause and /interview/resume.
type InterviewActionRequest struct {
	InterviewID int64 `json:"interview_id"`
}

// EndInterviewResponse is the JSON response for POST /interview/end.
type EndInterviewResponse struct {
	Status  string `json:"status"`
	Writeup string `json:"writeup"`
}

// InterviewDetailResponse is the JSON response for GET /interview/{id}.
type InterviewDetailResponse struct {
	Interview *Interview       `json:"interview"`
	Turns     []*InterviewTurn `json:"turns"`
}

// InterviewListResponse is the JSON response for GET /interview/list.
type InterviewListResponse struct {
	Interviews []*Interview `json:"interviews"`
}
