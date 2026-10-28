// SPDX-License-Identifier: AGPL-3.0-only

package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/PeiceOfToast/shrinkyourshit/internal/flow"
	"github.com/PeiceOfToast/shrinkyourshit/internal/server"
	"github.com/PeiceOfToast/shrinkyourshit/internal/store"
)

// libraryBody is the create/update request body (API.md).
type libraryBody struct {
	Name         string       `json:"name"`
	Path         string       `json:"path"`
	Target       *flow.Target `json:"target"`
	Watch        bool         `json:"watch"`
	Schedule     string       `json:"schedule"`
	ResultPolicy string       `json:"result_policy"`
}

// listLibraries returns every library.
func (a *API) listLibraries(w http.ResponseWriter, r *http.Request) {
	libs, err := a.svc.Libraries().List(r.Context())
	if err != nil {
		a.internal(w, err)
		return
	}
	items := make([]server.LibraryDTO, 0, len(libs))
	for _, l := range libs {
		items = append(items, server.NewLibraryDTO(l))
	}
	writeJSON(w, http.StatusOK, server.ListResponse[server.LibraryDTO]{Items: items, Total: len(items)})
}

// createLibrary adds a library and kicks off its first scan.
func (a *API) createLibrary(w http.ResponseWriter, r *http.Request) {
	body, err := decode[libraryBody](r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	params, err := a.libraryParams(body, false)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	lib, err := a.svc.Libraries().Create(r.Context(), params)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, server.NewLibraryDTO(lib))
}

// getLibrary returns one library.
func (a *API) getLibrary(w http.ResponseWriter, r *http.Request) {
	lib, ok := a.library(w, r)
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, server.NewLibraryDTO(lib))
}

// updateLibrary replaces the library and re-evaluates its files.
func (a *API) updateLibrary(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	body, err := decode[libraryBody](r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	params, err := a.libraryParams(body, true)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	lib, err := a.svc.Libraries().Update(r.Context(), id, params)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "no such library")
		return
	}
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, server.NewLibraryDTO(lib))
}

// deleteLibrary removes the library's DB records only.
func (a *API) deleteLibrary(w http.ResponseWriter, r *http.Request) {
	err := a.svc.Libraries().Delete(r.Context(), r.PathValue("id"))
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "no such library")
		return
	}
	if err != nil {
		a.internal(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// scanLibrary starts an asynchronous scan.
func (a *API) scanLibrary(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if _, err := a.svc.Libraries().Get(r.Context(), id); errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "no such library")
		return
	} else if err != nil {
		a.internal(w, err)
		return
	}
	a.svc.Scanner().ScanAsync(id)
	writeJSON(w, http.StatusAccepted, map[string]bool{"started": true})
}

// listFiles returns a library's files, paginated, optional ?status= filter.
func (a *API) listFiles(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if _, err := a.svc.Libraries().Get(r.Context(), id); errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "no such library")
		return
	} else if err != nil {
		a.internal(w, err)
		return
	}

	limit, offset, ok := pageParams(r, w)
	if !ok {
		return
	}

	var files []*store.File
	var err error
	if status := r.URL.Query().Get("status"); status != "" {
		if !validFileStatus(status) {
			writeError(w, http.StatusBadRequest, "invalid_request",
				"invalid status filter: want one of "+strings.Join(store.FileStatusValues(), ", "))
			return
		}
		files, err = a.st.ListFilesByStatus(r.Context(), id, status)
	} else {
		files, err = a.st.ListFiles(r.Context(), id)
	}
	if err != nil {
		a.internal(w, err)
		return
	}
	writePage(w, files, limit, offset, server.NewFileDTO)
}

// listJobs returns jobs, paginated, optional ?status= filter.
func (a *API) listJobs(w http.ResponseWriter, r *http.Request) {
	limit, offset, ok := pageParams(r, w)
	if !ok {
		return
	}
	filter := store.JobFilter{}
	if status := r.URL.Query().Get("status"); status != "" {
		if !validJobStatus(status) {
			writeError(w, http.StatusBadRequest, "invalid_request",
				"invalid status filter: want one of "+strings.Join(store.JobStatusValues(), ", "))
			return
		}
		filter.Status = status
	}
	jobs, err := a.st.ListJobs(r.Context(), filter)
	if err != nil {
		a.internal(w, err)
		return
	}
	writePage(w, jobs, limit, offset, server.NewJobDTO)
}

// getJob returns one job including its log tail.
func (a *API) getJob(w http.ResponseWriter, r *http.Request) {
	job, err := a.st.GetJob(r.Context(), r.PathValue("id"))
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "no such job")
		return
	}
	if err != nil {
		a.internal(w, err)
		return
	}
	writeJSON(w, http.StatusOK, server.NewJobDTO(job))
}

// cancelJob stops an active job.
func (a *API) cancelJob(w http.ResponseWriter, r *http.Request) {
	_, err := a.svc.Queue().Cancel(r.Context(), r.PathValue("id"))
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "no such job")
		return
	}
	if errors.Is(err, server.ErrJobNotActive) {
		writeError(w, http.StatusConflict, "job_not_active", "job is not queued, claimed or running")
		return
	}
	if err != nil {
		a.internal(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// retryJob requeues a failed or cancelled job.
func (a *API) retryJob(w http.ResponseWriter, r *http.Request) {
	job, err := a.svc.Queue().Retry(r.Context(), r.PathValue("id"))
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "no such job")
		return
	}
	if errors.Is(err, server.ErrJobNotRetryable) {
		writeError(w, http.StatusConflict, "job_not_retryable", "only failed or cancelled jobs can be retried")
		return
	}
	if errors.Is(err, server.ErrFileSatisfied) {
		writeError(w, http.StatusConflict, "file_satisfied", "the file no longer needs work")
		return
	}
	if err != nil {
		a.internal(w, err)
		return
	}
	writeJSON(w, http.StatusOK, server.NewJobDTO(job))
}

// browse is the server-side folder picker: directory listing only, never
// file contents.
func (a *API) browse(w http.ResponseWriter, r *http.Request) {
	p := r.URL.Query().Get("path")
	if p == "" {
		p = string(os.PathSeparator)
	}
	p = filepath.Clean(p)
	if !filepath.IsAbs(p) {
		writeError(w, http.StatusBadRequest, "invalid_request", "path must be absolute")
		return
	}
	st, err := os.Stat(p)
	if os.IsNotExist(err) {
		writeError(w, http.StatusNotFound, "not_found", "no such directory")
		return
	}
	if err != nil {
		a.internal(w, err)
		return
	}
	if !st.IsDir() {
		writeError(w, http.StatusBadRequest, "invalid_request", "path is not a directory")
		return
	}

	entries, err := os.ReadDir(p)
	if err != nil {
		a.internal(w, err)
		return
	}
	type entry struct {
		Name  string `json:"name"`
		Path  string `json:"path"`
		IsDir bool   `json:"is_dir"`
	}
	out := make([]entry, 0, len(entries))
	for _, e := range entries {
		full := filepath.Join(p, e.Name())
		isDir := false
		if info, err := os.Stat(full); err == nil {
			isDir = info.IsDir()
		}
		out = append(out, entry{Name: e.Name(), Path: full, IsDir: isDir})
	}

	parent := ""
	if dir := filepath.Dir(p); dir != p {
		parent = dir
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"path":    p,
		"parent":  parent,
		"entries": out,
	})
}

// events streams SSE. Each event is "event: <type>\n" + "data: <json>\n\n".
func (a *API) events(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "internal", "streaming unsupported")
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Accel-Buffering", "no")

	id, ch := a.svc.Events().Subscribe()
	defer a.svc.Events().Unsubscribe(id)

	// Tell the client how often to expect a keep-alive.
	if _, err := w.Write([]byte("retry: 3000\n\n")); err != nil {
		return
	}
	flusher.Flush()

	heartbeat := time.NewTicker(15 * time.Second)
	defer heartbeat.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case <-heartbeat.C:
			if _, err := w.Write([]byte(": keepalive\n\n")); err != nil {
				return
			}
			flusher.Flush()
		case ev, ok := <-ch:
			if !ok {
				return
			}
			if _, err := w.Write([]byte("event: " + ev.Type + "\ndata: " + string(ev.Data) + "\n\n")); err != nil {
				return
			}
			flusher.Flush()
		}
	}
}

// library or 404.
func (a *API) library(w http.ResponseWriter, r *http.Request) (*store.Library, bool) {
	lib, err := a.svc.Libraries().Get(r.Context(), r.PathValue("id"))
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "no such library")
		return nil, false
	}
	if err != nil {
		a.internal(w, err)
		return nil, false
	}
	return lib, true
}

// libraryParams converts the request body to manager params. On create a
// missing target falls back to the default; on update it is required (PUT
// semantics).
func (a *API) libraryParams(body libraryBody, update bool) (server.LibraryParams, error) {
	params := server.LibraryParams{
		Name:         body.Name,
		Path:         body.Path,
		Watch:        body.Watch,
		Schedule:     body.Schedule,
		ResultPolicy: body.ResultPolicy,
	}
	if body.Target != nil {
		raw, err := json.Marshal(*body.Target)
		if err != nil {
			return params, err
		}
		params.TargetJSON = raw
	} else if update {
		return params, errors.New("target is required on update")
	}
	return params, nil
}

// pageParams reads ?limit=&offset= with sane bounds.
func pageParams(r *http.Request, w http.ResponseWriter) (limit, offset int, ok bool) {
	limit, offset = 100, 0
	if v := r.URL.Query().Get("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 || n > 1000 {
			writeError(w, http.StatusBadRequest, "invalid_request", "limit must be 1..1000")
			return 0, 0, false
		}
		limit = n
	}
	if v := r.URL.Query().Get("offset"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 0 {
			writeError(w, http.StatusBadRequest, "invalid_request", "offset must be >= 0")
			return 0, 0, false
		}
		offset = n
	}
	return limit, offset, true
}

// writePage slices items to the page and wraps them in the list envelope.
func writePage[T any, R any](w http.ResponseWriter, items []T, limit, offset int, mapFn func(T) R) {
	total := len(items)
	start := min(offset, total)
	end := min(start+limit, total)
	page := items[start:end]
	out := make([]R, 0, len(page))
	for _, item := range page {
		out = append(out, mapFn(item))
	}
	writeJSON(w, http.StatusOK, server.ListResponse[R]{Items: out, Total: total})
}

func validFileStatus(s string) bool {
	for _, v := range store.FileStatusValues() {
		if v == s {
			return true
		}
	}
	return false
}

func validJobStatus(s string) bool {
	for _, v := range store.JobStatusValues() {
		if v == s {
			return true
		}
	}
	return false
}

// decode reads and unmarshals a JSON request body.
func decode[T any](r *http.Request) (T, error) {
	var body T
	err := json.NewDecoder(r.Body).Decode(&body)
	if err != nil {
		return body, errors.New("invalid JSON body")
	}
	return body, nil
}

// internal logs and returns a 500.
func (a *API) internal(w http.ResponseWriter, err error) {
	a.log.Error("api: internal error", "err", err)
	writeError(w, http.StatusInternalServerError, "internal", "internal error")
}
