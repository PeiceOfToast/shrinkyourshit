// SPDX-License-Identifier: AGPL-3.0-only

//go:build linux

package mount

import "golang.org/x/sys/unix"

// kernelMounter calls mount(2) and umount2(2) in this process. The container's
// CAP_SYS_ADMIN applies to these calls directly (task 79).
type kernelMounter struct{}

func (kernelMounter) Mount(source, target, fstype string, flags uintptr, data string) error {
	return unix.Mount(source, target, fstype, flags, data)
}

func (kernelMounter) Unmount(target string, flags int) error {
	return unix.Unmount(target, flags)
}

// defaultMounter is the production Mounter on Linux.
func defaultMounter() Mounter { return kernelMounter{} }
