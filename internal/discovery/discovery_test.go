// SPDX-License-Identifier: AGPL-3.0-only

package discovery

import (
	"context"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// freeUDPPort asks the kernel for an unused UDP port.
func freeUDPPort(t *testing.T) int {
	t.Helper()
	conn, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	require.NoError(t, err)
	port := conn.LocalAddr().(*net.UDPAddr).Port
	require.NoError(t, conn.Close())
	return port
}

// waitFor polls until cond holds or the deadline passes.
func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

// TestBroadcastRoundTrip covers the UDP fallback end to end: a server
// announcing on one port is found by Discover on the same host.
func TestBroadcastRoundTrip(t *testing.T) {
	udpPort := freeUDPPort(t)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- Announce(ctx, Announcement{
			ID: "id-broadcast", Name: "broadcast-test", Version: "vTest",
			Port:    8765,
			Options: Options{MDNS: false, Broadcast: true, Port: udpPort},
		})
	}()

	var found []ServerInfo
	waitFor(t, "broadcast discovery", func() bool {
		found = Discover(context.Background(), 2*time.Second,
			Options{MDNS: false, Broadcast: true, Port: udpPort})
		for _, info := range found {
			if info.ID == "id-broadcast" {
				return true
			}
		}
		return false
	})
	cancel()
	require.NoError(t, <-done)

	// The same server may answer via several interfaces (loopback and the
	// LAN address): every answer carries the announced id, and the loopback
	// answer always carries the canonical URL.
	var match *ServerInfo
	for i := range found {
		if found[i].ID == "id-broadcast" {
			require.Equal(t, "broadcast-test", found[i].Name)
			require.Equal(t, "vTest", found[i].Version)
			if found[i].URL == "http://127.0.0.1:8765" {
				match = &found[i]
			}
		}
	}
	require.NotNil(t, match, "loopback answer missing")
}

// TestDiscoverWithoutServers returns nothing instead of blocking.
func TestDiscoverWithoutServers(t *testing.T) {
	udpPort := freeUDPPort(t)
	got := Discover(context.Background(), 300*time.Millisecond,
		Options{MDNS: false, Broadcast: true, Port: udpPort})
	require.Empty(t, got)
}

// TestAnnounceStopsOnCancel makes sure Announce returns when ctx ends.
func TestAnnounceStopsOnCancel(t *testing.T) {
	udpPort := freeUDPPort(t)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- Announce(ctx, Announcement{
			ID: "id-stop", Name: "stop-test", Port: 8765,
			Options: Options{MDNS: false, Broadcast: true, Port: udpPort},
		})
	}()
	time.Sleep(200 * time.Millisecond)
	cancel()
	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("Announce did not stop after ctx cancel")
	}
}

// mdnsAvailable reports whether this host can use IP multicast at all.
// CI containers often cannot; the test then skips with a clear message
// (the broadcast fallback is tested unconditionally above).
func mdnsAvailable(t *testing.T) {
	t.Helper()
	group := &net.UDPAddr{IP: net.IPv4(224, 0, 0, 251), Port: 5353}
	conn, err := net.ListenMulticastUDP("udp4", nil, group)
	if err != nil {
		msg := fmt.Sprintf("skipping mDNS test: multicast unavailable here (%v); "+
			"set %s=1 to force a failure instead", err, "SHRINK_TEST_REQUIRE_MDNS")
		if os.Getenv("SHRINK_TEST_REQUIRE_MDNS") == "1" {
			t.Fatalf("mDNS required but unavailable: %v", err)
		}
		t.Skip(msg)
	}
	_ = conn.Close()
}

// TestMDNSRoundTrip covers the mDNS transport end to end, with broadcast
// disabled on both sides so only mDNS can answer (acceptance item 1).
func TestMDNSRoundTrip(t *testing.T) {
	mdnsAvailable(t)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- Announce(ctx, Announcement{
			ID: "id-mdns-" + strconv.FormatInt(time.Now().UnixNano(), 36),
			Name: "mdns-test", Version: "vTest", Port: 8765,
			Options: Options{MDNS: true, Broadcast: false},
		})
	}()
	defer func() {
		cancel()
		require.NoError(t, <-done)
	}()

	want := "id-mdns"
	var found []ServerInfo
	waitFor(t, "mDNS discovery", func() bool {
		found = Discover(context.Background(), 3*time.Second,
			Options{MDNS: true, Broadcast: false})
		for _, info := range found {
			if strings.HasPrefix(info.ID, want) {
				return true
			}
		}
		return false
	})

	var match *ServerInfo
	for i := range found {
		if strings.HasPrefix(found[i].ID, want) {
			match = &found[i]
		}
	}
	require.NotNil(t, match)
	require.Equal(t, "mdns-test", match.Name)
	require.Equal(t, "vTest", match.Version)
	require.True(t, strings.HasPrefix(match.URL, "http://"), match.URL)
	require.True(t, strings.HasSuffix(match.URL, ":8765"), match.URL)
}
