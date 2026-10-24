package security

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// Vault encrypts small secrets (credential header values) before they are
// written to SQLite, so the database never holds them in plaintext.
//
// The 256-bit key lives in a separate file with 0600 permissions inside the
// application data directory. This protects against casual disclosure of the
// database file (backups, sharing, sync) but not against an attacker who can
// read the user's files; OS keychain integration is a documented follow-up.
type Vault struct {
	aead cipher.AEAD
}

const keyFileName = "secret.key"

// OpenVault loads or creates the key in dir.
func OpenVault(dir string) (*Vault, error) {
	path := filepath.Join(dir, keyFileName)
	key, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		key = make([]byte, 32)
		if _, err := rand.Read(key); err != nil {
			return nil, fmt.Errorf("generate key: %w", err)
		}
		if err := os.WriteFile(path, key, 0o600); err != nil {
			return nil, fmt.Errorf("write key: %w", err)
		}
	} else if err != nil {
		return nil, fmt.Errorf("read key: %w", err)
	}
	if len(key) != 32 {
		return nil, fmt.Errorf("key file %s is corrupt (expected 32 bytes, got %d)", path, len(key))
	}
	return NewVault(key)
}

// NewVault creates a vault from a raw 32-byte key.
func NewVault(key []byte) (*Vault, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return &Vault{aead: aead}, nil
}

// Seal encrypts plaintext. The nonce is prepended to the ciphertext.
func (v *Vault) Seal(plaintext []byte) ([]byte, error) {
	nonce := make([]byte, v.aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}
	return v.aead.Seal(nonce, nonce, plaintext, nil), nil
}

// Open decrypts data produced by Seal.
func (v *Vault) Open(data []byte) ([]byte, error) {
	n := v.aead.NonceSize()
	if len(data) < n {
		return nil, errors.New("ciphertext too short")
	}
	return v.aead.Open(nil, data[:n], data[n:], nil)
}
