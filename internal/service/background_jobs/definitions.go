package backgroundjobs

// Canonical background job names (stored in background_jobs.job_name).
const (
	JobThumbnails               = "thumbnails"
	JobImageTagEmbeddings       = "image_tag_embeddings"
	JobImageAIClassification    = "image_ai_classification"
	JobMessageContextEmbeddings = "message_context_embeddings"
	JobEmailEmbeddings          = "email_embeddings"
)

// DefaultDefinitions lists every maintenance job surfaced in Configuration > Background Jobs.
var DefaultDefinitions = []JobDef{
	{
		Name:                   JobThumbnails,
		Title:                  "Generate image thumbnails",
		Description:            "Generate or refresh thumbnails for images that do not yet have one.",
		DefaultIntervalSeconds: 10 * 60,
	},
	{
		Name:                   JobImageAIClassification,
		Title:                  "Tag photos automatically",
		Description:            "Uses AI to look at each photo and describe what's in it, so you don't have to tag them by hand.",
		DefaultIntervalSeconds: 600,
	},
	{
		Name:                   JobImageTagEmbeddings,
		Title:                  "Make photo tags searchable",
		Description:            "Lets you find photos by what's in them (e.g. \"birthday cake\"), not just by filename.",
		DefaultIntervalSeconds: 10 * 60,
	},
	{
		Name:                   JobMessageContextEmbeddings,
		Title:                  "Make messages searchable",
		Description:            "Lets the AI find relevant messages and conversations by meaning, not just exact words.",
		DefaultIntervalSeconds: 600,
	},
	{
		Name:                   JobEmailEmbeddings,
		Title:                  "Make emails searchable",
		Description:            "Lets the AI find relevant emails by meaning, not just exact words.",
		DefaultIntervalSeconds: 600,
	},
}
