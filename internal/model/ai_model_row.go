package model

// AIModelRow is one row in the deployment-wide ai_models table.
type AIModelRow struct {
	ID          int64  `json:"id"`
	Key         string `json:"key"`
	DisplayName string `json:"display_name"`
	ModelSlug   string `json:"model_slug"`
	Enabled     bool   `json:"enabled"`
	SortOrder   int    `json:"sort_order"`
}
