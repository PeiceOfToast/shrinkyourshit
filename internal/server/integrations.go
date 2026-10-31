// SPDX-License-Identifier: AGPL-3.0-only

package server

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/PeiceOfToast/shrinkyourshit/internal/store"
)

// Integration is the service-level shape of one configured remote (task
// 23): a Jellyfin/Emby/Plex media server or a Sonarr/Radarr install the
// refresher notifies after a job ends. The API key exists only to be sent
// on the outbound call; it is never logged and never sent back out through
// the API.
type Integration struct {
	ID      string
	Kind    string
	Name    string
	BaseURL string
	APIKey  string
	Enabled bool
	// RefreshOnDone subscribes this integration to job completions. An
	// integration without it still works through the manual test call.
	RefreshOnDone bool
}

// IntegrationFromRow maps a stored row to the service shape.
func IntegrationFromRow(row *store.Integration) Integration {
	return Integration{
		ID:            row.ID,
		Kind:          row.Kind,
		Name:          row.Name,
		BaseURL:       row.BaseURL,
		APIKey:        row.APIKey,
		Enabled:       row.Enabled,
		RefreshOnDone: row.RefreshOnDone,
	}
}

// Row maps back to the stored shape for persistence.
func (in Integration) Row() *store.Integration {
	return &store.Integration{
		ID:            in.ID,
		Kind:          in.Kind,
		Name:          in.Name,
		BaseURL:       in.BaseURL,
		APIKey:        in.APIKey,
		Enabled:       in.Enabled,
		RefreshOnDone: in.RefreshOnDone,
	}
}

// ValidateIntegration checks one integration and normalizes it in place:
// surrounding whitespace is trimmed and trailing slashes are removed from
// base_url. It rejects an unknown kind, a base_url that does not parse or
// is not http/https (or has no host), and an empty base_url or api_key.
// The pointer is deliberate: the normalization must be visible to the
// caller so the trimmed base_url is what gets stored.
func ValidateIntegration(in *Integration) error {
	in.Kind = strings.TrimSpace(in.Kind)
	in.Name = strings.TrimSpace(in.Name)
	in.BaseURL = strings.TrimRight(strings.TrimSpace(in.BaseURL), "/")
	in.APIKey = strings.TrimSpace(in.APIKey)

	known := false
	for _, v := range store.IntegrationKindValues() {
		if in.Kind == v {
			known = true
			break
		}
	}
	if !known {
		return fmt.Errorf("integration: unknown kind %q: want one of %v", in.Kind, store.IntegrationKindValues())
	}
	if in.BaseURL == "" {
		return fmt.Errorf("integration: base_url is required")
	}
	u, err := url.Parse(in.BaseURL)
	if err != nil {
		return fmt.Errorf("integration: base_url does not parse: %v", err)
	}
	if (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return fmt.Errorf("integration: base_url must be an http or https URL with a host")
	}
	if in.APIKey == "" {
		return fmt.Errorf("integration: api_key is required")
	}
	return nil
}

// notifyTimeout bounds one outbound integration call (task 23: 5 seconds).
const notifyTimeout = 5 * time.Second

// CallIntegration performs the integration's refresh call. The exact
// request per kind is pinned here in one place (confirmed against the
// vendor APIs; any change happens in this one helper):
//
//	jellyfin, emby:  POST {base}/Library/Refresh           X-Emby-Token: {key}
//	plex:            GET  {base}/library/sections/all/refresh?X-Plex-Token={key}
//	sonarr:          POST {base}/api/v3/command            X-Api-Key: {key}  {"name":"RescanSeries"}
//	radarr:          POST {base}/api/v3/command            X-Api-Key: {key}  {"name":"RescanMovie"}
//
// Success is any 2xx. The returned status is the remote's, 0 when the
// request never got an answer. Errors never carry the base_url (a Plex
// URL has the token in its query string) and never the API key.
func CallIntegration(ctx context.Context, client *http.Client, in Integration) (int, error) {
	var (
		req *http.Request
		err error
	)
	switch in.Kind {
	case store.IntegrationKindJellyfin, store.IntegrationKindEmby:
		req, err = http.NewRequestWithContext(ctx, http.MethodPost, in.BaseURL+"/Library/Refresh", nil)
		if err == nil {
			req.Header.Set("X-Emby-Token", in.APIKey)
		}
	case store.IntegrationKindPlex:
		target := in.BaseURL + "/library/sections/all/refresh?X-Plex-Token=" + url.QueryEscape(in.APIKey)
		req, err = http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	case store.IntegrationKindSonarr, store.IntegrationKindRadarr:
		command := "RescanMovie"
		if in.Kind == store.IntegrationKindSonarr {
			command = "RescanSeries"
		}
		body, merr := json.Marshal(struct {
			Name string `json:"name"`
		}{command})
		if merr != nil {
			return 0, fmt.Errorf("integration %q: build command body", in.Name)
		}
		req, err = http.NewRequestWithContext(ctx, http.MethodPost, in.BaseURL+"/api/v3/command", strings.NewReader(string(body)))
		if err == nil {
			req.Header.Set("X-Api-Key", in.APIKey)
			req.Header.Set("Content-Type", "application/json")
		}
	default:
		return 0, fmt.Errorf("integration: unknown kind %q", in.Kind)
	}
	if err != nil {
		return 0, fmt.Errorf("integration %q: build request", in.Name)
	}

	resp, err := client.Do(req)
	if err != nil {
		// *url.Error embeds the full URL — which carries the Plex token
		// in its query string — so only the underlying cause survives.
		if ue, ok := err.(*url.Error); ok {
			err = ue.Err
		}
		return 0, fmt.Errorf("integration %q: request failed: %v", in.Name, err)
	}
	defer func() { _ = resp.Body.Close() }()
	_, _ = io.Copy(io.Discard, resp.Body)
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return resp.StatusCode, nil
	}
	return resp.StatusCode, fmt.Errorf("integration %q: unexpected status", in.Name)
}

// Notifier is the hook Queue.Complete calls after a done completion whose
// output was actually used (task 23). Implemented by *Refresher; nil on
// the Queue keeps the constructor working for tests that do not care.
type Notifier interface {
	Notify(ctx context.Context)
}

// notifyGap is the floor between two calls for the same integration id
// (task 23): a Notify arriving inside the window is dropped, not queued.
const notifyGap = 30 * time.Second

// refreshState is the per-integration coalescing record: at most one call
// in flight and a gap between attempt starts.
type refreshState struct {
	inFlight    bool
	lastAttempt time.Time
}

// Refresher notifies the configured integrations after a job ends. A
// finished job must not mean a request storm: one in-flight call per
// integration id, a 30 second floor between calls for the same id, and a
// 5 second per-request timeout. Failures are logged at warn with the
// integration name and status only — the key is never logged, and nothing
// here can fail a job.
type Refresher struct {
	st     store.Store
	log    *slog.Logger
	client *http.Client
	// gap is the per-id floor between calls. The default is notifyGap;
	// tests shrink it without a clock seam.
	gap time.Duration

	mu    sync.Mutex
	state map[string]*refreshState
}

// NewRefresher builds the refresher over a store.
func NewRefresher(st store.Store, log *slog.Logger) *Refresher {
	if log == nil {
		log = slog.Default()
	}
	return &Refresher{
		st:     st,
		log:    log,
		client: &http.Client{},
		gap:    notifyGap,
		state:  map[string]*refreshState{},
	}
}

// List returns every configured integration (the admin's list page),
// oldest first.
func (r *Refresher) List(ctx context.Context) ([]Integration, error) {
	rows, err := r.st.ListIntegrations(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]Integration, 0, len(rows))
	for _, row := range rows {
		out = append(out, IntegrationFromRow(row))
	}
	return out, nil
}

// Notify reads the enabled integrations with refresh_on_done and calls
// each one that is due. It returns as soon as the rows are listed: every
// call runs in its own goroutine, so a slow remote can never slow a
// completion. The calls use their own context — the caller's is a request
// context that dies with the worker response — bounded by notifyTimeout.
func (r *Refresher) Notify(ctx context.Context) {
	rows, err := r.st.ListIntegrations(ctx)
	if err != nil {
		r.log.Error("integrations: list", "err", err)
		return
	}
	for _, row := range rows {
		if !row.Enabled || !row.RefreshOnDone {
			continue
		}
		in := IntegrationFromRow(row)
		if !r.begin(in.ID) {
			continue // one in flight, or inside the 30 second floor
		}
		go r.call(in)
	}
}

// begin claims the right to call an integration now: false when a call is
// already in flight or the last attempt started less than gap ago.
func (r *Refresher) begin(id string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	st := r.state[id]
	if st == nil {
		st = &refreshState{}
		r.state[id] = st
	}
	now := time.Now()
	if st.inFlight || now.Sub(st.lastAttempt) < r.gap {
		return false
	}
	st.inFlight = true
	st.lastAttempt = now
	return true
}

// call performs one notification and clears the in-flight mark. A remote
// 500 or a dead host is a warn line, never an error out.
func (r *Refresher) call(in Integration) {
	defer func() {
		r.mu.Lock()
		if st := r.state[in.ID]; st != nil {
			st.inFlight = false
		}
		r.mu.Unlock()
	}()

	ctx, cancel := context.WithTimeout(context.Background(), notifyTimeout)
	defer cancel()
	status, err := CallIntegration(ctx, r.client, in)
	switch {
	case status >= 300:
		// The remote answered with a non-2xx: log the integration name
		// and the status code only, no URL and no key.
		r.log.Warn("integration refresh failed", "integration", in.Name, "status", status)
	case err != nil:
		// No answer at all (transport failure); the error text carries
		// neither the base_url nor the API key.
		r.log.Warn("integration refresh failed", "integration", in.Name, "err", err.Error())
	default:
		r.log.Debug("integration refreshed", "integration", in.Name, "status", status)
	}
}

// Test performs the integration's call once — the admin's Test button.
// It skips the coalescing on purpose: an explicit diagnostic, answered
// with the remote's status and error so the UI can show what happened.
func (r *Refresher) Test(ctx context.Context, in Integration) (int, error) {
	ctx, cancel := context.WithTimeout(ctx, notifyTimeout)
	defer cancel()
	return CallIntegration(ctx, r.client, in)
}
