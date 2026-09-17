package config

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/joho/godotenv"
)

const (
	sourceProcessEnv = "process environment"
	sourceBuiltin    = "built-in default"
)

var (
	envSourceMu   sync.RWMutex
	envSources    = map[string]string{} // key → source label (file path or process environment)
	envFilesLoaded []string
)

// knownConfigKeys are env vars the app reads at startup (plus common Electron-injected ones).
// Order is the display order in the Loaded Config dialog.
var knownConfigKeys = []string{
	"SQLITE_PATH",
	"ADMIN_SQLITE_PATH",
	"HOST_PORT",
	"PAGE_TITLE",
	"DEPLOYMENT_NATURE",
	"LOG_LEVEL",
	"SESSION_COOKIE_SECURE",
	"TEMPLATES_DIR",
	"ASSET_STATIC_DIR",
	"TLS_CERT_FILE",
	"TLS_KEY_FILE",
	"ADMIN_EMAIL",
	"ADMIN_PASSWORD",
	"KEYRING_PEPPER",
	"MCP_SERVER_URL",
	"MCP_AUTH_TOKEN",
	"ENABLE_PPROF",
	"LOCALAI_BASE_URL",
	"LOCALAI_EMBEDDING_BASE_URL",
	"LOCALAI_API_KEY",
	"LOCALAI_MODEL_NAME",
	"LOCALAI_EMBEDDING_MODEL",
	"LOCALAI_NUM_CTX",
	"GMAIL_CLIENT_ID",
	"GMAIL_CLIENT_SECRET",
	"GMAIL_REDIRECT_URL",
	"REGIONS_CONFIG_PATH",
	"SUGGESTIONS_CONFIG_PATH",
	"GUIDE_TOPICS_CONFIG_PATH",
	"GUIDE_TOPICS_RELOAD_FROM_FILE_ON_STARTUP",
	"AI_MODELS_CONFIG_PATH",
	"TUS_CHUNK_SIZE_MB",
	"TUS_MAX_UPLOAD_GB",
	"TUS_UPLOAD_DIR",
	"ATTACHMENT_ALLOWED_TYPES",
	"ATTACHMENT_MIN_SIZE",
	"FILESYSTEM_IMPORT_EXCLUDE_PATTERNS",
	"DEFAULT_PROCESS_ALL_FOLDERS",
	"DEFAULT_NEW_ONLY_OPTION",
	"DEFAULT_WHATSAPP_IMPORT_DIRECTORY",
	"DEFAULT_FACEBOOK_IMPORT_DIRECTORY",
	"DEFAULT_FACEBOOK_EXPORT_ROOT",
	"DEFAULT_FACEBOOK_USER_NAME",
	"DEFAULT_INSTAGRAM_IMPORT_DIRECTORY",
	"DEFAULT_INSTAGRAM_EXPORT_ROOT",
	"DEFAULT_INSTAGRAM_USER_NAME",
	"DEFAULT_IMESSAGE_DIRECTORY_PATH",
	"DEFAULT_FACEBOOK_ALBUMS_IMPORT_DIRECTORY",
	"DEFAULT_FACEBOOK_ALBUMS_EXPORT_ROOT",
	"DEFAULT_FILESYSTEM_IMPORT_DIRECTORY",
	"DEFAULT_FILESYSTEM_IMPORT_MAX_IMAGES",
	"DEFAULT_FILESYSTEM_IMPORT_CREATE_THUMBNAIL",
	"DEFAULT_IMAGE_EXPORT_DIRECTORY",
	"DEFAULT_IMAP_HOST",
	"DEFAULT_IMAP_PORT",
	"DEFAULT_IMAP_USERNAME",
}

// LoadedConfigItem is one configuration entry for the Loaded Config dialog.
type LoadedConfigItem struct {
	Key      string `json:"key"`
	Value    string `json:"value"`
	Source   string `json:"source"`
	Secret   bool   `json:"secret"`
	Resolved bool   `json:"resolved,omitempty"`
}

// LoadedConfigReport is returned by GET /api/loaded-config.
type LoadedConfigReport struct {
	FilesLoaded []string           `json:"files_loaded"`
	Items       []LoadedConfigItem `json:"items"`
}

func resetEnvSources() {
	envSourceMu.Lock()
	defer envSourceMu.Unlock()
	envSources = map[string]string{}
	envFilesLoaded = nil
}

func recordProcessEnvBaseline() {
	envSourceMu.Lock()
	defer envSourceMu.Unlock()
	for _, e := range os.Environ() {
		key, _, ok := strings.Cut(e, "=")
		if !ok || key == "" {
			continue
		}
		if _, exists := envSources[key]; !exists {
			envSources[key] = sourceProcessEnv
		}
	}
}

func applyDotEnvFile(path string) {
	st, err := os.Stat(path)
	if err != nil || st.IsDir() {
		return
	}
	m, err := godotenv.Read(path)
	if err != nil || len(m) == 0 {
		return
	}
	abs := path
	if a, errA := filepath.Abs(path); errA == nil {
		abs = a
	}
	envSourceMu.Lock()
	defer envSourceMu.Unlock()
	for k, v := range m {
		_ = os.Setenv(k, v)
		envSources[k] = abs
	}
	envFilesLoaded = append(envFilesLoaded, abs)
}

func sourceForKey(key string) string {
	envSourceMu.RLock()
	defer envSourceMu.RUnlock()
	if s, ok := envSources[key]; ok && s != "" {
		return s
	}
	return sourceProcessEnv
}

func filesLoadedCopy() []string {
	envSourceMu.RLock()
	defer envSourceMu.RUnlock()
	out := make([]string, len(envFilesLoaded))
	copy(out, envFilesLoaded)
	return out
}

func isSecretConfigKey(key string) bool {
	u := strings.ToUpper(key)
	switch {
	case strings.Contains(u, "PASSWORD"),
		strings.Contains(u, "SECRET"),
		strings.Contains(u, "API_KEY"),
		strings.Contains(u, "AUTH_TOKEN"),
		strings.Contains(u, "PEPPER"),
		u == "MCP_AUTH_TOKEN":
		return true
	default:
		return false
	}
}

func redactConfigValue(key, value string) (display string, secret bool) {
	secret = isSecretConfigKey(key)
	if !secret {
		return value, false
	}
	if strings.TrimSpace(value) == "" {
		return "(not set)", true
	}
	return "(set)", true
}

func effectiveValueForKey(key string, cfg *Config) (value string, fromDefault bool) {
	raw, ok := os.LookupEnv(key)
	if ok {
		raw = strings.TrimSpace(stripInlineEnvComment(raw))
	}
	if ok && raw != "" {
		return raw, false
	}

	// Fall back to values already resolved into Config (built-in defaults / computed).
	if cfg != nil {
		switch key {
		case "ADMIN_SQLITE_PATH":
			return cfg.DB.BillingSQLitePath, true
		case "SQLITE_PATH":
			return cfg.DB.SQLitePath, true
		case "HOST_PORT":
			return getenv("HOST_PORT", "8000"), true
		case "PAGE_TITLE":
			return cfg.App.PageTitle, true
		case "DEPLOYMENT_NATURE":
			return cfg.App.DeploymentNature, true
		case "TEMPLATES_DIR":
			return cfg.App.TemplatesDir, true
		case "ASSET_STATIC_DIR":
			return cfg.App.AssetStaticDir, true
		case "SESSION_COOKIE_SECURE":
			if cfg.Server.SessionCookieSecure {
				return "true", true
			}
			return "false", true
		case "LOCALAI_EMBEDDING_BASE_URL":
			return cfg.AI.LocalAIEmbeddingBaseURL, true
		case "LOCALAI_MODEL_NAME":
			return cfg.AI.LocalAIModelName, true
		case "LOCALAI_EMBEDDING_MODEL":
			return cfg.AI.LocalAIEmbeddingModel, true
		case "TUS_UPLOAD_DIR":
			return cfg.Upload.TUSUploadDir, true
		case "TUS_CHUNK_SIZE_MB":
			return getenv("TUS_CHUNK_SIZE_MB", "10"), true
		case "TUS_MAX_UPLOAD_GB":
			return getenv("TUS_MAX_UPLOAD_GB", "32"), true
		case "ATTACHMENT_MIN_SIZE":
			return getenv("ATTACHMENT_MIN_SIZE", "0"), true
		case "DEFAULT_PROCESS_ALL_FOLDERS":
			return getenv("DEFAULT_PROCESS_ALL_FOLDERS", "false"), true
		case "DEFAULT_NEW_ONLY_OPTION":
			return getenv("DEFAULT_NEW_ONLY_OPTION", "true"), true
		case "DEFAULT_IMAP_PORT":
			return getenv("DEFAULT_IMAP_PORT", "993"), true
		case "DEFAULT_FILESYSTEM_IMPORT_CREATE_THUMBNAIL":
			return getenv("DEFAULT_FILESYSTEM_IMPORT_CREATE_THUMBNAIL", "false"), true
		}
	}
	return "", true
}

// SnapshotLoadedConfig builds a report of known configuration keys, their effective
// values, and which .env file / process environment / default supplied each one.
func SnapshotLoadedConfig(cfg *Config) LoadedConfigReport {
	seen := map[string]bool{}
	items := make([]LoadedConfigItem, 0, len(knownConfigKeys)+8)

	add := func(key, value, source string, resolved bool) {
		display, secret := redactConfigValue(key, value)
		items = append(items, LoadedConfigItem{
			Key:      key,
			Value:    display,
			Source:   source,
			Secret:   secret,
			Resolved: resolved,
		})
		seen[key] = true
	}

	for _, key := range knownConfigKeys {
		value, fromDefault := effectiveValueForKey(key, cfg)
		source := sourceBuiltin
		if !fromDefault {
			source = sourceForKey(key)
		} else if key == "ADMIN_SQLITE_PATH" && cfg != nil && cfg.DB.BillingSQLitePath != "" {
			if raw, ok := os.LookupEnv("ADMIN_SQLITE_PATH"); ok && strings.TrimSpace(raw) != "" {
				source = sourceForKey(key) + " → resolved path"
			} else {
				source = sourceBuiltin + " (<exeDir>/data/admin.sqlite)"
			}
			value = cfg.DB.BillingSQLitePath
		} else if key == "SQLITE_PATH" && cfg != nil && cfg.DB.SQLitePath != "" {
			source = "resolved at startup (archive profiles / SQLITE_PATH)"
			value = cfg.DB.SQLitePath
		}
		add(key, value, source, false)
	}

	// Extra keys present in loaded .env files but not in the known list.
	envSourceMu.RLock()
	extra := make([]string, 0)
	for k := range envSources {
		if !seen[k] && !strings.HasPrefix(k, "=") {
			extra = append(extra, k)
		}
	}
	envSourceMu.RUnlock()
	sort.Strings(extra)
	for _, key := range extra {
		// Skip noisy Windows / shell noise that isn't app config.
		if strings.HasPrefix(key, "Path") || key == "PATH" || key == "PATHEXT" ||
			key == "OS" || key == "COMSPEC" || key == "SystemRoot" ||
			strings.HasPrefix(key, "PROCESSOR_") || strings.HasPrefix(key, "NUMBER_OF_") {
			continue
		}
		// Only include extras that came from a .env file (not the full process env dump).
		src := sourceForKey(key)
		if src == sourceProcessEnv {
			continue
		}
		raw, _ := os.LookupEnv(key)
		add(key, raw, src, false)
	}

	if cfg != nil {
		add("resolved.ADMIN_SQLITE_PATH", cfg.DB.BillingSQLitePath,
			"resolved billing/admin database path", true)
		add("resolved.SQLITE_PATH", cfg.DB.SQLitePath,
			"resolved main archive database path", true)
		add("resolved.TEMPLATES_DIR", cfg.App.TemplatesDir, "effective templates directory", true)
		add("resolved.ASSET_STATIC_DIR", cfg.App.AssetStaticDir, "effective static assets directory", true)
		add("resolved.REGIONS_CONFIG_FILE", cfg.App.RegionsConfigFile(), "effective regions seed path", true)
		add("resolved.SUGGESTIONS_CONFIG_FILE", cfg.App.SuggestionsConfigFile(), "effective suggestions seed path", true)
		add("resolved.GUIDE_TOPICS_CONFIG_FILE", cfg.App.GuideTopicsConfigFile(), "effective guide topics seed path", true)
		add("resolved.AI_MODELS_CONFIG_FILE", cfg.App.AIModelsConfigFile(), "effective AI models seed path", true)
	}

	return LoadedConfigReport{
		FilesLoaded: filesLoadedCopy(),
		Items:       items,
	}
}
