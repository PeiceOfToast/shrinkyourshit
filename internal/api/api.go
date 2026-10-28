// SPDX-License-Identifier: AGPL-3.0-only
package api

import (
	"encoding/json"
	"io/fs"
	"net/http"
	"path"
	"strings"
)

// New builds the root HTTP handler. version is reported by /api/v1/health;
// static is the built web UI (web/dist), served at / with an index.html
// fallback for unknown non-API paths so client-side routing works.
func New(version string, static fs.FS) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/health", health(version))
	mux.Handle("/api/", apiNotFound())
	mux.Handle("/", serveStatic(static))
	return mux
}

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
		name := strings.TrimPrefix(path.Clean("/" + r.URL.Path), "/")
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
