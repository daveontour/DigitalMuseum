// Package gmail provides OAuth2 token management and a Gmail API client.
package gmail

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
)

const (
	tokenConfigKey = "GMAIL_OAUTH_TOKEN"
	stateConfigKey = "GMAIL_OAUTH_STATE"
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
func SaveToken(ctx context.Context, pool *pgxpool.Pool, tok *oauth2.Token) error {
	b, err := json.Marshal(tok)
	if err != nil {
		return fmt.Errorf("marshal token: %w", err)
	}
	v := string(b)
	return upsertConfig(ctx, pool, tokenConfigKey, v)
}

// LoadToken retrieves and deserialises the stored token.
// Returns nil, nil if no token has been saved.
func LoadToken(ctx context.Context, pool *pgxpool.Pool) (*oauth2.Token, error) {
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
func DeleteToken(ctx context.Context, pool *pgxpool.Pool) error {
	_, err := pool.Exec(ctx, `DELETE FROM app_configuration WHERE key = $1`, tokenConfigKey)
	return err
}

// SaveState stores the ephemeral CSRF state string.
func SaveState(ctx context.Context, pool *pgxpool.Pool, state string) error {
	return upsertConfig(ctx, pool, stateConfigKey, state)
}

// LoadState retrieves the stored CSRF state string. Returns "" if absent.
func LoadState(ctx context.Context, pool *pgxpool.Pool) (string, error) {
	return getConfig(ctx, pool, stateConfigKey)
}

// DeleteState removes the stored CSRF state.
func DeleteState(ctx context.Context, pool *pgxpool.Pool) error {
	_, err := pool.Exec(ctx, `DELETE FROM app_configuration WHERE key = $1`, stateConfigKey)
	return err
}

// ── helpers ──────────────────────────────────────────────────────────────────

func upsertConfig(ctx context.Context, pool *pgxpool.Pool, key, value string) error {
	_, err := pool.Exec(ctx,
		`INSERT INTO app_configuration (key, value)
		 VALUES ($1, $2)
		 ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value, updated_at = NOW()`,
		key, value,
	)
	return err
}

func getConfig(ctx context.Context, pool *pgxpool.Pool, key string) (string, error) {
	var v *string
	err := pool.QueryRow(ctx,
		`SELECT value FROM app_configuration WHERE key = $1`, key,
	).Scan(&v)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", nil
		}
		return "", err
	}
	if v == nil {
		return "", nil
	}
	return *v, nil
}
