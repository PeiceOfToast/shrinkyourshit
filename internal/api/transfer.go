// SPDX-License-Identifier: AGPL-3.0-only

package api

import (
	"errors"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/PeiceOfToast/shrinkyourshit/internal/server"
	"github.com/PeiceOfToast/shrinkyourshit/internal/store"
)

// Transfer-mode endpoints (API.md, task 09): the shared-storage access
// check, the resumable source stream and the chunked result upload. All
// three are node-authenticated worker routes.

// accessCheckBody is the optional POST .../access-check body: the library
// the node asks about. The endpoint always answers for the job's own file;
// a mismatching library_id is rejected rather than answered.
type accessCheckBody struct {
	LibraryID string `json:"library_id"`
}

// workerAccessCheck answers POST /worker/jobs/{id}/access-check with the
// sample the node must reproduce from the file it sees through its path
// mappings. Match → the library is visible to the node (shared); anything
// else → transfer.
func (a *API) workerAccessCheck(w http.ResponseWriter, r *http.Request, node *store.Node) {
	job, ok := a.ownedJob(w, r, node)
	if !ok {
		return
	}
	body, err := decode[accessCheckBody](r)
	if err == nil && body.LibraryID != "" && body.LibraryID != job.LibraryID {
		writeError(w, http.StatusBadRequest, "invalid_request", "library_id does not match the job")
		return
	}
	f, err := a.st.GetFile(workerCtx(r), job.FileID)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "no such file")
		return
	}
	if err != nil {
		a.internal(w, err)
		return
	}
	challenge, err := a.svc.Transfer().Challenge(node, f)
	if err != nil {
		// The source is not readable on the server side: the node should
		// fall back to transfer (where the download will fail with the
		// same underlying reason, honestly reported on the job).
		a.internal(w, err)
		return
	}
	writeJSON(w, http.StatusOK, challenge)
}

// workerSource streams the job's source file (GET .../source). Range
// requests make interrupted downloads resumable; the ETag is the file's
// fingerprint, so If-Range on a resumed download guarantees the node
// continues the same file it was queued for.
func (a *API) workerSource(w http.ResponseWriter, r *http.Request, node *store.Node) {
	job, ok := a.ownedJob(w, r, node)
	if !ok {
		return
	}
	f, err := a.st.GetFile(r.Context(), job.FileID)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "no such file")
		return
	}
	if err != nil {
		a.internal(w, err)
		return
	}
	file, err := os.Open(f.Path)
	if err != nil {
		if os.IsNotExist(err) {
			writeError(w, http.StatusNotFound, "not_found", "source file is gone from the library")
			return
		}
		a.internal(w, err)
		return
	}
	defer func() { _ = file.Close() }()
	w.Header().Set("ETag", `"`+f.Fingerprint+`"`)
	http.ServeContent(w, r, "source", time.Unix(f.Mtime, 0), file)
}

// contentRange parses a strict "bytes start-end/total" Content-Range
// header. Wildcards are rejected: the worker always knows the sizes.
func parseContentRange(header string) (start, end, total int64, err error) {
	h := strings.TrimSpace(header)
	h = strings.TrimPrefix(h, "bytes ")
	startS, rest, ok := strings.Cut(h, "-")
	if !ok {
		return 0, 0, 0, fmt.Errorf("malformed Content-Range %q", header)
	}
	endS, totalS, ok := strings.Cut(rest, "/")
	if !ok || totalS == "*" {
		return 0, 0, 0, fmt.Errorf("malformed Content-Range %q", header)
	}
	start, err1 := strconv.ParseInt(strings.TrimSpace(startS), 10, 64)
	end, err2 := strconv.ParseInt(strings.TrimSpace(endS), 10, 64)
	total, err3 := strconv.ParseInt(strings.TrimSpace(totalS), 10, 64)
	if err1 != nil || err2 != nil || err3 != nil || start < 0 || end < start {
		return 0, 0, 0, fmt.Errorf("malformed Content-Range %q", header)
	}
	return start, end, total, nil
}

// workerResult receives one chunk of the job's result upload
// (PUT .../result). Every request carries Content-Range; the final one
// carries X-Checksum-SHA256, which the server verifies before the upload
// counts as received. 204 = chunk accepted, more to come; 200 = upload
// complete and verified.
func (a *API) workerResult(w http.ResponseWriter, r *http.Request, node *store.Node) {
	job, ok := a.ownedJob(w, r, node)
	if !ok {
		return
	}
	start, end, total, err := parseContentRange(r.Header.Get("Content-Range"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	if r.Body == nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "chunk body is required")
		return
	}
	count := end - start + 1
	if count > server.MaxChunkBytes {
		writeError(w, http.StatusBadRequest, "invalid_request",
			fmt.Sprintf("chunk exceeds %d bytes", server.MaxChunkBytes))
		return
	}
	// A chunk larger than declared would hide bytes from the checksum
	// accounting; limit the body to the declared size (plus slack for the
	// case where the client overshot slightly — the extra byte fails the
	// exact read below anyway).
	r.Body = http.MaxBytesReader(w, r.Body, count+1024)

	done, err := a.svc.Transfer().PutChunk(workerCtx(r), job, start, end, total, r.Body,
		r.Header.Get("X-Checksum-SHA256"))
	switch {
	case errors.Is(err, server.ErrUploadGap):
		writeError(w, http.StatusConflict, "upload_gap",
			"chunk does not continue the received upload; restart from byte 0")
		return
	case errors.Is(err, server.ErrChecksumMismatch):
		writeError(w, http.StatusUnprocessableEntity, "checksum_mismatch",
			"the uploaded result failed its SHA-256 verification; the original is untouched")
		return
	case err != nil:
		a.internal(w, err)
		return
	}
	if done {
		writeJSON(w, http.StatusOK, map[string]int64{"received": total})
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// transferCompleteResult is what the transfer-mode completion hands to
// Queue.Complete after the disk work is done.
type transferCompleteResult struct {
	OutputPath string
	SkipReason string
	BytesAfter int64
}

// transferComplete performs the server-side verify-and-replace of a
// transfer-mode job (task 09): the verified upload replaces the original
// according to the library's result policy (the same ApplyResultPolicy the
// worker uses in shared mode), then Complete records the outcome.
// Skipped outputs (replace_if_smaller, not smaller) leave the original and
// are reported through the skip reason.
func (a *API) transferComplete(w http.ResponseWriter, r *http.Request, node *store.Node) (transferCompleteResult, bool) {
	jobID := r.PathValue("id")
	ctx := workerCtx(r)
	job, err := a.st.GetJob(ctx, jobID)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "no such job")
		return transferCompleteResult{}, false
	}
	if err != nil {
		a.internal(w, err)
		return transferCompleteResult{}, false
	}
	f, err := a.st.GetFile(ctx, job.FileID)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "no such file")
		return transferCompleteResult{}, false
	}
	if err != nil {
		a.internal(w, err)
		return transferCompleteResult{}, false
	}
	lib, err := a.svc.Libraries().Get(ctx, job.LibraryID)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "no such library")
		return transferCompleteResult{}, false
	}
	if err != nil {
		a.internal(w, err)
		return transferCompleteResult{}, false
	}

	upload, size, err := a.svc.Transfer().VerifiedOutput(ctx, job)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusConflict, "result_missing",
			"no verified result has been uploaded for this job")
		return transferCompleteResult{}, false
	}
	if err != nil {
		a.internal(w, err)
		return transferCompleteResult{}, false
	}

	// The library's flow decides the result policy and rename template
	// (task 14): a skip_if_smaller step makes the job behave like
	// replace_if_smaller, and a rename step names the keep-both output.
	policy, rename, err := server.EffectivePolicy(lib)
	if err != nil {
		a.internal(w, err)
		return transferCompleteResult{}, false
	}
	applied, err := server.ApplyResultPolicy(ctx, policy, upload, f.Path, size, f.Size, nil, rename)
	if err != nil {
		a.internal(w, err)
		return transferCompleteResult{}, false
	}
	return transferCompleteResult{
		OutputPath: applied.FinalPath,
		SkipReason: applied.SkipReason,
		BytesAfter: size,
	}, true
}
