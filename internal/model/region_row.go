package model

// RegionRow is one row in the deployment-wide regions table.
type RegionRow struct {
	ID        int64  `json:"id"`
	Key       string `json:"key"`
	SortOrder int    `json:"sort_order"`
	Text      string `json:"text"`
}
