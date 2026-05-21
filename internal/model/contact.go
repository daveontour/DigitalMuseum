package model

import "github.com/daveontour/aimuseum/internal/sqlutil"

// Contact is a row from the contacts table (short version for list responses).
type Contact struct {
	ID           int64
	Name         string
	Email        *string
	NumEmails    *int
	FacebookID   *string
	NumFacebook  *int
	WhatsAppID   *string
	NumWhatsApp  *int
	IMessageID   *string
	NumIMessages *int
	SMSID        *string
	NumSMS       *int
	InstagramID  *string
	NumInstagram *int
}

// ContactDetail is a contact row loaded by id (owner / admin helpers).
type ContactDetail struct {
	ID               int64
	Name             string
	Email            *string
	AlternativeNames *string
}

// ContactGraph is used for the relationship graph query.
type ContactGraph struct {
	ID           int64
	Name         string
	RelType      *string
	NumEmails    *int
	NumIMessages *int
	NumFacebook  *int
	NumWhatsApp  *int
	NumSMS       *int
	NumInstagram *int
	Total        int64
}

// EmailMatch is a row from the email_matches table.
type EmailMatch struct {
	ID          int64
	PrimaryName string
	Email       string
	CreatedAt   sqlutil.DBTime
	UpdatedAt   sqlutil.DBTime
}

// EmailExclusion is a row from the email_exclusions table.
type EmailExclusion struct {
	ID        int64
	Email     string
	Name      string
	NameEmail bool
	CreatedAt sqlutil.DBTime
	UpdatedAt sqlutil.DBTime
}

// EmailClassification is a row from the email_classifications table.
type EmailClassification struct {
	ID             int64
	Name           string
	Classification string
	CreatedAt      sqlutil.DBTime
	UpdatedAt      sqlutil.DBTime
}

// AttachmentInfo holds an email_attachment media_items row joined to its parent email.
type AttachmentInfo struct {
	AttachmentID int64
	Filename     string
	ContentType  string
	Size         *int64
	EmailID      int64
	EmailSubject *string
	EmailFrom    *string
	EmailDate    sqlutil.NullDBTime
	EmailFolder  *string
}
