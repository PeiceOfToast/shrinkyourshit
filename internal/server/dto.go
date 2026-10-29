// SPDX-License-Identifier: AGPL-3.0-only

package server

import (
	"encoding/json"
	"strings"
	"time"

	"github.com/PeiceOfToast/shrinkyourshit/internal/flow"
	"github.com/PeiceOfToast/shrinkyourshit/internal/store"
)

// DTOs are the JSON shapes of the API resources (API.md) and the SSE event
// payloads. They live here — not in internal/api — so the event bus can
// publish them without an import cycle.

// LibraryDTO is the JSON shape of a library. Target is the parsed Target
// object (validated when the library was written), so UIs never re-parse it.
// Easy is the easy-screen state: the stored easy input when the library was
// saved from the easy screen, the described state when the Target is
// easy-shaped, or null when the library has no easy-mode representation.
type LibraryDTO struct {
	ID           string          `json:"id"`
	Name         string          `json:"name"`
	Path         string          `json:"path"`
	Target       flow.Target     `json:"target"`
	Easy         json.RawMessage `json:"easy"`
	Watch        bool            `json:"watch"`
	Schedule     string          `json:"schedule"`
	ResultPolicy string          `json:"result_policy"`
	CreatedAt    string          `json:"created_at"`
	UpdatedAt    string          `json:"updated_at"`
}

// FileDTO is the JSON shape of a file. Probe carries the stored probe
// result verbatim (null until the file has been probed).
type FileDTO struct {
	ID              string          `json:"id"`
	LibraryID       string          `json:"library_id"`
	Path            string          `json:"path"`
	Size            int64           `json:"size"`
	Mtime           int64           `json:"mtime"` // unix seconds; part of the fingerprint
	Fingerprint     string          `json:"fingerprint"`
	Probe           json.RawMessage `json:"probe"`
	Status          string          `json:"status"`
	SatisfiedReason string          `json:"satisfied_reason"`
	LastJobID       string          `json:"last_job_id"`
	BytesSaved      int64           `json:"bytes_saved"`
	CreatedAt       string          `json:"created_at"`
	UpdatedAt       string          `json:"updated_at"`
}

// JobDTO is the JSON shape of a job, including the log tail.
type JobDTO struct {
	ID              string   `json:"id"`
	FileID          string   `json:"file_id"`
	LibraryID       string   `json:"library_id"`
	NodeID          string   `json:"node_id"`
	Status          string   `json:"status"`
	Priority        int      `json:"priority"`
	Attempt         int      `json:"attempt"`
	EncoderUsed     string   `json:"encoder_used"`
	FallbackReasons []string `json:"fallback_reasons"`
	Percent         float64  `json:"percent"`
	Speed           float64  `json:"speed"`
	ETASeconds      int64    `json:"eta_seconds"`
	BytesBefore     int64    `json:"bytes_before"`
	BytesAfter      int64    `json:"bytes_after"`
	Log             string   `json:"log"`
	Error           string   `json:"error"`
	// AccessMode is how the node reached the source (shared or transfer);
	// empty until the node reported it (task 09).
	AccessMode  string `json:"access_mode,omitempty"`
	ClaimedAt   string `json:"claimed_at,omitempty"`
	StartedAt   string `json:"started_at,omitempty"`
	FinishedAt  string `json:"finished_at,omitempty"`
	HeartbeatAt string `json:"heartbeat_at,omitempty"`
	CreatedAt   string `json:"created_at"`
}

// NewLibraryDTO maps a stored library to its JSON shape. An unparseable
// Target (impossible through the manager, defensive for rows written by
// other means) yields a zero Target instead of failing the whole list.
func NewLibraryDTO(l *store.Library) LibraryDTO {
	target, err := flow.ParseTarget([]byte(l.TargetJSON))
	if err != nil {
		target = flow.Target{}
	}
	return LibraryDTO{
		ID:           l.ID,
		Name:         l.Name,
		Path:         l.Path,
		Target:       target,
		Easy:         easyFromLibrary(l, target),
		Watch:        l.Watch,
		Schedule:     l.Schedule,
		ResultPolicy: l.ResultPolicy,
		CreatedAt:    rfc3339(l.CreatedAt),
		UpdatedAt:    rfc3339(l.UpdatedAt),
	}
}

// easyFromLibrary picks the easy-screen state of a library (task 07): the
// stored easy input when the library was saved from the easy screen, the
// described state when the Target itself is easy-shaped (libraries created
// with a full target can still reopen in easy mode), else null.
func easyFromLibrary(l *store.Library, target flow.Target) json.RawMessage {
	if s := strings.TrimSpace(l.EasyJSON); s != "" {
		var in flow.EasyModeInput
		if err := json.Unmarshal([]byte(s), &in); err == nil {
			if raw, err := json.Marshal(in); err == nil {
				return raw
			}
		}
		// A corrupt stored value falls through to the describe; it must
		// not poison the whole response.
	}
	if in, ok := flow.DescribeTarget(target); ok {
		if raw, err := json.Marshal(in); err == nil {
			return raw
		}
	}
	return json.RawMessage("null")
}

// NewFileDTO maps a stored file to its JSON shape.
func NewFileDTO(f *store.File) FileDTO {
	return FileDTO{
		ID:              f.ID,
		LibraryID:       f.LibraryID,
		Path:            f.Path,
		Size:            f.Size,
		Mtime:           f.Mtime,
		Fingerprint:     f.Fingerprint,
		Probe:           rawOrEmpty(f.ProbeJSON),
		Status:          f.Status,
		SatisfiedReason: f.SatisfiedReason,
		LastJobID:       f.LastJobID,
		BytesSaved:      f.BytesSaved,
		CreatedAt:       rfc3339(f.CreatedAt),
		UpdatedAt:       rfc3339(f.UpdatedAt),
	}
}

// NewJobDTO maps a stored job to its JSON shape.
func NewJobDTO(j *store.Job) JobDTO {
	var fallback []string
	if j.FallbackReasons != "" {
		_ = json.Unmarshal([]byte(j.FallbackReasons), &fallback)
	}
	return JobDTO{
		ID:              j.ID,
		FileID:          j.FileID,
		LibraryID:       j.LibraryID,
		NodeID:          j.NodeID,
		Status:          j.Status,
		Priority:        j.Priority,
		Attempt:         j.Attempt,
		EncoderUsed:     j.EncoderUsed,
		FallbackReasons: fallback,
		Percent:         j.Percent,
		Speed:           j.Speed,
		ETASeconds:      j.ETASeconds,
		BytesBefore:     j.BytesBefore,
		BytesAfter:      j.BytesAfter,
		Log:             j.Log,
		Error:           j.Error,
		AccessMode:      j.AccessMode,
		ClaimedAt:       rfc3339(j.ClaimedAt),
		StartedAt:       rfc3339(j.StartedAt),
		FinishedAt:      rfc3339(j.FinishedAt),
		HeartbeatAt:     rfc3339(j.HeartbeatAt),
		CreatedAt:       rfc3339(j.CreatedAt),
	}
}

// NodeDTO is the JSON shape of a node. Capabilities carries the stored
// JSON verbatim (null when empty); KeyHash is never exposed. Task 08 adds
// the nodes API around this shape; task 09 adds the throughput limit;
// task 11 adds the lifecycle: desired_state (operator intent),
// effective_state (desired, or error while a transition failed to apply),
// the node's concurrency cap, the reason a transition failed and the id of
// the job it currently holds (listing responses only; events omit it).
type NodeDTO struct {
	ID              string          `json:"id"`
	Name            string          `json:"name"`
	OS              string          `json:"os"`
	Arch            string          `json:"arch"`
	Capabilities    json.RawMessage `json:"capabilities"`
	AccessMode      string          `json:"access_mode"`
	PathMappings    json.RawMessage `json:"path_mappings"`
	ThroughputLimit int64           `json:"throughput_limit_mbps"`
	DesiredState    string          `json:"desired_state"`
	EffectiveState  string          `json:"effective_state"`
	Concurrency     int             `json:"concurrency"`
	StateError      string          `json:"state_error,omitempty"`
	CurrentJobID    string          `json:"current_job_id,omitempty"`
	Status          string          `json:"status"`
	Enabled         bool            `json:"enabled"`
	LastHeartbeatAt string          `json:"last_heartbeat_at,omitempty"`
	CreatedAt       string          `json:"created_at"`
}

// NewNodeDTO maps a stored node to its JSON shape.
func NewNodeDTO(n *store.Node) NodeDTO {
	concurrency := n.Concurrency
	if concurrency < store.ConcurrencyMin {
		concurrency = store.ConcurrencyMin
	}
	return NodeDTO{
		ID:              n.ID,
		Name:            n.Name,
		OS:              n.OS,
		Arch:            n.Arch,
		Capabilities:    rawOrEmpty(n.Capabilities),
		AccessMode:      n.AccessMode,
		PathMappings:    rawOrEmpty(n.PathMappings),
		ThroughputLimit: n.ThroughputLimit,
		DesiredState:    n.DesiredState,
		EffectiveState:  EffectiveState(n),
		Concurrency:     concurrency,
		StateError:      n.StateError,
		Status:          n.Status,
		Enabled:         n.Enabled,
		LastHeartbeatAt: rfc3339(n.LastHeartbeatAt),
		CreatedAt:       rfc3339(n.CreatedAt),
	}
}

// DiscoveredServer is one server found on the LAN (GET /setup/servers,
// task 08). The id lets the wizard hide this server's own announcement.
type DiscoveredServer struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	URL     string `json:"url"`
	Version string `json:"version"`
}

// PairResponse is the POST /pair and GET /pair/{id} body (task 08). The
// node key is only ever set on the response that first delivers it; it
// never appears in any other payload or event.
type PairResponse struct {
	NodeID  string `json:"node_id"`
	Status  string `json:"status"`
	NodeKey string `json:"node_key,omitempty"`
}

// rfc3339 formats a time for JSON output; zero times become empty strings
// (omitted by omitempty where it matters).
func rfc3339(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339Nano)
}

// rawOrEmpty converts opaque JSON text to a RawMessage; empty becomes null.
func rawOrEmpty(s string) json.RawMessage {
	if strings.TrimSpace(s) == "" {
		return json.RawMessage("null")
	}
	return json.RawMessage(s)
}

// ListResponse is the standard list envelope (API.md).
type ListResponse[T any] struct {
	Items []T `json:"items"`
	Total int `json:"total"`
}

// JobSpec is the GET /api/v1/worker/jobs/{id} response (API.md, task 05):
// everything a worker needs to run one job. source_path is the node-side
// path (path mappings applied; identity for the built-in node). access_mode
// is the node's configured mode; for auto the worker resolves it with the
// shared-storage check and reports the choice (task 09). ThroughputLimitMBs
// caps transfer speed in MB/s (0 = unlimited).
type JobSpec struct {
	ID                 string      `json:"id"`
	FileID             string      `json:"file_id"`
	LibraryID          string      `json:"library_id"`
	Attempt            int         `json:"attempt"`
	Target             flow.Target `json:"target"`
	SourcePath         string      `json:"source_path"`
	SourceSize         int64       `json:"source_size"`
	SourceMtime        int64       `json:"source_mtime"`
	Fingerprint        string      `json:"fingerprint"`
	AccessMode         string      `json:"access_mode"`
	ResultPolicy       string      `json:"result_policy"`
	BytesBefore        int64       `json:"bytes_before"`
	ThroughputLimitMBs int64       `json:"throughput_limit_mbps"`
}
