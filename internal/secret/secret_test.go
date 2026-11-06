// SPDX-License-Identifier: AGPL-3.0-only

package secret

import (
	"bytes"
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testBox(t *testing.T, fill byte) *Box {
	t.Helper()
	key := bytes.Repeat([]byte{fill}, KeySize)
	box, err := NewBox(key)
	require.NoError(t, err)
	return box
}

// Round trip: what Seal produces, Open returns, with the same aad.
func TestSealOpenRoundTrip(t *testing.T) {
	box := testBox(t, 1)
	sealed, err := box.Seal([]byte("hunter2"), []byte("row-1"))
	require.NoError(t, err)
	assert.NotContains(t, sealed, "hunter2", "ciphertext must not contain the plaintext")

	plain, err := box.Open(sealed, []byte("row-1"))
	require.NoError(t, err)
	assert.Equal(t, "hunter2", string(plain))
}

// Two seals of the same plaintext differ (random nonce).
func TestSealUsesFreshNonce(t *testing.T) {
	box := testBox(t, 1)
	a, err := box.Seal([]byte("same"), nil)
	require.NoError(t, err)
	b, err := box.Seal([]byte("same"), nil)
	require.NoError(t, err)
	assert.NotEqual(t, a, b)
}

// A value opened with a different key fails closed with ErrOpen.
func TestOpenWrongKeyFailsClosed(t *testing.T) {
	sealed, err := testBox(t, 1).Seal([]byte("secret"), nil)
	require.NoError(t, err)

	plain, err := testBox(t, 2).Open(sealed, nil)
	assert.ErrorIs(t, err, ErrOpen)
	assert.Nil(t, plain)
}

// Changing one byte of the ciphertext fails closed.
func TestOpenTamperedFailsClosed(t *testing.T) {
	box := testBox(t, 1)
	sealed, err := box.Seal([]byte("secret"), nil)
	require.NoError(t, err)

	raw, err := base64.StdEncoding.DecodeString(sealed)
	require.NoError(t, err)
	raw[len(raw)-1] ^= 0x01
	tampered := base64.StdEncoding.EncodeToString(raw)

	plain, err := box.Open(tampered, nil)
	assert.ErrorIs(t, err, ErrOpen)
	assert.Nil(t, plain)
}

// Truncated ciphertext, empty input and non-base64 input all return ErrOpen
// and never panic.
func TestOpenTruncatedFailsClosed(t *testing.T) {
	box := testBox(t, 1)
	sealed, err := box.Seal([]byte("secret"), nil)
	require.NoError(t, err)
	raw, err := base64.StdEncoding.DecodeString(sealed)
	require.NoError(t, err)

	cases := map[string]string{
		"empty":        "",
		"nonce only":   base64.StdEncoding.EncodeToString(raw[:12]),
		"short tag":    base64.StdEncoding.EncodeToString(raw[:len(raw)-1]),
		"half":         base64.StdEncoding.EncodeToString(raw[:len(raw)/2]),
		"not base64":   "!!!not-base64!!!",
		"nonce+tag -1": base64.StdEncoding.EncodeToString(raw[:12+16-1]),
	}
	for name, input := range cases {
		t.Run(name, func(t *testing.T) {
			require.NotPanics(t, func() {
				plain, err := box.Open(input, nil)
				assert.ErrorIs(t, err, ErrOpen)
				assert.Nil(t, plain)
			})
		})
	}
}

// Additional data binds the value to its row: a different aad fails.
func TestOpenWrongAADFailsClosed(t *testing.T) {
	box := testBox(t, 1)
	sealed, err := box.Seal([]byte("secret"), []byte("row-1"))
	require.NoError(t, err)

	_, err = box.Open(sealed, []byte("row-2"))
	assert.ErrorIs(t, err, ErrOpen)
}

func TestNewBoxRejectsBadKeyLength(t *testing.T) {
	_, err := NewBox(make([]byte, 16))
	assert.Error(t, err)
}

func TestParseKey(t *testing.T) {
	good := base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{7}, KeySize))

	key, err := ParseKey("  " + good + "\n")
	require.NoError(t, err)
	assert.Len(t, key, KeySize)

	_, err = ParseKey(base64.StdEncoding.EncodeToString([]byte("short")))
	assert.Error(t, err)

	_, err = ParseKey("%%%")
	require.Error(t, err)
	assert.NotContains(t, err.Error(), "%%%", "the error must not echo the key")
}

// The key file is created on first use with mode 0600 and reused afterwards.
func TestLoadOrCreateKeyCreatesOnceWith0600(t *testing.T) {
	path := filepath.Join(t.TempDir(), "storage.key")

	first, err := LoadOrCreateKey(path)
	require.NoError(t, err)
	assert.Len(t, first, KeySize)

	info, err := os.Stat(path)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())

	second, err := LoadOrCreateKey(path)
	require.NoError(t, err)
	assert.Equal(t, first, second, "a second load must read the same key")
}

// A corrupt key file is an error, not a new key silently replacing it.
func TestLoadOrCreateKeyRejectsCorruptFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "storage.key")
	require.NoError(t, os.WriteFile(path, []byte("garbage"), 0o600))

	_, err := LoadOrCreateKey(path)
	assert.Error(t, err)

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, "garbage", string(data), "the corrupt file must be left alone")
}

// Source uses SHRINK_STORAGE_KEY when set and never touches the key file.
func TestSourceEnvKeyWins(t *testing.T) {
	envKey := base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{9}, KeySize))
	path := filepath.Join(t.TempDir(), "storage.key")

	src := NewSource(envKey, path)
	box, err := src.Box()
	require.NoError(t, err)

	sealed, err := box.Seal([]byte("pw"), nil)
	require.NoError(t, err)
	// The same key, built directly, opens it: the env key was used.
	direct, err := NewBox(bytes.Repeat([]byte{9}, KeySize))
	require.NoError(t, err)
	plain, err := direct.Open(sealed, nil)
	require.NoError(t, err)
	assert.Equal(t, "pw", string(plain))

	_, statErr := os.Stat(path)
	assert.True(t, os.IsNotExist(statErr), "no key file when the env key is set")
}

// Source without an env key creates the key file on first use only.
func TestSourceCreatesKeyFileLazily(t *testing.T) {
	path := filepath.Join(t.TempDir(), "storage.key")
	src := NewSource("", path)

	_, statErr := os.Stat(path)
	assert.True(t, os.IsNotExist(statErr), "nothing is created before first use")

	box, err := src.Box()
	require.NoError(t, err)
	_, err = os.Stat(path)
	require.NoError(t, err, "first use creates the key file")

	again, err := src.Box()
	require.NoError(t, err)
	assert.Same(t, box, again, "the resolved box is cached")
}

// A bad env key fails every call with the same error and no key text.
func TestSourceBadEnvKeyFailsClosed(t *testing.T) {
	src := NewSource("not-a-key", filepath.Join(t.TempDir(), "storage.key"))

	_, err1 := src.Box()
	require.Error(t, err1)
	_, err2 := src.Box()
	assert.Equal(t, err1, err2)
	assert.False(t, strings.Contains(err1.Error(), "not-a-key"))
}
