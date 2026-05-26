package model

// SuggestionRow is one row in the deployment-wide suggestions table.
type SuggestionRow struct {
	ID   int64  `json:"id"`
	Key  string `json:"key"`
	Text string `json:"text"`
}
