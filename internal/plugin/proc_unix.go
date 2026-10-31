// SPDX-License-Identifier: AGPL-3.0-only

//go:build unix

package plugin

import (
	"os/exec"
	"syscall"
)

// setProcessGroup puts the plugin process into its own process group and
// kills the whole group on timeout (task 31: "the whole process group
// killed on timeout"). A plugin that spawned children cannot leave them
// behind after the deadline. cmd.Cancel replaces CommandContext's plain
// child kill with a group SIGKILL.
func setProcessGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return nil
		}
		return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}
}
