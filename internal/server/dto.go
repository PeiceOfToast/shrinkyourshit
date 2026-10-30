// SPDX-License-Identifier: AGPL-3.0-only

package server

import (
	"encoding/json"
	"path/filepath"
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
// Flow is the canonical rule graph (ARCHITECTURE.md flow schema) and
// FlowSource records who owns it: easy, steps or advanced. The flow wins
// when it disagrees with the Target; legacy rows without a stored flow
// get the flow compiled from their Target.
type LibraryDTO struct {
	ID           string          `json:"id"`
	Name         string          `json:"name"`
	Path         string          `json:"path"`
	Target       flow.Target     `json:"target"`
	Easy         json.RawMessage `json:"easy"`
	Flow         *flow.Graph     `json:"flow"`
	FlowSource   string          `json:"flow_source"`
	Watch        bool            `json:"watch"`
	Schedule     string          `json:"schedule"`
	ResultPolicy string          `json:"result_policy"`
	CreatedAt    string          `json:"created_at"`
	UpdatedAt    string          `json:"updated_at"`
	// LastScanAt is when a scan of this library last finished without
	// error (task 19); omitted when it has never been scanned.
	LastScanAt string `json:"last_scan_at,omitempty"`
}

// FileDTO is the JSON shape of a file. Probe carries the stored probe
// result verbatim (null until the file has been probed). LastJobEncoder is
// the encoder of the file's most recent job (task 19, the file table's
// "encoder used" column); empty when the file has no job yet.
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
	LastJobEncoder  string          `json:"last_job_encoder,omitempty"`
	BytesSaved      int64           `json:"bytes_saved"`
	CreatedAt       string          `json:"created_at"`
	UpdatedAt       string          `json:"updated_at"`
}

// JobDTO is the JSON shape of a job, including the log tail. Task 18 adds
// the human-facing file identity (basename and full path, resolved from
// the job's file_id) and the live output size the worker reported on its
// last progress tick (0 until the first tick, bytes_after once done).
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
	BytesSoFar      int64    `json:"bytes_so_far"`
	FileName        string   `json:"file_name"`
	FilePath        string   `json:"file_path"`
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
	graph, source := libraryFlowOrNull(l)
	return LibraryDTO{
		ID:           l.ID,
		Name:         l.Name,
		Path:         l.Path,
		Target:       target,
		Easy:         easyFromLibrary(l, target),
		Flow:         graph,
		FlowSource:   source,
		Watch:        l.Watch,
		Schedule:     l.Schedule,
		ResultPolicy: l.ResultPolicy,
		CreatedAt:    rfc3339(l.CreatedAt),
		UpdatedAt:    rfc3339(l.UpdatedAt),
		LastScanAt:   rfc3339(l.LastScanAt),
	}
}

// libraryFlowOrNull returns the library's rule graph for the DTO. A legacy
// row (no stored flow) gets the flow compiled from its Target so every
// library always carries a flow; an unparsable Target yields a null flow.
func libraryFlowOrNull(l *store.Library) (*flow.Graph, string) {
	g, source, err := LibraryFlow(l)
	if err != nil {
		return nil, ""
	}
	return &g, source
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

// NewJobDTO maps a stored job to its JSON shape. f is the job's file
// (resolved from file_id by the caller); nil — a file row that vanished,
// or a caller that did not need the name — leaves file_name/file_path
// empty instead of failing the mapping.
func NewJobDTO(j *store.Job, f *store.File) JobDTO {
	var fallback []string
	if j.FallbackReasons != "" {
		_ = json.Unmarshal([]byte(j.FallbackReasons), &fallback)
	}
	dto := JobDTO{
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
		BytesSoFar:      j.BytesSoFar,
		Log:             j.Log,
		Error:           j.Error,
		AccessMode:      j.AccessMode,
		ClaimedAt:       rfc3339(j.ClaimedAt),
		StartedAt:       rfc3339(j.StartedAt),
		FinishedAt:      rfc3339(j.FinishedAt),
		HeartbeatAt:     rfc3339(j.HeartbeatAt),
		CreatedAt:       rfc3339(j.CreatedAt),
	}
	if f != nil {
		dto.FileName = filepath.Base(f.Path)
		dto.FilePath = f.Path
	}
	return dto
}

// NodeDTO is the JSON shape of a node. Capabilities carries the stored
// JSON verbatim (null when empty); KeyHash is never exposed. Task 08 adds
// the nodes API around this shape; task 09 adds the throughput limit;
// task 11 adds the lifecycle: desired_state (operator intent),
// effective_state (desired, or error while a transition failed to apply),
// the node's concurrency cap, the reason a transition failed and the id of
// the job it currently holds (listing responses only; events omit it).
// Task 18 adds Stats, the node's finished-job totals (listing responses
// only, same rule).
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
	Stats           *NodeStatsDTO   `json:"stats,omitempty"`
	Status          string          `json:"status"`
	Enabled         bool            `json:"enabled"`
	LastHeartbeatAt string          `json:"last_heartbeat_at,omitempty"`
	CreatedAt       string          `json:"created_at"`
}

// NodeStatsDTO is one node's finished-job totals (task 18): how many jobs
// it completed and how many bytes they saved (positive savings only). A
// node with no finished jobs reports zeros, never an absent field.
type NodeStatsDTO struct {
	FilesDone  int64 `json:"files_done"`
	BytesSaved int64 `json:"bytes_saved"`
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
// everything a worker needs to run one job. Task 14 adds the library's
// rule graph (Flow) and its source (easy, steps or advanced): the worker
// compiles it and resolves it against the probed source to get the
// effective Target, and flow wins over the Target field when they disagree
// (a mismatch is logged server-side).
type JobSpec struct {
	ID                 string      `json:"id"`
	FileID             string      `json:"file_id"`
	LibraryID          string      `json:"library_id"`
	Attempt            int         `json:"attempt"`
	Target             flow.Target `json:"target"`
	Flow               *flow.Graph `json:"flow,omitempty"`
	FlowSource         string      `json:"flow_source,omitempty"`
	SourcePath         string      `json:"source_path"`
	SourceSize         int64       `json:"source_size"`
	SourceMtime        int64       `json:"source_mtime"`
	Fingerprint        string      `json:"fingerprint"`
	AccessMode         string      `json:"access_mode"`
	ResultPolicy       string      `json:"result_policy"`
	BytesBefore        int64       `json:"bytes_before"`
	ThroughputLimitMBs int64       `json:"throughput_limit_mbps"`
}
