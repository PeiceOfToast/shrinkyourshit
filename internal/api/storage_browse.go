// SPDX-License-Identifier: AGPL-3.0-only

package api

import (
	"errors"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// Storage browse (task 69, spec 4.6). The picker lists directories only: no
// file names, no contents, no preview. A browse reads a network share as the
// container's root, so it is gated like the writes (login on), and every path
// is held inside the storage's own mountpoint, symlinks included.

// defaultStorageRoot is where the executor mounts shares: Root/<name> (spec 4.1).
const defaultStorageRoot = "/storage"

// WithStorageRoot sets the directory the storage mountpoints live under. Empty
// keeps the default. Tests point it at a temporary directory.
func WithStorageRoot(dir string) Option {
	return func(a *API) {
		if dir != "" {
			a.storageRoot = filepath.Clean(dir)
		}
	}
}

// browseEntry is one directory in a listing.
type browseEntry struct {
	Name  string `json:"name"`
	Path  string `json:"path"`
	IsDir bool   `json:"is_dir"`
}

// storageBrowseResponse is GET /storages/{id}/browse (API.md task 69).
type storageBrowseResponse struct {
	ID         string        `json:"id"`
	Name       string        `json:"name"`
	Mounted    bool          `json:"mounted"`
	Mountpoint string        `json:"mountpoint"`
	Path       string        `json:"path"`
	Parent     string        `json:"parent"`
	FreeBytes  *uint64       `json:"free_bytes"`
	TotalBytes *uint64       `json:"total_bytes"`
	Entries    []browseEntry `json:"entries"`
}

// browseStorage answers GET /storages/{id}/browse?path=. The path defaults to
// the mountpoint and must sit inside it. Only directories are listed.
func (a *API) browseStorage(w http.ResponseWriter, r *http.Request) {
	if !a.storageWriteAllowed(w, r) {
		return
	}
	st, ok := a.storedStorage(w, r)
	if !ok {
		return
	}
	if !a.mounter.Status(st).Mounted {
		writeError(w, http.StatusConflict, "not_mounted", "the storage is not mounted, so there is nothing to browse yet")
		return
	}

	mount := filepath.Join(a.storageRoot, st.Name)
	realMount, err := filepath.EvalSymlinks(mount)
	if err != nil {
		writeBrowseReadError(w, err, "the storage is not mounted, so there is nothing to browse yet")
		return
	}

	target := mount
	if p := r.URL.Query().Get("path"); p != "" {
		target = filepath.Clean(p)
	}
	if !filepath.IsAbs(target) {
		writeError(w, http.StatusBadRequest, "invalid_request", "path must be absolute")
		return
	}
	if !withinDir(target, mount) {
		writeError(w, http.StatusBadRequest, "outside_storage", "that path is outside this storage")
		return
	}

	resolved, err := filepath.EvalSymlinks(target)
	if err != nil {
		writeBrowseReadError(w, err, "no such folder on this storage")
		return
	}
	if !withinDir(resolved, realMount) {
		writeError(w, http.StatusBadRequest, "outside_storage", "that folder links to a place outside this storage")
		return
	}
	info, err := os.Stat(resolved)
	if err != nil {
		writeBrowseReadError(w, err, "no such folder on this storage")
		return
	}
	if !info.IsDir() {
		writeError(w, http.StatusBadRequest, "invalid_request", "path is not a folder")
		return
	}

	dirents, err := os.ReadDir(resolved)
	if err != nil {
		writeBrowseReadError(w, err, "the folder could not be read")
		return
	}
	entries := make([]browseEntry, 0, len(dirents))
	for _, d := range dirents {
		child := filepath.Join(resolved, d.Name())
		resolvedChild, err := filepath.EvalSymlinks(child)
		if err != nil || !withinDir(resolvedChild, realMount) {
			continue
		}
		if cinfo, err := os.Stat(resolvedChild); err != nil || !cinfo.IsDir() {
			continue
		}
		entries = append(entries, browseEntry{
			Name:  d.Name(),
			Path:  filepath.Join(target, d.Name()),
			IsDir: true,
		})
	}

	parent := ""
	if target != mount {
		parent = filepath.Dir(target)
	}
	free, total := storageSpace(realMount)
	writeJSON(w, http.StatusOK, storageBrowseResponse{
		ID:         st.ID,
		Name:       st.Name,
		Mounted:    true,
		Mountpoint: mount,
		Path:       target,
		Parent:     parent,
		FreeBytes:  free,
		TotalBytes: total,
		Entries:    entries,
	})
}

// withinDir reports whether p is dir itself or lies below it. Both must be
// clean absolute paths. The separator check stops /storage/nas2 matching /storage/nas.
func withinDir(p, dir string) bool {
	if p == dir {
		return true
	}
	return strings.HasPrefix(p, dir+string(filepath.Separator))
}

// writeBrowseReadError maps a filesystem failure to a readable answer. A
// folder the server cannot read is 403 `unreadable`, never an empty list.
func writeBrowseReadError(w http.ResponseWriter, err error, missing string) {
	switch {
	case errors.Is(err, fs.ErrNotExist):
		writeError(w, http.StatusNotFound, "not_found", missing)
	case errors.Is(err, fs.ErrPermission):
		writeError(w, http.StatusForbidden, "unreadable",
			"the server cannot read this folder (permission denied), so its contents are not shown")
	default:
		writeError(w, http.StatusInternalServerError, "unreadable",
			"the server could not read this folder: "+err.Error())
	}
}
