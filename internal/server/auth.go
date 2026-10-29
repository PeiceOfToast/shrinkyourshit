// SPDX-License-Identifier: AGPL-3.0-only

package server

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/PeiceOfToast/shrinkyourshit/internal/store"
)

// Auth owns first-run setup, the single admin account and its sessions.
// Only hashes are stored: the argon2id password hash in settings, the
// SHA-256 of each session token in the sessions table.
type Auth struct {
	st  store.Store
	log *slog.Logger
}

// NewAuth builds the auth service.
func NewAuth(st store.Store, log *slog.Logger) *Auth {
	if log == nil {
		log = slog.Default()
	}
	return &Auth{st: st, log: log}
}

// Errors the API maps to HTTP statuses.
var (
	// ErrSetupComplete: POST /setup after setup finished → 409.
	ErrSetupComplete = errors.New("auth: setup already complete")
	// ErrSessionUnauthorized: missing, unknown, expired or logged-out
	// session → 401. Distinct from node auth's ErrUnauthorized (worker API).
	ErrSessionUnauthorized = errors.New("auth: session invalid")
	// ErrInvalidCredentials: wrong username or password on login → 401.
	ErrInvalidCredentials = errors.New("auth: invalid credentials")
	// ErrSetupIncomplete: admin-authenticated action before setup → 401.
	ErrSetupIncomplete = errors.New("auth: setup incomplete")
	// ErrBadRequest wraps validation failures for the settings/setup
	// bodies; the message is user-facing.
	ErrBadRequest = errors.New("auth: invalid request")
)

// badRequest formats a validation failure with the given message.
func badRequest(format string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrBadRequest, fmt.Sprintf(format, args...))
}

// SessionTTL is how long a login session stays valid (task 06: 30 days).
const SessionTTL = 30 * 24 * time.Hour

// Status is the GET /setup/status body. Mode and Server are only set in
// the POST /setup response for a join (task 08).
type Status struct {
	SetupComplete bool   `json:"setup_complete"`
	LoginEnabled  bool   `json:"login_enabled"`
	Username      string `json:"username,omitempty"`
	Mode          string `json:"mode,omitempty"`
	Server        string `json:"server,omitempty"`
}

// SetupParams is the POST /setup body.
type SetupParams struct {
	Mode         string `json:"mode"`
	LoginEnabled bool   `json:"login_enabled"`
	Username     string `json:"username"`
	Password     string `json:"password"`
	// Server is the join target (mode join only).
	Server string `json:"server"`
}

// SetupModeServer is the setup mode "run as server" (task 06).
const SetupModeServer = "server"

// SetupModeJoin is the setup mode "join an existing server" (task 08):
// the process becomes a worker of Server.
const SetupModeJoin = "join"

// setupComplete reads the setup_complete flag.
func (a *Auth) setupComplete(ctx context.Context) (bool, error) {
	v, ok, err := a.st.GetSetting(ctx, store.SettingSetupComplete)
	if err != nil {
		return false, err
	}
	return ok && v == "true", nil
}

// Status reports whether setup has run and whether login is on.
func (a *Auth) Status(ctx context.Context) (Status, error) {
	login, username, _, err := a.credentials(ctx)
	if err != nil {
		return Status{}, err
	}
	complete, err := a.setupComplete(ctx)
	if err != nil {
		return Status{}, err
	}
	out := Status{SetupComplete: complete, LoginEnabled: login}
	if login {
		out.Username = username
	}
	return out, nil
}

// credentials reads the admin account settings.
func (a *Auth) credentials(ctx context.Context) (loginEnabled bool, username, passwordHash string, err error) {
	settings, err := a.st.ListSettings(ctx)
	if err != nil {
		return false, "", "", fmt.Errorf("auth: read settings: %w", err)
	}
	return settings[store.SettingLoginEnabled] == "true",
		settings[store.SettingAdminUsername],
		settings[store.SettingAdminPasswordHash], nil
}

// CompleteSetup finishes first-run setup. It is refused once setup_complete
// is set. Mode server: with login enabled it creates the admin account,
// with login disabled the API stays open (DECISIONS.md 16). Mode join
// (task 08): no local account is created; the caller has already recorded
// the role=worker decision.
func (a *Auth) CompleteSetup(ctx context.Context, params SetupParams) error {
	complete, err := a.setupComplete(ctx)
	if err != nil {
		return err
	}
	if complete {
		return ErrSetupComplete
	}
	switch params.Mode {
	case SetupModeServer:
		// fall through to the account flow below
	case SetupModeJoin:
		if strings.TrimSpace(params.Server) == "" {
			return badRequest("server URL is required to join")
		}
		if err := a.st.SetSetting(ctx, store.SettingSetupComplete, "true"); err != nil {
			return fmt.Errorf("auth: setup setting setup_complete: %w", err)
		}
		a.log.Info("setup: joined an existing server", "server", params.Server)
		return nil
	default:
		return badRequest("mode must be %q or %q", SetupModeServer, SetupModeJoin)
	}
	sets := []struct{ key, value string }{
		{store.SettingSetupComplete, "true"},
		{store.SettingLoginEnabled, strconv.FormatBool(params.LoginEnabled)},
	}
	if params.LoginEnabled {
		if err := validateUsername(params.Username); err != nil {
			return err
		}
		if err := validatePassword(params.Password); err != nil {
			return err
		}
		hash, err := HashPassword(params.Password)
		if err != nil {
			return err
		}
		sets = append(sets,
			struct{ key, value string }{store.SettingAdminUsername, params.Username},
			struct{ key, value string }{store.SettingAdminPasswordHash, hash})
	}
	for _, s := range sets {
		if err := a.st.SetSetting(ctx, s.key, s.value); err != nil {
			return fmt.Errorf("auth: setup setting %s: %w", s.key, err)
		}
	}
	a.log.Info("first-run setup complete", "login_enabled", params.LoginEnabled)
	return nil
}

// Login checks the admin credentials and returns the raw session token for
// the cookie. The token is random 32 bytes; only its SHA-256 is stored.
// Expired sessions are swept on the way.
func (a *Auth) Login(ctx context.Context, username, password string) (string, error) {
	login, wantName, wantHash, err := a.credentials(ctx)
	if err != nil {
		return "", err
	}
	complete, err := a.setupComplete(ctx)
	if err != nil {
		return "", err
	}
	// No account or setup unfinished: any attempt fails as invalid
	// credentials (the generic answer, never "which part was wrong").
	if !login || !complete || wantHash == "" || username != wantName {
		return "", ErrInvalidCredentials
	}
	if !VerifyPassword(wantHash, password) {
		return "", ErrInvalidCredentials
	}
	token, err := generateSessionToken()
	if err != nil {
		return "", err
	}
	now := time.Now().UTC()
	sess := &store.Session{TokenHash: HashToken(token), CreatedAt: now, ExpiresAt: now.Add(SessionTTL)}
	if err := a.st.CreateSession(ctx, sess); err != nil {
		return "", fmt.Errorf("auth: create session: %w", err)
	}
	if _, err := a.st.DeleteExpiredSessions(ctx, now); err != nil {
		a.log.Warn("auth: expired session sweep", "err", err)
	}
	return token, nil
}

// Authenticate resolves a raw session token to the admin username. Missing,
// unknown and expired tokens are ErrSessionUnauthorized. Expired rows are
// removed when seen so they cannot accumulate.
func (a *Auth) Authenticate(ctx context.Context, token string) (string, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return "", ErrSessionUnauthorized
	}
	sess, err := a.st.GetSessionByHash(ctx, HashToken(token))
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return "", ErrSessionUnauthorized
		}
		return "", err
	}
	now := time.Now().UTC()
	if !sess.ExpiresAt.After(now) {
		_ = a.st.DeleteSession(ctx, sess.TokenHash)
		return "", ErrSessionUnauthorized
	}
	username, err := a.Username(ctx)
	if err != nil {
		return "", err
	}
	return username, nil
}

// Logout deletes the session (cookie clearing is the API's job). Unknown
// tokens are already logged out: logout is idempotent.
func (a *Auth) Logout(ctx context.Context, token string) error {
	token = strings.TrimSpace(token)
	if token == "" {
		return nil
	}
	if err := a.st.DeleteSession(ctx, HashToken(token)); err != nil {
		return fmt.Errorf("auth: logout: %w", err)
	}
	return nil
}

// Username returns the admin username (empty when login is disabled or the
// setting is unset).
func (a *Auth) Username(ctx context.Context) (string, error) {
	v, ok, err := a.st.GetSetting(ctx, store.SettingAdminUsername)
	if err != nil {
		return "", fmt.Errorf("auth: read username: %w", err)
	}
	if !ok {
		return "", nil
	}
	return v, nil
}

// ChangePasswordParams is the password-change part of PUT /settings.
type ChangePasswordParams struct {
	CurrentPassword string // required when login is enabled
	NewPassword     string // empty means "keep the current password"
}

// ChangePassword validates and applies a password change. With login
// enabled the current password must verify first. Returns the stored
// (already hashed) result — callers never see the new password.
func (a *Auth) ChangePassword(ctx context.Context, params ChangePasswordParams) error {
	login, _, hash, err := a.credentials(ctx)
	if err != nil {
		return err
	}
	if !login {
		// Login is off: there is no password to protect and none to set
		// (enabling login carries its own password through settings).
		return nil
	}
	if params.NewPassword == "" {
		return nil // keep the current password
	}
	if hash == "" || !VerifyPassword(hash, params.CurrentPassword) {
		return ErrPasswordMismatch
	}
	return a.storePasswordHash(ctx, params.NewPassword)
}

// storePasswordHash hashes and stores a new password. The hash is written
// to settings only, never logged.
func (a *Auth) storePasswordHash(ctx context.Context, password string) error {
	if err := validatePassword(password); err != nil {
		return err
	}
	hash, err := HashPassword(password)
	if err != nil {
		return err
	}
	if err := a.st.SetSetting(ctx, store.SettingAdminPasswordHash, hash); err != nil {
		return fmt.Errorf("auth: store password hash: %w", err)
	}
	return nil
}

// EnableLogin turns login on with a username and initial password. Used by
// PUT /settings when login was off.
func (a *Auth) EnableLogin(ctx context.Context, username, password string) error {
	if err := validateUsername(username); err != nil {
		return err
	}
	if err := validatePassword(password); err != nil {
		return err
	}
	if err := a.st.SetSetting(ctx, store.SettingAdminUsername, username); err != nil {
		return fmt.Errorf("auth: enable login username: %w", err)
	}
	if err := a.storePasswordHash(ctx, password); err != nil {
		return err
	}
	return a.st.SetSetting(ctx, store.SettingLoginEnabled, "true")
}

// DisableLogin turns login off; the stored account stays for the next
// enable so the username does not have to be retyped.
func (a *Auth) DisableLogin(ctx context.Context) error {
	return a.st.SetSetting(ctx, store.SettingLoginEnabled, "false")
}

// validateUsername enforces a non-empty username.
func validateUsername(u string) error {
	if strings.TrimSpace(u) == "" {
		return badRequest("username is required")
	}
	return nil
}

// validatePassword enforces a non-empty password.
func validatePassword(p string) error {
	if len(p) < 1 {
		return badRequest("password is required")
	}
	return nil
}

// generateSessionToken returns a fresh 32-byte hex token.
func generateSessionToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("auth: session token: %w", err)
	}
	return hex.EncodeToString(b), nil
}

// HashToken returns the SHA-256 hex hash of a raw session token. Raw tokens
// are never stored.
func HashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}
