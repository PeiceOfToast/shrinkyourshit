// SPDX-License-Identifier: AGPL-3.0-only

//go:build !windows

package main

import (
	"fmt"
	"os"
	"syscall"
)

// restartSelf hands the process over to the freshly installed binary.
//
// Under systemd it exits 0 and the unit's Restart=always brings the new
// binary straight back up (the installer writes exactly that setting; see
// README "Updating"). Anywhere else it execs the new binary with the same
// arguments, so the update ends as a restart without leaving the process
// table — and the new binary's own output confirms it came up.
func restartSelf(exe string) error {
	if managedBySystemd() {
		fmt.Println("updated; exiting so systemd restarts on the new binary")
		return errExitZero
	}
	argv := append([]string{exe}, os.Args[1:]...)
	fmt.Printf("restarting on %s\n", exe)
	return syscall.Exec(exe, argv, os.Environ())
}

// managedBySystemd reports whether this process was started by systemd
// (INVOCATION_ID is set for every unit) and systemd is the running init.
func managedBySystemd() bool {
	if os.Getenv("INVOCATION_ID") == "" {
		return false
	}
	if _, err := os.Stat("/run/systemd/system"); err != nil {
		return false
	}
	return true
}
