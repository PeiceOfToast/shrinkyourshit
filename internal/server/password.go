// SPDX-License-Identifier: AGPL-3.0-only

package server

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"

	"golang.org/x/crypto/argon2"
)

// Password hashing (STACK.md: argon2id). Hashes are stored in PHC string
// form on the admin_password_hash setting:
//
//	$argon2id$v=19$m=65536,t=2,p=1$<salt-b64>$<hash-b64>
//
// The hash never leaves the server: no API response or log may contain it
// (task 06 acceptance).

// Argon2Prefix is the PHC prefix of every hash this package produces.
const Argon2Prefix = "$argon2id$"

// argon2 parameters. Memory in KiB: 64 MiB, 2 passes, 1 lane — within the
// OWASP recommended range for interactive logins.
const (
	argon2MemoryKiB = 64 * 1024
	argon2Time      = 2
	argon2Lanes     = 1
	argon2KeyLen    = 32
	argon2SaltLen   = 16
)

// ErrPasswordMismatch is returned when a password does not verify. The API
// maps it to 401 and never includes the attempted password in a response.
var ErrPasswordMismatch = errors.New("auth: password does not match")

// HashPassword hashes a password with argon2id and returns the PHC string.
func HashPassword(password string) (string, error) {
	salt := make([]byte, argon2SaltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("auth: salt: %w", err)
	}
	key := argon2.IDKey([]byte(password), salt, argon2Time, argon2MemoryKiB, argon2Lanes, argon2KeyLen)
	b64 := base64.RawStdEncoding
	return fmt.Sprintf("%sv=19$m=%d,t=%d,p=%d$%s$%s",
		Argon2Prefix, argon2MemoryKiB, argon2Time, argon2Lanes,
		b64.EncodeToString(salt), b64.EncodeToString(key)), nil
}

// VerifyPassword reports whether password matches the PHC-formatted hash.
// An unparseable or foreign-format hash is a mismatch, not a crash.
func VerifyPassword(hash, password string) bool {
	hash = strings.TrimSpace(hash)
	if !strings.HasPrefix(hash, Argon2Prefix) {
		return false
	}
	parts := strings.Split(hash, "$")
	// parts[0] is empty (leading $): "", "argon2id", "v=19", "m=..,t=..,p=..", salt, hash.
	if len(parts) != 6 {
		return false
	}
	var memory, time uint32
	var lanes uint8
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &memory, &time, &lanes); err != nil {
		return false
	}
	b64 := base64.RawStdEncoding
	salt, err := b64.DecodeString(parts[4])
	if err != nil {
		return false
	}
	want, err := b64.DecodeString(parts[5])
	if err != nil {
		return false
	}
	got := argon2.IDKey([]byte(password), salt, time, memory, lanes, uint32(len(want)))
	return subtle.ConstantTimeCompare(got, want) == 1
}
