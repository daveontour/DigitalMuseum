package ai

// Hidden tools/call argument keys the MCP client (NewMCPToolExecutor) attaches on top of
// whatever arguments the LLM supplied, carrying the caller context a single in-process call
// used to get from Go's request ctx automatically. Exported so cmd/mcpserver's arg parsing
// (withCallerContext) uses the same names instead of duplicating string literals.
//
// ArgUID and the ArgVisitor* flags are attached to every call (cheap, not secret — safe
// defaults for tools added later). ArgMasterPassword and ArgTavilyKey are secrets and are
// attached only for the specific tool names that need them (see mcpSecretArgTools in tools.go).
const (
	ArgUID                     = "_uid"
	ArgVisitorRestricted       = "_visitor_restricted"
	ArgVisitorCanMessagesChat  = "_visitor_can_messages_chat"
	ArgVisitorCanEmails        = "_visitor_can_emails"
	ArgVisitorCanContacts      = "_visitor_can_contacts"
	ArgVisitorCanRelationships = "_visitor_can_relationships"
	ArgVisitorCanSensitivePriv = "_visitor_can_sensitive_private"
	ArgMasterPassword          = "_master_password"
	ArgTavilyKey               = "_tavily_key"
)
