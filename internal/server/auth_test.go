// SPDX-License-Identifier: AGPL-3.0-only

package server

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/PeiceOfToast/shrinkyourshit/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// authHarness is a store plus the auth service over it.
type authHarness struct {
	t    *testing.T
	st   store.Store
	auth *Auth
	log  *bytes.Buffer // every log line, for the never-log assertions
}

func newAuthHarness(t *testing.T) *authHarness {
	t.Helper()
	st, err := store.New(t.TempDir() + "/test.db")
	require.NoError(t, err)
	t.Cleanup(func() { _ = st.Close() })
	buf := &bytes.Buffer{}
	log := slog.New(slog.NewTextHandler(buf, nil))
	return &authHarness{t: t, st: st, auth: NewAuth(st, log), log: buf}
}

func (h *authHarness) setupLoginEnabled(user, pass string) {
	h.t.Helper()
	require.NoError(h.t, h.auth.CompleteSetup(context.Background(), SetupParams{
		Mode: SetupModeServer, LoginEnabled: true, Username: user, Password: pass,
	}))
}

func (h *authHarness) hash(t *testing.T) string {
	t.Helper()
	v, ok, err := h.st.GetSetting(context.Background(), store.SettingAdminPasswordHash)
	require.NoError(t, err)
	require.True(t, ok, "password hash setting exists")
	return v
}

func TestSetupStatusFreshStore(t *testing.T) {
	h := newAuthHarness(t)
	status, err := h.auth.Status(context.Background())
	require.NoError(t, err)
	assert.False(t, status.SetupComplete, "fresh data dir: setup incomplete")
	assert.False(t, status.LoginEnabled)
}

func TestCompleteSetupWithLogin(t *testing.T) {
	h := newAuthHarness(t)
	ctx := context.Background()
	require.NoError(t, h.auth.CompleteSetup(ctx, SetupParams{
		Mode: SetupModeServer, LoginEnabled: true, Username: "admin", Password: "secret",
	}))

	status, err := h.auth.Status(ctx)
	require.NoError(t, err)
	assert.True(t, status.SetupComplete)
	assert.True(t, status.LoginEnabled)
	assert.Equal(t, "admin", status.Username)

	// The stored hash is argon2id, never the raw password.
	hash := h.hash(t)
	assert.True(t, strings.HasPrefix(hash, Argon2Prefix), "hash prefix must be argon2id")
	assert.NotContains(t, hash, "secret")

	// A second POST /setup is refused.
	err = h.auth.CompleteSetup(ctx, SetupParams{Mode: SetupModeServer})
	require.ErrorIs(t, err, ErrSetupComplete)
}

func TestCompleteSetupLoginDisabled(t *testing.T) {
	h := newAuthHarness(t)
	ctx := context.Background()
	require.NoError(t, h.auth.CompleteSetup(ctx, SetupParams{Mode: SetupModeServer, LoginEnabled: false}))

	status, err := h.auth.Status(ctx)
	require.NoError(t, err)
	assert.True(t, status.SetupComplete)
	assert.False(t, status.LoginEnabled)
	assert.Empty(t, status.Username, "no account exists")

	// No password hash is written.
	_, ok, err := h.st.GetSetting(ctx, store.SettingAdminPasswordHash)
	require.NoError(t, err)
	assert.False(t, ok)
}

func TestCompleteSetupValidation(t *testing.T) {
	h := newAuthHarness(t)
	ctx := context.Background()

	// Join mode is not accepted yet (task 08).
	err := h.auth.CompleteSetup(ctx, SetupParams{Mode: "join", LoginEnabled: false})
	require.ErrorIs(t, err, ErrBadRequest)

	// Login enabled without username or password.
	err = h.auth.CompleteSetup(ctx, SetupParams{Mode: SetupModeServer, LoginEnabled: true})
	require.ErrorIs(t, err, ErrBadRequest)
	err = h.auth.CompleteSetup(ctx, SetupParams{Mode: SetupModeServer, LoginEnabled: true, Username: "admin"})
	require.ErrorIs(t, err, ErrBadRequest)
	err = h.auth.CompleteSetup(ctx, SetupParams{Mode: SetupModeServer, LoginEnabled: true, Password: "x"})
	require.ErrorIs(t, err, ErrBadRequest)

	// Nothing was written.
	status, err := h.auth.Status(ctx)
	require.NoError(t, err)
	assert.False(t, status.SetupComplete)
}

func TestLoginAndSessions(t *testing.T) {
	h := newAuthHarness(t)
	ctx := context.Background()
	h.setupLoginEnabled("admin", "secret")

	// Correct login returns a token.
	token, err := h.auth.Login(ctx, "admin", "secret")
	require.NoError(t, err)
	require.Len(t, token, 64, "32-byte hex token")

	// Only the hash is stored.
	sess, err := h.st.GetSessionByHash(ctx, HashToken(token))
	require.NoError(t, err)
	assert.False(t, sess.ExpiresAt.IsZero())
	assert.True(t, sess.ExpiresAt.After(time.Now()), "session is not expired")

	// The raw token appears nowhere in the store settings.
	settings, err := h.st.ListSettings(ctx)
	require.NoError(t, err)
	for k, v := range settings {
		assert.NotContains(t, v, token, "setting %s leaks the raw token", k)
	}

	// Wrong password, wrong username, disabled login, pre-setup.
	_, err = h.auth.Login(ctx, "admin", "wrong")
	require.ErrorIs(t, err, ErrInvalidCredentials)
	_, err = h.auth.Login(ctx, "root", "secret")
	require.ErrorIs(t, err, ErrInvalidCredentials)

	fresh := newAuthHarness(t)
	_, err = fresh.auth.Login(ctx, "admin", "secret")
	require.ErrorIs(t, err, ErrInvalidCredentials, "login before setup")
}

func TestAuthenticateAndLogout(t *testing.T) {
	h := newAuthHarness(t)
	ctx := context.Background()
	h.setupLoginEnabled("admin", "secret")

	token, err := h.auth.Login(ctx, "admin", "secret")
	require.NoError(t, err)

	user, err := h.auth.Authenticate(ctx, token)
	require.NoError(t, err)
	assert.Equal(t, "admin", user)

	// Logout invalidates the session.
	require.NoError(t, h.auth.Logout(ctx, token))
	_, err = h.auth.Authenticate(ctx, token)
	require.ErrorIs(t, err, ErrSessionUnauthorized)

	// Logout is idempotent for unknown tokens.
	require.NoError(t, h.auth.Logout(ctx, "unknown-token"))

	// Expired sessions are rejected and swept.
	expired := &store.Session{TokenHash: HashToken("stale"), ExpiresAt: time.Now().Add(-time.Minute)}
	require.NoError(t, h.st.CreateSession(ctx, expired))
	_, err = h.auth.Authenticate(ctx, "stale")
	require.ErrorIs(t, err, ErrSessionUnauthorized)
	_, err = h.st.GetSessionByHash(ctx, HashToken("stale"))
	require.ErrorIs(t, err, store.ErrNotFound, "expired session swept")

	// Empty token.
	_, err = h.auth.Authenticate(ctx, "")
	require.ErrorIs(t, err, ErrSessionUnauthorized)
}

func TestChangePassword(t *testing.T) {
	h := newAuthHarness(t)
	ctx := context.Background()
	h.setupLoginEnabled("admin", "old-secret")

	// Wrong current password refused.
	err := h.auth.ChangePassword(ctx, ChangePasswordParams{CurrentPassword: "nope", NewPassword: "new-secret"})
	require.ErrorIs(t, err, ErrPasswordMismatch)

	// Correct current password: new password works, old does not.
	require.NoError(t, h.auth.ChangePassword(ctx, ChangePasswordParams{CurrentPassword: "old-secret", NewPassword: "new-secret"}))
	_, err = h.auth.Login(ctx, "admin", "old-secret")
	require.ErrorIs(t, err, ErrInvalidCredentials)
	token, err := h.auth.Login(ctx, "admin", "new-secret")
	require.NoError(t, err)

	// Empty new password keeps the current one.
	require.NoError(t, h.auth.ChangePassword(ctx, ChangePasswordParams{CurrentPassword: "new-secret"}))
	_, err = h.auth.Authenticate(ctx, token)
	require.NoError(t, err)

	assert.True(t, strings.HasPrefix(h.hash(t), Argon2Prefix))
}

func TestEnableDisableLogin(t *testing.T) {
	h := newAuthHarness(t)
	ctx := context.Background()
	require.NoError(t, h.auth.CompleteSetup(ctx, SetupParams{Mode: SetupModeServer, LoginEnabled: false}))

	// Login disabled: GET /libraries would be open; authenticate fails.
	_, err := h.auth.Authenticate(ctx, "anything")
	require.ErrorIs(t, err, ErrSessionUnauthorized)

	// Enable with account.
	require.NoError(t, h.auth.EnableLogin(ctx, "admin", "secret"))
	status, err := h.auth.Status(ctx)
	require.NoError(t, err)
	assert.True(t, status.LoginEnabled)
	token, err := h.auth.Login(ctx, "admin", "secret")
	require.NoError(t, err)
	_, err = h.auth.Authenticate(ctx, token)
	require.NoError(t, err)

	// Disable keeps the account for the next enable.
	require.NoError(t, h.auth.DisableLogin(ctx))
	status, err = h.auth.Status(ctx)
	require.NoError(t, err)
	assert.False(t, status.LoginEnabled)
}

func TestPasswordsNeverLogged(t *testing.T) {
	h := newAuthHarness(t)
	ctx := context.Background()
	const password = "super-secret-password"

	require.NoError(t, h.auth.CompleteSetup(ctx, SetupParams{
		Mode: SetupModeServer, LoginEnabled: true, Username: "admin", Password: password,
	}))
	token, err := h.auth.Login(ctx, "admin", password)
	require.NoError(t, err)
	require.NoError(t, h.auth.ChangePassword(ctx, ChangePasswordParams{CurrentPassword: password, NewPassword: password + "-2"}))
	require.NoError(t, h.auth.Logout(ctx, token))

	hash := h.hash(t)
	for _, needle := range []string{password, password + "-2", hash, token} {
		assert.NotContains(t, h.log.String(), needle, "log leaks a secret")
	}
}
