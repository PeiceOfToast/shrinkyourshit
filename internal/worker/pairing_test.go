// SPDX-License-Identifier: AGPL-3.0-only

package worker

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/PeiceOfToast/shrinkyourshit/internal/discovery"
	"github.com/PeiceOfToast/shrinkyourshit/internal/server"
	"github.com/stretchr/testify/require"
)

// pairStub is a fake /pair + /pair/{id} backend for the pairing flow.
type pairStub struct {
	mu sync.Mutex
	// pairResp is the POST /pair body; pairStatus its status.
	pairStatus int
	pairBody   string
	// polls maps call number → response (the last one repeats).
	polls     []string
	pollCount int
	// hits records "METHOD /path" lines.
	hits []string
}

func (s *pairStub) handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/v1/pair", func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		s.hits = append(s.hits, "POST /pair")
		status, body := s.pairStatus, s.pairBody
		s.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = io.WriteString(w, body)
	})
	mux.HandleFunc("GET /api/v1/pair/{id}", func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		s.pollCount++
		i := s.pollCount
		resp := "{}"
		if len(s.polls) > 0 {
			resp = s.polls[min(i, len(s.polls))-1]
		}
		s.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, resp)
	})
	return mux
}

func (s *pairStub) hitCount(path string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	n := 0
	for _, h := range s.hits {
		if h == path {
			n++
		}
	}
	return n
}

func (s *pairStub) pollCalls() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.pollCount
}

func TestEnsurePairedReturnsExistingKey(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, server.NodeKeyFile), []byte("already-paired-key\n"), 0o600))

	calls := 0
	url, key, err := EnsurePaired(context.Background(), PairConfig{
		ServerURL: "http://stub", NodeName: "w", DataDir: dir,
		Discover: func(ctx context.Context, timeout time.Duration) []discovery.ServerInfo {
			calls++
			return nil
		},
	})
	require.NoError(t, err)
	require.Equal(t, "http://stub", url)
	require.Equal(t, "already-paired-key", key)
	require.Equal(t, 0, calls) // no discovery, no pairing
}

func TestEnsurePairedWithTokenApproves(t *testing.T) {
	dir := t.TempDir()
	stub := &pairStub{pairStatus: http.StatusCreated, pairBody: `{"node_id":"n1","status":"approved","node_key":"tokened-key"}`}
	api := httptest.NewServer(stub.handler())
	t.Cleanup(api.Close)

	url, key, err := EnsurePaired(context.Background(), PairConfig{
		ServerURL: api.URL, Token: "one-time", NodeName: "worker-t", DataDir: dir,
		Log: slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	require.NoError(t, err)
	require.Equal(t, api.URL, url)
	require.Equal(t, "tokened-key", key)

	// The key is saved for the next start and the pending state is absent.
	raw, err := os.ReadFile(filepath.Join(dir, server.NodeKeyFile))
	require.NoError(t, err)
	require.Equal(t, "tokened-key\n", string(raw))
	_, err = os.Stat(filepath.Join(dir, pendingFile))
	require.True(t, os.IsNotExist(err))

	// The pair request carried the token and the node name.
	hits := stub.hitCount("POST /pair")
	require.Equal(t, 1, hits)
}

func TestEnsurePairedPendingApproval(t *testing.T) {
	dir := t.TempDir()
	stub := &pairStub{
		pairStatus: http.StatusCreated,
		pairBody:   `{"node_id":"n2","status":"pending"}`,
		polls: []string{
			`{"node_id":"n2","status":"pending"}`,
			`{"node_id":"n2","status":"pending"}`,
			`{"node_id":"n2","status":"approved","node_key":"approved-key"}`,
		},
	}
	api := httptest.NewServer(stub.handler())
	t.Cleanup(api.Close)

	url, key, err := EnsurePaired(context.Background(), PairConfig{
		ServerURL: api.URL, NodeName: "worker-p", DataDir: dir,
		PollInterval: 5 * time.Millisecond,
		Log:          slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	require.NoError(t, err)
	require.Equal(t, api.URL, url)
	require.Equal(t, "approved-key", key)

	// The pending state was consumed.
	_, err = os.Stat(filepath.Join(dir, pendingFile))
	require.True(t, os.IsNotExist(err))
	raw, err := os.ReadFile(filepath.Join(dir, server.NodeKeyFile))
	require.NoError(t, err)
	require.Equal(t, "approved-key\n", string(raw))

	// The stub polled three times (pending, pending, approved).
	require.Equal(t, 3, stub.pollCalls())
}

func TestEnsurePairedResumesPendingAcrossRestart(t *testing.T) {
	dir := t.TempDir()
	stub := &pairStub{
		pairStatus: http.StatusInternalServerError,
		pairBody:   `{"error":"must not be called","code":"x"}`,
		polls:      []string{`{"node_id":"n9","status":"approved","node_key":"resumed-key"}`},
	}
	api := httptest.NewServer(stub.handler())
	t.Cleanup(api.Close)

	// Simulate a restart: the pending state file exists from the earlier
	// run and points at this same server.
	require.NoError(t, os.WriteFile(filepath.Join(dir, pendingFile),
		[]byte(`{"node_id":"n9","server_url":"`+api.URL+`"}`), 0o600))

	url, key, err := EnsurePaired(context.Background(), PairConfig{
		ServerURL: api.URL, NodeName: "worker-p", DataDir: dir,
		PollInterval: 5 * time.Millisecond,
		Log:          slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	require.NoError(t, err)
	require.Equal(t, api.URL, url)
	require.Equal(t, "resumed-key", key)
	require.Equal(t, 0, stub.hitCount("POST /pair"))
}

func TestEnsurePairedDiscoverPicksServer(t *testing.T) {
	dir := t.TempDir()
	stub := &pairStub{pairStatus: http.StatusCreated, pairBody: `{"node_id":"n3","status":"approved","node_key":"disc-key"}`}
	api := httptest.NewServer(stub.handler())
	t.Cleanup(api.Close)

	url, key, err := EnsurePaired(context.Background(), PairConfig{
		NodeName: "worker-d", DataDir: dir,
		Discover: func(ctx context.Context, timeout time.Duration) []discovery.ServerInfo {
			return []discovery.ServerInfo{{URL: api.URL, Name: "the-server"}}
		},
		Log: slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	require.NoError(t, err)
	require.Equal(t, api.URL, url)
	require.Equal(t, "disc-key", key)
}

func TestEnsurePairedNoServerFound(t *testing.T) {
	_, _, err := EnsurePaired(context.Background(), PairConfig{
		NodeName: "w", DataDir: t.TempDir(),
		Discover: func(ctx context.Context, timeout time.Duration) []discovery.ServerInfo { return nil },
		Log:      slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "SHRINK_SERVER")
}

func TestEnsurePairedTokenRejected(t *testing.T) {
	stub := &pairStub{pairStatus: http.StatusUnauthorized, pairBody: `{"error":"bad token","code":"invalid_token"}`}
	api := httptest.NewServer(stub.handler())
	t.Cleanup(api.Close)

	_, _, err := EnsurePaired(context.Background(), PairConfig{
		ServerURL: api.URL, Token: "expired", NodeName: "w", DataDir: t.TempDir(),
		Log: slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	require.Error(t, err)
	var apiErr *APIError
	require.True(t, errors.As(err, &apiErr))
	require.Equal(t, http.StatusUnauthorized, apiErr.Status)
}

func TestEnsurePairedRevokedDuringPoll(t *testing.T) {
	dir := t.TempDir()
	stub := &pairStub{
		pairStatus: http.StatusCreated,
		pairBody:   `{"node_id":"n4","status":"pending"}`,
		polls:      []string{`{"node_id":"n4","status":"revoked"}`},
	}
	api := httptest.NewServer(stub.handler())
	t.Cleanup(api.Close)

	_, _, err := EnsurePaired(context.Background(), PairConfig{
		ServerURL: api.URL, NodeName: "worker-p", DataDir: dir,
		PollInterval: 5 * time.Millisecond,
		Log:          slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "rejected or revoked")
}
