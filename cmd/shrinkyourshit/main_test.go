// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"bytes"
	"errors"
	"testing"

	"github.com/PeiceOfToast/shrinkyourshit/internal/update"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestVersionOutput pins `shrinkyourshit version`: it prints the version,
// commit and build date injected at release build time, plus the runtime
// (binary or container). API.md task 16 and the task file both require
// exactly this.
func TestVersionOutput(t *testing.T) {
	var buf bytes.Buffer
	info := buildInfo{version: "v1.2.3", commit: "abc1234", date: "2026-01-02T03:04:05Z"}
	require.NoError(t, run([]string{"version"}, info, &buf))

	out := buf.String()
	assert.Contains(t, out, "version: v1.2.3")
	assert.Contains(t, out, "commit: abc1234")
	assert.Contains(t, out, "2026-01-02T03:04:05Z")
	if update.IsContainer() {
		assert.Contains(t, out, "runtime: container")
	} else {
		assert.Contains(t, out, "runtime: binary")
	}

	// A build without release flags reports v0.0.0, which the updater
	// refuses to self update from.
	buf.Reset()
	require.NoError(t, run([]string{"version"}, buildInfo{version: "v0.0.0"}, &buf))
	assert.Contains(t, buf.String(), "version: v0.0.0")
}

// TestUnknownSubcommand keeps the old behavior (error, exit 1).
func TestUnknownSubcommand(t *testing.T) {
	err := run([]string{"explode"}, buildInfo{}, &bytes.Buffer{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown subcommand")
}

// TestVersionCmdErrorFree also covers the io.Writer failure path
// (versionCmd returns the write error, it does not panic).
func TestVersionCmdErrorFree(t *testing.T) {
	err := versionCmd(buildInfo{}, errWriter{})
	assert.Error(t, err)
}

type errWriter struct{}

func (errWriter) Write([]byte) (int, error) { return 0, errors.New("boom") }
