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
		Title:                  "Image AI classification",
		Description:            "Use AI tools to generate tags for the image based on its content.",
		DefaultIntervalSeconds: 600,
	},
	{
		Name:                   JobImageTagEmbeddings,
		Title:                  "Image Classification Encoding",
		Description:            "Encode image tags to enable textual searches of the image content.",
		DefaultIntervalSeconds: 10 * 60,
	},
	{
		Name:                   JobMessageContextEmbeddings,
		Title:                  "Message Content Encoding",
		Description:            "Encode message content to enable similarity searches.",
		DefaultIntervalSeconds: 600,
	},
	{
		Name:                   JobEmailEmbeddings,
		Title:                  "Optimise Emails for Searching",
		Description:            "Embed email bodies for semantic similarity search.",
		DefaultIntervalSeconds: 600,
	},
}
