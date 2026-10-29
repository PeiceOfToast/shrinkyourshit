// SPDX-License-Identifier: AGPL-3.0-only

package server

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/PeiceOfToast/shrinkyourshit/internal/store"
)

// ErrUnauthorized is returned by NodeRegistry.Authenticate for a missing,
// unknown, revoked or disabled node key. The API maps it to 401.
var ErrUnauthorized = errors.New("node: unauthorized")

// NodeKeyFile is the name of the built-in node key inside the data dir
// (API.md: the built-in worker uses a key generated at first run).
const NodeKeyFile = "node.key"

// builtinNodeSuffix marks the server's own node row.
const builtinNodeSuffix = " (built-in)"

// BuiltinNodeName is the display name of the built-in node for a host.
func BuiltinNodeName(host string) string { return host + builtinNodeSuffix }

// HashKey returns the SHA-256 hex hash of a raw node key. Raw keys are
// never stored (ARCHITECTURE.md security).
func HashKey(key string) string {
	sum := sha256.Sum256([]byte(key))
	return hex.EncodeToString(sum[:])
}

// generateNodeKey returns a fresh node key: 32 random bytes, base64url
// (43 characters, no padding; task 08). The server stores only the SHA-256
// of the key (HashKey): argon2id would add nothing for a high-entropy
// random key, so the fast hash is the documented choice.
func generateNodeKey() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("node: generate key: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// PathMapping is one entry of a node's path_mappings JSON: the node sees
// NodePath where the server sees ServerPath. Matching is prefix-based on
// path boundaries and the longest match wins, so a more specific mount can
// override a broad one.
type PathMapping struct {
	ServerPath string `json:"server_path"`
	NodePath   string `json:"node_path"`
}

// ParsePathMappings decodes a node's path_mappings JSON; a blank or broken
// value yields no mappings (identity).
func ParsePathMappings(raw string) []PathMapping {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	var out []PathMapping
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return nil
	}
	return out
}

// ResolveNodePath maps a server-side library path to the path the node
// uses for it. Without mappings (the built-in node) the path is unchanged.
func ResolveNodePath(node *store.Node, serverPath string) string {
	best := -1
	var mapped PathMapping
	for _, m := range ParsePathMappings(node.PathMappings) {
		if n := mappingPrefixLen(m.ServerPath, serverPath); n > best {
			best = n
			mapped = m
		}
	}
	if best < 0 {
		return serverPath
	}
	return strings.TrimRight(mapped.NodePath, "/") + serverPath[best:]
}

// mappingPrefixLen returns the length of root when path is root or lies
// inside it, else -1. Path-boundary aware: /media must not match /media2.
func mappingPrefixLen(root, path string) int {
	root = strings.TrimRight(root, "/")
	if root == "" {
		return -1
	}
	if path == root {
		return len(root)
	}
	if strings.HasPrefix(path, root+"/") {
		return len(root)
	}
	return -1
}

// NodeRegistry owns node authentication, the built-in node and worker
// heartbeats, plus pairing (task 08): tokens, approve, revoke, update.
type NodeRegistry struct {
	st     store.Store
	events *EventBus
	log    *slog.Logger
	// queue requeues a node's in-flight jobs on revoke/delete; wired by
	// server.New after both services exist.
	queue *Queue
}

// NewNodeRegistry builds the registry. events may be nil (no publishing).
func NewNodeRegistry(st store.Store, events *EventBus, log *slog.Logger) *NodeRegistry {
	if log == nil {
		log = slog.Default()
	}
	return &NodeRegistry{st: st, events: events, log: log}
}

// SetQueue wires the queue used to requeue in-flight jobs when a node is
// revoked or deleted. Nil is allowed (registry used without a queue).
func (r *NodeRegistry) SetQueue(q *Queue) { r.queue = q }

// Authenticate resolves a raw node key to an approved, enabled node.
// Anything else is ErrUnauthorized (API.md: unpaired or revoked nodes
// cannot claim jobs or read files).
func (r *NodeRegistry) Authenticate(ctx context.Context, key string) (*store.Node, error) {
	if strings.TrimSpace(key) == "" {
		return nil, ErrUnauthorized
	}
	node, err := r.st.GetNodeByKeyHash(ctx, HashKey(key))
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil, ErrUnauthorized
		}
		return nil, err
	}
	if node.Status != store.NodeStatusApproved || !node.Enabled {
		return nil, ErrUnauthorized
	}
	return node, nil
}

// EnsureBuiltinNode makes sure the built-in node exists and matches the
// key file, returning the raw key. It is idempotent across restarts:
//   - the key file is read or created (0600) in the data dir;
//   - a node whose key hash matches is returned as-is;
//   - otherwise a node with the built-in name is re-keyed (a lost key file
//     heals instead of spawning duplicate nodes);
//   - otherwise the node is created approved with shared access.
func (r *NodeRegistry) EnsureBuiltinNode(ctx context.Context, dataDir, name string) (string, *store.Node, error) {
	key, err := readOrCreateKey(filepath.Join(dataDir, NodeKeyFile))
	if err != nil {
		return "", nil, err
	}
	hash := HashKey(key)

	node, err := r.st.GetNodeByKeyHash(ctx, hash)
	if err == nil {
		return key, node, nil
	}
	if !errors.Is(err, store.ErrNotFound) {
		return "", nil, fmt.Errorf("node: lookup built-in by key: %w", err)
	}

	nodes, err := r.st.ListNodes(ctx)
	if err != nil {
		return "", nil, fmt.Errorf("node: list for built-in: %w", err)
	}
	for _, existing := range nodes {
		if existing.Name != name {
			continue
		}
		existing.KeyHash = hash
		existing.Status = store.NodeStatusApproved
		existing.Enabled = true
		existing.AccessMode = store.NodeAccessShared
		if err := r.st.UpdateNode(ctx, existing); err != nil {
			return "", nil, fmt.Errorf("node: rekey built-in: %w", err)
		}
		r.log.Info("built-in node re-keyed (key file was lost)", "node", existing.ID)
		r.publish(EventNodeUpdated, NewNodeDTO(existing))
		return key, existing, nil
	}

	node = &store.Node{
		Name:         name,
		KeyHash:      hash,
		OS:           runtime.GOOS,
		Arch:         runtime.GOARCH,
		Capabilities: "{}",
		AccessMode:   store.NodeAccessShared,
		PathMappings: "[]",
		Status:       store.NodeStatusApproved,
		Enabled:      true,
	}
	if err := r.st.CreateNode(ctx, node); err != nil {
		return "", nil, fmt.Errorf("node: create built-in: %w", err)
	}
	r.log.Info("built-in node created", "node", node.ID, "name", node.Name)
	r.publish(EventNodeUpdated, NewNodeDTO(node))
	return key, node, nil
}

// Heartbeat records a node heartbeat, refreshes the heartbeats of the
// node's claimed/running jobs (a long encode with no progress must not be
// swept) and returns the worker concurrency setting. capabilities is stored
// verbatim when non-empty; unknown running job ids are ignored.
func (r *NodeRegistry) Heartbeat(ctx context.Context, node *store.Node, capabilities string, runningJobIDs []string) (int, error) {
	now := time.Now().UTC()
	if capabilities != "" {
		node.Capabilities = capabilities
	}
	node.LastHeartbeatAt = now
	if err := r.st.UpdateNode(ctx, node); err != nil {
		return 0, fmt.Errorf("node: heartbeat update: %w", err)
	}

	for _, id := range runningJobIDs {
		job, err := r.st.GetJob(ctx, id)
		if err != nil || job.NodeID != node.ID {
			continue // not ours (anymore): ignore
		}
		if job.Status != store.JobStatusClaimed && job.Status != store.JobStatusRunning {
			continue
		}
		job.HeartbeatAt = now
		// UpdateJobIfActive only applies while the job is still claimed or
		// running: a heartbeat racing a cancel or finish cannot resurrect
		// it, and a refused update simply ends this iteration.
		if _, err := r.st.UpdateJobIfActive(ctx, job); err != nil {
			r.log.Error("node: running job heartbeat", "job", job.ID, "err", err)
		}
	}

	r.publish(EventNodeUpdated, NewNodeDTO(node))

	n, err := r.Concurrency(ctx)
	if err != nil {
		return 1, err
	}
	return n, nil
}

// Concurrency reads the worker_concurrency setting (default and minimum 1).
func (r *NodeRegistry) Concurrency(ctx context.Context) (int, error) {
	v, ok, err := r.st.GetSetting(ctx, store.SettingWorkerConcurrency)
	if err != nil {
		return 1, fmt.Errorf("node: read concurrency: %w", err)
	}
	if !ok || strings.TrimSpace(v) == "" {
		return 1, nil
	}
	n, err := strconv.Atoi(strings.TrimSpace(v))
	if err != nil || n < 1 {
		r.log.Warn("node: bad worker_concurrency setting", "value", v)
		return 1, nil
	}
	return n, nil
}

// readOrCreateKey loads the node key file or creates it with a fresh key.
func readOrCreateKey(path string) (string, error) {
	if data, err := os.ReadFile(path); err == nil {
		if key := strings.TrimSpace(string(data)); key != "" {
			return key, nil
		}
	} else if !os.IsNotExist(err) {
		return "", fmt.Errorf("node: read key file: %w", err)
	}
	key, err := generateNodeKey()
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(path, []byte(key+"\n"), 0o600); err != nil {
		return "", fmt.Errorf("node: write key file: %w", err)
	}
	return key, nil
}

// publish is a nil-safe event publish.
func (r *NodeRegistry) publish(eventType string, payload any) {
	if r.events == nil {
		return
	}
	r.events.Publish(eventType, payload)
}
