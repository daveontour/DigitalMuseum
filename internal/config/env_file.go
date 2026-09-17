package config

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
)

// electronUserDataDirName matches electron/package.json "name". Electron's
// app.getPath('userData') is %APPDATA%\<name> on Windows (not productName).
const electronUserDataDirName = "digital-museum"

// legacyUserDataDirName is the older folder used by docs and early builds.
const legacyUserDataDirName = "Digital Museum"

// UserEnvFilePath returns the user .env file Electron reads/writes (digital-museum).
func UserEnvFilePath() string {
	return userEnvFilePathIn(electronUserDataDirName)
}

// LegacyUserEnvFilePath returns the older %APPDATA%\Digital Museum\.env path, if any.
func LegacyUserEnvFilePath() string {
	return userEnvFilePathIn(legacyUserDataDirName)
}

func userEnvFilePathIn(dirName string) string {
	if appdata := strings.TrimSpace(os.Getenv("APPDATA")); appdata != "" {
		return filepath.Join(appdata, dirName, ".env")
	}
	if home, err := os.UserHomeDir(); err == nil {
		if runtime.GOOS == "darwin" {
			return filepath.Join(home, "Library", "Application Support", dirName, ".env")
		}
		if xdg := strings.TrimSpace(os.Getenv("XDG_CONFIG_HOME")); xdg != "" {
			return filepath.Join(xdg, dirName, ".env")
		}
		return filepath.Join(home, ".config", dirName, ".env")
	}
	return ".env"
}

// UpsertEnvVarInContent sets or replaces key=value in dotenv file content.
func UpsertEnvVarInContent(content, key, value string) string {
	re := regexp.MustCompile(`(?m)^` + regexp.QuoteMeta(key) + `\s*=.*`)
	line := key + "=" + value
	if re.MatchString(content) {
		return re.ReplaceAllString(content, line)
	}
	content = strings.TrimRight(content, "\r\n")
	if content != "" {
		content += "\n"
	}
	return content + line + "\n"
}

// RemoveEnvVarFromContent removes key=… lines from dotenv content.
func RemoveEnvVarFromContent(content, key string) string {
	re := regexp.MustCompile(`(?m)^` + regexp.QuoteMeta(key) + `\s*=.*\n?`)
	return strings.TrimRight(re.ReplaceAllString(content, ""), "\r\n") + "\n"
}

// WriteUserEnvKeys upserts or removes keys in the user .env file.
func WriteUserEnvKeys(upserts map[string]string, removeKeys []string) error {
	path := UserEnvFilePath()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create env dir: %w", err)
	}
	content := ""
	if b, err := os.ReadFile(path); err == nil {
		content = string(b)
	}
	for _, key := range removeKeys {
		content = RemoveEnvVarFromContent(content, key)
	}
	for key, val := range upserts {
		content = UpsertEnvVarInContent(content, key, val)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}
