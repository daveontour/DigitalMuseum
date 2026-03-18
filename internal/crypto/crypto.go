// Package crypto provides the encryption primitives used by the sensitive-data
// feature. DeriveKey, Encrypt, Decrypt, and EncodedPassword are kept for
// migration support (used by legacyGetPrivateKey in keys.go). DecryptWithPrivateKeyHybrid
// is kept to decrypt old RSA-format records during migration.
package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	_ "embed"
	"encoding/base64"
	"encoding/pem"
	"errors"
	"fmt"

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

// Encrypt encrypts plainText using AES-256-GCM and returns base64 ciphertext.
// The nonce is fixed (bytes 16-28 of the seed) to produce deterministic output.
// Kept for migration support only.
func Encrypt(plainText, password, pepper string) (string, error) {
	key := DeriveKey(password, pepper)
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := staticSeed[16:28]
	ct := gcm.Seal(nil, nonce, []byte(plainText), nil)
	return base64.StdEncoding.EncodeToString(ct), nil
}

// Decrypt decrypts a base64-encoded AES-256-GCM ciphertext.
// Kept for migration support only.
func Decrypt(cryptoText, password, pepper string) (string, error) {
	data, err := base64.StdEncoding.DecodeString(cryptoText)
	if err != nil {
		return "", err
	}
	key := DeriveKey(password, pepper)
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := staticSeed[16:28]
	plain, err := gcm.Open(nil, nonce, data, nil)
	if err != nil {
		return "", errors.New("decryption failed: wrong password or tampered data")
	}
	return string(plain), nil
}

// EncodedPassword applies the triple-encryption used to protect user passwords
// before they are used as encryption keys for trusted-key records.
// Kept for migration support only.
func EncodedPassword(password, pepper string) string {
	p1, _ := Encrypt(password, password, pepper)
	p2, _ := Encrypt(p1, string(staticSeed[:16]), pepper)
	p3, _ := Encrypt(p2, p2, pepper)
	return p3
}

// DecryptWithPrivateKeyHybrid decrypts data produced by the old EncryptWithPublicKeyHybrid.
// Format: [RSA-encrypted AES key (256 bytes)][GCM nonce (12 bytes)][ciphertext].
// Kept for migration support only.
func DecryptWithPrivateKeyHybrid(privateKeyPEM []byte, data []byte) ([]byte, error) {
	priv, err := parseRSAPrivateKey(privateKeyPEM)
	if err != nil {
		return nil, err
	}
	const rsaEncLen = 256
	const gcmNonceLen = 12
	if len(data) < rsaEncLen+gcmNonceLen {
		return nil, fmt.Errorf("ciphertext too short")
	}
	hash := sha256.New()
	aesKey, err := rsa.DecryptOAEP(hash, rand.Reader, priv, data[:rsaEncLen], nil)
	if err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(aesKey)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonce := data[rsaEncLen : rsaEncLen+gcmNonceLen]
	return gcm.Open(nil, nonce, data[rsaEncLen+gcmNonceLen:], nil)
}

func parseRSAPrivateKey(privateKeyPEM []byte) (*rsa.PrivateKey, error) {
	block, _ := pem.Decode(privateKeyPEM)
	if block == nil {
		return nil, fmt.Errorf("failed to decode PEM block")
	}
	return x509.ParsePKCS1PrivateKey(block.Bytes)
}
