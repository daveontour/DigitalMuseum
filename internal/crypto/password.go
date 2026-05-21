package crypto

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"strings"

	"golang.org/x/crypto/argon2"
)

// Argon2id parameters for user account passwords.
// These are separate from DeriveKey (keyring DEK derivation) which uses
// different parameters for compatibility with the original Python importer.
const (
	pwArgon2Time    uint32 = 3
	pwArgon2Memory  uint32 = 64 * 1024 // 64 MB
	pwArgon2Threads uint8  = 4
	pwArgon2KeyLen  uint32 = 32
	pwArgon2SaltLen        = 16
)

// HashPassword hashes a plaintext password using Argon2id with a random salt.
// The returned string encodes all Argon2 parameters and is safe to store
// directly in the users.password_hash column.
func HashPassword(password string) (string, error) {
	salt := make([]byte, pwArgon2SaltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("generate salt: %w", err)
	}
	hash := argon2.IDKey(
		[]byte(password), salt,
		pwArgon2Time, pwArgon2Memory, pwArgon2Threads, pwArgon2KeyLen,
	)
	return fmt.Sprintf(
		"$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version,
		pwArgon2Memory, pwArgon2Time, pwArgon2Threads,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(hash),
	), nil
}

// VerifyPassword checks whether password matches a hash produced by HashPassword.
// Returns (false, nil) when the hash format is invalid or the password does not
// match — it does not distinguish these cases to prevent user enumeration.
// A timing-safe comparison is used to guard against timing attacks.
func VerifyPassword(password, encodedHash string) (bool, error) {
	// Expected format: $argon2id$v=19$m=65536,t=3,p=4$<salt>$<hash>
	parts := strings.Split(encodedHash, "$")
	if len(parts) != 6 || parts[1] != "argon2id" {
		return false, nil
	}

	var version int
	if _, err := fmt.Sscanf(parts[2], "v=%d", &version); err != nil {
		return false, nil
	}

	var m, t uint32
	var p uint8
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &m, &t, &p); err != nil {
		return false, nil
	}

	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		return false, nil
	}
	expectedHash, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil {
		return false, nil
	}

	candidate := argon2.IDKey([]byte(password), salt, t, m, p, uint32(len(expectedHash)))
	return subtle.ConstantTimeCompare(candidate, expectedHash) == 1, nil
}
