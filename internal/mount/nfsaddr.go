// SPDX-License-Identifier: AGPL-3.0-only

package mount

import (
	"context"
	"fmt"
	"net"
	"strings"
)

// The kernel's text based NFS mount needs the server address in the options.
// mount(2) with only "10.10.1.50:/export" and no addr= returns EINVAL, on an IP
// source as much as on a host name, because the kernel client cannot resolve
// the server itself: that path needs the dns_resolver upcall and /sbin/request-key,
// which a slim container image does not carry. mount.nfs used to do this
// resolution in userspace and add addr= before calling the kernel, and that is
// the piece this package has to reproduce now that it calls mount(2) itself.
//
// Proven on the lab NAS on 2026-11-06 (kernel 6.8.0):
//
//	mount("10.10.1.50:/mnt/DataSSD/SSD", "/mnt/t", "nfs", 0, "vers=4.2")              -> EINVAL
//	mount("10.10.1.50:/mnt/DataSSD/SSD", "/mnt/t", "nfs", 0, "addr=10.10.1.50,vers=4.2") -> OK

// Resolver turns a host name into addresses. It is a field on Options so tests
// can answer without DNS.
type Resolver func(ctx context.Context, host string) ([]string, error)

// nfsSourceHost returns the host half of an NFS source, host:/path.
func nfsSourceHost(source string) (string, bool) {
	host, _, ok := strings.Cut(source, ":")
	if !ok || host == "" {
		return "", false
	}
	if strings.HasPrefix(host, "/") || strings.Contains(host, "/") {
		return "", false
	}
	return host, true
}

// dataHasOption reports whether a comma separated data string already carries
// the named option, so a second addr= never lands in the same mount.
func dataHasOption(data, name string) bool {
	for _, opt := range strings.Split(data, ",") {
		if opt == name || strings.HasPrefix(opt, name+"=") {
			return true
		}
	}
	return false
}

// withServerAddr returns the spec with addr=<ip> in the data string for an NFS
// mount. A source that is already an IP needs no lookup. A host name is
// resolved here, in userspace, because the kernel cannot. Every other protocol
// comes back untouched.
func (e *Executor) withServerAddr(ctx context.Context, spec MountSpec) (MountSpec, error) {
	if spec.Fstype != "nfs" && spec.Fstype != "nfs4" {
		return spec, nil
	}
	if dataHasOption(spec.Data, "addr") {
		return spec, nil
	}
	host, ok := nfsSourceHost(spec.Source)
	if !ok {
		return spec, fmt.Errorf("source %q is not host:/path", spec.Source)
	}

	ip := host
	if net.ParseIP(host) == nil {
		if e.opt.Resolve == nil {
			return spec, fmt.Errorf("cannot resolve the NFS server %q: no resolver configured", host)
		}
		addrs, err := e.opt.Resolve(ctx, host)
		if err != nil {
			return spec, fmt.Errorf("cannot resolve the NFS server %q: %w", host, err)
		}
		if len(addrs) == 0 {
			return spec, fmt.Errorf("cannot resolve the NFS server %q: no addresses returned", host)
		}
		ip = addrs[0]
		if net.ParseIP(ip) == nil {
			return spec, fmt.Errorf("resolving the NFS server %q gave %q, which is not an address", host, ip)
		}
	}

	if spec.Data == "" {
		spec.Data = "addr=" + ip
	} else {
		spec.Data += ",addr=" + ip
	}
	return spec, nil
}

// lookupHost is the production resolver.
func lookupHost(ctx context.Context, host string) ([]string, error) {
	return net.DefaultResolver.LookupHost(ctx, host)
}
