// SPDX-License-Identifier: AGPL-3.0-only

package worker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/PeiceOfToast/shrinkyourshit/internal/discovery"
	"github.com/PeiceOfToast/shrinkyourshit/internal/server"
)

// Pairing (task 08): the first-run flow of a worker. The node key lives in
// <data-dir>/node.key; while a tokenless pairing waits for admin approval
// the pending node id is kept in <data-dir>/node.json so a restart resumes
// the same pending node instead of stacking duplicates.

// PairPollIntervalDefault is how often a pending worker asks whether it
// has been approved (task 08: every 5s).
const PairPollIntervalDefault = 5 * time.Second

// discoverTimeoutDefault is how long server discovery waits.
const discoverTimeoutDefault = 5 * time.Second

// pendingFile records an in-flight tokenless pairing.
const pendingFile = "node.json"

// pendingState is the node.json content.
type pendingState struct {
	NodeID    string `json:"node_id"`
	ServerURL string `json:"server_url"`
}

// PairConfig tunes EnsurePaired.
type PairConfig struct {
	// ServerURL is the server to join; empty → Discover must find one.
	ServerURL string
	// Token is an optional one-time pairing token (immediate approval).
	Token string
	// NodeName is the display name shown in the server UI (required).
	NodeName string
	// DataDir holds node.key and the pending state.
	DataDir string
	// Discover finds servers when ServerURL is empty. Wired by main to
	// discovery.Discover; tests inject fakes.
	Discover func(ctx context.Context, timeout time.Duration) []discovery.ServerInfo
	// DiscoverTimeout overrides the discovery wait (tests).
	DiscoverTimeout time.Duration
	// PollInterval overrides the approval poll cadence (tests).
	PollInterval time.Duration
	// Log may be nil (slog default).
	Log *slog.Logger
}

// EnsurePaired returns the server URL and node key to run with. When the
// data dir already has a key it is returned as-is (nothing to pair).
// Otherwise the worker finds a server (config or discovery), pairs — with
// the token when set, otherwise as pending — and, for a pending node,
// prints "Waiting for approval in the server UI" and polls until the admin
// approves. A pending pairing survives restarts through node.json.
func EnsurePaired(ctx context.Context, cfg PairConfig) (serverURL, key string, err error) {
	log := cfg.Log
	if log == nil {
		log = slog.Default()
	}
	if strings.TrimSpace(cfg.NodeName) == "" {
		return "", "", errors.New("worker: node name is required to pair (SHRINK_NODE_NAME)")
	}
	if cfg.DataDir == "" {
		return "", "", errors.New("worker: data dir is required to pair")
	}
	keyPath := filepath.Join(cfg.DataDir, server.NodeKeyFile)

	// An existing key means this worker already joined: nothing to do.
	if raw, err := os.ReadFile(keyPath); err == nil {
		if k := strings.TrimSpace(string(raw)); k != "" {
			return cfg.ServerURL, k, nil
		}
	} else if !os.IsNotExist(err) {
		return "", "", fmt.Errorf("worker: read node key: %w", err)
	}

	poll := cfg.PollInterval
	if poll <= 0 {
		poll = PairPollIntervalDefault
	}
	discoverTimeout := cfg.DiscoverTimeout
	if discoverTimeout <= 0 {
		discoverTimeout = discoverTimeoutDefault
	}

	// A pending pairing from an earlier run resumes on its own server;
	// an explicit SHRINK_SERVER that differs means the admin re-pointed
	// this worker, so the stale pending node is dropped.
	serverURL = strings.TrimRight(cfg.ServerURL, "/")
	if state, ok := readPending(cfg.DataDir); ok {
		if serverURL == "" || serverURL == strings.TrimRight(state.ServerURL, "/") {
			serverURL = state.ServerURL
			key, err = pollForApproval(ctx, log, serverURL, state.NodeID, cfg.DataDir, poll)
			if err != nil {
				return "", "", err
			}
			return serverURL, key, nil
		}
		log.Info("worker: dropping stale pending pairing for another server", "server", state.ServerURL)
		_ = os.Remove(filepath.Join(cfg.DataDir, pendingFile))
	}

	// Find the server when no URL was configured.
	if serverURL == "" {
		if cfg.Discover == nil {
			return "", "", errors.New("worker: no server configured and discovery unavailable: set SHRINK_SERVER")
		}
		log.Info("worker: looking for a server on the local network…")
		found := cfg.Discover(ctx, discoverTimeout)
		if len(found) == 0 {
			return "", "", errors.New("worker: no server found on the network: set SHRINK_SERVER, e.g. http://192.168.1.10:8765")
		}
		serverURL = strings.TrimRight(found[0].URL, "/")
		log.Info("worker: found server", "server", serverURL, "name", found[0].Name)
	}

	client := NewClient(serverURL, "")
	resp, err := client.Pair(ctx, PairInput{
		Token:        cfg.Token,
		Name:         cfg.NodeName,
		OS:           runtime.GOOS,
		Arch:         runtime.GOARCH,
		Capabilities: json.RawMessage(`{}`), // the first heartbeat refreshes this
	})
	if err != nil {
		return "", "", err
	}

	if resp.NodeKey != "" {
		if err := saveNodeKey(cfg.DataDir, resp.NodeKey); err != nil {
			return "", "", err
		}
		log.Info("worker: paired with server", "server", serverURL, "node", resp.NodeID)
		return serverURL, resp.NodeKey, nil
	}

	// Pending: remember the node id so restarts resume the same pairing.
	if err := writePending(cfg.DataDir, pendingState{NodeID: resp.NodeID, ServerURL: serverURL}); err != nil {
		return "", "", err
	}
	key, err = pollForApproval(ctx, log, serverURL, resp.NodeID, cfg.DataDir, poll)
	if err != nil {
		return "", "", err
	}
	return serverURL, key, nil
}

// pollForApproval polls GET /pair/{id} until the node is approved. The key
// arrives exactly once and is saved immediately; the message the task asks
// for is printed once.
func pollForApproval(ctx context.Context, log *slog.Logger, serverURL, nodeID, dataDir string, poll time.Duration) (string, error) {
	client := NewClient(serverURL, "")
	log.Info("Waiting for approval in the server UI", "server", serverURL, "node", nodeID)
	ticker := time.NewTicker(poll)
	defer ticker.Stop()
	for {
		resp, err := client.PairPoll(ctx, nodeID)
		if err != nil {
			if ctx.Err() != nil {
				return "", ctx.Err()
			}
			log.Warn("worker: pairing poll", "err", err)
		} else {
			switch {
			case resp.NodeKey != "":
				if err := saveNodeKey(dataDir, resp.NodeKey); err != nil {
					return "", err
				}
				log.Info("worker: paired with server", "server", serverURL, "node", nodeID)
				return resp.NodeKey, nil
			case resp.Status == "approved":
				return "", fmt.Errorf("worker: node %s was approved but its key was already retrieved; remove the node in the server UI and pair again", nodeID)
			case resp.Status == "revoked":
				return "", fmt.Errorf("worker: node %s was rejected or revoked on the server", nodeID)
			}
		}
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-ticker.C:
		}
	}
}

// saveNodeKey writes the key file with 0600 and clears the pending state.
func saveNodeKey(dataDir, key string) error {
	path := filepath.Join(dataDir, server.NodeKeyFile)
	if err := os.WriteFile(path, []byte(key+"\n"), 0o600); err != nil {
		return fmt.Errorf("worker: write node key: %w", err)
	}
	_ = os.Remove(filepath.Join(dataDir, pendingFile))
	return nil
}

// writePending records an in-flight tokenless pairing.
func writePending(dataDir string, state pendingState) error {
	raw, err := json.Marshal(state)
	if err != nil {
		return fmt.Errorf("worker: encode pending state: %w", err)
	}
	if err := os.WriteFile(filepath.Join(dataDir, pendingFile), raw, 0o600); err != nil {
		return fmt.Errorf("worker: write pending state: %w", err)
	}
	return nil
}

// readPending loads a recorded pending pairing, if any.
func readPending(dataDir string) (pendingState, bool) {
	raw, err := os.ReadFile(filepath.Join(dataDir, pendingFile))
	if err != nil {
		return pendingState{}, false
	}
	var state pendingState
	if json.Unmarshal(raw, &state) != nil || state.NodeID == "" || state.ServerURL == "" {
		return pendingState{}, false
	}
	return state, true
}
