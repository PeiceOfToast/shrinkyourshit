// SPDX-License-Identifier: AGPL-3.0-only

package store

import (
	"context"
	"fmt"
	"time"
)

// JobFilter narrows ListJobs. Empty fields mean "no filter". Order is ""
// (oldest first) or "desc" (newest first); anything else is rejected.
type JobFilter struct {
	Status    string
	FileID    string
	LibraryID string
	NodeID    string
	Order     string
	Limit     int // 0 = no limit
	Offset    int
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
	// SetLibraryScanResult stamps the library's last successful scan (task
	// 19) with the scan's summary JSON (task 34). Deliberately not part of
	// UpdateLibrary: the scanner stamps it without rewriting the rest of
	// the row.
	SetLibraryScanResult(ctx context.Context, id string, at time.Time, summary string) error

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
	// UpdateJobIfActive applies a job update only while the job is still
	// claimed/running, so a late worker report can never resurrect a
	// cancelled or finished job.
	ClaimNextJob(ctx context.Context, nodeID string) (*Job, error)
	RequeueStaleJobs(ctx context.Context, cutoff time.Time, maxAttempts int) ([]*Job, error)
	UpdateJobIfActive(ctx context.Context, j *Job) (bool, error)

	// Job log rows (task 27): every job keeps its complete log as
	// structured rows in SQLite. AppendJobLog enforces the per-job cap;
	// PruneJobLog applies retention by age and total size and never
	// touches a queued or running job's rows.
	AppendJobLog(ctx context.Context, jobID string, lines []JobLogLine) error
	ListJobLog(ctx context.Context, jobID string, afterID int64, level string, limit int) ([]JobLogLine, error)
	CountJobLog(ctx context.Context, jobID string) (int64, error)
	PruneJobLog(ctx context.Context, olderThan time.Time, maxTotal int) (int64, error)
	DeleteJobLog(ctx context.Context, jobID string) error

	// Nodes.
	CreateNode(ctx context.Context, n *Node) error
	GetNode(ctx context.Context, id string) (*Node, error)
	// GetNodeByKeyHash looks a node up by the SHA-256 hex hash of its raw
	// key; raw keys are never stored.
	GetNodeByKeyHash(ctx context.Context, keyHash string) (*Node, error)
	ListNodes(ctx context.Context) ([]*Node, error)
	// NodeJobStats folds every node's finished-job totals (files done,
	// bytes saved — positive savings only) into one grouped query (task
	// 18). Nodes without finished jobs are absent from the result.
	NodeJobStats(ctx context.Context) (map[string]NodeStat, error)
	UpdateNode(ctx context.Context, n *Node) error
	// TouchNodeHeartbeat records a heartbeat without writing the whole
	// row: a heartbeat loads the node before the lifecycle endpoints may
	// have moved desired_state, and a whole-row write resurrected that
	// state (a stop or start lost under a running heartbeat, task 11).
	// Only the heartbeat's own columns change; capabilities is stored
	// verbatim when non-empty. completingDrain moves a draining node to
	// stopped — conditionally, so it cannot override a concurrent
	// stop-now that landed while the heartbeat was in flight.
	TouchNodeHeartbeat(ctx context.Context, id string, capabilities string, completingDrain bool, now time.Time) error
	DeleteNode(ctx context.Context, id string) error
	// ConsumeNodeKeyOnce atomically returns and clears the node's one-time
	// raw key (pairing, task 08): the first caller gets the key, later
	// callers get "". Empty result also means "nothing stored".
	ConsumeNodeKeyOnce(ctx context.Context, nodeID string) (string, error)

	// Pairing tokens. GetPairingTokenByHash looks up by the SHA-256 of
	// the raw token; raw tokens are never stored.
	CreatePairingToken(ctx context.Context, t *PairingToken) error
	GetPairingToken(ctx context.Context, id string) (*PairingToken, error)
	GetPairingTokenByHash(ctx context.Context, tokenHash string) (*PairingToken, error)
	ListPairingTokens(ctx context.Context) ([]*PairingToken, error)
	UpdatePairingToken(ctx context.Context, t *PairingToken) error
	DeletePairingToken(ctx context.Context, id string) error

	// Sessions (task 06 admin auth). Only the SHA-256 of the raw token is
	// stored. GetSessionByHash returns the row even when expired; callers
	// check ExpiresAt. DeleteExpiredSessions prunes past expiry.
	CreateSession(ctx context.Context, s *Session) error
	GetSessionByHash(ctx context.Context, tokenHash string) (*Session, error)
	DeleteSession(ctx context.Context, tokenHash string) error
	DeleteExpiredSessions(ctx context.Context, now time.Time) (int64, error)

	// Integrations (task 23): remote media servers and download clients
	// the refresher notifies after a job ends. The API key is stored
	// verbatim for the outbound call and never leaves the server through
	// the API.
	CreateIntegration(ctx context.Context, in *Integration) error
	GetIntegration(ctx context.Context, id string) (*Integration, error)
	ListIntegrations(ctx context.Context) ([]*Integration, error)
	UpdateIntegration(ctx context.Context, in *Integration) error
	DeleteIntegration(ctx context.Context, id string) error

	// Webhooks (task 25): outbound HTTP hooks the dispatcher notifies on
	// job.done, job.failed and library.scanned. The secret signs every
	// delivery and is never echoed by the API after the create response.
	// AddWebhookDelivery records one delivery outcome;
	// PruneWebhookDeliveries keeps the newest keep rows per hook.
	CreateWebhook(ctx context.Context, w *Webhook) error
	GetWebhook(ctx context.Context, id string) (*Webhook, error)
	ListWebhooks(ctx context.Context) ([]*Webhook, error)
	UpdateWebhook(ctx context.Context, w *Webhook) error
	DeleteWebhook(ctx context.Context, id string) error
	AddWebhookDelivery(ctx context.Context, d *WebhookDelivery) error
	ListWebhookDeliveries(ctx context.Context, webhookID string, limit int) ([]*WebhookDelivery, error)
	PruneWebhookDeliveries(ctx context.Context, keep int) (int64, error)

	// Plugins (task 31): the index of installed plugins. The manifest
	// lives on disk under <data dir>/plugins/<name>/; the row carries the
	// identity, the enabled switch (installed plugins start disabled) and
	// the collected settings values. ListEnabledPlugins feeds the phase
	// hooks.
	CreatePlugin(ctx context.Context, p *Plugin) error
	GetPlugin(ctx context.Context, id string) (*Plugin, error)
	GetPluginByName(ctx context.Context, name string) (*Plugin, error)
	ListPlugins(ctx context.Context) ([]*Plugin, error)
	ListEnabledPlugins(ctx context.Context, pluginType string) ([]*Plugin, error)
	UpdatePlugin(ctx context.Context, p *Plugin) error
	DeletePlugin(ctx context.Context, id string) error

	// Settings is an open key/value table; the Setting* constants name
	// the keys ARCHITECTURE.md defines.
	GetSetting(ctx context.Context, key string) (string, bool, error)
	SetSetting(ctx context.Context, key, value string) error
	DeleteSetting(ctx context.Context, key string) error
	ListSettings(ctx context.Context) (map[string]string, error)

	// Backup writes a consistent snapshot of the whole database to dest
	// (task 16: the pre-update backup). VACUUM INTO is safe while the
	// server is running; a plain file copy could catch a mid-commit
	// state. An existing dest is removed first (VACUUM INTO refuses to
	// overwrite).
	Backup(ctx context.Context, dest string) error
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

// validateNodeDesiredState rejects unknown desired states on write. An
// empty value is stopped: the column default for rows created before the
// field existed.
func validateNodeDesiredState(v string) error {
	if v == "" {
		return nil
	}
	for _, ok := range NodeDesiredStateValues() {
		if v == ok {
			return nil
		}
	}
	return fmt.Errorf("store: invalid node desired_state %q: want one of %v", v, NodeDesiredStateValues())
}

// validateNodeConcurrency bounds the parallel-job cap (task 11: 1 to 16).
func validateNodeConcurrency(n int) error {
	if n < ConcurrencyMin || n > ConcurrencyMax {
		return fmt.Errorf("store: invalid node concurrency %d: want %d to %d", n, ConcurrencyMin, ConcurrencyMax)
	}
	return nil
}
