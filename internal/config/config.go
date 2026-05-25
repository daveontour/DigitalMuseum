// Package config loads and validates application configuration from environment variables.
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"github.com/joho/godotenv"
)

// Config holds all application configuration.
type Config struct {
	DB          DatabaseConfig
	Server      ServerConfig
	App         AppConfig
	Crypto      CryptoConfig
	Defaults    DefaultsConfig
	AI          AIConfig
	Attachments AttachmentConfig
	Filesystem  FilesystemConfig
	Gmail       GmailConfig
	Upload      UploadConfig
}

// DatabaseConfig holds SQLite file paths (single-user build).
type DatabaseConfig struct {
	SQLitePath        string
	BillingSQLitePath string
}

// BillingConfig returns a copy with a default billing DB path next to the main file when unset.
func (d DatabaseConfig) BillingConfig() DatabaseConfig {
	b := d
	if b.BillingSQLitePath == "" && b.SQLitePath != "" {
		b.BillingSQLitePath = filepath.Join(filepath.Dir(b.SQLitePath), "billing.sqlite")
	}
	return b
}

// SQLiteFileDSN builds a github.com/mattn/go-sqlite3 DSN for a database file path
// (foreign keys on, 5s busy timeout). filePath should be non-empty.
func SQLiteFileDSN(filePath string) string {
	p := filepath.Clean(filePath)
	return "file:" + filepath.ToSlash(p) + "?_foreign_keys=1&_busy_timeout=5000"
}

// SQLiteDSN builds a go-sqlite3 connection string for the main database.
func (d DatabaseConfig) SQLiteDSN() string {
	return SQLiteFileDSN(d.SQLitePath)
}

// ServerConfig holds HTTP server settings.
type ServerConfig struct {
	Port int
	// TLSCertFile and TLSKeyFile, when both non-empty, enable HTTPS (ListenAndServeTLS).
	// For local dev, generate certs with mkcert and set TLS_CERT_FILE / TLS_KEY_FILE.
	TLSCertFile string
	TLSKeyFile  string
	// SessionCookieSecure sets the Secure flag on session cookies.
	// Enable when serving over HTTPS (SESSION_COOKIE_SECURE=true).
	SessionCookieSecure bool
	// AdminEmail and AdminPassword authenticate /admin (ADMIN_EMAIL / ADMIN_PASSWORD).
	// They are not stored in archive SQLite; the first archive owner is users.id=2.
	AdminEmail    string
	AdminPassword string
}

// CryptoConfig holds secrets used for crypto/key-derivation.
type CryptoConfig struct {
	// KeyringPepper is an application secret mixed into key derivation for the
	// sensitive-keyring and encrypted reference documents.
	//
	// Set via KEYRING_PEPPER env var. Rotating this requires re-initialising the
	// keyring and re-encrypting any encrypted records.
	KeyringPepper string
}

// AppConfig holds application-level settings.
type AppConfig struct {
	PageTitle        string
	TemplatesDir     string // path to Jinja2 HTML templates
	AssetStaticDir   string // path to Python static directory (JS/data files for templating)
	DeploymentNature string // "local" = show filesystem path import tiles; unset/other = hide them
}

// DefaultsConfig holds default values shown in the control panel UI.
type DefaultsConfig struct {
	ProcessAllFolders bool
	NewOnlyOption     bool

	WhatsAppImportDirectory string

	FacebookImportDirectory string
	FacebookExportRoot      string
	FacebookUserName        string

	InstagramImportDirectory string
	InstagramExportRoot      string
	InstagramUserName        string

	IMessageDirectoryPath string

	FacebookAlbumsImportDirectory string
	FacebookAlbumsExportRoot      string

	FilesystemImportDirectory   string
	FilesystemImportMaxImages   string
	FilesystemImportCreateThumb bool

	ImageExportDirectory string

	IMAPHost     string
	IMAPPort     string
	IMAPUsername string
	IMAPUseSSL   bool
	IMAPNewOnly  bool
}

// AIConfig holds AI provider settings.
type AIConfig struct {
	GeminiAPIKey    string
	GeminiModelName string

	AnthropicAPIKey string
	ClaudeModelName string

	DeepSeekAPIKey    string
	DeepSeekModelName string

	TavilyAPIKey string

	LocalAIBaseURL        string
	LocalAIAPIKey         string
	LocalAIModelName      string
	LocalAIEmbeddingModel string
	// LocalAINumCtx is the per-request `num_ctx` sent to Ollama. Zero means
	// the request omits the field and Ollama applies its server-side default
	// (which Electron starts with via OLLAMA_NUM_CTX).
	LocalAINumCtx int
}

// AttachmentConfig holds attachment filtering settings.
type AttachmentConfig struct {
	// AllowedTypes is nil when all types are allowed.
	AllowedTypes []string
	MinSize      int64
	// RawAllowedTypes is the unmodified ATTACHMENT_ALLOWED_TYPES env value.
	RawAllowedTypes string
}

// FilesystemConfig holds filesystem import settings.
type FilesystemConfig struct {
	ExcludePatterns []string
}

// GmailConfig holds Google Gmail OAuth2 settings.
type GmailConfig struct {
	ClientID     string
	ClientSecret string
	RedirectURL  string
}

// UploadConfig holds tus resumable upload settings.
type UploadConfig struct {
	// TUSChunkSizeMB is the chunk size (in MB) the frontend will use for tus uploads.
	// Set via TUS_CHUNK_SIZE_MB env var. Default: 10.
	TUSChunkSizeMB int
	// TUSUploadDir is where tusd stores in-progress upload chunks.
	// Set via TUS_UPLOAD_DIR env var. Default: tmp/tus_uploads.
	TUSUploadDir string
	// MaxUploadBytes caps ZIP imports (tus and POST /import/upload). Derived from
	// TUS_MAX_UPLOAD_GB (GiB-scale, default 32). Capped at 512 to avoid misconfiguration.
	MaxUploadBytes int64
	// MaxUploadGB is the same limit rounded for JSON APIs (integer GiB).
	MaxUploadGB int
}

// Load reads environment variables (and optionally a .env file) and returns a Config.
// Returns an error if any required variable is missing or invalid.
func Load() (*Config, error) {
	loadDotEnv()

	db, err := loadDatabaseConfig()
	if err != nil {
		return nil, fmt.Errorf("database config: %w", err)
	}

	cryptoCfg, err := loadCryptoConfig()
	if err != nil {
		return nil, fmt.Errorf("crypto config: %w", err)
	}

	serverPort, err := parseInt(getenv("HOST_PORT", "8000"), "HOST_PORT")
	if err != nil {
		return nil, err
	}

	tlsCert := strings.TrimSpace(os.Getenv("TLS_CERT_FILE"))
	tlsKey := strings.TrimSpace(os.Getenv("TLS_KEY_FILE"))
	if (tlsCert != "" && tlsKey == "") || (tlsCert == "" && tlsKey != "") {
		return nil, fmt.Errorf("TLS: set both TLS_CERT_FILE and TLS_KEY_FILE, or leave both unset for plain HTTP")
	}

	attachments, err := loadAttachmentConfig()
	if err != nil {
		return nil, fmt.Errorf("attachment config: %w", err)
	}

	return &Config{
		DB: db,
		Server: ServerConfig{
			Port:                serverPort,
			TLSCertFile:         tlsCert,
			TLSKeyFile:          tlsKey,
			SessionCookieSecure: parseBool(getenv("SESSION_COOKIE_SECURE", "false")),
			AdminEmail:          loadAdminEmail(),
			AdminPassword:       loadAdminPassword(),
		},
		App: AppConfig{
			PageTitle:        getenv("PAGE_TITLE", "Digital Museum of SUBJECT_NAME"),
			TemplatesDir:     getenv("TEMPLATES_DIR", "../src/api/templates"),
			AssetStaticDir:   getenv("ASSET_STATIC_DIR", "../src/api/static"),
			DeploymentNature: getenv("DEPLOYMENT_NATURE", "web"),
		},
		Crypto:      cryptoCfg,
		Defaults:    loadDefaultsConfig(),
		AI:          loadAIConfig(),
		Attachments: attachments,
		Filesystem:  loadFilesystemConfig(),
		Gmail: GmailConfig{
			ClientID:     os.Getenv("GMAIL_CLIENT_ID"),
			ClientSecret: os.Getenv("GMAIL_CLIENT_SECRET"),
			RedirectURL:  os.Getenv("GMAIL_REDIRECT_URL"),
		},
		Upload: loadUploadConfig(),
	}, nil
}

func loadCryptoConfig() (CryptoConfig, error) {
	// Encryption / keyring removed in SQLite single-user build; pepper unused.
	return CryptoConfig{KeyringPepper: ""}, nil
}

func loadDatabaseConfig() (DatabaseConfig, error) {
	billing, err := ResolveAdminSQLitePath()
	if err != nil {
		return DatabaseConfig{}, fmt.Errorf("resolve admin sqlite path: %w", err)
	}
	return DatabaseConfig{
		SQLitePath:        "",
		BillingSQLitePath: billing,
	}, nil
}

func loadDefaultsConfig() DefaultsConfig {
	return DefaultsConfig{
		ProcessAllFolders: parseBool(getenv("DEFAULT_PROCESS_ALL_FOLDERS", "false")),
		NewOnlyOption:     parseBool(getenv("DEFAULT_NEW_ONLY_OPTION", "true")),

		WhatsAppImportDirectory: getenv("DEFAULT_WHATSAPP_IMPORT_DIRECTORY", ""),

		FacebookImportDirectory: getenv("DEFAULT_FACEBOOK_IMPORT_DIRECTORY", ""),
		FacebookExportRoot:      getenv("DEFAULT_FACEBOOK_EXPORT_ROOT", ""),
		FacebookUserName:        getenv("DEFAULT_FACEBOOK_USER_NAME", ""),

		InstagramImportDirectory: getenv("DEFAULT_INSTAGRAM_IMPORT_DIRECTORY", ""),
		InstagramExportRoot:      getenv("DEFAULT_INSTAGRAM_EXPORT_ROOT", ""),
		InstagramUserName:        getenv("DEFAULT_INSTAGRAM_USER_NAME", ""),

		IMessageDirectoryPath: getenv("DEFAULT_IMESSAGE_DIRECTORY_PATH", ""),

		FacebookAlbumsImportDirectory: getenv("DEFAULT_FACEBOOK_ALBUMS_IMPORT_DIRECTORY", ""),
		FacebookAlbumsExportRoot:      getenv("DEFAULT_FACEBOOK_ALBUMS_EXPORT_ROOT", ""),

		FilesystemImportDirectory:   getenv("DEFAULT_FILESYSTEM_IMPORT_DIRECTORY", ""),
		FilesystemImportMaxImages:   getenv("DEFAULT_FILESYSTEM_IMPORT_MAX_IMAGES", ""),
		FilesystemImportCreateThumb: parseBool(getenv("DEFAULT_FILESYSTEM_IMPORT_CREATE_THUMBNAIL", "false")),

		ImageExportDirectory: getenv("DEFAULT_IMAGE_EXPORT_DIRECTORY", ""),

		IMAPHost:     getenv("DEFAULT_IMAP_HOST", ""),
		IMAPPort:     getenv("DEFAULT_IMAP_PORT", "993"),
		IMAPUsername: getenv("DEFAULT_IMAP_USERNAME", ""),
		IMAPUseSSL:   true,
		IMAPNewOnly:  true,
	}
}

func loadAIConfig() AIConfig {
	return AIConfig{
		GeminiAPIKey:    os.Getenv("GEMINI_API_KEY"),
		GeminiModelName: getenv("GEMINI_MODEL_NAME", "gemini-2.5-flash"),

		AnthropicAPIKey: os.Getenv("ANTHROPIC_API_KEY"),
		ClaudeModelName: getenv("CLAUDE_MODEL_NAME", "claude-sonnet-4-6"),

		DeepSeekAPIKey:    os.Getenv("DEEPSEEK_API_KEY"),
		DeepSeekModelName: getenv("DEEPSEEK_MODEL_NAME", "deepseek-chat"),

		TavilyAPIKey: os.Getenv("TAVILY_API_KEY"),

		LocalAIBaseURL:        os.Getenv("LOCALAI_BASE_URL"),
		LocalAIAPIKey:         os.Getenv("LOCALAI_API_KEY"),
		LocalAIModelName:      getenv("LOCALAI_MODEL_NAME", "local-model"),
		LocalAIEmbeddingModel: getenv("LOCALAI_EMBEDDING_MODEL", ""),
		LocalAINumCtx:         parseNumCtxEnv(),
	}
}

// parseNumCtxEnv reads LOCALAI_NUM_CTX. Empty / unparseable / non-positive
// values resolve to 0, which means "do not send num_ctx in the request body".
func parseNumCtxEnv() int {
	raw := strings.TrimSpace(os.Getenv("LOCALAI_NUM_CTX"))
	if raw == "" {
		return 0
	}
	v, err := strconv.Atoi(raw)
	if err != nil || v <= 0 {
		return 0
	}
	return v
}

func loadAttachmentConfig() (AttachmentConfig, error) {
	raw := strings.TrimSpace(os.Getenv("ATTACHMENT_ALLOWED_TYPES"))
	var allowedTypes []string
	if raw != "" {
		for _, t := range strings.Split(raw, ",") {
			t = strings.TrimSpace(strings.ToLower(t))
			if t != "" {
				allowedTypes = append(allowedTypes, t)
			}
		}
	}

	minSizeStr := strings.TrimSpace(getenv("ATTACHMENT_MIN_SIZE", "0"))
	minSize, err := strconv.ParseInt(minSizeStr, 10, 64)
	if err != nil || minSize < 0 {
		return AttachmentConfig{}, fmt.Errorf("ATTACHMENT_MIN_SIZE must be a non-negative integer, got: %s", minSizeStr)
	}

	return AttachmentConfig{
		AllowedTypes:    allowedTypes,
		MinSize:         minSize,
		RawAllowedTypes: raw,
	}, nil
}

func loadFilesystemConfig() FilesystemConfig {
	raw := strings.TrimSpace(os.Getenv("FILESYSTEM_IMPORT_EXCLUDE_PATTERNS"))
	var patterns []string
	if raw != "" {
		for _, p := range strings.Split(raw, ",") {
			p = strings.TrimSpace(p)
			if p != "" {
				patterns = append(patterns, p)
			}
		}
	}
	return FilesystemConfig{ExcludePatterns: patterns}
}

func loadUploadConfig() UploadConfig {
	chunkMB := 10
	if v, err := parseInt(getenv("TUS_CHUNK_SIZE_MB", "10"), "TUS_CHUNK_SIZE_MB"); err == nil && v > 0 {
		chunkMB = v
	}

	maxGB, err := parseInt(getenv("TUS_MAX_UPLOAD_GB", "32"), "TUS_MAX_UPLOAD_GB")
	if err != nil || maxGB <= 0 {
		maxGB = 32
	}
	const maxUploadGBCap = 512
	if maxGB > maxUploadGBCap {
		maxGB = maxUploadGBCap
	}

	return UploadConfig{
		TUSChunkSizeMB: chunkMB,
		TUSUploadDir:   getenv("TUS_UPLOAD_DIR", defaultTUSUploadDir()),
		MaxUploadBytes: int64(maxGB) * (1 << 30),
		MaxUploadGB:    maxGB,
	}
}

// defaultTUSUploadDir picks a writable default: OS temp on Windows (avoids
// Controlled Folder Access / sync tools blocking writes under the repo tree).
func defaultTUSUploadDir() string {
	if runtime.GOOS == "windows" {
		return filepath.Join(os.TempDir(), "digitalmuseum-tus")
	}
	return "tmp/tus_uploads"
}

// loadDotEnv merges .env files so file values override stale OS environment variables.
// Later files win: executable dir, then APPDATA user config, then every .env found walking
// up from the working directory (nested cwd .env first, project root last).
func loadDotEnv() {
	tryLoadDotEnv := func(path string) {
		if st, err := os.Stat(path); err != nil || st.IsDir() {
			return
		}
		_ = godotenv.Overload(path)
	}

	if exe, err := os.Executable(); err == nil {
		tryLoadDotEnv(filepath.Join(filepath.Dir(exe), ".env"))
	}
	if appdata := strings.TrimSpace(os.Getenv("APPDATA")); appdata != "" {
		tryLoadDotEnv(filepath.Join(appdata, "Digital Museum", ".env"))
	}
	if cwd, err := os.Getwd(); err == nil {
		dir := cwd
		for {
			tryLoadDotEnv(filepath.Join(dir, ".env"))
			parent := filepath.Dir(dir)
			if parent == dir {
				break
			}
			dir = parent
		}
	}
}

func loadAdminEmail() string {
	return strings.ToLower(strings.TrimSpace(os.Getenv("ADMIN_EMAIL")))
}

func loadAdminPassword() string {
	return normalizeEnvSecret(os.Getenv("ADMIN_PASSWORD"))
}

// normalizeEnvSecret trims whitespace and stray CR bytes from env-provided secrets.
func normalizeEnvSecret(v string) string {
	v = strings.TrimSpace(v)
	v = strings.TrimSuffix(v, "\r")
	return v
}

// stripInlineEnvComment removes a shell-style trailing comment: an unquoted
// space followed by '#'. This matches common .env behaviour and fixes values
// mistakenly set in the OS environment as "path  # note" (see TUS_UPLOAD_DIR).
func stripInlineEnvComment(v string) string {
	v = strings.TrimSpace(v)
	if v == "" {
		return v
	}
	runes := []rune(v)
	var inSingle, inDouble bool
	for i := 0; i < len(runes)-1; i++ {
		r := runes[i]
		next := runes[i+1]
		switch {
		case r == '\'' && !inDouble:
			inSingle = !inSingle
		case r == '"' && !inSingle:
			inDouble = !inDouble
		case !inSingle && !inDouble && r == ' ' && next == '#':
			return strings.TrimSpace(string(runes[:i]))
		}
	}
	return v
}

// getenv returns the environment variable value or a default.
func getenv(key, def string) string {
	v, ok := os.LookupEnv(key)
	if !ok {
		return def
	}
	v = strings.TrimSpace(stripInlineEnvComment(v))
	if v == "" {
		return def
	}
	return v
}

func parseInt(s, varName string) (int, error) {
	v, err := strconv.Atoi(s)
	if err != nil {
		return 0, fmt.Errorf("%s must be an integer, got: %s", varName, s)
	}
	return v, nil
}

func parseBool(s string) bool {
	return strings.ToLower(strings.TrimSpace(s)) == "true"
}
