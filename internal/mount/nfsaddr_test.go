// SPDX-License-Identifier: AGPL-3.0-only

package mount

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/PeiceOfToast/shrinkyourshit/internal/store"
)

// newAddrExecutor returns an executor whose resolver is canned.
func newAddrExecutor(t *testing.T, resolve Resolver) *Executor {
	t.Helper()
	return New(&fakeStore{}, Options{Root: t.TempDir(), Resolve: resolve})
}

// An IP source needs no lookup, and the addr= the kernel requires must appear.
// Without it mount(2) answers EINVAL on a real kernel.
func TestWithServerAddrAddsAnAddressForAnIPSource(t *testing.T) {
	exec := newAddrExecutor(t, func(context.Context, string) ([]string, error) {
		t.Fatal("an IP source must not trigger a lookup")
		return nil, nil
	})

	spec := MountSpec{Fstype: "nfs", Source: "10.10.1.50:/mnt/DataSSD/SSD", Target: "/storage/ssdpool", Data: "vers=4.2,soft"}
	got, err := exec.withServerAddr(context.Background(), spec)
	if err != nil {
		t.Fatalf("withServerAddr: %v", err)
	}
	if got.Data != "vers=4.2,soft,addr=10.10.1.50" {
		t.Fatalf("data = %q, want addr=10.10.1.50 appended", got.Data)
	}
}

func TestWithServerAddrWithNoOptions(t *testing.T) {
	exec := newAddrExecutor(t, nil)
	spec := MountSpec{Fstype: "nfs", Source: "10.10.1.50:/export", Target: "/storage/x"}
	got, err := exec.withServerAddr(context.Background(), spec)
	if err != nil {
		t.Fatalf("withServerAddr: %v", err)
	}
	if got.Data != "addr=10.10.1.50" {
		t.Fatalf("data = %q, want just the address", got.Data)
	}
}

// A host name is resolved in userspace, because the kernel cannot do it without
// the dns_resolver helper.
func TestWithServerAddrResolvesAHostName(t *testing.T) {
	var asked string
	exec := newAddrExecutor(t, func(_ context.Context, host string) ([]string, error) {
		asked = host
		return []string{"10.10.1.50"}, nil
	})
	spec := MountSpec{Fstype: "nfs", Source: "nas.lab:/export", Target: "/storage/x", Data: "vers=4.2"}
	got, err := exec.withServerAddr(context.Background(), spec)
	if err != nil {
		t.Fatalf("withServerAddr: %v", err)
	}
	if asked != "nas.lab" {
		t.Fatalf("resolved %q, want nas.lab", asked)
	}
	if !strings.HasSuffix(got.Data, "addr=10.10.1.50") {
		t.Fatalf("data = %q, want addr=10.10.1.50", got.Data)
	}
}

func TestWithServerAddrDoesNotDoubleUp(t *testing.T) {
	exec := newAddrExecutor(t, func(context.Context, string) ([]string, error) {
		return []string{"10.9.9.9"}, nil
	})
	spec := MountSpec{Fstype: "nfs", Source: "nas.lab:/export", Target: "/storage/x", Data: "addr=10.10.1.50,vers=4.2"}
	got, err := exec.withServerAddr(context.Background(), spec)
	if err != nil {
		t.Fatalf("withServerAddr: %v", err)
	}
	if strings.Count(got.Data, "addr=") != 1 {
		t.Fatalf("data = %q, want exactly one addr=", got.Data)
	}
}

// CIFS resolves in the kernel, so the NFS address must not leak into an SMB mount.
func TestWithServerAddrLeavesSMBAlone(t *testing.T) {
	exec := newAddrExecutor(t, func(context.Context, string) ([]string, error) {
		t.Fatal("smb must not trigger a lookup")
		return nil, nil
	})
	spec := MountSpec{Fstype: "cifs", Source: "//nas/media", Target: "/storage/media", Data: "credentials=/tmp/c.cred,vers=3.0"}
	got, err := exec.withServerAddr(context.Background(), spec)
	if err != nil {
		t.Fatalf("withServerAddr: %v", err)
	}
	if got.Data != spec.Data {
		t.Fatalf("data = %q, want it untouched: %q", got.Data, spec.Data)
	}
}

func TestWithServerAddrReportsAFailedLookup(t *testing.T) {
	exec := newAddrExecutor(t, func(context.Context, string) ([]string, error) {
		return nil, errors.New("no such host")
	})
	spec := MountSpec{Fstype: "nfs", Source: "missing.lab:/export", Target: "/storage/x"}
	_, err := exec.withServerAddr(context.Background(), spec)
	if err == nil || !strings.Contains(err.Error(), "missing.lab") {
		t.Fatalf("error = %v, want it to name the host", err)
	}
}

func TestWithServerAddrRejectsARejectedSourceShape(t *testing.T) {
	exec := newAddrExecutor(t, nil)
	spec := MountSpec{Fstype: "nfs", Source: "noseparator", Target: "/storage/x"}
	if _, err := exec.withServerAddr(context.Background(), spec); err == nil {
		t.Fatal("a source with no host:/path shape should be refused")
	}
}

// The mount the app builds for the lab NAS has to carry addr= or the kernel
// rejects it, which is the defect this file exists for.
func TestMountSpecForTheLabNASCariesAddr(t *testing.T) {
	exec := newAddrExecutor(t, nil)
	st := &store.Storage{ID: "st-1", Name: "ssdpool", Protocol: "nfs",
		Source: "10.10.1.50:/mnt/DataSSD/SSD", Options: "vers=4.2,soft,timeo=50,retrans=2", Enabled: true}
	spec := BuildMountSpec(st, "/storage/ssdpool", "")
	spec, err := exec.withServerAddr(context.Background(), spec)
	if err != nil {
		t.Fatalf("withServerAddr: %v", err)
	}
	if !strings.Contains(spec.Data, "addr=10.10.1.50") {
		t.Fatalf("data = %q, want addr=10.10.1.50", spec.Data)
	}
	if spec.Fstype != "nfs" || spec.Flags != 0 {
		t.Fatalf("spec = %+v", spec)
	}
}
