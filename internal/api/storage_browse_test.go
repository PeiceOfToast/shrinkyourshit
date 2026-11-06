// SPDX-License-Identifier: AGPL-3.0-only

package api

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/PeiceOfToast/shrinkyourshit/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// browseFixture makes a root with one mounted storage "nas" holding folders
// and a file, plus a sibling folder that shares the name prefix ("nas2") and a
// folder outside the root. The returned mounter has the storage marked mounted.
func browseFixture(t *testing.T) (root string, outside string, m *fakeMounter) {
	t.Helper()
	base := t.TempDir()
	root = filepath.Join(base, "storage")
	outside = filepath.Join(base, "outside")
	for _, d := range []string{
		filepath.Join(root, "nas", "Movies", "2026"),
		filepath.Join(root, "nas", "TV"),
		filepath.Join(root, "nas2"),
		filepath.Join(root, "nas", ".hidden"),
		outside,
	} {
		require.NoError(t, os.MkdirAll(d, 0o755))
	}
	require.NoError(t, os.WriteFile(filepath.Join(root, "nas", "notes.txt"), []byte("secret"), 0o644))
	require.NoError(t, os.Symlink(outside, filepath.Join(root, "nas", "escape")))

	return root, outside, newFakeMounter()
}

// browseHarness builds the API over the fixture, with login on and the "nas"
// storage marked mounted. It returns the harness, the session, the storage id,
// the root, the outside folder and the mounter.
func browseHarness(t *testing.T) (*apiHarness, *http.Cookie, string, string, string, *fakeMounter) {
	t.Helper()
	root, outside, m := browseFixture(t)
	h := newStorageHarnessRoot(t, m, root)
	sess := loginAs(t, h)
	st := seedStorage(t, h, &store.Storage{Name: "nas"})
	m.mounted[st.ID] = true
	return h, sess, st.ID, root, outside, m
}

type browseBody struct {
	Mounted    bool    `json:"mounted"`
	Mountpoint string  `json:"mountpoint"`
	Path       string  `json:"path"`
	Parent     string  `json:"parent"`
	FreeBytes  *uint64 `json:"free_bytes"`
	TotalBytes *uint64 `json:"total_bytes"`
	Raw        []struct {
		Name  string `json:"name"`
		Path  string `json:"path"`
		IsDir bool   `json:"is_dir"`
	} `json:"entries"`
}

func decodeBrowse(t *testing.T, raw string) browseBody {
	t.Helper()
	var b browseBody
	require.NoError(t, json.Unmarshal([]byte(raw), &b), raw)
	return b
}

func entryNames(b browseBody) []string {
	names := make([]string, 0, len(b.Raw))
	for _, e := range b.Raw {
		names = append(names, e.Name)
	}
	return names
}

// The listing has directories only, no file names, and dot folders come back
// for the UI to hide. The root reports no parent.
func TestBrowseListsDirectoriesOnly(t *testing.T) {
	h, sess, id, root, _, _ := browseHarness(t)

	rec := h.doWith(t, sess, http.MethodGet, "/api/v1/storages/"+id+"/browse", nil)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	b := decodeBrowse(t, rec.Body.String())

	assert.True(t, b.Mounted)
	assert.Equal(t, filepath.Join(root, "nas"), b.Mountpoint)
	assert.Equal(t, filepath.Join(root, "nas"), b.Path)
	assert.Empty(t, b.Parent, "the mountpoint has no parent inside the storage")
	assert.ElementsMatch(t, []string{"Movies", "TV", ".hidden"}, entryNames(b), "a symlink leaving the share is not listed")
	for _, e := range b.Raw {
		assert.True(t, e.IsDir, e.Name)
	}
	assert.NotContains(t, rec.Body.String(), "notes.txt", "file names are never listed")
	require.NotNil(t, b.FreeBytes, "free space is reported on this platform")
	require.NotNil(t, b.TotalBytes)
	assert.Greater(t, *b.TotalBytes, uint64(0))
}

// Walking into a folder lists its own folders and names the parent.
func TestBrowseWalksIntoAFolder(t *testing.T) {
	h, sess, id, root, _, _ := browseHarness(t)
	target := filepath.Join(root, "nas", "Movies")

	rec := h.doWith(t, sess, http.MethodGet, "/api/v1/storages/"+id+"/browse?path="+target, nil)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	b := decodeBrowse(t, rec.Body.String())
	assert.Equal(t, target, b.Path)
	assert.Equal(t, filepath.Join(root, "nas"), b.Parent)
	assert.Equal(t, []string{"2026"}, entryNames(b))
}

// A path outside the storage is refused, including a dot dot walk out, a
// sibling that shares the name prefix, and a symlink that leaves the share.
func TestBrowseRefusesPathsOutsideTheStorage(t *testing.T) {
	h, sess, id, root, outside, _ := browseHarness(t)
	cases := map[string]string{
		"outside the root":    outside,
		"dot dot walk out":    root + "/nas/../nas2",
		"sibling name prefix": filepath.Join(root, "nas2"),
		"symlink leaving":     filepath.Join(root, "nas", "escape"),
		"relative path":       "nas/Movies",
	}
	for name, p := range cases {
		rec := h.doWith(t, sess, http.MethodGet, "/api/v1/storages/"+id+"/browse?path="+p, nil)
		assert.Equal(t, http.StatusBadRequest, rec.Code, name+": "+rec.Body.String())
		assert.NotContains(t, rec.Body.String(), `"entries"`, name)
	}
}

// A folder the server cannot read is an error with code unreadable, not an
// empty listing. Root can read anything, so the test skips when run as root.
func TestBrowseUnreadableFolderIsAnError(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("running as root: permissions do not stop the server, so there is no unreadable folder to test")
	}
	h, sess, id, root, _, _ := browseHarness(t)
	locked := filepath.Join(root, "nas", "Locked")
	require.NoError(t, os.MkdirAll(filepath.Join(locked, "inside"), 0o755))
	require.NoError(t, os.Chmod(locked, 0o000))
	t.Cleanup(func() { _ = os.Chmod(locked, 0o755) })

	rec := h.doWith(t, sess, http.MethodGet, "/api/v1/storages/"+id+"/browse?path="+locked, nil)
	require.Equal(t, http.StatusForbidden, rec.Code, rec.Body.String())
	var body struct {
		Code    string `json:"code"`
		Message string `json:"error"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.Equal(t, "unreadable", body.Code)
	assert.NotContains(t, rec.Body.String(), `"entries"`)
}

// A folder that does not exist is 404, and a storage the mounter reports as
// not mounted is 409, both before any listing.
func TestBrowseMissingFolderAndUnmountedStorage(t *testing.T) {
	h, sess, id, root, _, m := browseHarness(t)

	rec := h.doWith(t, sess, http.MethodGet, "/api/v1/storages/"+id+"/browse?path="+filepath.Join(root, "nas", "Nope"), nil)
	require.Equal(t, http.StatusNotFound, rec.Code, rec.Body.String())
	assert.Contains(t, rec.Body.String(), "not_found")

	// Unmount the fake and browse again.
	m.mu.Lock()
	delete(m.mounted, id)
	m.mu.Unlock()
	rec = h.doWith(t, sess, http.MethodGet, "/api/v1/storages/"+id+"/browse", nil)
	require.Equal(t, http.StatusConflict, rec.Code, rec.Body.String())
	assert.Contains(t, rec.Body.String(), "not_mounted")
}

// With login off the browse is refused before it reads anything.
func TestBrowseNeedsLoginOn(t *testing.T) {
	root, _, m := browseFixture(t)
	h := newStorageHarnessRoot(t, m, root)
	st := seedStorage(t, h, &store.Storage{Name: "nas"})
	m.mounted[st.ID] = true

	rec := h.do(http.MethodGet, "/api/v1/storages/"+st.ID+"/browse", nil)
	require.Equal(t, http.StatusForbidden, rec.Code, rec.Body.String())
	assert.Contains(t, rec.Body.String(), "login_required")
}
