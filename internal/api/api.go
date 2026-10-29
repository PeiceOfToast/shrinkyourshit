// SPDX-License-Identifier: AGPL-3.0-only
package api

import (
	"encoding/json"
	"io/fs"
	"log/slog"
	"net/http"
	"path"
	"strings"

	"github.com/PeiceOfToast/shrinkyourshit/internal/server"
	"github.com/PeiceOfToast/shrinkyourshit/internal/store"
)

// API is the HTTP surface of the server. svc may be nil only for the routes
// that predate task 03 (health, static files); service routes reject
// requests with 503 until a Server is wired in.
type API struct {
	svc *server.Server
	// st is the read surface for listing handlers; every write goes
	// through the server services.
	st  store.Store
	log *slog.Logger
	// logins rate-limits failed login attempts per IP (task 06).
	logins *loginLimiter
}

// New builds the root HTTP handler. version is reported by /api/v1/health;
// static is the built web UI (web/dist), served at / with an index.html
// fallback for unknown non-API paths so client-side routing works. svc holds
// the server services; requireAdmin enforces session auth (open when login
// is disabled).
func New(version string, static fs.FS, svc *server.Server) http.Handler {
	a := &API{svc: svc, log: slog.Default(), logins: newLoginLimiter()}
	if svc != nil {
		a.st = svc.Store()
	}
	mux := http.NewServeMux()

	mux.HandleFunc("GET /api/v1/health", health(version))

	if svc != nil {
		// First-run setup and login stay open; requireAdmin is applied to
		// every other admin route (task 06).
		mux.HandleFunc("GET /api/v1/setup/status", a.setupStatus)
		mux.HandleFunc("POST /api/v1/setup", a.completeSetup)
		mux.HandleFunc("POST /api/v1/auth/login", a.login)
		mux.HandleFunc("POST /api/v1/auth/logout", a.requireAdmin(a.logout))
		mux.HandleFunc("GET /api/v1/auth/me", a.requireAdmin(a.me))
		mux.HandleFunc("GET /api/v1/settings", a.requireAdmin(a.getSettings))
		mux.HandleFunc("PUT /api/v1/settings", a.requireAdmin(a.putSettings))

		mux.HandleFunc("GET /api/v1/libraries", a.requireAdmin(a.listLibraries))
		mux.HandleFunc("POST /api/v1/libraries", a.requireAdmin(a.createLibrary))
		mux.HandleFunc("GET /api/v1/libraries/{id}", a.requireAdmin(a.getLibrary))
		mux.HandleFunc("PUT /api/v1/libraries/{id}", a.requireAdmin(a.updateLibrary))
		mux.HandleFunc("DELETE /api/v1/libraries/{id}", a.requireAdmin(a.deleteLibrary))
		mux.HandleFunc("POST /api/v1/libraries/{id}/scan", a.requireAdmin(a.scanLibrary))
		mux.HandleFunc("GET /api/v1/libraries/{id}/files", a.requireAdmin(a.listFiles))
		mux.HandleFunc("GET /api/v1/jobs", a.requireAdmin(a.listJobs))
		mux.HandleFunc("GET /api/v1/jobs/{id}", a.requireAdmin(a.getJob))
		mux.HandleFunc("POST /api/v1/jobs/{id}/cancel", a.requireAdmin(a.cancelJob))
		mux.HandleFunc("POST /api/v1/jobs/{id}/retry", a.requireAdmin(a.retryJob))
		mux.HandleFunc("GET /api/v1/events", a.requireAdmin(a.events))
		mux.HandleFunc("GET /api/v1/fs/browse", a.requireAdmin(a.browse))
		mux.HandleFunc("GET /api/v1/stats", a.requireAdmin(a.getStats))

		// Worker API (task 05): bearer node-key auth, never session cookies.
		mux.HandleFunc("POST /api/v1/worker/heartbeat", a.nodeAuth(a.workerHeartbeat))
		mux.HandleFunc("POST /api/v1/worker/claim", a.nodeAuth(a.workerClaim))
		mux.HandleFunc("GET /api/v1/worker/jobs/{id}", a.nodeAuth(a.workerJobSpec))
		mux.HandleFunc("POST /api/v1/worker/jobs/{id}/progress", a.nodeAuth(a.workerProgress))
		mux.HandleFunc("POST /api/v1/worker/jobs/{id}/complete", a.nodeAuth(a.workerComplete))
		mux.HandleFunc("POST /api/v1/worker/jobs/{id}/fail", a.nodeAuth(a.workerFail))
	}

	mux.Handle("/api/", apiNotFound())
	mux.Handle("/", serveStatic(static))
	return mux
}

// requireAdmin lives in auth.go: it passes requests through when login is
// disabled and otherwise demands a valid sys_session cookie. Every admin
// route above is wrapped; /setup/*, /auth/login, /health and the /worker/*
// node routes (node-key auth) are registered unwrapped.

// healthResponse is the /api/v1/health body (API.md, task 01).
type healthResponse struct {
	Status  string `json:"status"`
	Version string `json:"version"`
}

func health(version string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, healthResponse{Status: "ok", Version: version})
	}
}

// apiNotFound returns JSON errors for unknown API paths instead of the HTML
// error page a reverse proxy user would otherwise see.
func apiNotFound() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeError(w, http.StatusNotFound, "not_found", "no such API endpoint")
	})
}

// writeJSON writes a JSON response with the right content type.
func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(body); err != nil {
		// The client walked away; nothing left to do.
		return
	}
}

// writeError writes the error body defined by API.md conventions.
func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, struct {
		Error string `json:"error"`
		Code  string `json:"code"`
	}{Error: message, Code: code})
}

// serveStatic serves the embedded web UI. Unknown paths fall back to
// index.html so the client-side router owns deep links.
func serveStatic(static fs.FS) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.Header().Set("Allow", "GET, HEAD")
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		name := strings.TrimPrefix(path.Clean("/"+r.URL.Path), "/")
		if name == "" {
			name = "index.html"
		}
		if info, err := fs.Stat(static, name); err != nil || info.IsDir() {
			// Unknown path: hand it to the single-page app.
			name = "index.html"
		}
		http.ServeFileFS(w, r, static, name)
	})
}
