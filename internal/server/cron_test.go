// SPDX-License-Identifier: AGPL-3.0-only

package server

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func mustSchedule(t *testing.T, expr string) *Schedule {
	t.Helper()
	s, err := ParseSchedule(expr)
	require.NoError(t, err)
	return s
}

func TestParseScheduleValid(t *testing.T) {
	for _, expr := range []string{
		"* * * * *",
		"0 3 * * *",
		"*/5 * * * *",
		"0 3 * * 1-5",
		"0 3 * * 0,6",
		"0 3 * * 7",
		"15,45 2,22 1,15 * 0-4/2",
		"30 1-5/2 * 3 *",
	} {
		_, err := ParseSchedule(expr)
		assert.NoError(t, err, expr)
	}
}

func TestParseScheduleInvalid(t *testing.T) {
	for _, expr := range []string{
		"",          // empty
		"* * * *",   // too few fields
		"* * * * * *", // too many fields
		"60 * * * *",  // minute out of range
		"* 24 * * *",  // hour out of range
		"0 3 0 * *",   // dom out of range
		"0 3 * 13 *",  // month out of range
		"0 3 * * 8",   // dow out of range (7 ok, 8 not)
		"*/0 * * * *", // zero step
		"a * * * *",   // non-numeric
		"1-  * * * *", // broken range
	} {
		_, err := ParseSchedule(expr)
		assert.Error(t, err, expr)
	}
}

func TestScheduleMatch(t *testing.T) {
	// Local time, so DST quirks do not leak into the assertions.
	loc := time.FixedZone("test", 0)

	assert.True(t, mustSchedule(t, "* * * * *").Match(time.Date(2026, 1, 1, 5, 7, 30, 0, loc)))

	s := mustSchedule(t, "30 3 * * *")
	assert.True(t, s.Match(time.Date(2026, 1, 1, 3, 30, 0, 0, loc)))
	assert.False(t, s.Match(time.Date(2026, 1, 1, 3, 31, 0, 0, loc)))
	assert.False(t, s.Match(time.Date(2026, 1, 1, 4, 30, 0, 0, loc)))

	// Weekdays only.
	wd := mustSchedule(t, "0 3 * * 1-5")
	assert.True(t, wd.Match(time.Date(2026, 1, 5, 3, 0, 0, 0, loc)))  // Monday
	assert.False(t, wd.Match(time.Date(2026, 1, 10, 3, 0, 0, 0, loc))) // Saturday

	// 7 means Sunday, same as 0.
	sun := mustSchedule(t, "0 3 * * 7")
	assert.True(t, sun.Match(time.Date(2026, 1, 4, 3, 0, 0, 0, loc))) // Sunday
	assert.False(t, sun.Match(time.Date(2026, 1, 5, 3, 0, 0, 0, loc)))

	// Both dom and dow restricted: either fires (vixie rule).
	both := mustSchedule(t, "0 3 1 * 0")
	assert.True(t, both.Match(time.Date(2026, 2, 1, 3, 0, 0, 0, loc)))  // first of month
	assert.True(t, both.Match(time.Date(2026, 2, 8, 3, 0, 0, 0, loc)))  // Sunday
	assert.False(t, both.Match(time.Date(2026, 2, 2, 3, 0, 0, 0, loc))) // Monday, not the 1st
}

func TestScheduleNext(t *testing.T) {
	loc := time.FixedZone("test", 0)
	s := mustSchedule(t, "30 3 * * *")
	from := time.Date(2026, 1, 1, 4, 0, 0, 0, loc)
	next, err := s.Next(from)
	require.NoError(t, err)
	assert.Equal(t, time.Date(2026, 1, 2, 3, 30, 0, 0, loc), next)

	// Same minute is skipped: Next is strictly after.
	sameMinute := time.Date(2026, 1, 2, 3, 30, 10, 0, loc)
	next, err = s.Next(sameMinute)
	require.NoError(t, err)
	assert.Equal(t, time.Date(2026, 1, 3, 3, 30, 0, 0, loc), next)
}
