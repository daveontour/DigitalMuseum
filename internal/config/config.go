// Package config loads and validates application configuration from environment variables.
package config

import (
	"fmt"
	"os"
	"path/filepath"
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
}

// DatabaseConfig holds PostgreSQL connection settings.
type DatabaseConfig struct {
	Host     string
	Port     int
	Name     string
	User     string
	Password string
}

// ConnectionString returns the pgx DSN.
func (d DatabaseConfig) ConnectionString() string {
	return fmt.Sprintf("postgresql://%s:%s@%s:%d/%s", d.User, d.Password, d.Host, d.Port, d.Name)
}

// AdminConnectionString returns a DSN targeting the postgres admin database (used to create databases).
func (d DatabaseConfig) AdminConnectionString() string {
	return fmt.Sprintf("postgresql://%s:%s@%s:%d/postgres", d.User, d.Password, d.Host, d.Port)
}

// ServerConfig holds HTTP server settings.
type ServerConfig struct {
	Port int
	// SessionCookieSecure sets the Secure flag on the keyring session cookie.
	// Enable when serving over HTTPS (e.g. SESSION_COOKIE_SECURE=true).
	SessionCookieSecure bool
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
	PageTitle      string
	TemplatesDir   string // path to Jinja2 HTML templates
	AssetStaticDir string // path to Python static directory (JS/data files for templating)
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

	TavilyAPIKey string
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

	attachments, err := loadAttachmentConfig()
	if err != nil {
		return nil, fmt.Errorf("attachment config: %w", err)
	}

	return &Config{
		DB: db,
		Server: ServerConfig{
			Port:                serverPort,
			SessionCookieSecure: parseBool(getenv("SESSION_COOKIE_SECURE", "false")),
		},
		App: AppConfig{
			PageTitle:      getenv("PAGE_TITLE", "Digital Museum of SUBJECT_NAME"),
			TemplatesDir:   getenv("TEMPLATES_DIR", "../src/api/templates"),
			AssetStaticDir: getenv("ASSET_STATIC_DIR", "../src/api/static"),
		},
		Crypto:      cryptoCfg,
		Defaults:    loadDefaultsConfig(),
		AI:          loadAIConfig(),
		Attachments: attachments,
		Filesystem:  loadFilesystemConfig(),
		Gmail: GmailConfig{
			ClientID:     os.Getenv("GMAIL_CLIENT_ID"),
			ClientSecret: os.Getenv("GMAIL_CLIENT_SECRET"),
			RedirectURL:  getenv("GMAIL_REDIRECT_URL", "http://localhost:8001/gmail/auth/callback"),
		},
	}, nil
}

func loadCryptoConfig() (CryptoConfig, error) {
	pepper := os.Getenv("KEYRING_PEPPER")
	if strings.TrimSpace(pepper) == "" {
		return CryptoConfig{}, fmt.Errorf("missing required variable: set KEYRING_PEPPER")
	}
	return CryptoConfig{KeyringPepper: pepper}, nil
}

func loadDatabaseConfig() (DatabaseConfig, error) {
	host := os.Getenv("DB_HOST")
	name := os.Getenv("DB_NAME")
	user := os.Getenv("DB_USER")
	password := os.Getenv("DB_PASSWORD")

	if host == "" || name == "" || user == "" || password == "" {
		return DatabaseConfig{}, fmt.Errorf(
			"missing required variables: set DB_HOST, DB_NAME, DB_USER, and DB_PASSWORD",
		)
	}

	port, err := parseInt(getenv("DB_PORT", "5432"), "DB_PORT")
	if err != nil {
		return DatabaseConfig{}, err
	}

	return DatabaseConfig{
		Host:     host,
		Port:     port,
		Name:     name,
		User:     user,
		Password: password,
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

		TavilyAPIKey: os.Getenv("TAVILY_API_KEY"),
	}
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

// loadDotEnv tries multiple strategies to find and load a .env file.
func loadDotEnv() {
	candidates := []string{}

	// 1. Same directory as the executable
	if exe, err := os.Executable(); err == nil {
		candidates = append(candidates, filepath.Join(filepath.Dir(exe), ".env"))
	}

	// 2. Working directory
	if cwd, err := os.Getwd(); err == nil {
		candidates = append(candidates, filepath.Join(cwd, ".env"))
	}

	for _, path := range candidates {
		if _, err := os.Stat(path); err == nil {
			_ = godotenv.Load(path)
			return
		}
	}

	// Fallback: let godotenv search upward
	_ = godotenv.Load()
}

// getenv returns the environment variable value or a default.
func getenv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
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
