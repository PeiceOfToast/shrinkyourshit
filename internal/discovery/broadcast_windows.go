// SPDX-License-Identifier: AGPL-3.0-only

//go:build windows

package discovery

import "net"

// enableBroadcast is a no-op on Windows: setsockopt there takes a
// syscall.Handle (not an int fd) and Windows refuses SO_BROADCAST on a
// socket bound before the call anyway. Broadcast probing is a unix
// convenience; a Windows worker reaches its server by configured URL or
// loopback probe, both of which work without the flag.
//
// Splitting this out of discovery.go keeps windows/amd64 compiling: the
// unix file cannot, because int(fd) is not a syscall.Handle there.
func enableBroadcast(_ *net.UDPConn) {}
