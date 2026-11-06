// SPDX-License-Identifier: AGPL-3.0-only

package mount

import (
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"
	"syscall"

	"github.com/PeiceOfToast/shrinkyourshit/internal/store"
)

// Linux ABI values for the mount(2) flags and the umount2(2) flag. They are
// written out here, not taken from x/sys, so the option translation and its
// tests build and run on every platform.
const (
	msRdOnly  uintptr = 1    // MS_RDONLY
	msNoSuid  uintptr = 2    // MS_NOSUID
	msNoDev   uintptr = 4    // MS_NODEV
	msNoExec  uintptr = 8    // MS_NOEXEC
	msNoAtime uintptr = 1024 // MS_NOATIME
	mntDetach int     = 2    // MNT_DETACH: a lazy unmount
)

// Mounter makes the kernel calls. The production implementation calls mount(2)
// and umount2(2) in this process, so the container's CAP_SYS_ADMIN applies to
// the call itself (task 79). Tests supply a fake.
type Mounter interface {
	Mount(source, target, fstype string, flags uintptr, data string) error
	Unmount(target string, flags int) error
}

// flagOptions are mount options the kernel takes as flag bits, not as data.
var flagOptions = map[string]uintptr{
	"ro":      msRdOnly,
	"nosuid":  msNoSuid,
	"nodev":   msNoDev,
	"noexec":  msNoExec,
	"noatime": msNoAtime,
}

// droppedOptions are userspace-only names that mount(8) reads and never passes
// on. rw is dropped as well: read write is the kernel default, and the kernel
// does not take rw as data.
var droppedOptions = map[string]bool{
	"_netdev":  true,
	"defaults": true,
	"rw":       true,
}

// FlagOptions lists, in order, the option names that become mount(2) flag bits.
func FlagOptions() []string { return slices.Sorted(maps.Keys(flagOptions)) }

// DroppedOptions lists, in order, the option names that are removed before the
// mount call.
func DroppedOptions() []string { return slices.Sorted(maps.Keys(droppedOptions)) }

// translateOptions splits a stored option list into the mount(2) flag bits and
// the comma joined data string. A name is a flag or dropped only when it has no
// value, so anything with "=" is always data.
func translateOptions(opts []string) (uintptr, string) {
	var flags uintptr
	var data []string
	for _, opt := range opts {
		if strings.Contains(opt, "=") {
			data = append(data, opt)
			continue
		}
		if bit, ok := flagOptions[opt]; ok {
			flags |= bit
			continue
		}
		if droppedOptions[opt] {
			continue
		}
		data = append(data, opt)
	}
	return flags, strings.Join(data, ",")
}

// MountSpec is one mount(2) call for a storage (spec 4.1): the filesystem type,
// the flag bits, and the data string the filesystem parses.
type MountSpec struct {
	Fstype string
	Source string
	Target string
	Flags  uintptr
	Data   string
}

// BuildMountSpec turns a storage row into the mount(2) call. The credentials path
// comes from the caller so the result can be checked exactly. The data holds the
// credentials option first (SMB only), then the stored options in order, then ro
// for a read only storage. No password ever reaches the data string.
func BuildMountSpec(st *store.Storage, mountpoint, credsPath string) MountSpec {
	spec := MountSpec{Fstype: "nfs", Source: st.Source, Target: mountpoint}
	var opts []string
	if st.Protocol == "smb" {
		spec.Fstype = "cifs"
		opts = append(opts, "credentials="+credsPath)
	}
	opts = append(opts, splitOptions(st.Options)...)
	if st.ReadOnly && !hasOption(opts, "ro") {
		opts = append(opts, "ro")
	}
	spec.Flags, spec.Data = translateOptions(opts)
	return spec
}

// ErrNeedsLinux is what every mount call returns outside Linux.
var ErrNeedsLinux = errors.New("in-container mounts need Linux; this build cannot mount shares")

// unsupportedMounter is the Mounter for builds that are not Linux. It compiles
// everywhere, so the rest of the package needs no build tags.
type unsupportedMounter struct{}

func (unsupportedMounter) Mount(string, string, string, uintptr, string) error {
	return ErrNeedsLinux
}

func (unsupportedMounter) Unmount(string, int) error { return ErrNeedsLinux }

// MountError is a failed mount(2) or umount2(2). Its message says what was
// attempted and, for the errno values an operator can act on, what to fix.
type MountError struct {
	Op     string // "mount" or "umount"
	Fstype string
	Source string
	Target string
	Errno  syscall.Errno
}

func (e *MountError) Error() string { return e.attempt() + ": " + e.explain() }

// Unwrap returns the errno, so errors.Is(err, syscall.EBUSY) works.
func (e *MountError) Unwrap() error { return e.Errno }

func (e *MountError) attempt() string {
	if e.Op == "umount" {
		return "umount " + e.Target
	}
	return fmt.Sprintf("mount -t %s %s at %s", e.Fstype, e.Source, e.Target)
}

func (e *MountError) explain() string {
	switch e.Errno {
	case syscall.EPERM, syscall.EACCES:
		return "not permitted; the container needs cap_add: SYS_ADMIN and security_opt: apparmor=unconfined"
	case syscall.ENODEV:
		if e.Op == "mount" {
			return fmt.Sprintf("no %s client is available; the image needs the %s package", e.Fstype, clientPackage(e.Fstype))
		}
	case syscall.ETIMEDOUT, syscall.EHOSTUNREACH:
		return fmt.Sprintf("the server did not answer (%s); check the host name and that it is reachable from the container", e.Source)
	}
	return e.Errno.Error()
}

// clientPackage names the image package that provides the filesystem's
// userspace side.
func clientPackage(fstype string) string {
	switch fstype {
	case "nfs":
		return "nfs-common"
	case "cifs":
		return "cifs-utils"
	}
	return fstype
}

// mountErr wraps an errno from mount(2) for spec. Any other error passes through.
func mountErr(spec MountSpec, err error) error {
	return wrapErrno(err, &MountError{Op: "mount", Fstype: spec.Fstype, Source: spec.Source, Target: spec.Target})
}

// umountErr wraps an errno from umount2(2) for target. Any other error passes through.
func umountErr(target string, err error) error {
	return wrapErrno(err, &MountError{Op: "umount", Target: target})
}

func wrapErrno(err error, e *MountError) error {
	if err == nil {
		return nil
	}
	var errno syscall.Errno
	if !errors.As(err, &errno) {
		return err
	}
	e.Errno = errno
	return e
}
