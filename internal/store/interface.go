// SPDX-License-Identifier: AGPL-3.0-only

package store

import (
	"context"
	"fmt"
	"time"
)

// JobFilter narrows ListJobs. Empty fields mean "no filter".
type JobFilter struct {
	Status string
	FileID string
	NodeID string
	Limit  int // 0 = no limit
	Offset int
}

// Store is the persistence interface. All access goes through it so the
// SQLite implementation can be swapped later (DECISIONS.md keeps Postgres
// possible). Implementations must be safe for concurrent use.
type Store interface {
	// Close releases the database resources.
	Close() error

	// Libraries.
	CreateLibrary(ctx context.Context, l *Library) error
	GetLibrary(ctx context.Context, id string) (*Library, error)
	ListLibraries(ctx context.Context) ([]*Library, error)
	UpdateLibrary(ctx context.Context, l *Library) error
	DeleteLibrary(ctx context.Context, id string) error

	// Files. UpsertFile keys on (LibraryID, Path): the scanner calls it
	// for new and changed files alike.
	UpsertFile(ctx context.Context, f *File) error
	GetFile(ctx context.Context, id string) (*File, error)
	GetFileByPath(ctx context.Context, libraryID, path string) (*File, error)
	ListFiles(ctx context.Context, libraryID string) ([]*File, error)
	ListFilesByStatus(ctx context.Context, libraryID, status string) ([]*File, error)
	UpdateFile(ctx context.Context, f *File) error
	DeleteFile(ctx context.Context, id string) error

	// Jobs.
	CreateJob(ctx context.Context, j *Job) error
	GetJob(ctx context.Context, id string) (*Job, error)
	ListJobs(ctx context.Context, filter JobFilter) ([]*Job, error)
	UpdateJob(ctx context.Context, j *Job) error
	DeleteJob(ctx context.Context, id string) error

	// Queue operations. ClaimNextJob atomically moves the highest-priority
	// oldest queued job to claimed for the given node, so concurrent
	// workers never receive the same job (ErrQueueEmpty when none is left).
	// RequeueStaleJobs returns claimed/running jobs whose heartbeat is
	// older than cutoff to queued (attempt+1, claim fields cleared) or, at
	// maxAttempts, fails them; it returns the jobs it transitioned.
	ClaimNextJob(ctx context.Context, nodeID string) (*Job, error)
	RequeueStaleJobs(ctx context.Context, cutoff time.Time, maxAttempts int) ([]*Job, error)

	// Nodes.
	CreateNode(ctx context.Context, n *Node) error
	GetNode(ctx context.Context, id string) (*Node, error)
	ListNodes(ctx context.Context) ([]*Node, error)
	UpdateNode(ctx context.Context, n *Node) error
	DeleteNode(ctx context.Context, id string) error

	// Pairing tokens. GetPairingTokenByHash looks up by the SHA-256 of
	// the raw token; raw tokens are never stored.
	CreatePairingToken(ctx context.Context, t *PairingToken) error
	GetPairingToken(ctx context.Context, id string) (*PairingToken, error)
	GetPairingTokenByHash(ctx context.Context, tokenHash string) (*PairingToken, error)
	ListPairingTokens(ctx context.Context) ([]*PairingToken, error)
	UpdatePairingToken(ctx context.Context, t *PairingToken) error
	DeletePairingToken(ctx context.Context, id string) error

	// Settings is an open key/value table; the Setting* constants name
	// the keys ARCHITECTURE.md defines.
	GetSetting(ctx context.Context, key string) (string, bool, error)
	SetSetting(ctx context.Context, key, value string) error
	DeleteSetting(ctx context.Context, key string) error
	ListSettings(ctx context.Context) (map[string]string, error)
}

// validateResultPolicy rejects unknown result policies on write.
func validateResultPolicy(v string) error {
	for _, ok := range ResultPolicyValues() {
		if v == ok {
			return nil
		}
	}
	return fmt.Errorf("store: invalid result_policy %q: want one of %v", v, ResultPolicyValues())
}

// validateFileStatus rejects unknown file statuses on write.
func validateFileStatus(v string) error {
	for _, ok := range FileStatusValues() {
		if v == ok {
			return nil
		}
	}
	return fmt.Errorf("store: invalid file status %q: want one of %v", v, FileStatusValues())
}

// validateJobStatus rejects unknown job statuses on write.
func validateJobStatus(v string) error {
	for _, ok := range JobStatusValues() {
		if v == ok {
			return nil
		}
	}
	return fmt.Errorf("store: invalid job status %q: want one of %v", v, JobStatusValues())
}

// validateNodeStatus rejects unknown node statuses on write.
func validateNodeStatus(v string) error {
	for _, ok := range NodeStatusValues() {
		if v == ok {
			return nil
		}
	}
	return fmt.Errorf("store: invalid node status %q: want one of %v", v, NodeStatusValues())
}

// validateNodeAccessMode rejects unknown access modes on write.
func validateNodeAccessMode(v string) error {
	for _, ok := range NodeAccessModeValues() {
		if v == ok {
			return nil
		}
	}
	return fmt.Errorf("store: invalid node access_mode %q: want one of %v", v, NodeAccessModeValues())
}
