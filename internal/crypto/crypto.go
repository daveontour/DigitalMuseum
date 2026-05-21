// Package crypto provides the encryption primitives used by the sensitive-data
// feature (Argon2id key derivation for the pgcrypto keyring).
package crypto

import (
	_ "embed"

	"golang.org/x/crypto/argon2"
)

//go:embed seed.txt
var staticSeed []byte

// DeriveKey derives a 32-byte AES key from password + pepper using Argon2id.
// Parameters mirror the original exactly: 1 iteration, 64 MB, 4 threads.
func DeriveKey(password, pepper string) []byte {
	combined := []byte(password + pepper)
	salt := staticSeed[:16]
	return argon2.IDKey(combined, salt, 1, 64*1024, 4, 32)
}
