// SPDX-License-Identifier: AGPL-3.0-only

//go:build !linux

package mount

// defaultMounter is the production Mounter outside Linux: every call returns
// ErrNeedsLinux, so the build compiles and the server still starts.
func defaultMounter() Mounter { return unsupportedMounter{} }
