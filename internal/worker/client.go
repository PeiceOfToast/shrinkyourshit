// SPDX-License-Identifier: AGPL-3.0-only

package worker

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/PeiceOfToast/shrinkyourshit/internal/encoders"
	"github.com/PeiceOfToast/shrinkyourshit/internal/server"
)

// serverNodeRunning is what an empty desired_state means (a server built
// before task 11): the old always-running behaviour.
const serverNodeRunning = "running"

// APIError is a non-2xx worker API response with its parsed error body.
type APIError struct {
	Status  int
	Code    string
	Message string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("worker api: %d %s: %s", e.Status, e.Code, e.Message)
}

// isAPIStatus reports whether err is an APIError with one of the statuses.
func isAPIStatus(err error, status int) bool {
	var apiErr *APIError
	return errors.As(err, &apiErr) && apiErr.Status == status
}

// Client talks to the server's worker API (API.md, task 05) with the node
// key. Remote workers and the built-in worker use the same calls.
type Client struct {
	base string
	key  string
	http *http.Client
	// stream runs the long-lived transfer calls (source download, chunked
	// result upload): no client-wide timeout, context-driven instead — a
	// multi-gigabyte transfer must not die at 60 seconds.
	stream *http.Client
}

// NewClient builds a client for a server base URL (e.g.
// http://192.168.1.10:8765) and a node key.
func NewClient(base, key string) *Client {
	return &Client{
		base:   strings.TrimRight(base, "/"),
		key:    key,
		http:   &http.Client{Timeout: 60 * time.Second},
		stream: &http.Client{},
	}
}

// do performs one request. out is filled from a 200 body; 204 leaves it
// untouched. The node key header is only sent when a key exists — the
// pairing calls run before the node has one.
func (c *Client) do(ctx context.Context, method, path string, body, out any) error {
	var reader io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("worker: encode request: %w", err)
		}
		reader = bytes.NewReader(raw)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.base+path, reader)
	if err != nil {
		return fmt.Errorf("worker: build request: %w", err)
	}
	if c.key != "" {
		req.Header.Set("Authorization", "Bearer "+c.key)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("worker: %s %s: %w", method, path, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode >= 400 {
		return parseAPIError(resp)
	}
	// Decode any 2xx that carries a body; 204/205 have none (claim's
	// empty-queue answer is a 204).
	if out != nil && resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusResetContent {
		if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
			return fmt.Errorf("worker: decode response from %s: %w", path, err)
		}
	}
	return nil
}

// parseAPIError reads the API.md error shape; an unreadable body still
// yields an error carrying the status.
func parseAPIError(resp *http.Response) error {
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return &APIError{Status: resp.StatusCode, Message: err.Error()}
	}
	var body struct {
		Error string `json:"error"`
		Code  string `json:"code"`
	}
	if err := json.Unmarshal(raw, &body); err != nil {
		return &APIError{Status: resp.StatusCode, Message: strings.TrimSpace(string(raw))}
	}
	return &APIError{Status: resp.StatusCode, Code: body.Code, Message: body.Error}
}

// HeartbeatInput is the heartbeat request body (API.md). State (task 11)
// is the worker's own lifecycle state; a `stopped` report while the node
// is draining completes the drain.
type HeartbeatInput struct {
	Capabilities  json.RawMessage `json:"capabilities"`
	RunningJobIDs []string        `json:"running_job_ids"`
	State         string          `json:"state,omitempty"`
}

// HeartbeatState is the heartbeat response (API.md, task 11): the
// instruction the worker honours until the next beat.
type HeartbeatState struct {
	Concurrency    int    `json:"concurrency"`
	DesiredState   string `json:"desired_state"`
	EffectiveState string `json:"effective_state"`
}

// Heartbeat sends the node heartbeat and returns the instruction: the
// desired state and the concurrency cap this worker may run.
func (c *Client) Heartbeat(ctx context.Context, in HeartbeatInput) (*HeartbeatState, error) {
	var out HeartbeatState
	if err := c.do(ctx, http.MethodPost, "/api/v1/worker/heartbeat", in, &out); err != nil {
		return nil, err
	}
	if out.DesiredState == "" {
		// A server built before task 11 sends no desired_state: keep the
		// old always-running behaviour.
		out.DesiredState = serverNodeRunning
	}
	if out.Concurrency < 1 {
		out.Concurrency = 1
	}
	return &out, nil
}

// ClaimedJob is what claim returns: enough to fetch the full spec.
type ClaimedJob struct {
	ID string `json:"id"`
}

// Claim requests one job; a 204 (empty queue) returns nil, nil.
func (c *Client) Claim(ctx context.Context) (*ClaimedJob, error) {
	var out ClaimedJob
	err := c.do(ctx, http.MethodPost, "/api/v1/worker/claim", nil, &out)
	if err != nil {
		return nil, err
	}
	if out.ID == "" {
		return nil, nil
	}
	return &out, nil
}

// JobSpec fetches the full job description.
func (c *Client) JobSpec(ctx context.Context, jobID string) (*server.JobSpec, error) {
	var out server.JobSpec
	if err := c.do(ctx, http.MethodGet, "/api/v1/worker/jobs/"+jobID, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// ProgressInput is the progress request body (API.md). The response's
// status lets the runner detect a cancelled job. AccessMode (task 09) is
// the mode the node resolved for the job; the server stores it.
// BytesSoFar (task 18) is the scratch output's size at report time: one
// stat per tick, so the node card can show the file growing.
type ProgressInput struct {
	Percent    float64 `json:"percent"`
	Speed      float64 `json:"speed"`
	ETASeconds int64   `json:"eta_seconds"`
	Encoder    string  `json:"encoder"`
	AccessMode string  `json:"access_mode"`
	BytesSoFar int64   `json:"bytes_so_far"`
}

// Progress reports progress and returns the job's current status.
func (c *Client) Progress(ctx context.Context, jobID string, in ProgressInput) (string, error) {
	var out struct {
		Status string `json:"status"`
	}
	if err := c.do(ctx, http.MethodPost, "/api/v1/worker/jobs/"+jobID+"/progress", in, &out); err != nil {
		return "", err
	}
	return out.Status, nil
}

// CompleteInput is the complete request body (API.md). OutputPath is the
// final path of the produced file in the library; SkippedReason is set when
// the result policy kept the original. AccessMode (task 09) is "transfer"
// when the node uploaded the result instead of replacing the file itself.
type CompleteInput struct {
	OutputPath      string   `json:"output_path"`
	BytesAfter      int64    `json:"bytes_after"`
	Encoder         string   `json:"encoder"`
	FallbackReasons []string `json:"fallback_reasons"`
	SkippedReason   string   `json:"skipped_reason"`
	LogTail         string   `json:"log_tail"`
	AccessMode      string   `json:"access_mode"`
}

// Complete reports a successful job.
func (c *Client) Complete(ctx context.Context, jobID string, in CompleteInput) error {
	return c.do(ctx, http.MethodPost, "/api/v1/worker/jobs/"+jobID+"/complete", in, nil)
}

// Fail reports a failed job with its log tail.
func (c *Client) Fail(ctx context.Context, jobID, errMsg, logTail string) error {
	return c.do(ctx, http.MethodPost, "/api/v1/worker/jobs/"+jobID+"/fail",
		struct {
			Error   string `json:"error"`
			LogTail string `json:"log_tail"`
		}{Error: errMsg, LogTail: logTail}, nil)
}

// fallbackStrings flattens encoder fallback reasons into the string form
// the API and the job record use ("nvenc: <error>").
func fallbackStrings(res encoders.Result) []string {
	out := make([]string, 0, len(res.Fallbacks))
	for _, f := range res.Fallbacks {
		out = append(out, f.Encoder+": "+f.Error)
	}
	return out
}

// PairInput is the POST /pair body (API.md, task 08). Token is optional.
type PairInput struct {
	Token        string          `json:"token,omitempty"`
	Name         string          `json:"name"`
	OS           string          `json:"os"`
	Arch         string          `json:"arch"`
	Capabilities json.RawMessage `json:"capabilities,omitempty"`
}

// Pair asks the server to join. With a valid token the node is approved
// immediately and PairResponse.NodeKey carries the key. Without one the
// node is pending and the caller polls PairPoll until approval.
func (c *Client) Pair(ctx context.Context, in PairInput) (*server.PairResponse, error) {
	var out server.PairResponse
	if err := c.do(ctx, http.MethodPost, "/api/v1/pair", in, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// PairPoll asks about a pending node: pending (keep polling), approved
// (NodeKey set on the first retrieval only) or revoked (give up).
func (c *Client) PairPoll(ctx context.Context, nodeID string) (*server.PairResponse, error) {
	var out server.PairResponse
	if err := c.do(ctx, http.MethodGet, "/api/v1/pair/"+nodeID, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// AccessChallenge is the shared-storage check payload (API.md, task 09):
// hash the same 1 MiB sample from the same offset of the file the node sees
// at Path and the library is shared; anything else means transfer.
type AccessChallenge struct {
	Path         string `json:"path"`
	Size         int64  `json:"size"`
	SampleOffset int64  `json:"sample_offset"`
	SampleSHA256 string `json:"sample_sha256"`
}

// AccessCheck asks for the shared-storage sample of a claimed job.
func (c *Client) AccessCheck(ctx context.Context, jobID, libraryID string) (*AccessChallenge, error) {
	var out AccessChallenge
	body := struct {
		LibraryID string `json:"library_id"`
	}{LibraryID: libraryID}
	if err := c.do(ctx, http.MethodPost, "/api/v1/worker/jobs/"+jobID+"/access-check", body, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// SourceStream opens the job's source stream (API.md task 09). rangeStart
// resumes from a partial download (0 = full download); etag, when set, is
// sent as If-Range so the server restarts the response if the file changed.
// The caller must close the response body.
func (c *Client) SourceStream(ctx context.Context, jobID, etag string, rangeStart int64) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.base+"/api/v1/worker/jobs/"+jobID+"/source", nil)
	if err != nil {
		return nil, fmt.Errorf("worker: build source request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.key)
	if rangeStart > 0 {
		req.Header.Set("Range", "bytes="+strconv.FormatInt(rangeStart, 10)+"-")
		if etag != "" {
			req.Header.Set("If-Range", etag)
		}
	}
	resp, err := c.stream.Do(req)
	if err != nil {
		return nil, fmt.Errorf("worker: source stream: %w", err)
	}
	if resp.StatusCode >= 400 {
		defer func() { _ = resp.Body.Close() }()
		return nil, parseAPIError(resp)
	}
	return resp, nil
}

// UploadResult sends the verified output to the server (API.md task 09) in
// uploadChunkSize chunks: sequential PUTs with Content-Range, the final
// one carrying X-Checksum-SHA256 over the whole file. A chunk that times
// out or is rejected as a gap is retried (gaps restart the upload); a
// checksum mismatch (422) is fatal — the server kept the original.
func (c *Client) UploadResult(ctx context.Context, jobID, path string, limitMBs int64, log *slog.Logger) error {
	st, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("worker: stat result: %w", err)
	}
	total := st.Size()
	checksum, err := fileSHA256(path)
	if err != nil {
		return err
	}

	chunk := int64(uploadChunkSize)
	restarts := 0
	for start := int64(0); start < total; {
		end := min(start+chunk-1, total-1)
		uploaded, err := c.putChunk(ctx, jobID, path, start, end, total, checksum, limitMBs)
		if err != nil {
			var apiErr *APIError
			if errors.As(err, &apiErr) && apiErr.Status == http.StatusConflict {
				// The server lost the partial upload (restart): begin again
				// from byte 0 — twice at most, then give up honestly.
				restarts++
				if restarts > 2 {
					return err
				}
				log.Warn("worker: result upload restart from zero", "job", jobID, "err", err)
				start = 0
				continue
			}
			return err
		}
		start = uploaded
	}
	return nil
}

// putChunk sends one range and returns the offset the next chunk starts at.
func (c *Client) putChunk(ctx context.Context, jobID, path string, start, end, total int64, checksum string, limitMBs int64) (int64, error) {
	length := end - start + 1
	f, err := os.Open(path)
	if err != nil {
		return 0, fmt.Errorf("worker: open result: %w", err)
	}
	defer func() { _ = f.Close() }()
	if _, err := f.Seek(start, io.SeekStart); err != nil {
		return 0, fmt.Errorf("worker: seek result: %w", err)
	}
	body := io.Reader(io.LimitReader(f, length))
	if limitMBs > 0 {
		body = newPacedReader(body, limitMBs*1_000_000, ctx)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPut,
		c.base+"/api/v1/worker/jobs/"+jobID+"/result", body)
	if err != nil {
		return 0, fmt.Errorf("worker: build result request: %w", err)
	}
	req.ContentLength = length
	req.Header.Set("Authorization", "Bearer "+c.key)
	req.Header.Set("Content-Type", "application/octet-stream")
	req.Header.Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, end, total))
	if end+1 >= total {
		req.Header.Set("X-Checksum-SHA256", checksum)
	}
	resp, err := c.stream.Do(req)
	if err != nil {
		return 0, fmt.Errorf("worker: result chunk %d: %w", start, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode >= 400 {
		return 0, parseAPIError(resp)
	}
	return end + 1, nil
}

// uploadChunkSize is the result upload granularity (task 09: 8 MiB).
const uploadChunkSize = 8 << 20

// fileSHA256 hashes a whole file streaming.
func fileSHA256(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("worker: open %s: %w", path, err)
	}
	defer func() { _ = f.Close() }()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", fmt.Errorf("worker: hash %s: %w", path, err)
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
