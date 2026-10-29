// SPDX-License-Identifier: AGPL-3.0-only

// Package store defines the Store interface and its SQLite implementation
// (modernc.org/sqlite) plus the embedded SQL migrations. It is
// media-agnostic: probe results and Targets are stored as opaque JSON
// strings, and the media-specific logic lives in internal/probe and
// internal/flow.
package store

import (
	"errors"
	"time"
)

// ErrNotFound is returned when a row does not exist.
var ErrNotFound = errors.New("store: not found")

// ErrQueueEmpty is returned by ClaimNextJob when no queued job exists.
var ErrQueueEmpty = errors.New("store: queue empty")

// File statuses (ARCHITECTURE.md; do not add values without updating it).
const (
	FileStatusNew        = "new"
	FileStatusSatisfied  = "satisfied"
	FileStatusQueued     = "queued"
	FileStatusProcessing = "processing"
	FileStatusDone       = "done"
	FileStatusFailed     = "failed"
	FileStatusMissing    = "missing"
)

// Library is a folder plus a Target and watch/schedule settings. Target and
// easy are opaque JSON validated by internal/flow (easy is the easy-screen
// input that compiled to Target; empty when the library was created from a
// full Target).
type Library struct {
	ID           string
	Name         string
	Path         string
	TargetJSON   string
	EasyJSON     string
	Watch        bool
	Schedule     string // cron string or empty
	ResultPolicy string // replace_if_smaller | replace | keep_both
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// File is one media file inside a library. Probe is opaque JSON produced by
// internal/probe.
type File struct {
	ID              string
	LibraryID       string
	Path            string
	Size            int64
	Mtime           int64 // unix seconds
	Fingerprint     string
	ProbeJSON       string
	Status          string // new | satisfied | queued | processing | done | failed | missing
	SatisfiedReason string // joined Evaluate reasons, empty when satisfied
	LastJobID       string
	BytesSaved      int64
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

// Job statuses (ARCHITECTURE.md; do not add values without updating it).
const (
	JobStatusQueued    = "queued"
	JobStatusClaimed   = "claimed"
	JobStatusRunning   = "running"
	JobStatusDone      = "done"
	JobStatusFailed    = "failed"
	JobStatusCancelled = "cancelled"
)

// Job is one unit of work: apply a Target to one File on one Node.
type Job struct {
	ID              string
	FileID          string
	LibraryID       string
	NodeID          string
	Status          string // queued | claimed | running | done | failed | cancelled
	Priority        int
	Attempt         int
	EncoderUsed     string
	FallbackReasons string // JSON array of strings
	Percent         float64
	Speed           float64
	ETASeconds      int64
	BytesBefore     int64
	BytesAfter      int64
	Log             string // capped at 64KiB tail
	Error           string
	ClaimedAt       time.Time
	StartedAt       time.Time
	FinishedAt      time.Time
	HeartbeatAt     time.Time
	CreatedAt       time.Time
}

// Node statuses (ARCHITECTURE.md; do not add values without updating it).
const (
	NodeStatusPending  = "pending"
	NodeStatusApproved = "approved"
	NodeStatusRevoked  = "revoked"
)

// Node access modes (ARCHITECTURE.md; do not add values without updating it).
const (
	NodeAccessAuto     = "auto"
	NodeAccessShared   = "shared"
	NodeAccessTransfer = "transfer"
)

// Node is a registered worker machine.
type Node struct {
	ID              string
	Name            string
	KeyHash         string
	// KeyOnce holds the raw key between admin approval and the node's
	// first retrieval (pairing flow, task 08). It is wiped on first
	// retrieval, revoke and delete — afterwards only the hash remains.
	KeyOnce         string
	OS              string
	Arch            string
	Capabilities    string // JSON: {encoders: [{name, codecs: []}], cpu_cores, ffmpeg_version}
	AccessMode      string // auto | shared | transfer
	PathMappings    string // JSON
	Status          string // pending | approved | revoked
	Enabled         bool
	LastHeartbeatAt time.Time
	CreatedAt       time.Time
}

// PairingToken is a one-time token for joining a node.
type PairingToken struct {
	ID        string
	TokenHash string
	ExpiresAt time.Time
	UsedAt    time.Time
}

// Settings keys from ARCHITECTURE.md.
const (
	SettingLoginEnabled      = "login_enabled"
	SettingAdminUsername     = "admin_username"
	SettingAdminPasswordHash = "admin_password_hash"
	SettingWorkerConcurrency = "worker_concurrency"
	SettingDefaultResultPol  = "default_result_policy"
	SettingScratchDir        = "scratch_dir"
	SettingSetupComplete     = "setup_complete"
	// Task 06 additions: easy-mode defaults (task 07 reads these).
	SettingPreferredLanguages = "preferred_languages"
	SettingDefaultVideoCodec  = "default_video_codec"
)

// Result policies (ARCHITECTURE.md; do not add values without updating it).
const (
	ResultPolicyReplaceIfSmaller = "replace_if_smaller"
	ResultPolicyReplace          = "replace"
	ResultPolicyKeepBoth         = "keep_both"
)

// ResultPolicyValues lists the valid result policies.
func ResultPolicyValues() []string {
	return []string{ResultPolicyReplaceIfSmaller, ResultPolicyReplace, ResultPolicyKeepBoth}
}

// FileStatusValues lists the valid file statuses.
func FileStatusValues() []string {
	return []string{FileStatusNew, FileStatusSatisfied, FileStatusQueued, FileStatusProcessing, FileStatusDone, FileStatusFailed, FileStatusMissing}
}

// JobStatusValues lists the valid job statuses.
func JobStatusValues() []string {
	return []string{JobStatusQueued, JobStatusClaimed, JobStatusRunning, JobStatusDone, JobStatusFailed, JobStatusCancelled}
}

// NodeStatusValues lists the valid node statuses.
func NodeStatusValues() []string {
	return []string{NodeStatusPending, NodeStatusApproved, NodeStatusRevoked}
}

// NodeAccessModeValues lists the valid node access modes.
func NodeAccessModeValues() []string {
	return []string{NodeAccessAuto, NodeAccessShared, NodeAccessTransfer}
}
