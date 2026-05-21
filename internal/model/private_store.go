package model

import "github.com/daveontour/aimuseum/internal/sqlutil"

// PrivateStoreEntry is the internal representation of a private_store row.
// Value is the plaintext string after decryption.
type PrivateStoreEntry struct {
	ID        int64
	Key       string
	Value     string
	CreatedAt sqlutil.DBTime
	UpdatedAt sqlutil.DBTime
}

// PrivateStoreResponse is the JSON-serialisable form returned by the API.
type PrivateStoreResponse struct {
	ID        int64  `json:"id"`
	Key       string `json:"key"`
	Value     string `json:"value"`
	CreatedAt any    `json:"created_at"`
	UpdatedAt any    `json:"updated_at"`
}
