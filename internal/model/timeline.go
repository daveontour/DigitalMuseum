package model

// TimelineMonth is one month's item counts across sources (GET /api/timeline).
type TimelineMonth struct {
	Month    string `json:"month"` // "YYYY-MM"
	Messages int64  `json:"messages"`
	Emails   int64  `json:"emails"`
	Photos   int64  `json:"photos"`
	Posts    int64  `json:"posts"`
}

// TimelineConversationDay groups one day's messages in one conversation.
type TimelineConversationDay struct {
	Date           string `json:"date"` // "YYYY-MM-DD"
	ChatSession    string `json:"chat_session"`
	Count          int64  `json:"count"`
	FirstMessageID int64  `json:"first_message_id"`
}

// TimelineEmail is one email in a timeline month.
type TimelineEmail struct {
	ID      int64  `json:"id"`
	Date    string `json:"date"` // "YYYY-MM-DD"
	Time    string `json:"time,omitempty"`
	Subject string `json:"subject"`
	From    string `json:"from"`
}

// TimelinePhoto is one photo in a timeline month (Day may be 0 when unknown).
type TimelinePhoto struct {
	ID    int64  `json:"id"`
	Day   int    `json:"day"`
	Title string `json:"title"`
}

// TimelinePost is one Facebook post in a timeline month.
type TimelinePost struct {
	ID    int64  `json:"id"`
	Date  string `json:"date"` // "YYYY-MM-DD"
	Title string `json:"title"`
	Text  string `json:"text"`
}

// TimelineMonthDetail is GET /api/timeline/{year}/{month}. Totals are the full counts for the
// month; the slices may be capped (see Limits).
type TimelineMonthDetail struct {
	Month         string                    `json:"month"`
	Conversations []TimelineConversationDay `json:"conversations"`
	Emails        []TimelineEmail           `json:"emails"`
	Photos        []TimelinePhoto           `json:"photos"`
	Posts         []TimelinePost            `json:"posts"`
	Totals        TimelineMonth             `json:"totals"`
	Limits        map[string]int            `json:"limits"`
	Hidden        []string                  `json:"hidden"` // sources not available to this session
}

// QuickSearchHit is one quick-search result.
type QuickSearchHit struct {
	Kind    string `json:"kind"` // message | email | photo | facebook_post
	ID      int64  `json:"id"`
	Title   string `json:"title"`
	Snippet string `json:"snippet,omitempty"`
	Date    string `json:"date,omitempty"`
}

// QuickSearchResult is GET /api/quick-search.
type QuickSearchResult struct {
	Query  string           `json:"query"`
	Hits   []QuickSearchHit `json:"hits"`
	Hidden []string         `json:"hidden"`
}
