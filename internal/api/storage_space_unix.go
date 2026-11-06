// SPDX-License-Identifier: AGPL-3.0-only

//go:build linux || darwin

package api

import "syscall"

// storageSpace reports the free and total bytes of the filesystem holding
// dir. Either is nil when the probe fails, so the picker shows no figure
// instead of a wrong one.
func storageSpace(dir string) (free, total *uint64) {
	var st syscall.Statfs_t
	if err := syscall.Statfs(dir, &st); err != nil {
		return nil, nil
	}
	bsize := uint64(st.Bsize)
	f := uint64(st.Bavail) * bsize
	t := uint64(st.Blocks) * bsize
	return &f, &t
}
