// SPDX-License-Identifier: AGPL-3.0-only

//go:build !linux && !darwin

package api

// storageSpace has no probe on this platform. Storages are mounted by the
// container (Linux), so the picker shows no free space here.
func storageSpace(string) (free, total *uint64) { return nil, nil }
