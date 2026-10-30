// SPDX-License-Identifier: AGPL-3.0-only

package update

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// repo is the release feed of this project (STACK.md release section).
const repo = "repos/PeiceOfToast/shrinkyourshit"

// Channels for the update_channel setting.
const (
	ChannelStable     = "stable"
	ChannelPrerelease = "prerelease"
)

// ChannelValues lists the valid channels.
func ChannelValues() []string {
	return []string{ChannelStable, ChannelPrerelease}
}

// cacheTTL keeps a check free of API calls for six hours; the ETag keeps
// the next actual check free of response bodies (304).
const cacheTTL = 6 * time.Hour

// Asset is one downloadable file of a release.
type Asset struct {
	Name string `json:"name"`
	URL  string `json:"browser_download_url"`
	Size int64  `json:"size"`
}

// Release is the slice of the GitHub release document the updater needs.
type Release struct {
	Tag        string    `json:"tag_name"`
	Name       string    `json:"name"`
	URL        string    `json:"html_url"`
	Notes      string    `json:"body"`
	Prerelease bool      `json:"prerelease"`
	Draft      bool      `json:"draft"`
	Published  time.Time `json:"published_at"`
	Assets     []Asset   `json:"assets"`
}

// cacheFile is the on-disk release cache in the data dir, one file per
// channel so a channel switch never serves the other feed.
type cacheFile struct {
	ETag      string    `json:"etag,omitempty"`
	FetchedAt time.Time `json:"fetched_at"`
	Release   Release   `json:"release"`
}

// cachePath is <data-dir>/update-cache-<channel>.json.
func cachePath(dataDir, channel string) string {
	return filepath.Join(dataDir, "update-cache-"+channel+".json")
}

// ArchiveName is the release archive for goos/goarch, exactly the name
// scripts/release.sh produces (tar.gz for unix, zip for windows).
func ArchiveName(goos, goarch string) string {
	if goos == "windows" {
		return fmt.Sprintf("shrinkyourshit-%s-%s.zip", goos, goarch)
	}
	return fmt.Sprintf("shrinkyourshit-%s-%s.tar.gz", goos, goarch)
}

// SelectAsset finds the archive for goos/goarch among a release's assets.
func SelectAsset(assets []Asset, goos, goarch string) (Asset, error) {
	want := ArchiveName(goos, goarch)
	for _, a := range assets {
		if a.Name == want {
			return a, nil
		}
	}
	return Asset{}, fmt.Errorf("%w: want %s among %d assets", ErrNoAsset, want, len(assets))
}

// ParseChecksums reads sha256sum format ("<64 hex>  <name>") into a
// name-to-digest map. Comment and empty lines are skipped; malformed lines
// are skipped too — verification only ever trusts well-formed entries and
// refuses to continue when the wanted asset is missing from the file.
func ParseChecksums(data string) map[string]string {
	out := make(map[string]string)
	for _, line := range strings.Split(data, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		idx := strings.Index(line, "  ")
		if idx != 64 { // digest is exactly 64 hex characters
			continue
		}
		name := strings.TrimSpace(line[idx+2:])
		if name == "" || !isHex(line[:64]) {
			continue
		}
		out[name] = strings.ToLower(line[:64])
	}
	return out
}

func isHex(s string) bool {
	for _, c := range s {
		switch {
		case c >= '0' && c <= '9', c >= 'a' && c <= 'f', c >= 'A' && c <= 'F':
		default:
			return false
		}
	}
	return true
}

// Latest returns the newest release of the configured channel. It reads
// through an on-disk cache in the data dir: within the 6h TTL no API call
// is made; after it, an If-None-Match request revalidates the cached body
// (304 → cache stays authoritative, so an unchanged feed costs no API
// budget). When the API is unreachable but a cache exists, the cached
// release is served with a warning — a check must degrade, not fail, when
// the network is down.
func Latest(ctx context.Context, opts Options) (Release, error) {
	channel := opts.Channel
	if channel == "" {
		channel = ChannelStable
	}
	switch channel {
	case ChannelStable, ChannelPrerelease:
	default:
		return Release{}, fmt.Errorf("update: invalid channel %q: want stable or prerelease", channel)
	}

	path := cachePath(opts.DataDir, channel)
	cached := readCache(path)
	if cached != nil && opts.now().Sub(cached.FetchedAt) < cacheTTL {
		return cached.Release, nil
	}

	url := opts.apiBase() + "/" + repo + "/releases/latest"
	if channel == ChannelPrerelease {
		url = opts.apiBase() + "/" + repo + "/releases?per_page=20"
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return Release{}, fmt.Errorf("update: request: %w", err)
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	if cached != nil && cached.ETag != "" {
		req.Header.Set("If-None-Match", cached.ETag)
	}

	resp, err := opts.httpClient().Do(req)
	if err != nil {
		if cached != nil {
			opts.logger().Warn("update: release check failed, serving cached", "err", err)
			return cached.Release, nil
		}
		return Release{}, fmt.Errorf("update: release check: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	switch resp.StatusCode {
	case http.StatusNotModified:
		// Unchanged: keep the body, refresh the timestamp so the TTL
		// window restarts without spending another API call.
		cached.FetchedAt = opts.now()
		_ = writeCache(path, cached)
		return cached.Release, nil
	case http.StatusOK:
		// fall through to decode
	default:
		if cached != nil {
			opts.logger().Warn("update: release check failed, serving cached",
				"status", resp.StatusCode)
			return cached.Release, nil
		}
		if resp.StatusCode == http.StatusNotFound {
			// GitHub answers 404 for releases/latest when no non-prerelease
			// release exists (yet).
			return Release{}, fmt.Errorf("update: no published release on the %s channel", channel)
		}
		return Release{}, fmt.Errorf("update: release check: unexpected status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		if cached != nil {
			return cached.Release, nil
		}
		return Release{}, fmt.Errorf("update: release check: %w", err)
	}

	rel, err := decodeRelease(channel, body)
	if err != nil {
		return Release{}, err
	}
	entry := &cacheFile{
		ETag:      resp.Header.Get("ETag"),
		FetchedAt: opts.now(),
		Release:   rel,
	}
	_ = writeCache(path, entry)
	return entry.Release, nil
}

func (o Options) apiBase() string {
	if o.APIBase != "" {
		return o.APIBase
	}
	return "https://api.github.com"
}

// decodeRelease parses the API response: a single document for stable, a
// list for prerelease (newest non-draft first, prereleases included).
func decodeRelease(channel string, body []byte) (Release, error) {
	if channel == ChannelStable {
		var rel Release
		if err := json.Unmarshal(body, &rel); err != nil {
			return Release{}, fmt.Errorf("update: decode release: %w", err)
		}
		if rel.Tag == "" {
			return Release{}, fmt.Errorf("update: release response without a tag")
		}
		return rel, nil
	}
	var list []Release
	if err := json.Unmarshal(body, &list); err != nil {
		return Release{}, fmt.Errorf("update: decode releases: %w", err)
	}
	for _, rel := range list {
		if !rel.Draft && rel.Tag != "" {
			return rel, nil
		}
	}
	return Release{}, fmt.Errorf("update: no published release on the prerelease feed")
}

// readCache loads the cache file, tolerating absence and corruption.
func readCache(path string) *cacheFile {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var c cacheFile
	if err := json.Unmarshal(data, &c); err != nil {
		return nil
	}
	return &c
}

// writeCache persists the cache atomically; a failed write only costs
// cache hits, so the error is deliberately ignored by callers.
func writeCache(path string, c *cacheFile) error {
	data, err := json.Marshal(c)
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
