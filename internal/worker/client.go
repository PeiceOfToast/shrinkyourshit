// SPDX-License-Identifier: AGPL-3.0-only

package worker

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/PeiceOfToast/shrinkyourshit/internal/encoders"
	"github.com/PeiceOfToast/shrinkyourshit/internal/server"
)

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
}

// NewClient builds a client for a server base URL (e.g.
// http://192.168.1.10:8765) and a node key.
func NewClient(base, key string) *Client {
	return &Client{
		base: strings.TrimRight(base, "/"),
		key:  key,
		http: &http.Client{Timeout: 60 * time.Second},
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

// HeartbeatInput is the heartbeat request body (API.md).
type HeartbeatInput struct {
	Capabilities  json.RawMessage `json:"capabilities"`
	RunningJobIDs []string        `json:"running_job_ids"`
}

// Heartbeat sends the node heartbeat and returns the concurrency the server
// wants this worker to run.
func (c *Client) Heartbeat(ctx context.Context, in HeartbeatInput) (int, error) {
	var out struct {
		Concurrency int `json:"concurrency"`
	}
	if err := c.do(ctx, http.MethodPost, "/api/v1/worker/heartbeat", in, &out); err != nil {
		return 0, err
	}
	if out.Concurrency < 1 {
		return 1, nil
	}
	return out.Concurrency, nil
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
// status lets the runner detect a cancelled job.
type ProgressInput struct {
	Percent    float64 `json:"percent"`
	Speed      float64 `json:"speed"`
	ETASeconds int64   `json:"eta_seconds"`
	Encoder    string  `json:"encoder"`
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
// the result policy kept the original.
type CompleteInput struct {
	OutputPath      string   `json:"output_path"`
	BytesAfter      int64    `json:"bytes_after"`
	Encoder         string   `json:"encoder"`
	FallbackReasons []string `json:"fallback_reasons"`
	SkippedReason   string   `json:"skipped_reason"`
	LogTail         string   `json:"log_tail"`
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
