// SPDX-License-Identifier: AGPL-3.0-only

package api

import (
	"errors"
	"net/http"

	"github.com/PeiceOfToast/shrinkyourshit/internal/plugin"
	"github.com/PeiceOfToast/shrinkyourshit/internal/server"
	"github.com/PeiceOfToast/shrinkyourshit/internal/store"
)

// pluginDTO is the API shape of one installed plugin (task 31). It merges
// the stored row with the manifest on disk: identity plus the manifest's
// description and the settings the UI builds its form from. The collected
// values travel back to the server (they are operator input, not secrets
// the server must hide from itself) — but they are only shown in the
// settings form, never in a list response.
type pluginDTO struct {
	ID          string           `json:"id"`
	Name        string           `json:"name"`
	Version     string           `json:"version"`
	Type        string           `json:"type"`
	Enabled     bool             `json:"enabled"`
	Source      string           `json:"source"`
	Description string           `json:"description,omitempty"`
	Author      string           `json:"author,omitempty"`
	Timeout     int              `json:"timeout_seconds"`
	Env         []string         `json:"env,omitempty"`
	Settings    []plugin.Setting `json:"settings,omitempty"`
	// Values are the collected setting values; the form pre-fills them.
	Values map[string]string `json:"values,omitempty"`
	// ManifestError is set when the manifest on disk cannot be read (the
	// directory was removed by hand): the plugin stays listed but nothing
	// runs, and the row is still removable.
	ManifestError string `json:"manifest_error,omitempty"`
	InstalledAt   string `json:"installed_at"`
}

func toPluginDTO(in server.Installed) pluginDTO {
	return pluginDTO{
		ID:            in.ID,
		Name:          in.Name,
		Version:       in.Version,
		Type:          in.Type,
		Enabled:       in.Enabled,
		Source:        in.Source,
		Description:   in.Description,
		Author:        in.Author,
		Timeout:       in.Timeout,
		Env:           in.Env,
		Settings:      in.Settings,
		Values:        in.Values,
		ManifestError: in.ManifestError,
		InstalledAt:   in.InstalledAt.Format("2006-01-02T15:04:05Z07:00"),
	}
}

// installBody is POST /plugins/install: exactly one of git_url (+ ref) or
// path.
type installBody struct {
	GitURL string `json:"git_url"`
	Ref    string `json:"ref"`
	Path   string `json:"path"`
}

// updateBody is PUT /plugins/{id}: the enabled switch and/or the
// collected settings values. A nil enabled leaves the switch unchanged.
type updateBody struct {
	Enabled  *bool             `json:"enabled"`
	Settings map[string]string `json:"settings"`
}

// listPlugins returns every installed plugin, oldest first.
func (a *API) listPlugins(w http.ResponseWriter, r *http.Request) {
	rows, err := a.svc.Plugins().List(r.Context())
	if err != nil {
		a.internal(w, err)
		return
	}
	items := make([]pluginDTO, 0, len(rows))
	for _, row := range rows {
		items = append(items, toPluginDTO(row))
	}
	writeJSON(w, http.StatusOK, server.ListResponse[pluginDTO]{Items: items, Total: len(items)})
}

// getPlugin returns one installed plugin.
func (a *API) getPlugin(w http.ResponseWriter, r *http.Request) {
	in, err := a.svc.Plugins().Get(r.Context(), r.PathValue("id"))
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "no such plugin")
		return
	}
	if err != nil {
		a.internal(w, err)
		return
	}
	writeJSON(w, http.StatusOK, toPluginDTO(in))
}

// installPlugin installs from a git URL or a directory path. The new
// plugin is disabled; nothing runs until the PUT enables it.
func (a *API) installPlugin(w http.ResponseWriter, r *http.Request) {
	body, err := decode[installBody](r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	in, err := a.svc.Plugins().Install(r.Context(), server.InstallRequest{
		GitURL: body.GitURL,
		Ref:    body.Ref,
		Path:   body.Path,
	})
	switch {
	case err == nil:
	case errors.Is(err, plugin.ErrAlreadyInstalled):
		writeError(w, http.StatusConflict, "already_installed", err.Error())
		return
	case errors.Is(err, plugin.ErrGitUnavailable):
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	default:
		// An install rejection always names the manifest field or the
		// source problem: a validation error, not a server fault.
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, toPluginDTO(in))
}

// updatePlugin applies the enabled switch and/or new settings values.
func (a *API) updatePlugin(w http.ResponseWriter, r *http.Request) {
	body, err := decode[updateBody](r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	in, err := a.svc.Plugins().Update(r.Context(), r.PathValue("id"), server.UpdateRequest{
		Enabled:  body.Enabled,
		Settings: body.Settings,
	})
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "no such plugin")
		return
	}
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, toPluginDTO(in))
}

// deletePlugin removes a disabled plugin (row and directory).
func (a *API) deletePlugin(w http.ResponseWriter, r *http.Request) {
	err := a.svc.Plugins().Delete(r.Context(), r.PathValue("id"))
	switch {
	case err == nil:
		w.WriteHeader(http.StatusNoContent)
	case errors.Is(err, store.ErrNotFound):
		writeError(w, http.StatusNotFound, "not_found", "no such plugin")
	default:
		// An enabled plugin must be turned off first: a client error, not
		// a server fault.
		writeError(w, http.StatusConflict, "plugin_enabled", err.Error())
	}
}

// pluginTestResponse is POST /plugins/{id}/test: one run against a
// synthetic job, with the captured output and the exit code. 200 even
// when the plugin failed.
type pluginTestResponse struct {
	OK       bool     `json:"ok"`
	Skipped  bool     `json:"skipped"`
	Skip     string   `json:"skip_reason,omitempty"`
	ExitCode int      `json:"exit_code"`
	Error    string   `json:"error,omitempty"`
	Output   []string `json:"output,omitempty"`
}

// testPlugin runs the plugin once against a synthetic job (task 31) and
// answers 200 with the outcome regardless of how the run went.
func (a *API) testPlugin(w http.ResponseWriter, r *http.Request) {
	res, err := a.svc.Plugins().Test(r.Context(), r.PathValue("id"))
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "no such plugin")
		return
	}
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	resp := pluginTestResponse{
		OK:       res.OK,
		Skipped:  !res.Result.Ran,
		Skip:     res.Result.SkipReason,
		ExitCode: res.Result.ExitCode,
		Error:    res.Result.Error,
		Output:   res.Result.OutputText(),
	}
	writeJSON(w, http.StatusOK, resp)
}
