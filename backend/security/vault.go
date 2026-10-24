package security

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"errors"
	"io"
)

// Vault encrypts small secrets (credential header values) before they are
// written to SQLite, so the database never holds them in plaintext.
//
// The 256-bit key lives in a separate 0600 file in the data directory, or,
// once a master password is set, only wrapped with a key derived from that
// password (see password.go).
type Vault struct {
	aead cipher.AEAD
}

const keyFileName = "secret.key"

// OpenVault loads or creates the plain key file in dir (no master password).
func OpenVault(dir string) (*Vault, error) {
	key, err := readOrCreateKeyFile(dir)
	if err != nil {
		return nil, err
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
