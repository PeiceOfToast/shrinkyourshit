// SPDX-License-Identifier: AGPL-3.0-only

package update_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/PeiceOfToast/shrinkyourshit/internal/update"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestAssetSelection pins the archive names the updater picks for the five
// release targets; they must equal what scripts/release.sh packages
// (TestReleaseArtifacts checks the other side of that contract).
func TestAssetSelection(t *testing.T) {
	targets := []struct{ goos, goarch, want string }{
		{"linux", "amd64", "shrinkyourshit-linux-amd64.tar.gz"},
		{"linux", "arm64", "shrinkyourshit-linux-arm64.tar.gz"},
		{"windows", "amd64", "shrinkyourshit-windows-amd64.zip"},
		{"darwin", "amd64", "shrinkyourshit-darwin-amd64.tar.gz"},
		{"darwin", "arm64", "shrinkyourshit-darwin-arm64.tar.gz"},
	}
	for _, tt := range targets {
		assert.Equal(t, tt.want, update.ArchiveName(tt.goos, tt.goarch), "archive name for %s/%s", tt.goos, tt.goarch)
	}

	assets := []update.Asset{
		{Name: "checksums.txt"},
		{Name: "shrinkyourshit-linux-arm64.tar.gz"},
		{Name: "shrinkyourshit-linux-amd64.tar.gz"},
		{Name: "shrinkyourshit-windows-amd64.zip"},
	}
	got, err := update.SelectAsset(assets, "linux", "amd64")
	require.NoError(t, err)
	assert.Equal(t, "shrinkyourshit-linux-amd64.tar.gz", got.Name)

	_, err = update.SelectAsset(assets, "darwin", "arm64")
	assert.ErrorIs(t, err, update.ErrNoAsset)
}

func TestParseChecksums(t *testing.T) {
	sums := update.ParseChecksums("" +
		"# comment line\n" +
		"e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855  shrinkyourshit-linux-amd64.tar.gz\n" +
		"0e4ee2c2a7c5785b98cb0eda955820d5c6c4cad78cd53e4ca3857cb772fcfa8f  shrinkyourshit-windows-amd64.zip\n" +
		"\n" +
		"not-a-digest  broken.txt\n" +
		"short  tooshort.txt\n")
	assert.Len(t, sums, 2)
	assert.Equal(t, "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855", sums["shrinkyourshit-linux-amd64.tar.gz"])
	assert.Equal(t, "0e4ee2c2a7c5785b98cb0eda955820d5c6c4cad78cd53e4ca3857cb772fcfa8f", sums["shrinkyourshit-windows-amd64.zip"])
	assert.NotContains(t, sums, "broken.txt")
	assert.NotContains(t, sums, "tooshort.txt")
}

// fakeHub is a GitHub API shaped test server: it serves one release
// document and the assets the updater downloads from it.
type fakeHub struct {
	t            *testing.T
	server       *httptest.Server
	tag          string
	etag         string // current ETag; empty = no conditional handling
	request      int
	bin          string // content of the binary inside the archive
	wrong        bool   // publish a checksum that does not match the archive
	checksumBody string // when set, replaces the generated checksums.txt
}

func newFakeHub(t *testing.T, tag, bin string) *fakeHub {
	t.Helper()
	hub := &fakeHub{t: t, tag: tag, bin: bin, etag: `"r1"`}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /repos/PeiceOfToast/shrinkyourshit/releases/latest", hub.latest)
	mux.HandleFunc("GET /repos/PeiceOfToast/shrinkyourshit/releases", hub.list)
	mux.HandleFunc("GET /download/{name}", hub.download)
	hub.server = httptest.NewServer(mux)
	t.Cleanup(hub.server.Close)
	return hub
}

// archiveBytes builds the tar.gz release archive: one entry holding the
// binary, named like the workflow names it.
func (h *fakeHub) archiveBytes() []byte {
	return tarGzBytes(h.t, "shrinkyourshit-linux-amd64", []byte(h.bin))
}

func (h *fakeHub) asset(name string) update.Asset {
	return update.Asset{
		Name: name,
		URL:  h.server.URL + "/download/" + name,
		Size: 1,
	}
}

func (h *fakeHub) release() update.Release {
	return update.Release{
		Tag:       h.tag,
		Name:      h.tag,
		URL:       h.server.URL + "/releases/tag/" + h.tag,
		Published: time.Date(2026, 10, 30, 0, 0, 0, 0, time.UTC),
		Assets: []update.Asset{
			h.asset("checksums.txt"),
			h.asset(update.ArchiveName("linux", "amd64")),
			h.asset(update.ArchiveName("linux", "arm64")),
			h.asset(update.ArchiveName("windows", "amd64")),
		},
	}
}

func (h *fakeHub) latest(w http.ResponseWriter, r *http.Request) {
	h.request++
	if r.Header.Get("If-None-Match") == h.etag && h.etag != "" {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	w.Header().Set("ETag", h.etag)
	writeJSONBody(w, h.release())
}

func (h *fakeHub) list(w http.ResponseWriter, r *http.Request) {
	h.request++
	rel := h.release()
	older := rel
	older.Tag = "v0.9.0"
	older.Draft = false
	draft := rel
	draft.Tag = "v9.9.9"
	draft.Draft = true
	writeJSONBody(w, []update.Release{draft, rel, older})
}

func (h *fakeHub) download(w http.ResponseWriter, r *http.Request) {
	switch r.PathValue("name") {
	case "checksums.txt":
		if h.checksumBody != "" {
			_, _ = w.Write([]byte(h.checksumBody))
			return
		}
		sum := sha256Hex(h.t, h.archiveBytes())
		if h.wrong {
			sum = "0000000000000000000000000000000000000000000000000000000000000000"
		}
		_, _ = w.Write([]byte(sum + "  " + update.ArchiveName("linux", "amd64") + "\n"))
	case update.ArchiveName("linux", "amd64"):
		_, _ = w.Write(h.archiveBytes())
	default:
		http.NotFound(w, r)
	}
}

func TestLatestCachesWithETag(t *testing.T) {
	hub := newFakeHub(t, "v1.2.3", "bin")
	now := time.Date(2026, 10, 30, 12, 0, 0, 0, time.UTC)
	opts := update.Options{
		DataDir: t.TempDir(),
		APIBase: hub.server.URL,
		Now:     func() time.Time { return now },
	}

	first, err := update.Latest(context.Background(), opts)
	require.NoError(t, err)
	assert.Equal(t, "v1.2.3", first.Tag)
	assert.Equal(t, 1, hub.request, "first check must hit the API once")

	// Inside the TTL: no API call.
	again, err := update.Latest(context.Background(), opts)
	require.NoError(t, err)
	assert.Equal(t, "v1.2.3", again.Tag)
	assert.Equal(t, 1, hub.request)

	// Past the TTL with an unchanged feed: one revalidation request that
	// answers 304, cached body reused.
	now = now.Add(7 * time.Hour)
	third, err := update.Latest(context.Background(), opts)
	require.NoError(t, err)
	assert.Equal(t, "v1.2.3", third.Tag)
	assert.Equal(t, 2, hub.request)

	// The refreshed timestamp restarts the TTL window.
	fourth, err := update.Latest(context.Background(), opts)
	require.NoError(t, err)
	assert.Equal(t, "v1.2.3", fourth.Tag)
	assert.Equal(t, 2, hub.request)
}

func TestLatestServesCacheWhenAPIIsDown(t *testing.T) {
	hub := newFakeHub(t, "v1.2.3", "bin")
	now := time.Date(2026, 10, 30, 12, 0, 0, 0, time.UTC)
	opts := update.Options{DataDir: t.TempDir(), APIBase: hub.server.URL, Now: func() time.Time { return now }}
	_, err := update.Latest(context.Background(), opts)
	require.NoError(t, err)

	// Past the TTL with the API unreachable: the cached release is served
	// instead of failing the check.
	hub.server.Close()
	now = now.Add(7 * time.Hour)
	rel, err := update.Latest(context.Background(), opts)
	require.NoError(t, err, "a cached check must degrade, not fail, when offline")
	assert.Equal(t, "v1.2.3", rel.Tag)
}

func TestLatestPrereleaseChannel(t *testing.T) {
	hub := newFakeHub(t, "v1.3.0-rc1", "bin")
	opts := update.Options{DataDir: t.TempDir(), APIBase: hub.server.URL, Channel: update.ChannelPrerelease}
	rel, err := update.Latest(context.Background(), opts)
	require.NoError(t, err)
	// The draft and the older stable are skipped; the newest published
	// entry wins whatever its prerelease flag says.
	assert.Equal(t, "v1.3.0-rc1", rel.Tag)
}

func TestLatestRejectsUnknownChannel(t *testing.T) {
	_, err := update.Latest(context.Background(), update.Options{
		DataDir: t.TempDir(),
		APIBase: "http://127.0.0.1:1",
		Channel: "beta",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid channel")
}

// TestRuntimeDetection pins the container detection used by the version
// endpoint and the updater's refusal.
func TestRuntimeDetection(t *testing.T) {
	t.Setenv("SHRINK_IN_CONTAINER", "")
	assert.Equal(t, update.RuntimeBinary, update.Runtime())
	t.Setenv("SHRINK_IN_CONTAINER", "1")
	assert.Equal(t, update.RuntimeContainer, update.Runtime())
}

// --- shared test helpers -----------------------------------------------------

func writeJSONBody(w http.ResponseWriter, body any) {
	w.Header().Set("Content-Type", "application/json")
	enc := jsonEncoder(w)
	_ = enc(body)
}

func ensureDir(t *testing.T, path string) string {
	t.Helper()
	require.NoError(t, os.MkdirAll(path, 0o755))
	return path
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	return string(data)
}
