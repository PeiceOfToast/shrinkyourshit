// SPDX-License-Identifier: AGPL-3.0-only

//go:build integration

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/PeiceOfToast/shrinkyourshit/internal/testutil"
	"github.com/stretchr/testify/require"
)

// TestIntegrationWorkerJoinsViaDiscovery is acceptance item 1: two real
// processes on one machine, separate data dirs. The worker finds the
// server without being told its address — once via mDNS, once via the UDP
// broadcast fallback (each with the other transport disabled, per the
// task) — pairs with a token and completes a job.
func TestIntegrationWorkerJoinsViaDiscovery(t *testing.T) {
	ffmpeg, ffprobe := testutil.RequireFFmpeg(t)
	bin := buildBinary(t)

	t.Run("broadcast", func(t *testing.T) {
		runJoinScenario(t, bin, ffmpeg, ffprobe, discoveryToggles{mdns: false, broadcast: true})
	})

	t.Run("mdns", func(t *testing.T) {
		requireMulticast(t)
		runJoinScenario(t, bin, ffmpeg, ffprobe, discoveryToggles{mdns: true, broadcast: false})
	})
}

type discoveryToggles struct {
	mdns      bool
	broadcast bool
	port      int // UDP broadcast port; 0 → default
}

func (d discoveryToggles) env() []string {
	env := []string{
		"SHRINK_DISCOVERY_MDNS=" + boolEnv(d.mdns),
		"SHRINK_DISCOVERY_BROADCAST=" + boolEnv(d.broadcast),
	}
	if d.port != 0 {
		env = append(env, "SHRINK_DISCOVERY_PORT="+fmt.Sprint(d.port))
	}
	return env
}

func boolEnv(b bool) string {
	if b {
		return "1"
	}
	return "0"
}

// buildBinary compiles the real binary once for the whole test.
func buildBinary(t *testing.T) string {
	t.Helper()
	out := filepath.Join(t.TempDir(), "shrinkyourshit")
	cmd := exec.Command("go", "build", "-o", out, "./cmd/shrinkyourshit")
	cmd.Dir = "../.."
	if outBytes, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("go build failed: %v\n%s", err, outBytes)
	}
	return out
}

// requireMulticast skips when this host cannot use IP multicast at all
// (common in CI containers); the broadcast subtest always runs.
func requireMulticast(t *testing.T) {
	t.Helper()
	group := &net.UDPAddr{IP: net.IPv4(224, 0, 0, 251), Port: 5353}
	conn, err := net.ListenMulticastUDP("udp4", nil, group)
	if err != nil {
		t.Skipf("skipping mDNS test: multicast unavailable here (%v)", err)
	}
	_ = conn.Close()
}

// freePort asks the kernel for an unused TCP port.
func freePort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer func() { _ = l.Close() }()
	return l.Addr().(*net.TCPAddr).Port
}

// freeUDPPort asks the kernel for an unused UDP port.
func freeUDPPort(t *testing.T) int {
	t.Helper()
	conn, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	require.NoError(t, err)
	port := conn.LocalAddr().(*net.UDPAddr).Port
	require.NoError(t, conn.Close())
	return port
}

// startProcess runs the binary with env additions and returns a cleanup
// handle plus the captured output (for failure messages).
func startProcess(t *testing.T, bin string, args []string, extraEnv []string) *strings.Builder {
	t.Helper()
	cmd := exec.Command(bin, args...)
	cmd.Env = append(os.Environ(), extraEnv...)
	output := &strings.Builder{}
	cmd.Stdout = output
	cmd.Stderr = output
	require.NoError(t, cmd.Start(), "process start failed")
	t.Cleanup(func() {
		_ = cmd.Process.Kill()
		_, _ = cmd.Process.Wait()
	})
	return output
}

// waitForHealth polls the server's health endpoint.
func waitForHealth(t *testing.T, url string) {
	t.Helper()
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		resp, err := http.Get(url + "/api/v1/health")
		if err == nil {
			defer func() { _ = resp.Body.Close() }()
			if resp.StatusCode == http.StatusOK {
				return
			}
		}
		time.Sleep(200 * time.Millisecond)
	}
	t.Fatal("server did not become healthy")
}

// apiCall performs a JSON API call and decodes the reply when out is set.
func apiCall(t *testing.T, method, url string, body, out any) {
	t.Helper()
	var reader io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		require.NoError(t, err)
		reader = strings.NewReader(string(raw))
	}
	req, err := http.NewRequest(method, url, reader)
	require.NoError(t, err)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	raw, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Less(t, resp.StatusCode, 300, "%s %s: %s", method, url, raw)
	if out != nil {
		require.NoError(t, json.Unmarshal(raw, out), "%s %s", method, url)
	}
}

// runJoinScenario is one discovery variant end to end.
func runJoinScenario(t *testing.T, bin, ffmpeg, ffprobe string, toggles discoveryToggles) {
	t.Helper()
	port := freePort(t)
	base := fmt.Sprintf("http://127.0.0.1:%d", port)
	// A private broadcast port: nothing else on this machine (leftover
	// manual runs, other test binaries) may answer the probes.
	udpPort := freeUDPPort(t)
	toggles.port = udpPort

	serverDir := t.TempDir()
	workerDir := t.TempDir()
	libDir := t.TempDir()

	var serverLog, workerLog *strings.Builder
	defer func() {
		if t.Failed() {
			if serverLog != nil {
				t.Logf("=== server log ===\n%s", serverLog.String())
			}
			if workerLog != nil {
				t.Logf("=== worker log ===\n%s", workerLog.String())
			}
		}
	}()

	// A wasteful h264 source so a quality-1 hevc encode is clearly
	// smaller (the same trick the worker e2e test uses).
	src := makeFatH264(t, ffmpeg)
	dst := filepath.Join(libDir, "movie.mkv")
	data, err := os.ReadFile(src)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(dst, data, 0o600))
	origSize := len(data)

	// The server: role server (no built-in worker racing the remote one),
	// announcing itself on the LAN with the given transports.
	serverLog = startProcess(t, bin, []string{"serve"}, append([]string{
		"SHRINK_DATA_DIR=" + serverDir,
		"SHRINK_PORT=" + fmt.Sprint(port),
		"SHRINK_ROLE=server",
		"SHRINK_NODE_NAME=server-a",
		"SHRINK_FFMPEG_PATH=" + ffmpeg,
	}, toggles.env()...))
	waitForHealth(t, base)

	// Admin mints a one-time token for the worker.
	var tokenResp struct {
		Token string `json:"token"`
	}
	apiCall(t, http.MethodPost, base+"/api/v1/nodes/tokens", nil, &tokenResp)
	require.NotEmpty(t, tokenResp.Token)

	// The library with a quality-1 hevc target.
	target := map[string]any{
		"video":     map[string]any{"codec": "hevc", "quality": 1, "reencode_if_same_codec": false},
		"audio":     map[string]any{"mode": "keep_all", "preferred_languages": []string{"eng"}, "compact_codec": "aac"},
		"subtitles": map[string]any{"mode": "keep_all", "preferred_languages": []string{"eng"}, "extract_removed": false},
		"container": "mkv",
	}
	var lib struct {
		ID string `json:"id"`
	}
	apiCall(t, http.MethodPost, base+"/api/v1/libraries", map[string]any{
		"name": "Movies", "path": libDir, "target": target, "watch": false,
		"result_policy": "replace_if_smaller",
	}, &lib)
	apiCall(t, http.MethodPost, base+"/api/v1/libraries/"+lib.ID+"/scan", nil, nil)

	// The worker: no SHRINK_SERVER — it must find the server via the
	// enabled transports, pair with the token and start working.
	workerLog = startProcess(t, bin, []string{"worker"}, append([]string{
		"SHRINK_DATA_DIR=" + workerDir,
		"SHRINK_NODE_NAME=worker-b",
		"SHRINK_TOKEN=" + tokenResp.Token,
		"SHRINK_FFMPEG_PATH=" + ffmpeg,
	}, toggles.env()...))

	// The worker appears as an approved node (token pairing). Pairing
	// alone starts nothing (task 11): the admin starts the node, and the
	// running worker picks that up from its next heartbeat.
	var startedNodeID string
	waitFor(t, "worker-b approved", func() bool {
		var nodes struct {
			Items []struct {
				ID     string `json:"id"`
				Name   string `json:"name"`
				Status string `json:"status"`
			} `json:"items"`
		}
		apiCall(t, http.MethodGet, base+"/api/v1/nodes", nil, &nodes)
		for _, n := range nodes.Items {
			if n.Name == "worker-b" && n.Status == "approved" {
				startedNodeID = n.ID
				return true
			}
		}
		return false
	}, 60*time.Second)
	apiCall(t, http.MethodPost, base+"/api/v1/nodes/"+startedNodeID+"/start", nil, nil)

	waitFor(t, "job done", func() bool {
		var jobs struct {
			Items []struct {
				Status string `json:"status"`
			} `json:"items"`
			Total int `json:"total"`
		}
		apiCall(t, http.MethodGet, base+"/api/v1/jobs", nil, &jobs)
		for _, j := range jobs.Items {
			if j.Status == "done" {
				return true
			}
		}
		return false
	}, 90*time.Second)

	// The output replaced the original: same path, hevc, smaller.
	info := probeOutput(t, ffprobe, dst)
	require.Equal(t, "hevc", info, "the file must be re-encoded to hevc")
	st, err := os.Stat(dst)
	require.NoError(t, err)
	require.Less(t, st.Size(), int64(origSize), "output must be smaller")

	// The worker process got through pairing (its key file exists).
	_, err = os.Stat(filepath.Join(workerDir, "node.key"))
	require.NoError(t, err)
}

// waitFor polls until cond holds or the timeout passes.
func waitFor(t *testing.T, what string, cond func() bool, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(300 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

// probeOutput returns the video codec of a file via ffprobe.
func probeOutput(t *testing.T, ffprobe, path string) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	out, err := exec.CommandContext(ctx, ffprobe, "-v", "error",
		"-select_streams", "v:0", "-show_entries", "stream=codec_name",
		"-of", "json", path).Output()
	require.NoError(t, err)
	var parsed struct {
		Streams []struct {
			CodecName string `json:"codec_name"`
		} `json:"streams"`
	}
	require.NoError(t, json.Unmarshal(out, &parsed))
	require.NotEmpty(t, parsed.Streams)
	return parsed.Streams[0].CodecName
}

// makeFatH264 builds a deliberately wasteful h264 file (crf 1).
func makeFatH264(t *testing.T, ffmpeg string) string {
	t.Helper()
	out := filepath.Join(t.TempDir(), "fat.mkv")
	cmd := exec.Command(ffmpeg, "-hide_banner", "-loglevel", "error", "-y",
		"-f", "lavfi", "-i", "testsrc=duration=2:size=160x120:rate=10",
		"-f", "lavfi", "-i", "sine=duration=2",
		"-map", "0:v", "-map", "1:a",
		"-c:v", "libx264", "-preset", "ultrafast", "-crf", "1", "-pix_fmt", "yuv420p",
		"-c:a", "aac", "-b:a", "64k",
		out)
	if outBytes, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("fixture ffmpeg failed: %v\n%s", err, outBytes)
	}
	return out
}
