// SPDX-License-Identifier: AGPL-3.0-only

//go:build windows

package main

import (
	"fmt"
	"os"
)

// restartSelf hands the process over to the freshly installed binary on
// Windows. There is no exec(2): the new binary is started as a sibling
// process and this one exits 0. A service wrapper (scheduled task, NSSM)
// keeps the worker running; the freshly started process serves until the
// wrapper takes over on the next host reboot.
func restartSelf(exe string) error {
	argv := append([]string{exe}, os.Args[1:]...)
	fmt.Printf("restarting on %s\n", exe)
	p, err := os.StartProcess(exe, argv, &os.ProcAttr{
		Files: []*os.File{os.Stdin, os.Stdout, os.Stderr},
	})
	if err != nil {
		return err
	}
	_ = p.Release()
	return errExitZero
}
