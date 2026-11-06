// SPDX-License-Identifier: AGPL-3.0-only

// Package mount is the storage mount executor (task 66, spec 4.1). It mounts
// NFS and SMB shares inside the container with mount(2), unmounts them, tests
// them, reports their status from /proc/mounts, remounts every enabled storage
// at startup and keeps them mounted with a watchdog.
//
// The package never builds a shell command line and never runs mount(8). Each
// call is a mount(2) with fixed arguments, made in this process so the
// container's CAP_SYS_ADMIN applies (task 79). The SMB password reaches the
// kernel only through a 0600 credentials file that is removed as soon as mount
// returns.
package mount

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/PeiceOfToast/shrinkyourshit/internal/store"
)

// Defaults. Each one can be overridden in Options; tests use short values.
const (
	DefaultRoot          = "/storage"
	DefaultMountTimeout  = 60 * time.Second
	DefaultStatTimeout   = 5 * time.Second
	DefaultWatchInterval = 30 * time.Second
	DefaultBackoffBase   = 10 * time.Second
	DefaultBackoffMax    = 10 * time.Minute
	// DefaultBusyWait is the pause before a busy unmount is retried with a
	// lazy detach (task 79, step 4).
	DefaultBusyWait = 500 * time.Millisecond
	// probeName is written into a share root by Test (spec 4.4). It is removed
	// again before Test returns.
	probeName = ".shrinkyourshit-probe"
)

// Store is the slice of the store the executor needs. The real store.SQLite
// satisfies it; tests supply a fake.
type Store interface {
	ListStorages(ctx context.Context) ([]*store.Storage, error)
	StorageSecret(ctx context.Context, id string) (string, error)
}

// Options configures an Executor. Zero values take the defaults above.
type Options struct {
	// Root is the directory mountpoints live under: Root/<name>.
	Root string
	// ScratchDir holds the SMB credentials file while mount runs.
	ScratchDir string
	// ProcMounts is the mount table to read. Defaults to /proc/mounts.
	ProcMounts string
	// Mounter makes the mount(2) and umount2(2) calls. Defaults to the kernel
	// on Linux and to an error everywhere else.
	Mounter Mounter
	// ProcStatus is the file read for the CAP_SYS_ADMIN check at startup.
	// Defaults to /proc/self/status.
	ProcStatus string
	// Resolve turns an NFS host name into addresses. Defaults to the system
	// resolver. An NFS source that is already an IP never uses it.
	Resolve Resolver
	// BusyWait is the pause before a busy unmount is retried with MNT_DETACH.
	// Defaults to DefaultBusyWait.
	BusyWait time.Duration
	// Allow is the host allowlist gate (spec 4.4). It runs before every
	// mount-side action, so a host removed from the setting cannot be mounted
	// by a row that already exists. Nil allows everything.
	Allow func(ctx context.Context, st *store.Storage) error
	Log   *slog.Logger
	// Now is the clock for the watchdog backoff. Defaults to time.Now.
	Now func() time.Time

	MountTimeout  time.Duration
	StatTimeout   time.Duration
	WatchInterval time.Duration
	BackoffBase   time.Duration
	BackoffMax    time.Duration
}

// State is what Status reports about one storage.
type State struct {
	Mounted bool
	Error   string
}

// health is the executor's memory for one storage: the last error the operator
// should see, and the watchdog's retry schedule.
type health struct {
	lastErr  string
	failures int
	nextTry  time.Time
}

// Executor mounts, unmounts, tests and watches storages. Mount, Unmount and
// Test take a per-storage lock, and so does the watchdog, so no two of them
// touch one storage at once.
type Executor struct {
	st  Store
	opt Options

	mu     sync.Mutex
	locks  map[string]*sync.Mutex
	health map[string]*health
	// busy marks a storage whose kernel call is still running, usually after a
	// timeout. A busy storage is not mounted or unmounted again until it returns.
	busy map[string]bool
}

// New returns an executor. Nothing runs until Startup, Watch or Run, or until
// the API calls one of its methods.
func New(st Store, opt Options) *Executor {
	if opt.Root == "" {
		opt.Root = DefaultRoot
	}
	if opt.ScratchDir == "" {
		opt.ScratchDir = os.TempDir()
	}
	if opt.ProcMounts == "" {
		opt.ProcMounts = "/proc/mounts"
	}
	if opt.Mounter == nil {
		opt.Mounter = defaultMounter()
	}
	if opt.ProcStatus == "" {
		opt.ProcStatus = "/proc/self/status"
	}
	if opt.Resolve == nil {
		opt.Resolve = lookupHost
	}
	if opt.BusyWait <= 0 {
		opt.BusyWait = DefaultBusyWait
	}
	if opt.Log == nil {
		opt.Log = slog.Default()
	}
	if opt.Now == nil {
		opt.Now = time.Now
	}
	if opt.MountTimeout <= 0 {
		opt.MountTimeout = DefaultMountTimeout
	}
	if opt.StatTimeout <= 0 {
		opt.StatTimeout = DefaultStatTimeout
	}
	if opt.WatchInterval <= 0 {
		opt.WatchInterval = DefaultWatchInterval
	}
	if opt.BackoffBase <= 0 {
		opt.BackoffBase = DefaultBackoffBase
	}
	if opt.BackoffMax <= 0 {
		opt.BackoffMax = DefaultBackoffMax
	}
	return &Executor{
		st:     st,
		opt:    opt,
		locks:  map[string]*sync.Mutex{},
		health: map[string]*health{},
		busy:   map[string]bool{},
	}
}

// ---- Validation -------------------------------------------------------------

var (
	storageIDRE = regexp.MustCompile(`^[A-Za-z0-9-]{1,64}$`)
	// sourceRE: host:/path, //host/share, with IPv6 brackets and a hidden
	// share's trailing $. It never starts with a dash, so it cannot read as a
	// command line flag.
	sourceRE = regexp.MustCompile(`^[A-Za-z0-9._:/\[\]$][A-Za-z0-9._:/\[\]$-]*$`)
	// optionsRE is the characters a mount option string may hold. Commas
	// separate options; nothing here can start a shell word or a second -o.
	optionsRE = regexp.MustCompile(`^[A-Za-z0-9_.,=-]*$`)
)

// refusedOptions are names the executor sets itself. A stored option with one of
// these names would override the credentials file or the access mode.
var refusedOptions = map[string]bool{
	"credentials": true, "password": true, "pass": true,
	"username": true, "user": true, "domain": true, "rw": true,
}

// validate refuses anything the mount command must not receive. The API checks
// the same rules on save; the executor repeats them because the watchdog reads
// rows the API did not write in this process.
func validate(st *store.Storage) error {
	if st == nil {
		return errors.New("no storage given")
	}
	if err := store.ValidStorageName(st.Name); err != nil {
		return err
	}
	if !storageIDRE.MatchString(st.ID) {
		return fmt.Errorf("storage id %q is not safe to use in a file name", st.ID)
	}
	switch st.Protocol {
	case "nfs", "smb":
	default:
		return fmt.Errorf("unknown protocol %q", st.Protocol)
	}
	if !sourceRE.MatchString(st.Source) {
		return fmt.Errorf("source %q is not a valid %s source", st.Source, st.Protocol)
	}
	if !optionsRE.MatchString(st.Options) {
		return errors.New("mount options hold a character that is not allowed")
	}
	for _, opt := range splitOptions(st.Options) {
		name, _, _ := strings.Cut(opt, "=")
		if refusedOptions[name] {
			return fmt.Errorf("mount option %q is set by the app, not by the storage", name)
		}
		if st.ReadOnly && name == "rw" {
			return errors.New("a read only storage cannot take the rw option")
		}
	}
	if st.Protocol == "smb" {
		for _, field := range []string{st.Username, st.Domain} {
			if strings.ContainsAny(field, "\r\n\x00") {
				return errors.New("credentials cannot contain a line break")
			}
		}
	}
	return nil
}

// splitOptions returns the non-empty comma separated options.
func splitOptions(s string) []string {
	var out []string
	for _, part := range strings.Split(s, ",") {
		if part = strings.TrimSpace(part); part != "" {
			out = append(out, part)
		}
	}
	return out
}

// Mountpoint is where a storage is mounted: Root/<name>.
func (e *Executor) Mountpoint(st *store.Storage) string {
	return filepath.Join(e.opt.Root, st.Name)
}

// CredentialsPath is the SMB credentials file for one storage. It is fixed per
// storage so the per-storage lock is enough to keep two mounts apart.
func (e *Executor) CredentialsPath(st *store.Storage) string {
	return filepath.Join(e.opt.ScratchDir, "cifs-"+st.ID+".cred")
}

func hasOption(opts []string, name string) bool {
	for _, o := range opts {
		if o == name {
			return true
		}
	}
	return false
}

// writeCredentials writes the cifs credentials file with mode 0600. The file is
// created exclusively, after any stale copy from a crash is removed.
func writeCredentials(path string, st *store.Storage, password string) error {
	if strings.ContainsAny(password, "\r\n\x00") {
		return errors.New("the storage password cannot contain a line break")
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("remove stale credentials: %w", err)
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("create credentials file: %w", err)
	}
	var b strings.Builder
	fmt.Fprintf(&b, "username=%s\n", st.Username)
	fmt.Fprintf(&b, "password=%s\n", password)
	if st.Domain != "" {
		fmt.Fprintf(&b, "domain=%s\n", st.Domain)
	}
	_, werr := f.WriteString(b.String())
	cerr := f.Close()
	if werr != nil {
		_ = os.Remove(path)
		return fmt.Errorf("write credentials file: %w", werr)
	}
	if cerr != nil {
		_ = os.Remove(path)
		return fmt.Errorf("close credentials file: %w", cerr)
	}
	return nil
}

// ---- Locks and health -------------------------------------------------------

func (e *Executor) lock(id string) func() {
	e.mu.Lock()
	m, ok := e.locks[id]
	if !ok {
		m = &sync.Mutex{}
		e.locks[id] = m
	}
	e.mu.Unlock()
	m.Lock()
	return m.Unlock
}

func (e *Executor) healthOf(id string) *health {
	h, ok := e.health[id]
	if !ok {
		h = &health{}
		e.health[id] = h
	}
	return h
}

// noteError records the message the operator should see for a storage.
func (e *Executor) noteError(id string, err error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.healthOf(id).lastErr = err.Error()
}

// noteSuccess clears the recorded error and the watchdog backoff.
func (e *Executor) noteSuccess(id string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	h := e.healthOf(id)
	h.lastErr = ""
	h.failures = 0
	h.nextTry = time.Time{}
}

// scheduleRetry is the watchdog's failure path: it counts the failure and sets
// the next attempt after an exponential backoff.
func (e *Executor) scheduleRetry(id string, err error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	h := e.healthOf(id)
	h.lastErr = err.Error()
	h.failures++
	h.nextTry = e.opt.Now().Add(e.backoff(h.failures))
}

// backoff is BackoffBase doubled per failure, capped at BackoffMax.
func (e *Executor) backoff(failures int) time.Duration {
	d := e.opt.BackoffBase
	for i := 1; i < failures; i++ {
		d *= 2
		if d >= e.opt.BackoffMax {
			return e.opt.BackoffMax
		}
	}
	if d > e.opt.BackoffMax {
		return e.opt.BackoffMax
	}
	return d
}

// due reports whether the watchdog may attempt a remount now.
func (e *Executor) due(id string) bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	h, ok := e.health[id]
	return !ok || !e.opt.Now().Before(h.nextTry)
}

// ---- /proc/mounts ----------------------------------------------------------

// unescapeMount undoes the octal escapes the kernel uses in /proc/mounts.
var mountEscapes = strings.NewReplacer(`\040`, " ", `\011`, "\t", `\012`, "\n", `\134`, `\`)

// isMounted reports whether mountpoint appears as a mount target in the mount
// table. It reads the table only and never touches the mounted filesystem, so a
// hung share cannot block it.
func (e *Executor) isMounted(mountpoint string) (bool, error) {
	data, err := os.ReadFile(e.opt.ProcMounts)
	if err != nil {
		return false, fmt.Errorf("read mount table: %w", err)
	}
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 2 && mountEscapes.Replace(fields[1]) == mountpoint {
			return true, nil
		}
	}
	return false, nil
}

// ---- Bounded filesystem access ---------------------------------------------

// bounded runs fn and gives up after d. A hung share makes fn block forever;
// the watchdog must not block with it, so fn runs in its own goroutine and is
// left behind on timeout.
func bounded(ctx context.Context, d time.Duration, fn func() error) error {
	done := make(chan error, 1)
	go func() { done <- fn() }()
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case err := <-done:
		return err
	case <-timer.C:
		return fmt.Errorf("the share did not answer within %s", d)
	case <-ctx.Done():
		return ctx.Err()
	}
}

// errBusy is returned while an earlier kernel call for the same storage has not
// returned yet.
var errBusy = errors.New("a mount or unmount of this storage is still running; it is tried again once that finishes")

// callBounded runs one kernel call for a storage and waits at most d. A
// mount(2) cannot be cancelled, so on timeout the call keeps running in its own
// goroutine, and the storage stays busy until it returns. No second mount or
// unmount of that storage starts meanwhile, so a late success cannot stack a
// second mount on the first.
func (e *Executor) callBounded(ctx context.Context, id string, d time.Duration, fn func() error) error {
	e.mu.Lock()
	if e.busy[id] {
		e.mu.Unlock()
		return errBusy
	}
	e.busy[id] = true
	e.mu.Unlock()

	done := make(chan error, 1)
	go func() {
		err := fn()
		e.mu.Lock()
		delete(e.busy, id)
		e.mu.Unlock()
		done <- err
	}()
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case err := <-done:
		return err
	case <-timer.C:
		return fmt.Errorf("the mount call did not finish within %s; it may still be running, and this storage is not tried again until it returns", d)
	case <-ctx.Done():
		return ctx.Err()
	}
}

// umount unmounts target without forcing. Only EBUSY earns a retry: wait
// BusyWait, then one lazy detach. The first attempt never detaches.
func (e *Executor) umount(target string) error {
	err := e.opt.Mounter.Unmount(target, 0)
	if errors.Is(err, syscall.EBUSY) {
		e.opt.Log.Warn("storage busy; retrying with a lazy unmount", "target", target)
		time.Sleep(e.opt.BusyWait)
		err = e.opt.Mounter.Unmount(target, mntDetach)
	}
	return umountErr(target, err)
}

// unmountTarget runs umount for a storage under callBounded.
func (e *Executor) unmountTarget(ctx context.Context, id, target string) error {
	return e.callBounded(ctx, id, e.opt.MountTimeout, func() error { return e.umount(target) })
}

// ---- Mount, unmount, test ---------------------------------------------------

// mountOnce mounts a storage whose row has passed validate and the gate. The
// caller holds the storage lock. A storage already in the mount table is left
// as it is and counts as mounted.
func (e *Executor) mountOnce(ctx context.Context, st *store.Storage) error {
	mp := e.Mountpoint(st)
	mounted, err := e.isMounted(mp)
	if err != nil {
		return err
	}
	if mounted {
		return nil
	}
	if err := os.MkdirAll(mp, 0o755); err != nil {
		return fmt.Errorf("create mountpoint %s: %w", mp, err)
	}

	credsPath := ""
	if st.Protocol == "smb" {
		credsPath = e.CredentialsPath(st)
		if strings.ContainsAny(credsPath, ", \t\n") {
			return fmt.Errorf("scratch directory %q cannot hold a comma or a space", e.opt.ScratchDir)
		}
		if err := os.MkdirAll(e.opt.ScratchDir, 0o700); err != nil {
			return fmt.Errorf("create scratch directory: %w", err)
		}
		password, err := e.st.StorageSecret(ctx, st.ID)
		if err != nil {
			return fmt.Errorf("storage password cannot be opened: %w", err)
		}
		// Removed on every path out of this function, success or failure.
		defer func() { _ = os.Remove(credsPath) }()
		if err := writeCredentials(credsPath, st, password); err != nil {
			return err
		}
	}

	spec := BuildMountSpec(st, mp, credsPath)
	spec, err = e.withServerAddr(ctx, spec)
	if err != nil {
		return err
	}
	return e.callBounded(ctx, st.ID, e.opt.MountTimeout, func() error {
		return mountErr(spec, e.opt.Mounter.Mount(spec.Source, spec.Target, spec.Fstype, spec.Flags, spec.Data))
	})
}

// checked runs validation and the host gate. Every mount-side path calls it
// before it touches the mount table.
func (e *Executor) checked(ctx context.Context, st *store.Storage) error {
	if err := validate(st); err != nil {
		return err
	}
	if e.opt.Allow != nil {
		if err := e.opt.Allow(ctx, st); err != nil {
			return err
		}
	}
	return nil
}

// Mount mounts a storage, under its lock. The error, when there is one, names
// the mount(2) call and what the errno means. The error is also kept for Status.
func (e *Executor) Mount(ctx context.Context, st *store.Storage) error {
	unlock := e.lock(st.ID)
	defer unlock()
	err := e.checked(ctx, st)
	if err == nil {
		err = e.mountOnce(ctx, st)
	}
	if err != nil {
		e.noteError(st.ID, err)
		e.opt.Log.Warn("storage mount failed", "storage", st.Name, "error", err)
		return err
	}
	e.noteSuccess(st.ID)
	e.opt.Log.Info("storage mounted", "storage", st.Name, "mountpoint", e.Mountpoint(st))
	return nil
}

// Unmount unmounts a storage, under its lock. A storage that is not mounted is
// not an error.
func (e *Executor) Unmount(ctx context.Context, st *store.Storage) error {
	unlock := e.lock(st.ID)
	defer unlock()
	if err := validate(st); err != nil {
		return err
	}
	mp := e.Mountpoint(st)
	mounted, err := e.isMounted(mp)
	if err != nil {
		e.noteError(st.ID, err)
		return err
	}
	if !mounted {
		return nil
	}
	if err := e.unmountTarget(ctx, st.ID, mp); err != nil {
		e.noteError(st.ID, err)
		e.opt.Log.Warn("storage unmount failed", "storage", st.Name, "error", err)
		return err
	}
	e.noteSuccess(st.ID)
	e.opt.Log.Info("storage unmounted", "storage", st.Name)
	return nil
}

// Timings is what one Test took, in milliseconds. It is logged, not returned,
// because the API answers {ok, error}.
type Timings struct {
	MountMS   int64
	StatMS    int64
	ProbeMS   int64
	UnmountMS int64
}

// Test mounts a storage if it is not mounted, stats the mountpoint, writes and
// removes a probe file unless the storage is read only, then unmounts what it
// mounted. A storage that was already mounted is probed in place and left
// mounted. The error is the first failure, and the unmount always runs.
func (e *Executor) Test(ctx context.Context, st *store.Storage) error {
	unlock := e.lock(st.ID)
	defer unlock()
	err := e.checked(ctx, st)
	var t Timings
	if err == nil {
		t, err = e.test(ctx, st)
	}
	if err != nil {
		e.noteError(st.ID, err)
		e.opt.Log.Warn("storage test failed", "storage", st.Name, "error", err)
		return err
	}
	e.noteSuccess(st.ID)
	e.opt.Log.Info("storage test passed", "storage", st.Name,
		"mount_ms", t.MountMS, "stat_ms", t.StatMS, "probe_ms", t.ProbeMS, "unmount_ms", t.UnmountMS)
	return nil
}

func (e *Executor) test(ctx context.Context, st *store.Storage) (t Timings, err error) {
	mp := e.Mountpoint(st)
	wasMounted, err := e.isMounted(mp)
	if err != nil {
		return t, err
	}
	ownMount := !wasMounted
	start := time.Now()
	if ownMount {
		if err := e.mountOnce(ctx, st); err != nil {
			return t, err
		}
	}
	t.MountMS = time.Since(start).Milliseconds()
	if ownMount {
		defer func() {
			// The unmount runs even when a check failed. Its error only replaces
			// the result when the checks passed.
			start := time.Now()
			if uerr := e.unmountTarget(context.Background(), st.ID, mp); uerr != nil && err == nil {
				err = fmt.Errorf("unmount after test: %w", uerr)
			}
			t.UnmountMS = time.Since(start).Milliseconds()
		}()
	}

	start = time.Now()
	if serr := bounded(ctx, e.opt.StatTimeout, func() error {
		_, err := os.Stat(mp)
		return err
	}); serr != nil {
		return t, fmt.Errorf("stat %s: %w", mp, serr)
	}
	t.StatMS = time.Since(start).Milliseconds()

	if !st.ReadOnly {
		start = time.Now()
		probe := filepath.Join(mp, probeName)
		if perr := bounded(ctx, e.opt.StatTimeout, func() error {
			if err := os.WriteFile(probe, []byte("shrinkyourshit\n"), 0o600); err != nil {
				return err
			}
			return os.Remove(probe)
		}); perr != nil {
			return t, fmt.Errorf("write test file in %s: %w", mp, perr)
		}
		t.ProbeMS = time.Since(start).Milliseconds()
	}
	return t, nil
}

// Status reports whether a storage is in the mount table and the last error the
// executor recorded for it. It reads the mount table only.
func (e *Executor) Status(st *store.Storage) State {
	mounted, err := e.isMounted(e.Mountpoint(st))
	if err != nil {
		e.opt.Log.Debug("storage status: mount table unreadable", "storage", st.Name, "error", err)
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	msg := ""
	if h, ok := e.health[st.ID]; ok {
		msg = h.lastErr
	}
	return State{Mounted: mounted, Error: msg}
}

// ---- Startup and watchdog ---------------------------------------------------

// Startup remounts every enabled storage once. It logs one line per storage
// and never returns an error: one broken share must not stop the server.
func (e *Executor) Startup(ctx context.Context) {
	list, err := e.st.ListStorages(ctx)
	if err != nil {
		e.opt.Log.Error("storage startup: list storages", "error", err)
		return
	}
	for _, st := range list {
		if !st.Enabled {
			continue
		}
		unlock := e.lock(st.ID)
		err := e.checked(ctx, st)
		if err == nil {
			err = e.mountOnce(ctx, st)
		}
		unlock()
		if err != nil {
			e.scheduleRetry(st.ID, err)
			e.opt.Log.Warn("storage startup mount failed", "storage", st.Name, "error", err)
			continue
		}
		e.noteSuccess(st.ID)
		e.opt.Log.Info("storage startup mount ok", "storage", st.Name)
	}
}

// Watch is one watchdog pass. A missing enabled mount is remounted, once it is
// due under the backoff. A mount that is listed but does not answer a stat
// within StatTimeout is reported in Status.Error and left alone, so a hung share
// cannot stall the pass.
func (e *Executor) Watch(ctx context.Context) {
	list, err := e.st.ListStorages(ctx)
	if err != nil {
		e.opt.Log.Error("storage watchdog: list storages", "error", err)
		return
	}
	for _, st := range list {
		if !st.Enabled {
			continue
		}
		if err := validate(st); err != nil {
			e.noteError(st.ID, err)
			continue
		}
		mp := e.Mountpoint(st)
		mounted, err := e.isMounted(mp)
		if err != nil {
			e.opt.Log.Warn("storage watchdog: mount table unreadable", "storage", st.Name, "error", err)
			continue
		}
		if mounted {
			e.checkResponsive(ctx, st, mp)
			continue
		}
		if !e.due(st.ID) {
			continue
		}
		unlock := e.lock(st.ID)
		err = e.checked(ctx, st)
		if err == nil {
			err = e.mountOnce(ctx, st)
		}
		unlock()
		if err != nil {
			e.scheduleRetry(st.ID, err)
			e.opt.Log.Warn("storage watchdog remount failed", "storage", st.Name, "error", err)
			continue
		}
		e.noteSuccess(st.ID)
		e.opt.Log.Info("storage remounted by watchdog", "storage", st.Name)
	}
}

// checkResponsive stats a mounted path with a short timeout. A failure is kept
// for Status. A clean stat clears a stale report from an earlier pass.
func (e *Executor) checkResponsive(ctx context.Context, st *store.Storage, mp string) {
	err := bounded(ctx, e.opt.StatTimeout, func() error {
		_, err := os.Stat(mp)
		return err
	})
	if err != nil {
		e.noteError(st.ID, fmt.Errorf("%s is listed as mounted but does not respond: %w", mp, err))
		e.opt.Log.Warn("storage watchdog: mount does not respond", "storage", st.Name, "error", err)
		return
	}
	e.noteSuccess(st.ID)
}

// Run does the startup remount, then runs the watchdog every WatchInterval until
// ctx ends. It is meant to run in its own goroutine.
func (e *Executor) Run(ctx context.Context) {
	e.checkCapability()
	e.Startup(ctx)
	ticker := time.NewTicker(e.opt.WatchInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			e.Watch(ctx)
		}
	}
}
