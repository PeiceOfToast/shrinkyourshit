// SPDX-License-Identifier: AGPL-3.0-only

package api

import (
	"bytes"
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/PeiceOfToast/shrinkyourshit/internal/flow"
	"github.com/PeiceOfToast/shrinkyourshit/internal/probe"
	"github.com/PeiceOfToast/shrinkyourshit/internal/server"
	"github.com/PeiceOfToast/shrinkyourshit/internal/store"
)

// libraryBody is the create/update request body (API.md). Target, Easy and
// ResultPolicy are the pre-profile rule fields (task 74): they are accepted
// for one release and ignored, and the reply names each one that was sent.
// The rule a library runs is its encode profile's, chosen with ProfileID.
type libraryBody struct {
	Name         string              `json:"name"`
	Path         string              `json:"path"`
	Target       *flow.Target        `json:"target"`
	Easy         *flow.EasyModeInput `json:"easy"`
	Watch        bool                `json:"watch"`
	Schedule     string              `json:"schedule"`
	ResultPolicy string              `json:"result_policy"`
	// FileKinds is a pointer so the API can tell "not sent" from "sent
	// empty": a missing field defaults to ["video"], an explicit empty
	// list is rejected (task 34).
	FileKinds *[]string `json:"file_kinds"`
	// ProfileID is the encode profile the library follows (task 73). Empty
	// keeps the library's current profile on update and takes the default
	// profile on create; a library never detaches (task 74).
	ProfileID string `json:"profile_id"`
}

// deprecatedField is one entry of the deprecated list a library save answers:
// a field the request sent that the server ignored, or a default it applied.
type deprecatedField struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

// libraryReply is the library create and update reply: the library, plus the
// deprecated list (task 74). The list is always present, empty when the
// request used only current fields.
type libraryReply struct {
	server.LibraryDTO
	Deprecated []deprecatedField `json:"deprecated"`
}

// newLibraryReply builds the save reply for lib from the request body.
func newLibraryReply(lib *store.Library, body libraryBody) libraryReply {
	notes := []deprecatedField{}
	const ignored = "ignored: a library runs its encode profile's rule; set it on the profile (PUT /api/v1/profiles/{id})"
	if body.Target != nil {
		notes = append(notes, deprecatedField{Field: "target", Message: ignored})
	}
	if body.Easy != nil {
		notes = append(notes, deprecatedField{Field: "easy", Message: ignored})
	}
	if body.ResultPolicy != "" {
		notes = append(notes, deprecatedField{Field: "result_policy", Message: "ignored: the result policy is the profile's; set it on the profile"})
	}
	if body.ProfileID == "" {
		notes = append(notes, deprecatedField{Field: "profile_id", Message: "not sent: the library follows encode profile " + lib.ProfileID + "; send profile_id to choose another"})
	}
	return libraryReply{LibraryDTO: server.NewLibraryDTO(lib), Deprecated: notes}
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

// createLibrary adds a library and kicks off its first scan. An empty name
// defaults to the folder's base name (the easy screen has no name field).
func (a *API) createLibrary(w http.ResponseWriter, r *http.Request) {
	body, err := decode[libraryBody](r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	if body.Name == "" && body.Path != "" {
		body.Name = filepath.Base(filepath.Clean(body.Path))
	}
	lib, err := a.svc.Libraries().Create(r.Context(), libraryParams(body))
	if err != nil {
		writeLibraryError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, newLibraryReply(lib, body))
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
	lib, err := a.svc.Libraries().Update(r.Context(), id, libraryParams(body))
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "no such library")
		return
	}
	if err != nil {
		writeLibraryError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, newLibraryReply(lib, body))
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

// listFiles returns a library's files, paginated, with an optional status
// filter plus the task 19 search and sort: `q` matches the file's base name
// (case-insensitive substring), `sort` orders by name, size or bytes saved
// and `dir` flips the order. Unknown sort or dir values are 400. Filtering
// and sorting happen here, server side; the browser never re-sorts a page.
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
	sortKey, dir, ok := fileSortParams(r, w)
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

	if q := queryText(r); q != "" {
		files = filterFilesByName(files, q)
	}
	sortFiles(files, sortKey, dir)

	// The file table shows the encoder that produced the file's current
	// state (task 19): resolved from the file's most recent job, one
	// lookup per page row (writePage maps only the sliced page). A missing
	// job leaves the field empty rather than failing the response.
	writePage(w, files, limit, offset, func(f *store.File) server.FileDTO {
		dto := server.NewFileDTO(f)
		if f.LastJobID != "" {
			if j, err := a.st.GetJob(r.Context(), f.LastJobID); err == nil {
				dto.LastJobEncoder = j.EncoderUsed
			}
		}
		return dto
	})
}

// fileSortParams reads ?sort= and ?dir= (task 19). An empty sort keeps the
// store's order and dir is ignored; unknown values are 400 invalid_request.
func fileSortParams(r *http.Request, w http.ResponseWriter) (sort, dir string, ok bool) {
	sort = r.URL.Query().Get("sort")
	switch sort {
	case "", "name", "size", "saved":
	default:
		writeError(w, http.StatusBadRequest, "invalid_request",
			"sort must be name, size or saved")
		return "", "", false
	}
	dir = r.URL.Query().Get("dir")
	switch dir {
	case "", "asc", "desc":
	default:
		writeError(w, http.StatusBadRequest, "invalid_request",
			"dir must be asc or desc")
		return "", "", false
	}
	return sort, dir, true
}

// queryText reads and normalizes the ?q= search text (task 19): trimmed and
// lowercased, so matching is case-insensitive against the lowered name.
func queryText(r *http.Request) string {
	return strings.ToLower(strings.TrimSpace(r.URL.Query().Get("q")))
}

// filterFilesByName keeps the files whose base name contains q
// (case-insensitive; q arrives normalized from queryText). The directory
// part of the path never matches, so a folder called "movies" does not
// summon every file inside it.
func filterFilesByName(files []*store.File, q string) []*store.File {
	kept := files[:0]
	for _, f := range files {
		if strings.Contains(strings.ToLower(filepath.Base(f.Path)), q) {
			kept = append(kept, f)
		}
	}
	return kept
}

// sortFiles orders files in place (task 19). sort is a validated key
// (fileSortParams); dir is asc (default) or desc. An empty sort key keeps
// the store's created_at order and dir is ignored. Stable, so equal keys
// keep their store order in both directions.
func sortFiles(files []*store.File, sort, dir string) {
	if sort == "" {
		return
	}
	desc := dir == "desc"
	slices.SortStableFunc(files, func(a, b *store.File) int {
		c := compareFiles(a, b, sort)
		if desc {
			return -c
		}
		return c
	})
}

// compareFiles returns -1, 0 or 1 for one sort key.
func compareFiles(a, b *store.File, sort string) int {
	switch sort {
	case "name":
		an, bn := strings.ToLower(filepath.Base(a.Path)), strings.ToLower(filepath.Base(b.Path))
		return strings.Compare(an, bn)
	case "size":
		return cmp.Compare(a.Size, b.Size)
	case "saved":
		return cmp.Compare(a.BytesSaved, b.BytesSaved)
	}
	return 0
}

// jobDTO maps a job to its DTO with the file identity resolved (task 18):
// one best-effort file lookup, so responses carry the file's basename and
// full path. An unknown file leaves the name fields empty rather than
// failing the response.
func (a *API) jobDTO(ctx context.Context, j *store.Job) server.JobDTO {
	var f *store.File
	if j.FileID != "" {
		if got, err := a.st.GetFile(ctx, j.FileID); err == nil {
			f = got
		}
	}
	dto := server.NewJobDTO(j, f)
	// Task 27: the log row count and has_log ride with every job response.
	if total, err := a.st.CountJobLog(ctx, j.ID); err == nil {
		dto.LogLines = total
		dto.HasLog = total > 0
	}
	return dto
}

// listJobs returns jobs, paginated, optional ?status= filter.
func (a *API) listJobs(w http.ResponseWriter, r *http.Request) {
	limit, offset, ok := pageParams(r, w)
	if !ok {
		return
	}
	filter, ok := jobFilterParams(r, w)
	if !ok {
		return
	}
	jobs, err := a.st.ListJobs(r.Context(), filter)
	if err != nil {
		a.internal(w, err)
		return
	}
	writePage(w, jobs, limit, offset, func(j *store.Job) server.JobDTO {
		return a.jobDTO(r.Context(), j)
	})
}

// fileJobs answers GET /files/{id}/jobs (task 19): one file's job history,
// oldest first. Unknown file is 404. The filter parsing is the same as
// GET /jobs, with the file id pinned.
func (a *API) fileJobs(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if _, err := a.st.GetFile(r.Context(), id); errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "no such file")
		return
	} else if err != nil {
		a.internal(w, err)
		return
	}
	limit, offset, ok := pageParams(r, w)
	if !ok {
		return
	}
	filter, ok := jobFilterParams(r, w)
	if !ok {
		return
	}
	filter.FileID = id
	jobs, err := a.st.ListJobs(r.Context(), filter)
	if err != nil {
		a.internal(w, err)
		return
	}
	writePage(w, jobs, limit, offset, func(j *store.Job) server.JobDTO {
		return a.jobDTO(r.Context(), j)
	})
}

// jobFilterParams reads the shared job list query parameters: ?status= and
// ?order= (oldest first unless desc). Invalid values are 400.
func jobFilterParams(r *http.Request, w http.ResponseWriter) (store.JobFilter, bool) {
	filter := store.JobFilter{}
	if status := r.URL.Query().Get("status"); status != "" {
		if !validJobStatus(status) {
			writeError(w, http.StatusBadRequest, "invalid_request",
				"invalid status filter: want one of "+strings.Join(store.JobStatusValues(), ", "))
			return filter, false
		}
		filter.Status = status
	}
	switch order := r.URL.Query().Get("order"); order {
	case "":
	case "desc":
		filter.Order = order
	default:
		writeError(w, http.StatusBadRequest, "invalid_request", "order must be desc")
		return filter, false
	}
	return filter, true
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
	writeJSON(w, http.StatusOK, a.jobDTO(r.Context(), job))
}

// jobLogLineDTO is the JSON shape of one job log row (task 27).
type jobLogLineDTO struct {
	ID       int64  `json:"id"`
	Seq      int64  `json:"seq"`
	At       string `json:"at"`
	Level    string `json:"level"`
	Source   string `json:"source"`
	Message  string `json:"message"`
	Repeated int64  `json:"repeated"`
}

// jobLogResponse is the GET /jobs/{id}/log envelope: the rows in id order,
// the job's total row count and whether newer rows exist beyond this page
// (a viewer follows a running job with after_id without re-reading all).
// Status is the job's current status, so a viewer can stop polling when the
// job goes terminal without a second request.
type jobLogResponse struct {
	Lines   []jobLogLineDTO `json:"lines"`
	Total   int64           `json:"total"`
	HasMore bool            `json:"has_more"`
	Status  string          `json:"status"`
}

// jobLogParams are the GET /jobs/{id}/log query parameters (task 27).
const (
	jobLogDefaultLimit = 500
	jobLogMaxLimit     = 2000
)

// getJobLog answers GET /jobs/{id}/log: one page of the job's structured
// log rows. ?after_id=<id> returns only newer rows, ?level=<info|warn|error>
// filters, ?limit=<n> caps the page (default 500, max 2000). A job with no
// log rows answers 200 with an empty list; 404 only when the job is missing.
func (a *API) getJobLog(w http.ResponseWriter, r *http.Request) {
	job, err := a.st.GetJob(r.Context(), r.PathValue("id"))
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "no such job")
		return
	}
	if err != nil {
		a.internal(w, err)
		return
	}
	var afterID int64
	if raw := r.URL.Query().Get("after_id"); raw != "" {
		v, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || v < 0 {
			writeError(w, http.StatusBadRequest, "invalid_request", "after_id must be a non-negative integer")
			return
		}
		afterID = v
	}
	level := r.URL.Query().Get("level")
	if level != "" && level != store.JobLogInfo && level != store.JobLogWarn && level != store.JobLogError {
		writeError(w, http.StatusBadRequest, "invalid_request", "level must be info, warn or error")
		return
	}
	limit := jobLogDefaultLimit
	if raw := r.URL.Query().Get("limit"); raw != "" {
		v, err := strconv.Atoi(raw)
		if err != nil || v < 1 {
			writeError(w, http.StatusBadRequest, "invalid_request", "limit must be a positive integer")
			return
		}
		limit = min(v, jobLogMaxLimit)
	}
	// One extra row tells has_more without a second query.
	rows, err := a.st.ListJobLog(r.Context(), job.ID, afterID, level, limit+1)
	if err != nil {
		a.internal(w, err)
		return
	}
	total, err := a.st.CountJobLog(r.Context(), job.ID)
	if err != nil {
		a.internal(w, err)
		return
	}
	hasMore := len(rows) > limit
	if hasMore {
		rows = rows[:limit]
	}
	lines := make([]jobLogLineDTO, 0, len(rows))
	for _, row := range rows {
		lines = append(lines, jobLogLineDTO{
			ID:       row.ID,
			Seq:      row.Seq,
			At:       row.At.Format(time.RFC3339Nano),
			Level:    row.Level,
			Source:   row.Source,
			Message:  row.Message,
			Repeated: row.Repeated,
		})
	}
	writeJSON(w, http.StatusOK, jobLogResponse{
		Lines: lines, Total: total, HasMore: hasMore, Status: job.Status,
	})
}

// downloadJobLog answers GET /jobs/{id}/log/download: the same rows as
// plain text, one row per line as `<at> <level> <source> <message>`.
func (a *API) downloadJobLog(w http.ResponseWriter, r *http.Request) {
	job, err := a.st.GetJob(r.Context(), r.PathValue("id"))
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "no such job")
		return
	}
	if err != nil {
		a.internal(w, err)
		return
	}
	rows, err := a.st.ListJobLog(r.Context(), job.ID, 0, "", 0)
	if err != nil {
		a.internal(w, err)
		return
	}
	var buf bytes.Buffer
	for _, row := range rows {
		fmt.Fprintf(&buf, "%s %s %s %s\n",
			row.At.Format(time.RFC3339Nano), row.Level, row.Source, row.Message)
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", "job-"+job.ID+".log"))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(buf.Bytes())
}

// cancelJob stops an active job. A transfer-mode upload (task 09) is
// discarded with it: a cancelled job's partial upload is dead weight.
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
	a.svc.Transfer().Discard(r.PathValue("id"))
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
	writeJSON(w, http.StatusOK, a.jobDTO(r.Context(), job))
}

// getStats answers GET /stats: totals plus the per-library breakdown
// (API.md, task 07).
func (a *API) getStats(w http.ResponseWriter, r *http.Request) {
	stats, err := a.svc.Stats().Get(r.Context())
	if err != nil {
		a.internal(w, err)
		return
	}
	writeJSON(w, http.StatusOK, stats)
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

// libraryParams converts the request body to manager params. The rule fields
// (target, easy, result_policy) are not params any more (task 74): the library
// follows the profile named by profile_id.
func libraryParams(body libraryBody) server.LibraryParams {
	// File kinds (task 34): a missing field defaults to ["video"], an
	// explicitly empty list is rejected by the manager's validation.
	kinds := []string{string(probe.KindVideo)}
	if body.FileKinds != nil {
		kinds = *body.FileKinds
	}
	return server.LibraryParams{
		Name:      body.Name,
		Path:      body.Path,
		Watch:     body.Watch,
		Schedule:  body.Schedule,
		FileKinds: kinds,
		ProfileID: body.ProfileID,
	}
}

// writeLibraryError maps a failed library save: a flow the profile or the
// library cannot run is 400 invalid_flow, a plugin step that is not installed
// is 422, anything else (a bad name, path or profile) is 400 invalid_request.
func writeLibraryError(w http.ResponseWriter, err error) {
	switch {
	case server.IsInvalidFlow(err):
		flowErr := &server.ErrInvalidFlow{}
		_ = errors.As(err, &flowErr)
		writeFlowErrors(w, flowErr.Errors)
	case server.IsPluginStepError(err):
		writeError(w, http.StatusUnprocessableEntity, "plugin_unavailable", err.Error())
	default:
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
	}
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

// decodeLenient reads an optional JSON request body: an empty body yields
// the zero struct (the stop endpoint's drain flag defaults to false),
// anything present must parse as JSON.
func decodeLenient[T any](r *http.Request) (T, error) {
	var body T
	raw, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		return body, errors.New("could not read the request body")
	}
	if len(bytes.TrimSpace(raw)) == 0 {
		return body, nil
	}
	if err := json.Unmarshal(raw, &body); err != nil {
		return body, errors.New("invalid JSON body")
	}
	return body, nil
}

// internal logs and returns a 500.
func (a *API) internal(w http.ResponseWriter, err error) {
	a.log.Error("api: internal error", "err", err)
	writeError(w, http.StatusInternalServerError, "internal", "internal error")
}
