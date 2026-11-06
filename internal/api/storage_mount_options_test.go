// SPDX-License-Identifier: AGPL-3.0-only

package api

import (
	"testing"

	"github.com/PeiceOfToast/shrinkyourshit/internal/mount"
)

// The allowlist here and the mount package's option table must agree. The mount
// package cannot import this package, so this test is where they meet: a flag
// added to one side only fails here.
func TestMountTranslationAgreesWithAllowlist(t *testing.T) {
	// Every option mount(2) takes as a flag bit must be a no-value option on the
	// allowlist, and the allowlist must not offer any other name as a flag.
	wantFlags := []string{"noatime", "nodev", "noexec", "nosuid", "ro"}
	gotFlags := mount.FlagOptions()
	if len(gotFlags) != len(wantFlags) {
		t.Fatalf("mount flag options = %v, want %v", gotFlags, wantFlags)
	}
	for i, name := range wantFlags {
		if gotFlags[i] != name {
			t.Fatalf("mount flag options = %v, want %v", gotFlags, wantFlags)
		}
		rule, ok := storageMountOptions[name]
		if !ok || rule.value {
			t.Errorf("mount takes %q as a flag, but the allowlist does not offer it as a flag option", name)
		}
	}

	// Each userspace-only name mount drops must be a no-value allowlisted option
	// too, so the API does not accept an option that mount would silently drop.
	// "defaults" is refused by the API and dropped only as a safety net.
	for _, name := range mount.DroppedOptions() {
		if name == "defaults" {
			continue
		}
		rule, ok := storageMountOptions[name]
		if !ok || rule.value {
			t.Errorf("mount drops %q, but the allowlist does not offer it as a flag option", name)
		}
	}

	// Every allowlisted flag option must be known to mount, as a flag or as a dropped name.
	known := map[string]bool{}
	for _, name := range gotFlags {
		known[name] = true
	}
	for _, name := range mount.DroppedOptions() {
		known[name] = true
	}
	for name, rule := range storageMountOptions {
		if rule.value || name == "soft" || name == "hard" {
			continue
		}
		if !known[name] {
			t.Errorf("allowlist option %q is neither a mount flag nor dropped, so mount would send it as data", name)
		}
	}
}
