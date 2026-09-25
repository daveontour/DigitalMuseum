package backgroundjobs

// Canonical background job names (stored in background_jobs.job_name).
const (
	JobThumbnails               = "thumbnails"
	JobImageTagEmbeddings       = "image_tag_embeddings"
	JobImageAIClassification    = "image_ai_classification"
	JobMessageContextEmbeddings = "message_context_embeddings"
	JobEmailEmbeddings          = "email_embeddings"
	JobFaceDetection            = "face_detection"
	JobFaceClustering           = "face_clustering"
	JobFaceCropBackfill         = "face_crop_backfill"
	JobFaceSuggestions          = "face_suggestions"
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
	{
		Name:                   JobFaceDetection,
		Title:                  "Detect faces in photos",
		Description:            "Finds faces in your photos so you can group them by person and search for someone by name.",
		DefaultIntervalSeconds: 10 * 60,
	},
	{
		Name:                   JobFaceClustering,
		Title:                  "Group similar faces",
		Description:            "Groups newly detected faces with people you've already named, and clusters the rest for you to review.",
		DefaultIntervalSeconds: 10 * 60,
	},
	{
		Name:                   JobFaceSuggestions,
		Title:                  "Find possible matches for unnamed people",
		Description:            "Compares each unnamed group of faces with the people you've already named, so People in Photos can show a \"Possible Matches\" list. Re-run after naming more people.",
		DefaultIntervalSeconds: 60 * 60,
	},
	{
		Name:                   JobFaceCropBackfill,
		Title:                  "Backfill face crop thumbnails",
		Description:            "Pre-generates and stores a thumbnail for every detected face found before this was done automatically, so People in Photos loads instantly instead of re-cropping each photo on every view.",
		DefaultIntervalSeconds: 0,
	},
}
