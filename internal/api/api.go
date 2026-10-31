// SPDX-License-Identifier: AGPL-3.0-only
package api

import (
	"encoding/json"
	"io/fs"
	"log/slog"
	"net/http"
	"path"
	"strings"

	shrink "github.com/PeiceOfToast/shrinkyourshit"
	"github.com/PeiceOfToast/shrinkyourshit/internal/server"
	"github.com/PeiceOfToast/shrinkyourshit/internal/store"
	"github.com/PeiceOfToast/shrinkyourshit/internal/update"
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
	// version is the server's build version; /health reports it and the
	// installer scripts (task 09) bake it into the release download URL.
	// commit and date (task 16) ride with /api/v1/version.
	version string
	commit  string
	date    string
	// runtimeFn overrides the runtime the version endpoint reports
	// (task 17); nil uses update.Runtime().
	runtimeFn func() string
	// logins rate-limits failed login attempts per IP (task 06).
	logins *loginLimiter
	// join records a setup "join" decision (task 08); main owns the
	// persistence and the role switch. Nil → joining is refused with 500.
	join func(serverURL, token string) error
	// builtinNode is the built-in node's id (task 11): the setup wizard's
	// explicit "start the built-in node now" choice acts on it. Empty →
	// the wizard's start choice cannot be applied (nothing starts).
	builtinNode string
}

// Option customizes New.
type Option func(*API)

// WithJoin wires the setup "join an existing server" callback (task 08):
// persist role=worker and the server URL, then hand the URL and optional
// token back to the process so it can switch to worker mode.
func WithJoin(fn func(serverURL, token string) error) Option {
	return func(a *API) { a.join = fn }
}

// WithBuiltinNode wires the built-in node's id (task 11) so the setup
// wizard's explicit "start the built-in node now" choice can act on it.
func WithBuiltinNode(nodeID string) Option {
	return func(a *API) { a.builtinNode = nodeID }
}

// WithBuild wires the commit and build date (task 16) reported by
// /api/v1/version. Empty values are what a build without release flags
// reports.
func WithBuild(commit, date string) Option {
	return func(a *API) { a.commit, a.date = commit, date }
}

// WithRuntime overrides the runtime string /api/v1/version reports
// (task 17 tests); nil keeps update.Runtime().
func WithRuntime(fn func() string) Option {
	return func(a *API) { a.runtimeFn = fn }
}

// New builds the root HTTP handler. version is reported by /api/v1/health;
// static is the built web UI (web/dist), served at / with an index.html
// fallback for unknown non-API paths so client-side routing works. svc holds
// the server services; requireAdmin enforces session auth (open when login
// is disabled).
func New(version string, static fs.FS, svc *server.Server, opts ...Option) http.Handler {
	a := &API{svc: svc, log: slog.Default(), logins: newLoginLimiter(), version: version}
	for _, o := range opts {
		o(a)
	}
	if svc != nil {
		a.st = svc.Store()
	}
	mux := http.NewServeMux()

	mux.HandleFunc("GET /api/v1/health", health(version))
	mux.HandleFunc("GET /api/v1/version", a.buildVersion)

	// Installer scripts (task 09): open, templated with this request's
	// origin so a node reaches the server the way the admin's browser does.
	mux.HandleFunc("GET /install.sh", a.serveInstaller(shrink.InstallShell,
		"text/x-shellscript; charset=utf-8"))
	mux.HandleFunc("GET /install.ps1", a.serveInstaller(shrink.InstallPowerShell,
		"text/plain; charset=utf-8"))

	if svc != nil {
		// First-run setup and login stay open; requireAdmin is applied to
		// every other admin route (task 06). /setup/servers stays open so
		// the wizard can list discovered servers before an account exists.
		mux.HandleFunc("GET /api/v1/setup/status", a.setupStatus)
		mux.HandleFunc("GET /api/v1/setup/servers", a.setupServers)
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
		// One file's job history (task 19): the panel on the library
		// detail page shows what happened to a file over time.
		mux.HandleFunc("GET /api/v1/files/{id}/jobs", a.requireAdmin(a.fileJobs))
		mux.HandleFunc("GET /api/v1/events", a.requireAdmin(a.events))
		mux.HandleFunc("GET /api/v1/fs/browse", a.requireAdmin(a.browse))
		mux.HandleFunc("GET /api/v1/stats", a.requireAdmin(a.getStats))

		// Flow API (task 14): the rule graph of a library, the shared
		// validator and the step palette. The preview endpoint (added per
		// AGENTS rule 4) compiles an easy input or a graph to plain
		// language so the UI never duplicates the compiler.
		mux.HandleFunc("GET /api/v1/libraries/{id}/flow", a.requireAdmin(a.getLibraryFlow))
		mux.HandleFunc("PUT /api/v1/libraries/{id}/flow", a.requireAdmin(a.putLibraryFlow))
		mux.HandleFunc("POST /api/v1/flows/validate", a.requireAdmin(a.validateFlow))
		mux.HandleFunc("GET /api/v1/flows/step-types", a.requireAdmin(a.stepTypes))
		mux.HandleFunc("POST /api/v1/flows/preview", a.requireAdmin(a.previewFlow))

		// Update check and in-app apply (task 17). The version endpoint
		// carries the update settings; the status endpoint answers from
		// the last check without touching the network.
		mux.HandleFunc("GET /api/v1/update/status", a.requireAdmin(a.updateStatus))
		mux.HandleFunc("POST /api/v1/update/check", a.requireAdmin(a.updateCheck))
		mux.HandleFunc("POST /api/v1/update/apply", a.requireAdmin(a.updateApply))

		// External integrations (task 23): the media servers and download
		// clients the refresher notifies after a job ends. All admin only;
		// no response body ever carries an api key.
		mux.HandleFunc("GET /api/v1/integrations", a.requireAdmin(a.listIntegrations))
		mux.HandleFunc("POST /api/v1/integrations", a.requireAdmin(a.createIntegration))
		mux.HandleFunc("PUT /api/v1/integrations/{id}", a.requireAdmin(a.updateIntegration))
		mux.HandleFunc("DELETE /api/v1/integrations/{id}", a.requireAdmin(a.deleteIntegration))
		mux.HandleFunc("POST /api/v1/integrations/{id}/test", a.requireAdmin(a.testIntegration))

		// Node administration (task 08) plus lifecycle controls (task 11).
		// The lifecycle endpoints move desired_state only: start sets
		// running, stop chooses stopped or draining, pause-all/resume-all
		// do the same for every node at once.
		mux.HandleFunc("GET /api/v1/nodes", a.requireAdmin(a.listNodes))
		mux.HandleFunc("POST /api/v1/nodes/tokens", a.requireAdmin(a.createNodeToken))
		mux.HandleFunc("POST /api/v1/nodes/{id}/approve", a.requireAdmin(a.approveNode))
		mux.HandleFunc("POST /api/v1/nodes/{id}/revoke", a.requireAdmin(a.revokeNode))
		mux.HandleFunc("POST /api/v1/nodes/{id}/start", a.requireAdmin(a.startNode))
		mux.HandleFunc("POST /api/v1/nodes/{id}/stop", a.requireAdmin(a.stopNode))
		mux.HandleFunc("POST /api/v1/nodes/{id}/drain", a.requireAdmin(a.drainNode))
		mux.HandleFunc("POST /api/v1/nodes/pause-all", a.requireAdmin(a.pauseAllNodes))
		mux.HandleFunc("POST /api/v1/nodes/resume-all", a.requireAdmin(a.resumeAllNodes))
		mux.HandleFunc("PUT /api/v1/nodes/{id}", a.requireAdmin(a.updateNode))
		mux.HandleFunc("DELETE /api/v1/nodes/{id}", a.requireAdmin(a.deleteNode))

		// Pairing (task 08): open — an unpaired node has no key yet.
		mux.HandleFunc("POST /api/v1/pair", a.pair)
		mux.HandleFunc("GET /api/v1/pair/{id}", a.pairPoll)

		// Worker API (task 05): bearer node-key auth, never session cookies.
		mux.HandleFunc("POST /api/v1/worker/heartbeat", a.nodeAuth(a.workerHeartbeat))
		mux.HandleFunc("POST /api/v1/worker/claim", a.nodeAuth(a.workerClaim))
		mux.HandleFunc("GET /api/v1/worker/jobs/{id}", a.nodeAuth(a.workerJobSpec))
		mux.HandleFunc("POST /api/v1/worker/jobs/{id}/progress", a.nodeAuth(a.workerProgress))
		mux.HandleFunc("POST /api/v1/worker/jobs/{id}/complete", a.nodeAuth(a.workerComplete))
		mux.HandleFunc("POST /api/v1/worker/jobs/{id}/fail", a.nodeAuth(a.workerFail))

		// Transfer mode (task 09): shared-storage access check, resumable
		// source stream, chunked result upload.
		mux.HandleFunc("POST /api/v1/worker/jobs/{id}/access-check", a.nodeAuth(a.workerAccessCheck))
		mux.HandleFunc("GET /api/v1/worker/jobs/{id}/source", a.nodeAuth(a.workerSource))
		mux.HandleFunc("PUT /api/v1/worker/jobs/{id}/result", a.nodeAuth(a.workerResult))
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

// versionResponse is the /api/v1/version body (API.md, task 16): the full
// build identity plus how this install runs (binary or container). Task 17
// adds the update check fields to this endpoint.
type versionResponse struct {
	Version string `json:"version"`
	Commit  string `json:"commit"`
	Date    string `json:"date"`
	Runtime string `json:"runtime"`
	// Task 17: whether automatic update checks are on and which release
	// feed is configured. Defaults apply when no server is wired.
	UpdateCheckEnabled bool   `json:"update_check_enabled"`
	Channel            string `json:"channel"`
}

func (a *API) buildVersion(w http.ResponseWriter, r *http.Request) {
	out := versionResponse{
		Version: a.version,
		Commit:  a.commit,
		Date:    a.date,
		// Settings defaults; a wired server overrides these below.
		UpdateCheckEnabled: server.DefaultUpdateCheck,
		Channel:            update.ChannelStable,
	}
	// The runtime comes from the update service when one is wired, so the
	// version and update-status endpoints always agree (one override in
	// tests); without a server the API-level override applies.
	if a.svc != nil {
		out.Runtime = a.svc.Update().Runtime()
		if values, err := a.svc.Settings().Get(r.Context()); err == nil {
			out.UpdateCheckEnabled = values.UpdateCheck
			out.Channel = values.UpdateChannel
		}
	} else {
		out.Runtime = a.runtime()
	}
	writeJSON(w, http.StatusOK, out)
}

// runtime returns how this install runs, honouring the test override.
func (a *API) runtime() string {
	if a.runtimeFn != nil {
		return a.runtimeFn()
	}
	return update.Runtime()
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

// serveInstaller answers GET /install.sh and /install.ps1 with the
// embedded installer script templated for this request's origin
// (X-Forwarded-Proto aware, like the pairing commands) and the server's
// version, so the node downloads the release matching the server it joins.
func (a *API) serveInstaller(render func(serverURL, version string) string, contentType string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.Header().Set("Allow", "GET, HEAD")
			writeError(w, http.StatusMethodNotAllowed, "method_not_allowed",
				"method not allowed")
			return
		}
		w.Header().Set("Content-Type", contentType)
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Cache-Control", "no-store")
		_, _ = w.Write([]byte(render(baseURL(r), a.version)))
	}
}

// serveStatic serves the embedded web UI. Unknown paths fall back to
// index.html so the client-side router owns deep links.
func serveStatic(static fs.FS) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.Header().Set("Allow", "GET, HEAD")
			writeError(w, http.StatusMethodNotAllowed, "method_not_allowed",
				"method not allowed")
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
