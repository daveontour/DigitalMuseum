// Package model contains shared domain types used across handler/service/repository layers.
package model

import "time"

// Email is the domain representation of a row in the emails table.
type Email struct {
	ID             int64
	UID            string
	Folder         string
	Subject        *string
	FromAddress    *string
	ToAddresses    *string
	CCAddresses    *string
	BCCAddresses   *string
	Date           *time.Time
	RawMessage     *string
	PlainText      *string
	Snippet        *string
	Embedding      *string
	HasAttachments bool
	UserDeleted    bool
	IsPersonal     bool
	IsBusiness     bool
	IsSocial       bool
	IsPromotional  bool
	IsSpam         bool
	IsImportant    bool
	UseByAI        bool
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

// EmailMetadataResponse is the JSON shape returned by the metadata, search, and label endpoints.
// It matches the Python EmailMetadataResponse Pydantic model exactly.
type EmailMetadataResponse struct {
	ID            int64      `json:"id"`
	UID           string     `json:"uid"`
	Folder        string     `json:"folder"`
	Subject       *string    `json:"subject"`
	FromAddress   *string    `json:"from_address"`
	ToAddresses   *string    `json:"to_addresses"`
	CCAddresses   *string    `json:"cc_addresses"`
	BCCAddresses  *string    `json:"bcc_addresses"`
	Date          *time.Time `json:"date"`
	Snippet       *string    `json:"snippet"`
	AttachmentIDs []int64    `json:"attachment_ids"`
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`
	IsPersonal    bool       `json:"is_personal"`
	IsBusiness    bool       `json:"is_business"`
	IsImportant   bool       `json:"is_important"`
	UseByAI       bool       `json:"use_by_ai"`
}

// EmailSearchParams holds the optional filters for GET /emails/search.
type EmailSearchParams struct {
	FromAddress    *string
	ToAddress      *string
	Month          *int
	Year           *int
	Subject        *string
	ToFrom         *string // comma-separated addresses; matched against both to and from
	HasAttachments *bool
}
