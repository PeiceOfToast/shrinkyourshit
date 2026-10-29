// SPDX-License-Identifier: AGPL-3.0-only

package api

import (
	"errors"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/PeiceOfToast/shrinkyourshit/internal/server"
	"github.com/PeiceOfToast/shrinkyourshit/internal/store"
)

// Session cookie (API.md): HttpOnly, SameSite=Lax, 30-day TTL matching the
// session TTL. Path=/ so it covers every /api/v1 route. No Secure flag:
// TLS is optional and expected to terminate at a reverse proxy.
const (
	sessionCookieName = "sys_session"
	sessionMaxAge     = int(server.SessionTTL / time.Second) // 30 days
)

// requireAdmin guards the admin API. Login disabled: every admin endpoint
// is open (API.md). Login enabled: a valid sys_session cookie is required.
// The setup, login, health and worker routes are registered unwrapped, so
// nothing to exclude here (the node routes authenticate by node key).
func (a *API) requireAdmin(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		loginEnabled, err := a.loginEnabled(r)
		if err != nil {
			a.internal(w, err)
			return
		}
		if !loginEnabled {
			next(w, r)
			return
		}
		cookie, err := r.Cookie(sessionCookieName)
		if err != nil || strings.TrimSpace(cookie.Value) == "" {
			writeError(w, http.StatusUnauthorized, "unauthorized", "login required")
			return
		}
		if _, err := a.svc.Auth().Authenticate(r.Context(), cookie.Value); err != nil {
			if errors.Is(err, server.ErrSessionUnauthorized) {
				writeError(w, http.StatusUnauthorized, "unauthorized", "session invalid or expired")
				return
			}
			a.internal(w, err)
			return
		}
		next(w, r)
	}
}

// loginEnabled reads the login_enabled setting.
func (a *API) loginEnabled(r *http.Request) (bool, error) {
	v, ok, err := a.st.GetSetting(r.Context(), store.SettingLoginEnabled)
	if err != nil {
		return false, err
	}
	return ok && v == "true", nil
}

// setupStatus answers GET /setup/status: is setup done, is login on.
func (a *API) setupStatus(w http.ResponseWriter, r *http.Request) {
	status, err := a.svc.Auth().Status(r.Context())
	if err != nil {
		a.internal(w, err)
		return
	}
	writeJSON(w, http.StatusOK, status)
}

// loginBody is the POST /auth/login request.
type loginBody struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// login answers POST /auth/login, sets the session cookie and returns the
// account summary. Wrong credentials are a generic 401 (never "which part
// was wrong"). The sixth failed attempt within a minute from one IP gets
// 429.
func (a *API) login(w http.ResponseWriter, r *http.Request) {
	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		ip = r.RemoteAddr
	}
	if !a.logins.allow(ip) {
		writeError(w, http.StatusTooManyRequests, "too_many_attempts", "too many failed login attempts, try again in a minute")
		return
	}
	body, err := decode[loginBody](r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	token, err := a.svc.Auth().Login(r.Context(), body.Username, body.Password)
	if errors.Is(err, server.ErrInvalidCredentials) {
		a.logins.fail(ip)
		writeError(w, http.StatusUnauthorized, "invalid_credentials", "wrong username or password")
		return
	}
	if err != nil {
		a.internal(w, err)
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    token,
		Path:     "/",
		MaxAge:   sessionMaxAge,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
	writeJSON(w, http.StatusOK, server.Status{SetupComplete: true, LoginEnabled: true, Username: body.Username})
}

// logout answers POST /auth/logout: kills the session and clears the
// cookie. Idempotent.
func (a *API) logout(w http.ResponseWriter, r *http.Request) {
	if cookie, err := r.Cookie(sessionCookieName); err == nil && cookie.Value != "" {
		if err := a.svc.Auth().Logout(r.Context(), cookie.Value); err != nil {
			a.internal(w, err)
			return
		}
	}
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1, // delete the cookie in the browser
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
	w.WriteHeader(http.StatusNoContent)
}

// me answers GET /auth/me with the account summary the UI's route guard
// uses. The API never includes credentials here.
func (a *API) me(w http.ResponseWriter, r *http.Request) {
	status, err := a.svc.Auth().Status(r.Context())
	if err != nil {
		a.internal(w, err)
		return
	}
	writeJSON(w, http.StatusOK, status)
}

// settingsBody is the PUT /settings request. Pointers distinguish "not
// sent" from a zero value. password is the initial password when enabling
// login, or a password change (then current_password must verify).
type settingsBody struct {
	WorkerConcurrency   *int      `json:"worker_concurrency"`
	ScratchDir          *string   `json:"scratch_dir"`
	DefaultResultPolicy *string   `json:"default_result_policy"`
	PreferredLanguages  *[]string `json:"preferred_languages"`
	DefaultVideoCodec   *string   `json:"default_video_codec"`
	LoginEnabled        *bool     `json:"login_enabled"`
	Username            string    `json:"admin_username"`
	Password            string    `json:"password"`
	CurrentPassword     string    `json:"current_password"`
}

// getSettings answers GET /settings. The password hash is never in the
// response (server.Settings.Values has no field for it).
func (a *API) getSettings(w http.ResponseWriter, r *http.Request) {
	vals, err := a.svc.Settings().Get(r.Context())
	if err != nil {
		a.internal(w, err)
		return
	}
	writeJSON(w, http.StatusOK, vals)
}

// putSettings answers PUT /settings.
func (a *API) putSettings(w http.ResponseWriter, r *http.Request) {
	body, err := decode[settingsBody](r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	err = a.svc.Settings().Update(r.Context(), a.svc.Auth(), server.Update{
		WorkerConcurrency:   body.WorkerConcurrency,
		ScratchDir:          body.ScratchDir,
		DefaultResultPolicy: body.DefaultResultPolicy,
		PreferredLanguages:  body.PreferredLanguages,
		DefaultVideoCodec:   body.DefaultVideoCodec,
		LoginEnabled:        body.LoginEnabled,
		Username:            body.Username,
		Password:            body.Password,
		CurrentPassword:     body.CurrentPassword,
	})
	if errors.Is(err, server.ErrBadRequest) {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	if errors.Is(err, server.ErrPasswordMismatch) {
		writeError(w, http.StatusUnauthorized, "invalid_credentials", "current password is wrong")
		return
	}
	if err != nil {
		a.internal(w, err)
		return
	}
	vals, err := a.svc.Settings().Get(r.Context())
	if err != nil {
		a.internal(w, err)
		return
	}
	writeJSON(w, http.StatusOK, vals)
}
