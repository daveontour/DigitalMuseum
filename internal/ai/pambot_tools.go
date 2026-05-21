package ai

// PamBotToolDefinitions returns tool JSON schemas used only by the Pam Bot companion.
// It is an editable copy of the relevant entries from toolDefinitions() so PamBot prompts,
// descriptions, and parameter shapes can diverge from main chat without changing shared tools.
func PamBotToolDefinitions() []map[string]any {
	return []map[string]any{
		{
			"name":        "search_facebook_albums",
			"description": "Search Facebook photo albums by a partial keyword match against album name or description.",
			"parameters": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"keyword": map[string]any{"type": "string", "description": "Partial keyword to search for in album names and descriptions"},
				},
				"required": []string{"keyword"},
			},
		},
		{
			"name":        "search_facebook_posts",
			"description": "Search Facebook posts where the post description partially matches the input.",
			"parameters": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"description": map[string]any{"type": "string", "description": "Partial text to search for within Facebook post descriptions"},
				},
				"required": []string{"description"},
			},
		},
		{
			"name":        "get_album_images",
			"description": "Get the first 5 images in a Facebook album by album_id. Returns id, title, year, and month for each image so you can choose the most relevant one to display alongside the memory. Call this after search_facebook_albums when you want to show a photo.",
			"parameters": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"album_id": map[string]any{"type": "integer", "description": "The numeric album ID returned by search_facebook_albums"},
				},
				"required": []string{"album_id"},
			},
		},
		{
			"name":        "get_reference_document",
			"description": "Retrieve the full content of one or more reference documents by their IDs.",
			"parameters": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"document_ids": map[string]any{
						"type":        "array",
						"items":       map[string]any{"type": "integer"},
						"description": "List of reference document IDs to retrieve",
					},
				},
				"required": []string{"document_ids"},
			},
		},
		{
			"name":        "get_available_reference_documents",
			"description": "Get the title, description, id and tags of all reference documents that are available for task.",
			"parameters":  map[string]any{"type": "object", "properties": map[string]any{}, "required": []string{}},
		},

		{
			"name":        "search_chat_messages_globally",
			"description": "Search message text across all chat sessions for a keyword. Returns chat_session_id, message_id, and content per match.",
			"parameters": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"keyword": map[string]any{"type": "string", "description": "Text to find (partial match)"},
				},
				"required": []string{"keyword"},
			},
		},
		{
			"name":        "search_chat_messages_in_session",
			"description": "Search message text in one chat session for a keyword. Returns chat_session_id, message_id, and content per match.",
			"parameters": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"chat_session": map[string]any{"type": "string", "description": "Chat session name"},
					"keyword":      map[string]any{"type": "string", "description": "Text to find (partial match)"},
				},
				"required": []string{"chat_session", "keyword"},
			},
		},
		{
			"name":        "get_emails_by_contact",
			"description": "Get plain text of emails where the sender or receiver matches the specified name or email address.",
			"parameters": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"name": map[string]any{"type": "string", "description": "The name or email address to search for in sender or receiver fields"},
				},
				"required": []string{"name"},
			},
		},
		{
			"name":        "list_available_chat_sessions",
			"description": "List distinct chat session names in the messages archive (WhatsApp, SMS, iMessage, Facebook chats). Use before get_imessages_by_chat_session when you need valid session names or the user is unsure of the exact spelling.",
			"parameters":  map[string]any{"type": "object", "properties": map[string]any{}, "required": []string{}},
		},
		{
			"name":        "get_imessages_by_chat_session",
			"description": "Get all messages for WhatsApp, SMS, iMessage and Facebook messages for a specific chat. Use this when the user asks about messages, conversations, or chats with a specific person or group.",
			"parameters": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"chat_session": map[string]any{"type": "string", "description": "The chat session name (person or group name) to retrieve messages for"},
				},
				"required": []string{"chat_session"},
			},
		},
		{
			"name":        "get_messages_around_in_chat",
			"description": "Return up to 20 messages before and after a specific message inside a chat session. Messages are ordered chronologically.",
			"parameters": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"chat_session": map[string]any{"type": "string", "description": "Chat session name"},
					"message_id":   map[string]any{"type": "integer", "description": "Database id of the anchor message (messages.id)"},
				},
				"required": []string{"chat_session", "message_id"},
			},
		},
	}
}
