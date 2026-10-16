package model

// ── On This Day ─────────────────────────────────────────────────────────────

// OnThisDayYear is everything from one earlier year that happened on the requested month/day.
// Slices are capped (see OnThisDayResult.Limits); Totals are the full counts.
type OnThisDayYear struct {
	Year          int                       `json:"year"`
	YearsAgo      int                       `json:"years_ago"`
	Conversations []TimelineConversationDay `json:"conversations"`
	Emails        []TimelineEmail           `json:"emails"`
	Photos        []TimelinePhoto           `json:"photos"`
	Posts         []TimelinePost            `json:"posts"`
	Totals        TimelineMonth             `json:"totals"` // Month holds "YYYY-MM-DD"
}

// OnThisDayResult is GET /api/on-this-day.
type OnThisDayResult struct {
	Date   string          `json:"date"` // "MM-DD"
	Year   int             `json:"year"` // the "today" year; only earlier years are returned
	Years  []OnThisDayYear `json:"years"` // newest first
	Total  int64           `json:"total"`
	Limits map[string]int  `json:"limits"`
	Hidden []string        `json:"hidden"` // sources not available to this session
}

// ── Memory Challenge (archive trivia) ───────────────────────────────────────

// TriviaSource points at the archive record a question was built from, so the UI can open it
// with Chat.openSource after the player answers.
type TriviaSource struct {
	Kind string `json:"kind"` // message | email | photo | facebook_post
	ID   int64  `json:"id"`
}

// TriviaQuestion is one multiple-choice question generated straight from archive data — every
// answer is a fact read from the database, never AI-invented.
type TriviaQuestion struct {
	Type         string        `json:"type"` // face_who | photo_year | photo_place | message_sender | email_sender | post_year | top_contact_year
	Question     string        `json:"question"`
	Quote        string        `json:"quote,omitempty"`     // message/email/post excerpt shown with the question
	ImageURL     string        `json:"image_url,omitempty"` // photo preview or face crop
	Options      []string      `json:"options"`
	CorrectIndex int           `json:"correct_index"`
	Explanation  string        `json:"explanation"` // shown after answering
	Source       *TriviaSource `json:"source,omitempty"`
}

// TriviaResult is GET /api/trivia.
type TriviaResult struct {
	Questions []TriviaQuestion `json:"questions"`
	// Available lists the question types this archive has enough data for (so the UI can explain
	// a short quiz, e.g. "name some faces in People in Photos to unlock face questions").
	Available []string `json:"available"`
}
