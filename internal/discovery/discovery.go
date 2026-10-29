// SPDX-License-Identifier: AGPL-3.0-only

// Package discovery lets a fresh worker find the server on the LAN and
// lets the server be found (ARCHITECTURE.md: one binary, three roles).
// Two transports, both best effort:
//
//   - mDNS (Bonjour/zeroconf), service type _shrinkyourshit._tcp, TXT
//     records version, name and id (the id is the server's built-in node
//     id, so the setup wizard can hide itself from its own list);
//   - a UDP broadcast fallback on port 8766: a client sends the probe
//     magic "SYS-DISCOVER" and every server answers with a small JSON
//     {id,name,url,version} — the url is the answer, the rest feeds the
//     setup wizard's server list.
//
// Tests can disable either transport (Options), which is how CI exercises
// both paths independently.
package discovery

import (
	"context"
	"encoding/json"
	"log/slog"
	"net"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/grandcat/zeroconf"
)

// ServiceType is the mDNS service type; Domain is always "local".
const (
	ServiceType = "_shrinkyourshit._tcp"
	Domain      = "local"
	// DefaultPort is the UDP broadcast port (task 08).
	DefaultPort = 8766
	// probeMagic is the datagram a discovering client sends.
	probeMagic = "SYS-DISCOVER"
)

// ServerInfo is one discovered server.
type ServerInfo struct {
	ID      string `json:"id"`      // unique instance id (built-in node id)
	Name    string `json:"name"`    // display name
	URL     string `json:"url"`     // base URL, e.g. http://192.168.1.10:8765
	Version string `json:"version"` // server version
}

// Options selects which transports participate and on which UDP port.
// The zero value means "both transports, default port".
type Options struct {
	MDNS      bool // announce/browse via mDNS
	Broadcast bool // answer/send the UDP broadcast probe
	Port      int  // UDP broadcast port; 0 → DefaultPort
}

// withDefaults fills the unset fields.
func (o Options) withDefaults() Options {
	if o.Port == 0 {
		o.Port = DefaultPort
	}
	return o
}

// Announcement is what a running server tells the LAN.
type Announcement struct {
	ID      string // unique instance id (built-in node id)
	Name    string // display name (hostname by default)
	Version string // binary version
	Port    int    // HTTP port the server answers on
	Options Options
	Log     *slog.Logger // nil → slog default
}

// Announce blocks until ctx is done, advertising the server via mDNS
// (when enabled) and answering UDP probes (when enabled). A transport
// that cannot start (no multicast interfaces, port busy) is logged and
// skipped: the other transport still works.
func Announce(ctx context.Context, a Announcement) error {
	log := a.Log
	if log == nil {
		log = slog.Default()
	}
	opts := a.Options.withDefaults()

	done := make(chan struct{})
	var wg sync.WaitGroup
	if opts.Broadcast {
		wg.Add(1)
		go func() {
			defer wg.Done()
			serveBroadcast(ctx, opts.Port, a, log)
		}()
	}
	go func() {
		wg.Wait()
		close(done)
	}()

	if !opts.MDNS {
		<-ctx.Done()
		<-done
		return nil
	}

	text := []string{
		"version=" + a.Version,
		"name=" + a.Name,
		"id=" + a.ID,
	}
	// The instance name must be unique on the LAN; the id guarantees it
	// even with two servers on one host.
	zs, err := zeroconf.Register(a.ID, ServiceType, Domain, a.Port, text, nil)
	if err != nil {
		log.Warn("discovery: mDNS announce failed; broadcast only", "err", err)
		<-ctx.Done()
		<-done
		return nil
	}
	defer zs.Shutdown()
	<-ctx.Done()
	<-done
	return nil
}

// reply is the UDP probe answer.
type reply = ServerInfo

// serveBroadcast answers "SYS-DISCOVER" datagrams with the server's URL.
// A one-second read deadline keeps the loop checking ctx; timeouts are
// the normal wake-up, everything else logs once and stops.
func serveBroadcast(ctx context.Context, port int, a Announcement, log *slog.Logger) {
	conn, err := net.ListenUDP("udp4", &net.UDPAddr{Port: port})
	if err != nil {
		log.Warn("discovery: broadcast responder unavailable", "port", port, "err", err)
		return
	}
	defer func() { _ = conn.Close() }()

	buf := make([]byte, 1500)
	for ctx.Err() == nil {
		_ = conn.SetReadDeadline(time.Now().Add(time.Second))
		n, from, err := conn.ReadFromUDP(buf)
		if err != nil {
			continue // deadline tick or transient error
		}
		if strings.TrimSpace(string(buf[:n])) != probeMagic {
			continue // not our probe; ignore quietly
		}
		url := localURLToward(conn, from, a.Port)
		if url == "" {
			continue
		}
		out, err := json.Marshal(reply{ID: a.ID, Name: a.Name, URL: url, Version: a.Version})
		if err != nil {
			continue
		}
		_, _ = conn.WriteToUDP(out, from)
	}
}

// localURLToward returns http://<local ip>:<port> for the interface the
// requester reached us on: a connected UDP socket to the sender reveals
// the source address the kernel would pick.
func localURLToward(conn *net.UDPConn, from *net.UDPAddr, httpPort int) string {
	probe, err := net.DialUDP("udp4", nil, from)
	if err != nil {
		return ""
	}
	defer func() { _ = probe.Close() }()
	local, ok := probe.LocalAddr().(*net.UDPAddr)
	if !ok || local.IP == nil || local.IP.IsUnspecified() {
		if la, ok2 := conn.LocalAddr().(*net.UDPAddr); ok2 && la.IP != nil && !la.IP.IsUnspecified() {
			local = la
		} else {
			local = &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)}
		}
	}
	return "http://" + net.JoinHostPort(local.IP.String(), strconv.Itoa(httpPort))
}

// Discover probes the LAN for servers for up to timeout and returns the
// answers, deduplicated by URL. Order is not meaningful.
func Discover(ctx context.Context, timeout time.Duration, opts Options) []ServerInfo {
	opts = opts.withDefaults()
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	results := make(chan ServerInfo, 64)
	var wg sync.WaitGroup
	if opts.MDNS {
		wg.Add(1)
		go func() {
			defer wg.Done()
			browseMDNS(ctx, results)
		}()
	}
	if opts.Broadcast {
		wg.Add(1)
		go func() {
			defer wg.Done()
			sendBroadcastProbes(ctx, opts.Port, results)
		}()
	}
	go func() {
		wg.Wait()
		close(results)
	}()

	seen := map[string]bool{}
	var out []ServerInfo
	for info := range results {
		if info.URL == "" || seen[info.URL] {
			continue
		}
		seen[info.URL] = true
		out = append(out, info)
	}
	return out
}

// browseMDNS collects mDNS answers until ctx is done.
func browseMDNS(ctx context.Context, results chan<- ServerInfo) {
	resolver, err := zeroconf.NewResolver(zeroconf.SelectIPTraffic(zeroconf.IPv4))
	if err != nil {
		return
	}
	entries := make(chan *zeroconf.ServiceEntry, 64)
	if err := resolver.Browse(ctx, ServiceType, Domain, entries); err != nil {
		return
	}
	for entry := range entries {
		if entry.Port == 0 {
			continue
		}
		info := ServerInfo{ID: entry.Instance}
		for _, txt := range entry.Text {
			switch {
			case strings.HasPrefix(txt, "name="):
				info.Name = strings.TrimPrefix(txt, "name=")
			case strings.HasPrefix(txt, "version="):
				info.Version = strings.TrimPrefix(txt, "version=")
			}
		}
		if info.Name == "" {
			info.Name = entry.Instance
		}
		for _, ip := range entry.AddrIPv4 {
			info.URL = "http://" + net.JoinHostPort(ip.String(), strconv.Itoa(entry.Port))
			break // one URL per entry; it is the dedupe key
		}
		if info.URL == "" {
			continue
		}
		select {
		case results <- info:
		case <-ctx.Done():
			return
		}
	}
}

// sendBroadcastProbes sends SYS-DISCOVER to the loopback address and every
// interface broadcast address, then collects replies until ctx is done.
// Loopback is included so two processes on one machine (tests, and a
// worker joining a server on the same host) always find each other.
func sendBroadcastProbes(ctx context.Context, port int, results chan<- ServerInfo) {
	conn, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4zero})
	if err != nil {
		return
	}
	defer func() { _ = conn.Close() }()
	enableBroadcast(conn)

	dst := []*net.UDPAddr{{IP: net.IPv4(127, 0, 0, 1), Port: port}}
	for _, b := range broadcastAddrs() {
		dst = append(dst, &net.UDPAddr{IP: b, Port: port})
	}
	for _, d := range dst {
		if _, err := conn.WriteToUDP([]byte(probeMagic), d); err != nil && ctx.Err() == nil {
			continue // unreachable broadcast target: expected on some hosts
		}
	}

	buf := make([]byte, 1500)
	for ctx.Err() == nil {
		_ = conn.SetReadDeadline(time.Now().Add(300 * time.Millisecond))
		n, _, err := conn.ReadFromUDP(buf)
		if err != nil {
			continue // deadline tick
		}
		var r reply
		if json.Unmarshal(buf[:n], &r) != nil || r.URL == "" {
			continue
		}
		select {
		case results <- r:
		case <-ctx.Done():
			return
		}
	}
}

// enableBroadcast sets SO_BROADCAST so datagrams may target interface
// broadcast addresses. Unix only (Windows refuses it on a bound socket);
// a failure is harmless — loopback probes never need the flag.
func enableBroadcast(conn *net.UDPConn) {
	raw, err := conn.SyscallConn()
	if err != nil {
		return
	}
	_ = raw.Control(func(fd uintptr) {
		_ = syscall.SetsockoptInt(int(fd), syscall.SOL_SOCKET, syscall.SO_BROADCAST, 1)
	})
}

// broadcastAddrs lists the IPv4 broadcast address of every up, non-loopback
// interface.
func broadcastAddrs() []net.IP {
	var out []net.IP
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil
	}
	for _, ifi := range ifaces {
		if ifi.Flags&net.FlagUp == 0 || ifi.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, err := ifi.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			ipnet, ok := addr.(*net.IPNet)
			if !ok {
				continue
			}
			if b := broadcastOf(ipnet); b != nil {
				out = append(out, b)
			}
		}
	}
	return out
}

// broadcastOf computes the x.x.x.255-style broadcast address of a prefix.
func broadcastOf(ipnet *net.IPNet) net.IP {
	ip := ipnet.IP.To4()
	if ip == nil || len(ipnet.Mask) != 4 {
		return nil
	}
	out := make(net.IP, 4)
	for i := range 4 {
		out[i] = ip[i] | ^ipnet.Mask[i]
	}
	return out
}
