package model

// GuideTopicRow is one row in the deployment-wide guide_topics table.
type GuideTopicRow struct {
	ID   int64  `json:"id"`
	Key  string `json:"key"`
	Text string `json:"text"`
}
