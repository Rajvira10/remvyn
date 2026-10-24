package security

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"golang.org/x/crypto/argon2"
)

// ErrWrongPassword means a master password did not unwrap the key.
var ErrWrongPassword = errors.New("wrong master password")

// MinPasswordLength is the shortest accepted master password.
const MinPasswordLength = 8

// KDF describes how a key-encryption key is derived from a password.
type KDF struct {
	Alg       string `json:"alg"` // argon2id
	Time      uint32 `json:"time"`
	MemoryKiB uint32 `json:"memoryKiB"`
	Threads   uint8  `json:"threads"`
	Salt      []byte `json:"salt"`
}

// WrappedKey is a 32-byte key encrypted with a password-derived key. It is
// safe to store in the open (locally, or in a sync repository).
type WrappedKey struct {
	KDF     KDF    `json:"kdf"`
	Wrapped []byte `json:"wrapped"`
}

// DefaultKDF returns fresh Argon2id parameters with a random salt.
func DefaultKDF() (KDF, error) {
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return KDF{}, err
	}
	return KDF{Alg: "argon2id", Time: 3, MemoryKiB: 64 * 1024, Threads: 4, Salt: salt}, nil
}

func (k KDF) derive(password string) ([]byte, error) {
	if k.Alg != "argon2id" {
		return nil, fmt.Errorf("unsupported key derivation %q", k.Alg)
	}
	if k.Time == 0 || k.Time > 20 || k.MemoryKiB < 8*1024 || k.MemoryKiB > 1024*1024 || k.Threads == 0 || len(k.Salt) < 16 {
		return nil, errors.New("invalid key derivation parameters")
	}
	return argon2.IDKey([]byte(password), k.Salt, k.Time, k.MemoryKiB, k.Threads, 32), nil
}

// WrapKey encrypts key with password.
func WrapKey(key []byte, password string) (WrappedKey, error) {
	if len(key) != 32 {
		return WrappedKey{}, errors.New("key must be 32 bytes")
	}
	kdf, err := DefaultKDF()
	if err != nil {
		return WrappedKey{}, err
	}
	kek, err := kdf.derive(password)
	if err != nil {
		return WrappedKey{}, err
	}
	v, err := NewVault(kek)
	if err != nil {
		return WrappedKey{}, err
	}
	sealed, err := v.Seal(key)
	if err != nil {
		return WrappedKey{}, err
	}
	return WrappedKey{KDF: kdf, Wrapped: sealed}, nil
}

// Unwrap decrypts the key with password.
func (w WrappedKey) Unwrap(password string) ([]byte, error) {
	kek, err := w.KDF.derive(password)
	if err != nil {
		return nil, err
	}
	v, err := NewVault(kek)
	if err != nil {
		return nil, err
	}
	key, err := v.Open(w.Wrapped)
	if err != nil || len(key) != 32 {
		return nil, ErrWrongPassword
	}
	return key, nil
}

// NewKey returns a random 32-byte key.
func NewKey() ([]byte, error) {
	k := make([]byte, 32)
	_, err := rand.Read(k)
	return k, err
}

// ---------------------------------------------------------------- local key store

// The data key lives either in secret.key (no master password) or wrapped
// in vault.json (master password set). Exactly one of the two exists.
const lockFileName = "vault.json"

type lockFile struct {
	Version int        `json:"version"`
	Key     WrappedKey `json:"key"`
}

// KeyState reports how the local data key is stored.
type KeyState struct {
	// Protected is true when a master password is set.
	Protected bool
	// Key is the data key when it is available without a password.
	Key []byte
}

// LoadKeyState reads the key files in dir, creating an unprotected key on
// first run.
func LoadKeyState(dir string) (KeyState, error) {
	if _, err := os.Stat(filepath.Join(dir, lockFileName)); err == nil {
		return KeyState{Protected: true}, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return KeyState{}, err
	}
	key, err := readOrCreateKeyFile(dir)
	if err != nil {
		return KeyState{}, err
	}
	return KeyState{Key: key}, nil
}

func readOrCreateKeyFile(dir string) ([]byte, error) {
	path := filepath.Join(dir, keyFileName)
	key, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		if key, err = NewKey(); err != nil {
			return nil, fmt.Errorf("generate key: %w", err)
		}
		if err := writeFileAtomic(path, key); err != nil {
			return nil, fmt.Errorf("write key: %w", err)
		}
	} else if err != nil {
		return nil, fmt.Errorf("read key: %w", err)
	}
	if len(key) != 32 {
		return nil, fmt.Errorf("key file %s is corrupt (expected 32 bytes, got %d)", path, len(key))
	}
	return key, nil
}

// UnlockKey returns the data key protected by the master password.
func UnlockKey(dir, password string) ([]byte, error) {
	raw, err := os.ReadFile(filepath.Join(dir, lockFileName))
	if err != nil {
		return nil, err
	}
	var lf lockFile
	if err := json.Unmarshal(raw, &lf); err != nil {
		return nil, fmt.Errorf("vault.json is corrupt: %w", err)
	}
	return lf.Key.Unwrap(password)
}

// ProtectKey stores key wrapped with password and removes the plain key
// file. It also re-wraps an already protected key (password change).
func ProtectKey(dir string, key []byte, password string) error {
	if len(password) < MinPasswordLength {
		return fmt.Errorf("master password must be at least %d characters", MinPasswordLength)
	}
	w, err := WrapKey(key, password)
	if err != nil {
		return err
	}
	raw, err := json.MarshalIndent(lockFile{Version: 1, Key: w}, "", "  ")
	if err != nil {
		return err
	}
	if err := writeFileAtomic(filepath.Join(dir, lockFileName), raw); err != nil {
		return err
	}
	if err := os.Remove(filepath.Join(dir, keyFileName)); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

// UnprotectKey writes the plain key file and removes vault.json.
func UnprotectKey(dir string, key []byte) error {
	if err := writeFileAtomic(filepath.Join(dir, keyFileName), key); err != nil {
		return err
	}
	if err := os.Remove(filepath.Join(dir, lockFileName)); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

// ResetKeys deletes both key files (used when wiping a vault whose password
// was forgotten).
func ResetKeys(dir string) error {
	for _, name := range []string{lockFileName, keyFileName} {
		if err := os.Remove(filepath.Join(dir, name)); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	return nil
}

// KeysEqual compares two keys in constant time.
func KeysEqual(a, b []byte) bool { return subtle.ConstantTimeCompare(a, b) == 1 }

func writeFileAtomic(path string, data []byte) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".tmp-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}
