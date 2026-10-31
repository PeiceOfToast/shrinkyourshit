// SPDX-License-Identifier: AGPL-3.0-only

//go:build windows

package plugin

import (
	"os/exec"
)

// setProcessGroup is a no-op on Windows: there are no process groups to
// set, and CommandContext's kill already terminates the direct child on
// timeout (a plugin that spawns detached children outlives it there; the
// documented behavior is Unix-first).
func setProcessGroup(cmd *exec.Cmd) {}
