// SPDX-License-Identifier: AGPL-3.0-only

package server

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHashPasswordIsArgon2id(t *testing.T) {
	hash, err := HashPassword("hunter2")
	require.NoError(t, err)
	require.True(t, strings.HasPrefix(hash, Argon2Prefix), "stored hash must be argon2id, got %q", hash)
	assert.Contains(t, hash, "v=19")
	assert.NotContains(t, hash, "hunter2", "hash must not contain the password")

	// Two hashes of the same password differ (random salt).
	other, err := HashPassword("hunter2")
	require.NoError(t, err)
	assert.NotEqual(t, hash, other)
}

func TestVerifyPassword(t *testing.T) {
	hash, err := HashPassword("correct horse")
	require.NoError(t, err)
	assert.True(t, VerifyPassword(hash, "correct horse"))
	assert.False(t, VerifyPassword(hash, "wrong"))
	// Garbage and foreign formats never verify.
	assert.False(t, VerifyPassword("", "x"))
	assert.False(t, VerifyPassword("$2y$10$whatever", "x"))
	assert.False(t, VerifyPassword(Argon2Prefix+"bogus", "x"))
}
