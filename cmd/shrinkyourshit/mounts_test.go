// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/PeiceOfToast/shrinkyourshit/internal/mount"
	"github.com/PeiceOfToast/shrinkyourshit/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The host gate reads storage_allowed_hosts from the real store, so a host
// removed from the setting is refused on the next mount-side call with no
// restart.
func TestStorageHostGateReadsTheSettingEveryCall(t *testing.T) {
	st, err := store.New(filepath.Join(t.TempDir(), "shrink.db"))
	require.NoError(t, err)
	defer func() { _ = st.Close() }()
	ctx := context.Background()
	gate := storageHostGate(st)
	row := &store.Storage{Name: "media", Protocol: "nfs", Source: "nas.lan:/export"}

	// Unset means any host may be mounted.
	assert.NoError(t, gate(ctx, row))

	require.NoError(t, st.SetSetting(ctx, store.SettingStorageAllowedHosts, "nas.lan, 10.0.0.0/24"))
	assert.NoError(t, gate(ctx, row))

	// The host is taken off the list while the row still exists.
	require.NoError(t, st.SetSetting(ctx, store.SettingStorageAllowedHosts, "other.lan"))
	err = gate(ctx, row)
	require.Error(t, err)
	assert.Contains(t, err.Error(), `"nas.lan"`)
}

// An invalid entry in the setting refuses every host and names the entry.
func TestStorageHostGateFailsClosedOnBadEntry(t *testing.T) {
	st, err := store.New(filepath.Join(t.TempDir(), "shrink.db"))
	require.NoError(t, err)
	defer func() { _ = st.Close() }()
	ctx := context.Background()
	require.NoError(t, st.SetSetting(ctx, store.SettingStorageAllowedHosts, "10.0.0.0/99"))
	err = storageHostGate(st)(ctx, &store.Storage{Name: "media", Protocol: "nfs", Source: "nas:/export"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "10.0.0.0/99")
}

// The adapter keeps the executor's answers, converted to the API's status type.
func TestMountAdapterStatusConvertsTheState(t *testing.T) {
	st, err := store.New(filepath.Join(t.TempDir(), "shrink.db"))
	require.NoError(t, err)
	defer func() { _ = st.Close() }()
	exec := mount.New(st, mount.Options{ProcMounts: filepath.Join(t.TempDir(), "mounts")})
	a := mountAdapter{exec}
	state := a.Status(&store.Storage{ID: "st-1", Name: "media", Protocol: "nfs", Source: "nas:/x"})
	assert.False(t, state.Mounted)
	assert.Equal(t, "", state.Error)
}
