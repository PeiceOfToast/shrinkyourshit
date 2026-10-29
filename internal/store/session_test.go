// SPDX-License-Identifier: AGPL-3.0-only

package store

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestSessionsLifecycle(t *testing.T) {
	ctx := context.Background()
	st, err := New(t.TempDir() + "/test.db")
	require.NoError(t, err)
	t.Cleanup(func() { _ = st.Close() })

	sess := &Session{TokenHash: "hash1", ExpiresAt: time.Now().Add(time.Hour)}
	require.NoError(t, st.CreateSession(ctx, sess))
	require.False(t, sess.CreatedAt.IsZero(), "created_at filled")

	got, err := st.GetSessionByHash(ctx, "hash1")
	require.NoError(t, err)
	require.Equal(t, "hash1", got.TokenHash)
	require.WithinDuration(t, sess.ExpiresAt.UTC(), got.ExpiresAt.UTC(), time.Second)

	// Unknown hash.
	_, err = st.GetSessionByHash(ctx, "nope")
	require.ErrorIs(t, err, ErrNotFound)

	// Logout deletes only the target session.
	require.NoError(t, st.CreateSession(ctx, &Session{TokenHash: "hash2", ExpiresAt: time.Now().Add(time.Hour)}))
	require.NoError(t, st.DeleteSession(ctx, "hash1"))
	_, err = st.GetSessionByHash(ctx, "hash1")
	require.ErrorIs(t, err, ErrNotFound)
	_, err = st.GetSessionByHash(ctx, "hash2")
	require.NoError(t, err)

	// Validation.
	require.Error(t, st.CreateSession(ctx, &Session{ExpiresAt: time.Now().Add(time.Hour)}))
	require.Error(t, st.CreateSession(ctx, &Session{TokenHash: "x"}))
}

func TestDeleteExpiredSessions(t *testing.T) {
	ctx := context.Background()
	st, err := New(t.TempDir() + "/test.db")
	require.NoError(t, err)
	t.Cleanup(func() { _ = st.Close() })

	now := time.Now()
	require.NoError(t, st.CreateSession(ctx, &Session{TokenHash: "old", ExpiresAt: now.Add(-time.Minute)}))
	require.NoError(t, st.CreateSession(ctx, &Session{TokenHash: "live", ExpiresAt: now.Add(time.Hour)}))

	n, err := st.DeleteExpiredSessions(ctx, now)
	require.NoError(t, err)
	require.EqualValues(t, 1, n)
	_, err = st.GetSessionByHash(ctx, "old")
	require.ErrorIs(t, err, ErrNotFound)
	_, err = st.GetSessionByHash(ctx, "live")
	require.NoError(t, err)
}
