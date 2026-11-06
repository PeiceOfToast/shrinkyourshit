// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"context"
	"fmt"

	"github.com/PeiceOfToast/shrinkyourshit/internal/api"
	"github.com/PeiceOfToast/shrinkyourshit/internal/mount"
	"github.com/PeiceOfToast/shrinkyourshit/internal/store"
)

// mountAdapter lets the mount executor stand in for the API's StorageMounter.
// The two packages each define their own status type, so the one method that
// differs is converted here. Mount, Unmount and Test are promoted unchanged.
type mountAdapter struct {
	*mount.Executor
}

// Status converts the executor's state to the API's shape.
func (a mountAdapter) Status(st *store.Storage) api.StorageState {
	s := a.Executor.Status(st)
	return api.StorageState{Mounted: s.Mounted, Error: s.Error}
}

// storageHostGate applies storage_allowed_hosts before every mount-side action,
// including the startup remount and the watchdog, which do not pass through
// the API handlers.
func storageHostGate(st store.Store) func(context.Context, *store.Storage) error {
	return func(ctx context.Context, row *store.Storage) error {
		raw, _, err := st.GetSetting(ctx, store.SettingStorageAllowedHosts)
		if err != nil {
			return fmt.Errorf("read storage_allowed_hosts: %w", err)
		}
		return api.CheckStorageHost(raw, row.Protocol, row.Source)
	}
}
