// SPDX-License-Identifier: AGPL-3.0-only

// Package secret encrypts values at rest (storage passwords, spec 4.3). It
// uses AES-256-GCM from the standard library and a 32 byte key that comes
// from SHRINK_STORAGE_KEY or from a key file in the data dir, created on
// first use with mode 0600.
package secret

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
)

// KeySize is the AES-256 key length in bytes.
const KeySize = 32

// ErrOpen is returned when a ciphertext does not authenticate: the wrong key,
// a tampered value, a truncated value or a mismatched additional data. The
// caller gets no partial plaintext.
var ErrOpen = errors.New("secret: cannot decrypt value (wrong key, tampered or truncated)")

// Box seals and opens values with one key. It is safe for concurrent use.
type Box struct {
	aead cipher.AEAD
}

// NewBox builds a Box from a 32 byte key.
func NewBox(key []byte) (*Box, error) {
	if len(key) != KeySize {
		return nil, fmt.Errorf("secret: key must be %d bytes, got %d", KeySize, len(key))
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("secret: cipher: %w", err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("secret: gcm: %w", err)
	}
	return &Box{aead: aead}, nil
}

// Seal encrypts plaintext and returns base64(nonce || ciphertext). aad is
// authenticated but not encrypted: a value sealed for one row cannot be
// opened as another row's value when the caller passes the row id as aad.
func (b *Box) Seal(plaintext, aad []byte) (string, error) {
	nonce := make([]byte, b.aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("secret: nonce: %w", err)
	}
	sealed := b.aead.Seal(nonce, nonce, plaintext, aad)
	return base64.StdEncoding.EncodeToString(sealed), nil
}

// Open decrypts a value produced by Seal with the same aad. Any failure
// returns ErrOpen and no plaintext.
func (b *Box) Open(encoded string, aad []byte) ([]byte, error) {
	raw, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, ErrOpen
	}
	ns := b.aead.NonceSize()
	if len(raw) < ns+b.aead.Overhead() {
		return nil, ErrOpen
	}
	plain, err := b.aead.Open(nil, raw[:ns], raw[ns:], aad)
	if err != nil {
		return nil, ErrOpen
	}
	return plain, nil
}

// ParseKey decodes a base64 key (SHRINK_STORAGE_KEY). Surrounding white space
// is ignored. The error never includes the key text.
func ParseKey(encoded string) ([]byte, error) {
	key, err := base64.StdEncoding.DecodeString(strings.TrimSpace(encoded))
	if err != nil {
		return nil, errors.New("secret: storage key is not valid base64")
	}
	if len(key) != KeySize {
		return nil, fmt.Errorf("secret: storage key must decode to %d bytes, got %d", KeySize, len(key))
	}
	return key, nil
}

// LoadOrCreateKey reads the base64 key file at path. When the file does not
// exist it generates a random key and writes it with mode 0600, failing if
// another writer created the file first.
func LoadOrCreateKey(path string) ([]byte, error) {
	data, err := os.ReadFile(path)
	if err == nil {
		return ParseKey(string(data))
	}
	if !os.IsNotExist(err) {
		return nil, fmt.Errorf("secret: read key file: %w", err)
	}
	key := make([]byte, KeySize)
	if _, err := io.ReadFull(rand.Reader, key); err != nil {
		return nil, fmt.Errorf("secret: generate key: %w", err)
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		if os.IsExist(err) {
			// Lost a race with another creator: use the key it wrote.
			return LoadOrCreateKey(path)
		}
		return nil, fmt.Errorf("secret: create key file: %w", err)
	}
	encoded := base64.StdEncoding.EncodeToString(key) + "\n"
	if _, err := f.WriteString(encoded); err != nil {
		_ = f.Close()
		return nil, fmt.Errorf("secret: write key file: %w", err)
	}
	if err := f.Close(); err != nil {
		return nil, fmt.Errorf("secret: close key file: %w", err)
	}
	return key, nil
}

// Source resolves the Box on first use and caches it. The key is
// SHRINK_STORAGE_KEY when set, otherwise the key file at KeyPath, which is
// created if missing. A failed resolution is cached too, so a bad key gives
// the same clear error on every call instead of a retry storm.
type Source struct {
	envKey  string
	keyPath string

	once sync.Once
	box  *Box
	err  error
}

// NewSource builds a lazy key source. envKey is the raw SHRINK_STORAGE_KEY
// value (empty means use the key file); keyPath is the key file path.
func NewSource(envKey, keyPath string) *Source {
	return &Source{envKey: envKey, keyPath: keyPath}
}

// Box returns the sealing box, resolving the key on the first call.
func (s *Source) Box() (*Box, error) {
	s.once.Do(func() {
		var key []byte
		if strings.TrimSpace(s.envKey) != "" {
			key, s.err = ParseKey(s.envKey)
		} else {
			key, s.err = LoadOrCreateKey(s.keyPath)
		}
		if s.err != nil {
			return
		}
		s.box, s.err = NewBox(key)
	})
	return s.box, s.err
}
