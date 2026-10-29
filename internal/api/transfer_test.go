// SPDX-License-Identifier: AGPL-3.0-only

package api

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/PeiceOfToast/shrinkyourshit/internal/flow"
	"github.com/PeiceOfToast/shrinkyourshit/internal/probe"
	"github.com/PeiceOfToast/shrinkyourshit/internal/server"
	"github.com/PeiceOfToast/shrinkyourshit/internal/store"
	"github.com/stretchr/testify/require"
)

// transferFixture is a claimed job over a real file on disk, with the node
// that owns it. The node is configured auto by default (transfer/e2e
// territory); tests force the mode through node.AccessMode.
type transferFixture struct {
	h      *apiHarness
	key    string
	node   *store.Node
	job    *store.Job
	file   *store.File
	lib    *store.Library
	srcDir string
	src    string
	data   string
}

// newTransferFixture queues a job for a real file and claims it with a
// fresh approved node.
func newTransferFixture(t *testing.T, accessMode string) *transferFixture {
	t.Helper()
	ctx := context.Background()
	h := newAPIHarness(t)
	f := &transferFixture{h: h, srcDir: t.TempDir()}
	f.data = strings.Repeat("MOVIE-BYTES-", 40_000)
	f.src = filepath.Join(f.srcDir, "movie.mkv")
	require.NoError(t, os.WriteFile(f.src, []byte(f.data), 0o644))
	st, err := os.Stat(f.src)
	require.NoError(t, err)
	fp, err := probe.Fingerprint(f.src)
	require.NoError(t, err)

	target, err := json.Marshal(flow.Default())
	require.NoError(t, err)
	f.lib, err = h.svc.Libraries().Create(ctx, server.LibraryParams{
		Name: "Movies", Path: f.srcDir, TargetJSON: target,
		ResultPolicy: store.ResultPolicyReplaceIfSmaller,
	})
	require.NoError(t, err)

	f.file = &store.File{LibraryID: f.lib.ID, Path: f.src, Size: st.Size(),
		Mtime: st.ModTime().Unix(), Fingerprint: fp,
		Status: store.FileStatusQueued, ProbeJSON: `{"container":"mkv"}`}
	require.NoError(t, h.st.UpsertFile(ctx, f.file))

	f.job = &store.Job{FileID: f.file.ID, LibraryID: f.lib.ID,
		Status: store.JobStatusQueued, BytesBefore: st.Size()}
	require.NoError(t, h.st.CreateJob(ctx, f.job))

	f.key = "transfer-node-key"
	f.node = &store.Node{ID: "transfer-node", Name: "remote", KeyHash: server.HashKey(f.key),
		AccessMode: accessMode, PathMappings: "[]",
		Status: store.NodeStatusApproved, Enabled: true}
	require.NoError(t, h.st.CreateNode(ctx, f.node))

	resp := h.doNode(http.MethodPost, "/api/v1/worker/claim", nil, f.key)
	require.Equal(t, http.StatusOK, resp.Code, resp.Body.String())
	return f
}

// doNodePut sends a request with a raw body and extra headers using the
// node key.
func (h *apiHarness) doNodePut(path string, extra map[string][]string, key string, body string) *httptest.ResponseRecorder {
	h.t.Helper()
	req := httptest.NewRequest(http.MethodPut, path, strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+key)
	for k, vs := range extra {
		for _, v := range vs {
			req.Header.Add(k, v)
		}
	}
	rec := httptest.NewRecorder()
	h.mux.ServeHTTP(rec, req)
	return rec
}

// checksumOf hashes a byte slice the way the transfer protocol expects.
func checksumOf(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// itoa formats an int64 for header values.
func itoa(v int64) string { return strconv.FormatInt(v, 10) }

func TestAccessCheckRequiresAnOwnedActiveJob(t *testing.T) {
	f := newTransferFixture(t, store.NodeAccessShared)
	otherKey, _ := f.h.addNode(t, "intruder", store.NodeStatusApproved, true)

	// Another node's key gets 403.
	resp := f.h.doNode(http.MethodPost, "/api/v1/worker/jobs/"+f.job.ID+"/access-check",
		map[string]any{}, otherKey)
	require.Equal(t, http.StatusForbidden, resp.Code)

	// A queued (unclaimed) job gets 409.
	_, _, queued := f.h.queueOne(t)
	resp = f.h.doNode(http.MethodPost, "/api/v1/worker/jobs/"+queued.ID+"/access-check",
		map[string]any{}, f.key)
	require.Equal(t, http.StatusConflict, resp.Code)
}

func TestAccessCheckReturnsMappedPathAndSample(t *testing.T) {
	f := newTransferFixture(t, store.NodeAccessShared)
	// Give the node a mapping: the returned path must be node-side.
	f.node.PathMappings = `[{"server_path": "` + f.srcDir + `", "node_path": "/mnt/nas"}]`
	require.NoError(t, f.h.st.UpdateNode(context.Background(), f.node))

	resp := f.h.doNode(http.MethodPost, "/api/v1/worker/jobs/"+f.job.ID+"/access-check",
		map[string]any{"library_id": f.lib.ID}, f.key)
	require.Equal(t, http.StatusOK, resp.Code, resp.Body.String())
	var ch struct {
		Path         string `json:"path"`
		Size         int64  `json:"size"`
		SampleOffset int64  `json:"sample_offset"`
		SampleSHA256 string `json:"sample_sha256"`
	}
	require.NoError(t, json.Unmarshal(resp.Body.Bytes(), &ch))
	require.Equal(t, "/mnt/nas/movie.mkv", ch.Path)
	require.Equal(t, int64(len(f.data)), ch.Size)
	require.GreaterOrEqual(t, ch.SampleOffset, int64(0))
	require.Less(t, ch.SampleOffset, ch.Size)

	// The digest matches the file's bytes at that offset.
	file, err := os.Open(f.src)
	require.NoError(t, err)
	defer func() { _ = file.Close() }()
	_, err = file.Seek(ch.SampleOffset, io.SeekStart)
	require.NoError(t, err)
	hsh := sha256.New()
	_, err = io.CopyN(hsh, file, min(int64(1<<20), ch.Size-ch.SampleOffset))
	require.NoError(t, err)
	require.Equal(t, hex.EncodeToString(hsh.Sum(nil)), ch.SampleSHA256)
}

func TestAccessCheckRejectsForeignLibraryID(t *testing.T) {
	f := newTransferFixture(t, store.NodeAccessShared)
	resp := f.h.doNode(http.MethodPost, "/api/v1/worker/jobs/"+f.job.ID+"/access-check",
		map[string]any{"library_id": "not-this-library"}, f.key)
	require.Equal(t, http.StatusBadRequest, resp.Code)
}

func TestSourceSupportsRangeAndETag(t *testing.T) {
	f := newTransferFixture(t, store.NodeAccessShared)

	// Full download carries the fingerprint ETag.
	resp := f.h.doNode(http.MethodGet, "/api/v1/worker/jobs/"+f.job.ID+"/source", nil, f.key)
	require.Equal(t, http.StatusOK, resp.Code)
	require.Equal(t, `"`+f.file.Fingerprint+`"`, resp.Header().Get("ETag"))
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Equal(t, f.data, string(body))

	// A range request returns exactly that slice with 206.
	req := httptest.NewRequest(http.MethodGet, "/api/v1/worker/jobs/"+f.job.ID+"/source", nil)
	req.Header.Set("Range", "bytes=10-19")
	req.Header.Set("Authorization", "Bearer "+f.key)
	rec := httptest.NewRecorder()
	f.h.mux.ServeHTTP(rec, req)
	require.Equal(t, http.StatusPartialContent, rec.Code)
	require.Equal(t, f.data[10:20], rec.Body.String())
	require.Equal(t, "bytes 10-19/"+itoa(int64(len(f.data))), rec.Header().Get("Content-Range"))

	// If-Range with the right ETag still resumes…
	req = httptest.NewRequest(http.MethodGet, "/api/v1/worker/jobs/"+f.job.ID+"/source", nil)
	req.Header.Set("Range", "bytes=20-")
	req.Header.Set("If-Range", `"`+f.file.Fingerprint+`"`)
	req.Header.Set("Authorization", "Bearer "+f.key)
	rec = httptest.NewRecorder()
	f.h.mux.ServeHTTP(rec, req)
	require.Equal(t, http.StatusPartialContent, rec.Code)
	require.Equal(t, f.data[20:], rec.Body.String())

	// …but a changed file restarts from scratch (200).
	req = httptest.NewRequest(http.MethodGet, "/api/v1/worker/jobs/"+f.job.ID+"/source", nil)
	req.Header.Set("Range", "bytes=20-")
	req.Header.Set("If-Range", `"0:0:stale"`)
	req.Header.Set("Authorization", "Bearer "+f.key)
	rec = httptest.NewRecorder()
	f.h.mux.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)
}

func TestResultUploadChunks(t *testing.T) {
	f := newTransferFixture(t, store.NodeAccessTransfer)
	payload := strings.Repeat("OUT-", 30_000)
	total := int64(len(payload))
	chunk := int64(8 << 10)
	checksum := checksumOf([]byte(payload))
	base := "/api/v1/worker/jobs/" + f.job.ID + "/result"

	// Chunk 1 → 204.
	resp := f.h.doNodePut(base, map[string][]string{
		"Content-Range": {"bytes 0-" + itoa(chunk-1) + "/" + itoa(total)},
	}, f.key, payload[:chunk])
	require.Equal(t, http.StatusNoContent, resp.Code, resp.Body.String())

	// Final chunk with checksum → 200 and the output is verified on the
	// server side.
	resp = f.h.doNodePut(base, map[string][]string{
		"Content-Range":     {"bytes " + itoa(chunk) + "-" + itoa(total-1) + "/" + itoa(total)},
		"X-Checksum-SHA256": {checksum},
	}, f.key, payload[chunk:])
	require.Equal(t, http.StatusOK, resp.Code, resp.Body.String())

	path, size, err := f.h.svc.Transfer().VerifiedOutput(context.Background(), f.job)
	require.NoError(t, err)
	require.Equal(t, total, size)
	got, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, payload, string(got))
}

func TestResultUploadRejectsGapAndBadChecksum(t *testing.T) {
	f := newTransferFixture(t, store.NodeAccessTransfer)
	base := "/api/v1/worker/jobs/" + f.job.ID + "/result"

	// Start beyond the received prefix → 409 upload_gap.
	resp := f.h.doNodePut(base, map[string][]string{
		"Content-Range": {"bytes 100-199/1000"},
	}, f.key, strings.Repeat("x", 100))
	require.Equal(t, http.StatusConflict, resp.Code)
	var errBody struct {
		Code string `json:"code"`
	}
	require.NoError(t, json.Unmarshal(resp.Body.Bytes(), &errBody))
	require.Equal(t, "upload_gap", errBody.Code)

	// A full upload with a wrong checksum → 422 and nothing kept.
	payload := strings.Repeat("A", 64)
	resp = f.h.doNodePut(base, map[string][]string{
		"Content-Range":     {"bytes 0-" + itoa(int64(len(payload))-1) + "/" + itoa(int64(len(payload)))},
		"X-Checksum-SHA256": {checksumOf([]byte(strings.Repeat("B", 64)))},
	}, f.key, payload)
	require.Equal(t, http.StatusUnprocessableEntity, resp.Code)
	require.NoError(t, json.Unmarshal(resp.Body.Bytes(), &errBody))
	require.Equal(t, "checksum_mismatch", errBody.Code)
	_, _, err := f.h.svc.Transfer().VerifiedOutput(context.Background(), f.job)
	require.ErrorIs(t, err, store.ErrNotFound)
}

func TestProgressStoresAccessMode(t *testing.T) {
	f := newTransferFixture(t, store.NodeAccessAuto)

	resp := f.h.doNode(http.MethodPost, "/api/v1/worker/jobs/"+f.job.ID+"/progress",
		map[string]any{"percent": 10.0, "access_mode": "transfer"}, f.key)
	require.Equal(t, http.StatusOK, resp.Code)
	job, err := f.h.st.GetJob(context.Background(), f.job.ID)
	require.NoError(t, err)
	require.Equal(t, store.NodeAccessTransfer, job.AccessMode)

	// An unknown mode is rejected.
	resp = f.h.doNode(http.MethodPost, "/api/v1/worker/jobs/"+f.job.ID+"/progress",
		map[string]any{"access_mode": "teleport"}, f.key)
	require.Equal(t, http.StatusBadRequest, resp.Code)
}

func TestCompleteInTransferModeRequiresUpload(t *testing.T) {
	f := newTransferFixture(t, store.NodeAccessTransfer)
	resp := f.h.doNode(http.MethodPost, "/api/v1/worker/jobs/"+f.job.ID+"/complete",
		map[string]any{"access_mode": "transfer", "bytes_after": 10, "encoder": "cpu"}, f.key)
	require.Equal(t, http.StatusConflict, resp.Code)
	var body struct {
		Code string `json:"code"`
	}
	require.NoError(t, json.Unmarshal(resp.Body.Bytes(), &body))
	require.Equal(t, "result_missing", body.Code)
}

func TestNodeThroughputLimitRoundTrip(t *testing.T) {
	f := newTransferFixture(t, store.NodeAccessTransfer)

	resp := f.h.do(http.MethodPut, "/api/v1/nodes/"+f.node.ID,
		map[string]any{"throughput_limit_mbps": 50})
	require.Equal(t, http.StatusOK, resp.Code, resp.Body.String())
	var node server.NodeDTO
	require.NoError(t, json.Unmarshal(resp.Body.Bytes(), &node))
	require.Equal(t, int64(50), node.ThroughputLimit)

	// The spec carries it to the worker.
	spec := f.h.doNode(http.MethodGet, "/api/v1/worker/jobs/"+f.job.ID, nil, f.key)
	require.Equal(t, http.StatusOK, spec.Code)
	var js server.JobSpec
	require.NoError(t, json.Unmarshal(spec.Body.Bytes(), &js))
	require.Equal(t, int64(50), js.ThroughputLimitMBs)
	require.Equal(t, store.NodeAccessTransfer, js.AccessMode)

	// Negative is rejected.
	resp = f.h.do(http.MethodPut, "/api/v1/nodes/"+f.node.ID,
		map[string]any{"throughput_limit_mbps": -1})
	require.Equal(t, http.StatusBadRequest, resp.Code)
}

func TestSpecKeepsAutoAsAuto(t *testing.T) {
	f := newTransferFixture(t, store.NodeAccessAuto)
	spec := f.h.doNode(http.MethodGet, "/api/v1/worker/jobs/"+f.job.ID, nil, f.key)
	require.Equal(t, http.StatusOK, spec.Code)
	var js server.JobSpec
	require.NoError(t, json.Unmarshal(spec.Body.Bytes(), &js))
	require.Equal(t, store.NodeAccessAuto, js.AccessMode,
		"auto must reach the worker unresolved so it can run the access check")
}

func TestParseContentRange(t *testing.T) {
	start, end, total, err := parseContentRange("bytes 0-8388607/25165824")
	require.NoError(t, err)
	require.Equal(t, int64(0), start)
	require.Equal(t, int64(8388607), end)
	require.Equal(t, int64(25165824), total)

	_, _, _, err = parseContentRange("bytes */1000")
	require.Error(t, err)
	_, _, _, err = parseContentRange("bytes 5-4/100")
	require.Error(t, err)
	_, _, _, err = parseContentRange("garbage")
	require.Error(t, err)
}
