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
type LibraryDTO struct {
	ID           string      `json:"id"`
	Name         string      `json:"name"`
	Path         string      `json:"path"`
	Target       flow.Target `json:"target"`
	Watch        bool        `json:"watch"`
	Schedule     string      `json:"schedule"`
	ResultPolicy string      `json:"result_policy"`
	CreatedAt    string      `json:"created_at"`
	UpdatedAt    string      `json:"updated_at"`
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
	ClaimedAt       string   `json:"claimed_at,omitempty"`
	StartedAt       string   `json:"started_at,omitempty"`
	FinishedAt      string   `json:"finished_at,omitempty"`
	HeartbeatAt     string   `json:"heartbeat_at,omitempty"`
	CreatedAt       string   `json:"created_at"`
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
		Watch:        l.Watch,
		Schedule:     l.Schedule,
		ResultPolicy: l.ResultPolicy,
		CreatedAt:    rfc3339(l.CreatedAt),
		UpdatedAt:    rfc3339(l.UpdatedAt),
	}
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
		ClaimedAt:       rfc3339(j.ClaimedAt),
		StartedAt:       rfc3339(j.StartedAt),
		FinishedAt:      rfc3339(j.FinishedAt),
		HeartbeatAt:     rfc3339(j.HeartbeatAt),
		CreatedAt:       rfc3339(j.CreatedAt),
	}
}

// NodeDTO is the JSON shape of a node. Capabilities carries the stored
// JSON verbatim (null when empty); KeyHash is never exposed. Task 08 adds
// the nodes API around this shape.
type NodeDTO struct {
	ID              string          `json:"id"`
	Name            string          `json:"name"`
	OS              string          `json:"os"`
	Arch            string          `json:"arch"`
	Capabilities    json.RawMessage `json:"capabilities"`
	AccessMode      string          `json:"access_mode"`
	PathMappings    json.RawMessage `json:"path_mappings"`
	Status          string          `json:"status"`
	Enabled         bool            `json:"enabled"`
	LastHeartbeatAt string          `json:"last_heartbeat_at,omitempty"`
	CreatedAt       string          `json:"created_at"`
}

// NewNodeDTO maps a stored node to its JSON shape.
func NewNodeDTO(n *store.Node) NodeDTO {
	return NodeDTO{
		ID:              n.ID,
		Name:            n.Name,
		OS:              n.OS,
		Arch:            n.Arch,
		Capabilities:    rawOrEmpty(n.Capabilities),
		AccessMode:      n.AccessMode,
		PathMappings:    rawOrEmpty(n.PathMappings),
		Status:          n.Status,
		Enabled:         n.Enabled,
		LastHeartbeatAt: rfc3339(n.LastHeartbeatAt),
		CreatedAt:       rfc3339(n.CreatedAt),
	}
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
