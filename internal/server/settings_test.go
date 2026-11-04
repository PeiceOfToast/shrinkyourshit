// SPDX-License-Identifier: AGPL-3.0-only

package server

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/PeiceOfToast/shrinkyourshit/internal/flow"
	"github.com/PeiceOfToast/shrinkyourshit/internal/store"
	"github.com/PeiceOfToast/shrinkyourshit/internal/update"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSettingsDefaults(t *testing.T) {
	h := newAuthHarness(t)
	vals, err := NewSettings(h.st, nil).Get(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 1, vals.WorkerConcurrency)
	assert.Equal(t, "", vals.ScratchDir)
	assert.Equal(t, store.ResultPolicyReplaceIfSmaller, vals.DefaultResultPolicy)
	assert.False(t, vals.LoginEnabled)
	assert.Equal(t, []string{"eng"}, vals.PreferredLanguages)
	assert.Equal(t, VideoCodecHEVC, vals.DefaultVideoCodec)
	// Self update (task 16): checks on, stable channel, backups on.
	assert.True(t, vals.UpdateCheck)
	assert.Equal(t, update.ChannelStable, vals.UpdateChannel)
	assert.True(t, vals.BackupBeforeUpdate)
	// QSV opt-ins (task 35): off — the default must stay today's behaviour.
	assert.False(t, vals.QSVExtraQuality)
	assert.False(t, vals.QSVLowPower)
}

// TestSettingsQSVToggles (task 35): the two QSV opt-ins round-trip and can
// be turned off again.
func TestSettingsQSVToggles(t *testing.T) {
	h := newAuthHarness(t)
	ctx := context.Background()
	s := NewSettings(h.st, nil)

	on, off := true, false
	require.NoError(t, s.Update(ctx, h.auth, Update{QSVExtraQuality: &on, QSVLowPower: &on}))
	vals, err := s.Get(ctx)
	require.NoError(t, err)
	assert.True(t, vals.QSVExtraQuality)
	assert.True(t, vals.QSVLowPower)

	require.NoError(t, s.Update(ctx, h.auth, Update{QSVExtraQuality: &off}))
	vals, err = s.Get(ctx)
	require.NoError(t, err)
	assert.False(t, vals.QSVExtraQuality)
	assert.True(t, vals.QSVLowPower)
}

func TestSettingsUpdateValidation(t *testing.T) {
	h := newAuthHarness(t)
	ctx := context.Background()
	s := NewSettings(h.st, nil)

	// Concurrency bounds.
	n := 0
	require.ErrorIs(t, s.Update(ctx, h.auth, Update{WorkerConcurrency: &n}), ErrBadRequest)
	n = 100
	require.ErrorIs(t, s.Update(ctx, h.auth, Update{WorkerConcurrency: &n}), ErrBadRequest)
	n = 2
	require.NoError(t, s.Update(ctx, h.auth, Update{WorkerConcurrency: &n}))

	// Unknown result policy and video codec.
	bad := "explode"
	require.ErrorIs(t, s.Update(ctx, h.auth, Update{DefaultResultPolicy: &bad}), ErrBadRequest)
	require.ErrorIs(t, s.Update(ctx, h.auth, Update{DefaultVideoCodec: &bad}), ErrBadRequest)

	// Valid values round-trip.
	policy := store.ResultPolicyKeepBoth
	codec := VideoCodecAV1
	langs := []string{"eng", "deu"}
	scratch := "/scratch"
	require.NoError(t, s.Update(ctx, h.auth, Update{
		DefaultResultPolicy: &policy, DefaultVideoCodec: &codec,
		PreferredLanguages: &langs, ScratchDir: &scratch,
	}))
	vals, err := s.Get(ctx)
	require.NoError(t, err)
	assert.Equal(t, 2, vals.WorkerConcurrency)
	assert.Equal(t, store.ResultPolicyKeepBoth, vals.DefaultResultPolicy)
	assert.Equal(t, VideoCodecAV1, vals.DefaultVideoCodec)
	assert.Equal(t, []string{"deu", "eng"}, vals.PreferredLanguages)
	assert.Equal(t, "/scratch", vals.ScratchDir)
}

func TestSettingsUpdatePreferences(t *testing.T) {
	h := newAuthHarness(t)
	ctx := context.Background()
	s := NewSettings(h.st, nil)

	// Unknown channel.
	bad := "beta"
	require.ErrorIs(t, s.Update(ctx, h.auth, Update{UpdateChannel: &bad}), ErrBadRequest)

	// Valid round-trip, including turning the toggles off.
	channel := update.ChannelPrerelease
	check, backup := false, false
	require.NoError(t, s.Update(ctx, h.auth, Update{
		UpdateChannel:      &channel,
		UpdateCheck:        &check,
		BackupBeforeUpdate: &backup,
	}))
	vals, err := s.Get(ctx)
	require.NoError(t, err)
	assert.Equal(t, update.ChannelPrerelease, vals.UpdateChannel)
	assert.False(t, vals.UpdateCheck)
	assert.False(t, vals.BackupBeforeUpdate)
}

func TestSettingsLoginToggle(t *testing.T) {
	h := newAuthHarness(t)
	ctx := context.Background()
	s := NewSettings(h.st, nil)
	on, off := true, false

	// Setup with login disabled.
	require.NoError(t, h.auth.CompleteSetup(ctx, SetupParams{Mode: SetupModeServer, LoginEnabled: false}))

	// Turning login on without a password is refused.
	require.ErrorIs(t, s.Update(ctx, h.auth, Update{LoginEnabled: &on, Username: "admin"}), ErrBadRequest)

	// Turning login on with a password works and the account logs in.
	require.NoError(t, s.Update(ctx, h.auth, Update{LoginEnabled: &on, Username: "admin", Password: "secret"}))
	vals, err := s.Get(ctx)
	require.NoError(t, err)
	assert.True(t, vals.LoginEnabled)
	token, err := h.auth.Login(ctx, "admin", "secret")
	require.NoError(t, err)
	_, err = h.auth.Authenticate(ctx, token)
	require.NoError(t, err)

	// Turning login off works.
	require.NoError(t, s.Update(ctx, h.auth, Update{LoginEnabled: &off}))
	vals, err = s.Get(ctx)
	require.NoError(t, err)
	assert.False(t, vals.LoginEnabled)

	// Re-enabling keeps the stored username but needs a password.
	require.ErrorIs(t, s.Update(ctx, h.auth, Update{LoginEnabled: &on}), ErrBadRequest)
	require.NoError(t, s.Update(ctx, h.auth, Update{LoginEnabled: &on, Password: "again"}))
	_, err = h.auth.Login(ctx, "admin", "again")
	require.NoError(t, err)
}

func TestSettingsPasswordChangeRequiresCurrent(t *testing.T) {
	h := newAuthHarness(t)
	ctx := context.Background()
	s := NewSettings(h.st, nil)
	require.NoError(t, h.auth.CompleteSetup(ctx, SetupParams{
		Mode: SetupModeServer, LoginEnabled: true, Username: "admin", Password: "old",
	}))

	// Wrong current password.
	err := s.Update(ctx, h.auth, Update{CurrentPassword: "wrong", Password: "new"})
	require.ErrorIs(t, err, ErrPasswordMismatch)

	// Correct current password.
	require.NoError(t, s.Update(ctx, h.auth, Update{CurrentPassword: "old", Password: "new"}))
	_, err = h.auth.Login(ctx, "admin", "new")
	require.NoError(t, err)
}

// TestSettingsAllowRawArgs (task 58): the raw arguments switch defaults on,
// round-trips off and on, and refuses only graphs that hold a raw_args step.
func TestSettingsAllowRawArgs(t *testing.T) {
	h := newAuthHarness(t)
	ctx := context.Background()
	s := NewSettings(h.st, nil)

	vals, err := s.Get(ctx)
	require.NoError(t, err)
	assert.True(t, vals.AllowRawArgs, "raw arguments are on by default")

	off := false
	require.NoError(t, s.Update(ctx, h.auth, Update{AllowRawArgs: &off}))
	vals, err = s.Get(ctx)
	require.NoError(t, err)
	assert.False(t, vals.AllowRawArgs)

	raw := flow.Graph{Schema: flow.FlowSchema, Nodes: []flow.Step{
		{ID: "probe", Type: flow.StepProbe},
		{ID: "raw", Type: flow.StepRawArgs, Params: json.RawMessage(`{"stage":"output","args":"-preset fast"}`)},
		{ID: "remux", Type: flow.StepRemuxContainer, Params: json.RawMessage(`{"container":"mkv"}`)},
	}, Edges: []flow.Edge{{From: "probe", To: "raw"}, {From: "raw", To: "remux"}}}
	refused, err := s.RefusedRawArgs(ctx, raw)
	require.NoError(t, err)
	require.Len(t, refused, 1)
	assert.Equal(t, "nodes[1].type", refused[0].Path)

	plain := flow.Graph{Schema: flow.FlowSchema, Nodes: []flow.Step{{ID: "probe", Type: flow.StepProbe}}}
	refused, err = s.RefusedRawArgs(ctx, plain)
	require.NoError(t, err)
	assert.Empty(t, refused, "a graph without the step is never refused by the switch")

	on := true
	require.NoError(t, s.Update(ctx, h.auth, Update{AllowRawArgs: &on}))
	refused, err = s.RefusedRawArgs(ctx, raw)
	require.NoError(t, err)
	assert.Empty(t, refused)
}
