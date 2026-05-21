// Package gmail provides OAuth2 token management and a Gmail API client.
package gmail

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
)

const (
	tokenConfigKey = "GMAIL_OAUTH_TOKEN"
	stateConfigKey = "GMAIL_OAUTH_STATE"
	popupConfigKey = "GMAIL_OAUTH_POPUP"
	// GmailReadonlyScope is the OAuth2 scope required for read-only Gmail access.
	GmailReadonlyScope = "https://www.googleapis.com/auth/gmail.readonly"
)

// OAuthConfig builds an oauth2.Config from the provided credentials.
func OAuthConfig(clientID, clientSecret, redirectURL string) *oauth2.Config {
	return &oauth2.Config{
		ClientID:     clientID,
		ClientSecret: clientSecret,
		RedirectURL:  redirectURL,
		Scopes:       []string{GmailReadonlyScope},
		Endpoint:     google.Endpoint,
	}
}

// SaveToken stores a JSON-serialised oauth2.Token in app_configuration.
func SaveToken(ctx context.Context, pool *sql.DB, tok *oauth2.Token) error {
	b, err := json.Marshal(tok)
	if err != nil {
		return fmt.Errorf("marshal token: %w", err)
	}
	v := string(b)
	return upsertConfig(ctx, pool, tokenConfigKey, v)
}

// LoadToken retrieves and deserialises the stored token.
// Returns nil, nil if no token has been saved.
func LoadToken(ctx context.Context, pool *sql.DB) (*oauth2.Token, error) {
	v, err := getConfig(ctx, pool, tokenConfigKey)
	if err != nil {
		return nil, err
	}
	if v == "" {
		return nil, nil
	}
	var tok oauth2.Token
	if err := json.Unmarshal([]byte(v), &tok); err != nil {
		return nil, fmt.Errorf("unmarshal token: %w", err)
	}
	return &tok, nil
}

// DeleteToken removes the stored OAuth2 token.
func DeleteToken(ctx context.Context, pool *sql.DB) error {
	_, err := pool.ExecContext(ctx, `DELETE FROM app_configuration WHERE key = ?1`, tokenConfigKey)
	return err
}

// SaveState stores the ephemeral CSRF state string.
func SaveState(ctx context.Context, pool *sql.DB, state string) error {
	return upsertConfig(ctx, pool, stateConfigKey, state)
}

// LoadState retrieves the stored CSRF state string. Returns "" if absent.
func LoadState(ctx context.Context, pool *sql.DB) (string, error) {
	return getConfig(ctx, pool, stateConfigKey)
}

// DeleteState removes the stored CSRF state.
func DeleteState(ctx context.Context, pool *sql.DB) error {
	_, err := pool.ExecContext(ctx, `DELETE FROM app_configuration WHERE key = ?1`, stateConfigKey)
	return err
}

// SaveOAuthPopup records that the in-flight OAuth flow was started from a browser popup.
func SaveOAuthPopup(ctx context.Context, pool *sql.DB) error {
	return upsertConfig(ctx, pool, popupConfigKey, "1")
}

// ConsumeOAuthPopup returns whether popup mode was set and clears the flag.
func ConsumeOAuthPopup(ctx context.Context, pool *sql.DB) (bool, error) {
	v, err := getConfig(ctx, pool, popupConfigKey)
	if err != nil {
		return false, err
	}
	_ = deleteConfigKey(ctx, pool, popupConfigKey)
	return strings.TrimSpace(v) == "1", nil
}

func deleteConfigKey(ctx context.Context, pool *sql.DB, key string) error {
	_, err := pool.ExecContext(ctx, `DELETE FROM app_configuration WHERE key = ?1`, key)
	return err
}

// ── helpers ──────────────────────────────────────────────────────────────────

func upsertConfig(ctx context.Context, pool *sql.DB, key, value string) error {
	// Global config rows use user_id IS NULL; the unique constraint is partial
	// (uq_app_config_global), so ON CONFLICT must specify the same predicate.
	_, err := pool.ExecContext(ctx,
		`INSERT INTO app_configuration (key, value, user_id)
		 VALUES (?1, ?2, NULL)
		 ON CONFLICT (key) WHERE user_id IS NULL
		 DO UPDATE SET value = EXCLUDED.value, updated_at = CURRENT_TIMESTAMP`,
		key, value,
	)
	return err
}

func getConfig(ctx context.Context, pool *sql.DB, key string) (string, error) {
	var v *string
	err := pool.QueryRowContext(ctx,
		`SELECT value FROM app_configuration WHERE key = ?1`, key,
	).Scan(&v)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", nil
		}
		return "", err
	}
	if v == nil {
		return "", nil
	}
	return *v, nil
}
