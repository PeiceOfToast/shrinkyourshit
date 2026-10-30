// SPDX-License-Identifier: AGPL-3.0-only

//go:build unix

package discovery

import (
	"net"
	"syscall"
)

// enableBroadcast sets SO_BROADCAST so datagrams may target interface
// broadcast addresses. Unix only (Windows refuses it on a bound socket);
// a failure is harmless — loopback probes never need the flag.
//
// The socket option call lives in its own file because the fd type differs
// per OS: unix passes int, windows a syscall.Handle. That difference broke
// the windows/amd64 release build once (v0.1.0 binaries job), so the
// windows variant is a no-op in broadcast_windows.go.
func enableBroadcast(conn *net.UDPConn) {
	raw, err := conn.SyscallConn()
	if err != nil {
		return
	}
	_ = raw.Control(func(fd uintptr) {
		_ = syscall.SetsockoptInt(int(fd), syscall.SOL_SOCKET, syscall.SO_BROADCAST, 1)
	})
}
