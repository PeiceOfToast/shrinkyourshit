// SPDX-License-Identifier: AGPL-3.0-only

package mount

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/PeiceOfToast/shrinkyourshit/internal/store"
)

// ---- Fakes ------------------------------------------------------------------

// fakeStore serves storage rows and their secrets.
type fakeStore struct {
	rows      []*store.Storage
	secrets   map[string]string
	secretErr error
}

func (f *fakeStore) ListStorages(context.Context) ([]*store.Storage, error) {
	return f.rows, nil
}

func (f *fakeStore) StorageSecret(_ context.Context, id string) (string, error) {
	if f.secretErr != nil {
		return "", f.secretErr
	}
	return f.secrets[id], nil
}

// call is one kernel call the fake Mounter saw.
// testNASHost is what the harness resolver answers for every NFS host name.
const testNASHost = "10.0.0.9"

type call struct {
	op     string    // "mount" or "umount"
	spec   MountSpec // set for mount
	target string    // set for umount
	flags  int       // set for umount
}

// credsSeen is what the fake saw in the credentials file while mount ran.
type credsSeen struct {
	path    string
	mode    os.FileMode
	content string
}

// fakeMounter records every kernel call. It keeps a fake mount table in
// procFile, so a successful mount shows up in Status and an unmount removes it.
// It needs no privileges.
type fakeMounter struct {
	procFile string

	mu          sync.Mutex
	calls       []call
	mountErrs   map[string]error // mountpoint -> error from mount(2)
	umountHook  func(target string, flags int) error
	hook        func(c call) // runs before the call takes effect
	delay       time.Duration
	inflight    int
	maxInflight int
	creds       []credsSeen
}

func (f *fakeMounter) Mount(source, target, fstype string, flags uintptr, data string) error {
	c := call{op: "mount", spec: MountSpec{Fstype: fstype, Source: source, Target: target, Flags: flags, Data: data}}
	f.start(c)
	defer f.finish()
	if path := credsPathOf(data); path != "" {
		f.recordCreds(path)
	}
	f.mu.Lock()
	err := f.mountErrs[target]
	f.mu.Unlock()
	if err != nil {
		return err
	}
	f.addMount(source, target)
	return nil
}

func (f *fakeMounter) Unmount(target string, flags int) error {
	f.start(call{op: "umount", target: target, flags: flags})
	defer f.finish()
	if f.umountHook != nil {
		if err := f.umountHook(target, flags); err != nil {
			return err
		}
	}
	f.removeMount(target)
	return nil
}

func (f *fakeMounter) start(c call) {
	f.mu.Lock()
	f.calls = append(f.calls, c)
	f.inflight++
	if f.inflight > f.maxInflight {
		f.maxInflight = f.inflight
	}
	delay, hook := f.delay, f.hook
	f.mu.Unlock()
	if delay > 0 {
		time.Sleep(delay)
	}
	if hook != nil {
		hook(c)
	}
}

func (f *fakeMounter) finish() {
	f.mu.Lock()
	f.inflight--
	f.mu.Unlock()
}

// credsPathOf returns the credentials= path from a mount data string.
func credsPathOf(data string) string {
	for _, opt := range strings.Split(data, ",") {
		if p, ok := strings.CutPrefix(opt, "credentials="); ok {
			return p
		}
	}
	return ""
}

func (f *fakeMounter) recordCreds(path string) {
	seen := credsSeen{path: path}
	if info, err := os.Stat(path); err == nil {
		seen.mode = info.Mode().Perm()
		if data, err := os.ReadFile(path); err == nil {
			seen.content = string(data)
		}
	}
	f.mu.Lock()
	f.creds = append(f.creds, seen)
	f.mu.Unlock()
}

func (f *fakeMounter) addMount(src, mp string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	data, _ := os.ReadFile(f.procFile)
	line := fmt.Sprintf("%s %s nfs rw 0 0\n", src, mp)
	_ = os.WriteFile(f.procFile, append(data, []byte(line)...), 0o644)
}

func (f *fakeMounter) removeMount(mp string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	data, _ := os.ReadFile(f.procFile)
	var keep []string
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 2 && fields[1] == mp {
			continue
		}
		if line != "" {
			keep = append(keep, line)
		}
	}
	out := ""
	for _, k := range keep {
		out += k + "\n"
	}
	_ = os.WriteFile(f.procFile, []byte(out), 0o644)
}

// vanish drops a mount from the table without a umount, as a stale or lost
// share looks from the outside.
func (f *fakeMounter) vanish(mp string) { f.removeMount(mp) }

func (f *fakeMounter) allCalls() []call {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]call(nil), f.calls...)
}

func (f *fakeMounter) mountCount() int {
	n := 0
	for _, c := range f.allCalls() {
		if c.op == "mount" {
			n++
		}
	}
	return n
}

// ---- Harness ----------------------------------------------------------------

type harness struct {
	t       *testing.T
	exec    *Executor
	st      *fakeStore
	run     *fakeMounter
	root    string
	scratch string
	proc    string
	now     time.Time
}

func newHarness(t *testing.T, opt func(*Options), rows ...*store.Storage) *harness {
	t.Helper()
	dir := t.TempDir()
	h := &harness{
		t:       t,
		st:      &fakeStore{rows: rows, secrets: map[string]string{}},
		root:    filepath.Join(dir, "storage"),
		scratch: filepath.Join(dir, "scratch"),
		proc:    filepath.Join(dir, "mounts"),
		now:     time.Date(2026, 11, 6, 12, 0, 0, 0, time.UTC),
	}
	if err := os.MkdirAll(h.scratch, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(h.proc, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	h.run = &fakeMounter{procFile: h.proc, mountErrs: map[string]error{}}
	o := Options{
		Root:         h.root,
		ScratchDir:   h.scratch,
		ProcMounts:   h.proc,
		Mounter:      h.run,
		Log:          slog.New(slog.NewTextHandler(io.Discard, nil)),
		Now:          func() time.Time { return h.now },
		MountTimeout: 5 * time.Second,
		StatTimeout:  200 * time.Millisecond,
		BackoffBase:  10 * time.Second,
		BackoffMax:   10 * time.Minute,
		BusyWait:     time.Millisecond,
		// The lab fixtures use host name sources. The kernel cannot resolve
		// those itself, so the executor does it in userspace; tests answer
		// without touching DNS.
		Resolve: func(context.Context, string) ([]string, error) { return []string{testNASHost}, nil },
	}
	if opt != nil {
		opt(&o)
	}
	h.exec = New(h.st, o)
	return h
}

func (h *harness) mountpoint(name string) string { return filepath.Join(h.root, name) }

func nfsStorage(id, name string) *store.Storage {
	return &store.Storage{
		ID: id, Name: name, Protocol: "nfs", Source: "nas:/export",
		Options: "vers=4.2,soft,timeo=50,retrans=2", Enabled: true,
	}
}

func smbStorage(id, name string) *store.Storage {
	return &store.Storage{
		ID: id, Name: name, Protocol: "smb", Source: "//nas/media",
		Options: "vers=3.0", Username: "bob", Domain: "WORK", HasSecret: true, Enabled: true,
	}
}

// ---- Mount specs and option translation ------------------------------------

func TestBuildMountSpecNFSExact(t *testing.T) {
	st := nfsStorage("st-1", "media")
	got := BuildMountSpec(st, "/storage/media", "")
	want := MountSpec{Fstype: "nfs", Source: "nas:/export", Target: "/storage/media", Flags: 0, Data: "vers=4.2,soft,timeo=50,retrans=2"}
	if got != want {
		t.Fatalf("nfs spec:\n got %+v\nwant %+v", got, want)
	}
}

func TestBuildMountSpecNFSReadOnlyAddsRoFlag(t *testing.T) {
	st := nfsStorage("st-1", "media")
	st.Options = "vers=3"
	st.ReadOnly = true
	got := BuildMountSpec(st, "/storage/media", "")
	want := MountSpec{Fstype: "nfs", Source: "nas:/export", Target: "/storage/media", Flags: msRdOnly, Data: "vers=3"}
	if got != want {
		t.Fatalf("nfs read only spec:\n got %+v\nwant %+v", got, want)
	}
}

func TestBuildMountSpecNFSNoOptionsIsEmptyData(t *testing.T) {
	st := nfsStorage("st-1", "media")
	st.Options = ""
	got := BuildMountSpec(st, "/storage/media", "")
	if got.Data != "" || got.Flags != 0 || got.Fstype != "nfs" {
		t.Fatalf("nfs no options spec = %+v", got)
	}
}

func TestBuildMountSpecSMBExactWithCredentialsPath(t *testing.T) {
	st := smbStorage("st-2", "media")
	got := BuildMountSpec(st, "/storage/media", "/scratch/cifs-st-2.cred")
	want := MountSpec{Fstype: "cifs", Source: "//nas/media", Target: "/storage/media", Flags: 0,
		Data: "credentials=/scratch/cifs-st-2.cred,vers=3.0"}
	if got != want {
		t.Fatalf("smb spec:\n got %+v\nwant %+v", got, want)
	}
}

func TestBuildMountSpecSMBReadOnlyExact(t *testing.T) {
	st := smbStorage("st-2", "media")
	st.ReadOnly = true
	got := BuildMountSpec(st, "/storage/media", "/scratch/cifs-st-2.cred")
	want := MountSpec{Fstype: "cifs", Source: "//nas/media", Target: "/storage/media", Flags: msRdOnly,
		Data: "credentials=/scratch/cifs-st-2.cred,vers=3.0"}
	if got != want {
		t.Fatalf("smb read only spec:\n got %+v\nwant %+v", got, want)
	}
}

// The option translation table. Flags become bits, the userspace-only names are
// dropped, and everything else reaches data in its original order.
func TestTranslateOptionsTable(t *testing.T) {
	all := msRdOnly | msNoAtime | msNoDev | msNoExec | msNoSuid
	cases := []struct {
		name  string
		opts  []string
		flags uintptr
		data  string
	}{
		{"flags become bits", []string{"ro", "noatime", "nodev", "noexec", "nosuid"}, all, ""},
		{"ro alone", []string{"ro"}, msRdOnly, ""},
		{"userspace only names are dropped", []string{"_netdev", "defaults"}, 0, ""},
		{"rw is the kernel default", []string{"rw"}, 0, ""},
		{"data keeps its order", []string{"vers=4.2", "soft", "timeo=50", "retrans=2"}, 0, "vers=4.2,soft,timeo=50,retrans=2"},
		{"flags are removed from the data", []string{"vers=4.2", "ro", "soft"}, msRdOnly, "vers=4.2,soft"},
		{"a value keeps a flag name as data", []string{"credentials=/x", "ro"}, msRdOnly, "credentials=/x"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			flags, data := translateOptions(tc.opts)
			if flags != tc.flags || data != tc.data {
				t.Fatalf("translateOptions(%q) = flags %#x data %q, want flags %#x data %q",
					tc.opts, flags, data, tc.flags, tc.data)
			}
		})
	}
}

// The data string that reaches mount(2) for the spec 4.4 example, in order.
func TestMountDataKeepsOrderForVers42(t *testing.T) {
	st := nfsStorage("st-1", "media")
	st.Options = "vers=4.2,soft,timeo=50,retrans=2"
	got := BuildMountSpec(st, "/storage/media", "")
	if got.Data != "vers=4.2,soft,timeo=50,retrans=2" {
		t.Fatalf("data = %q", got.Data)
	}
}

// The spec that the fake kernel receives, for both protocols, with the real
// credentials path the executor chose.
func TestMountRunsExactSpecNFS(t *testing.T) {
	h := newHarness(t, nil, nfsStorage("st-1", "media"))
	if err := h.exec.Mount(context.Background(), h.st.rows[0]); err != nil {
		t.Fatalf("mount: %v", err)
	}
	calls := h.run.allCalls()
	if len(calls) != 1 {
		t.Fatalf("calls = %d, want 1: %+v", len(calls), calls)
	}
	want := MountSpec{Fstype: "nfs", Source: "nas:/export", Target: h.mountpoint("media"),
		Data: "vers=4.2,soft,timeo=50,retrans=2,addr=" + testNASHost}
	if calls[0].op != "mount" || calls[0].spec != want {
		t.Fatalf("mount call:\n got %+v\nwant %+v", calls[0].spec, want)
	}
}

func TestMountRunsExactSpecSMBWithCredentialsPath(t *testing.T) {
	h := newHarness(t, nil, smbStorage("st-2", "media"))
	h.st.secrets["st-2"] = "s3cret"
	if err := h.exec.Mount(context.Background(), h.st.rows[0]); err != nil {
		t.Fatalf("mount: %v", err)
	}
	creds := filepath.Join(h.scratch, "cifs-st-2.cred")
	want := MountSpec{Fstype: "cifs", Source: "//nas/media", Target: h.mountpoint("media"),
		Data: "credentials=" + creds + ",vers=3.0"}
	calls := h.run.allCalls()
	if len(calls) != 1 || calls[0].spec != want {
		t.Fatalf("mount call:\n got %+v\nwant %+v", calls, want)
	}
}

// ---- Errno messages ---------------------------------------------------------

// The operator sees these strings in the storage row and the log, so the exact
// text is asserted.
func TestMountErrnoMessages(t *testing.T) {
	const capHint = "not permitted; the container needs cap_add: SYS_ADMIN and security_opt: apparmor=unconfined"
	cases := []struct {
		name  string
		proto string
		errno syscall.Errno
		want  func(mp string) string
	}{
		{"EPERM on nfs", "nfs", syscall.EPERM, func(mp string) string {
			return "mount -t nfs nas:/export at " + mp + ": " + capHint
		}},
		{"EACCES on cifs", "smb", syscall.EACCES, func(mp string) string {
			return "mount -t cifs //nas/media at " + mp + ": " + capHint
		}},
		{"ENODEV on nfs names nfs-common", "nfs", syscall.ENODEV, func(mp string) string {
			return "mount -t nfs nas:/export at " + mp + ": no nfs client is available; the image needs the nfs-common package"
		}},
		{"ENODEV on cifs names cifs-utils", "smb", syscall.ENODEV, func(mp string) string {
			return "mount -t cifs //nas/media at " + mp + ": no cifs client is available; the image needs the cifs-utils package"
		}},
		{"ETIMEDOUT names the source", "nfs", syscall.ETIMEDOUT, func(mp string) string {
			return "mount -t nfs nas:/export at " + mp + ": the server did not answer (nas:/export); check the host name and that it is reachable from the container"
		}},
		{"EHOSTUNREACH names the source", "smb", syscall.EHOSTUNREACH, func(mp string) string {
			return "mount -t cifs //nas/media at " + mp + ": the server did not answer (//nas/media); check the host name and that it is reachable from the container"
		}},
		{"default is the errno text", "nfs", syscall.EINVAL, func(mp string) string {
			return "mount -t nfs nas:/export at " + mp + ": " + syscall.EINVAL.Error()
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			st := nfsStorage("st-1", "media")
			if tc.proto == "smb" {
				st = smbStorage("st-1", "media")
				st.Protocol = "smb"
			}
			h := newHarness(t, nil, st)
			h.st.secrets["st-1"] = "s3cret"
			h.run.mountErrs[h.mountpoint("media")] = tc.errno
			err := h.exec.Mount(context.Background(), st)
			if err == nil {
				t.Fatal("expected the mount to fail")
			}
			if got, want := err.Error(), tc.want(h.mountpoint("media")); got != want {
				t.Fatalf("message:\n got %q\nwant %q", got, want)
			}
			if !errors.Is(err, tc.errno) {
				t.Fatalf("error %v does not wrap %v", err, tc.errno)
			}
			if got := h.exec.Status(st); got.Error != err.Error() {
				t.Fatalf("status error = %q, want the same message", got.Error)
			}
		})
	}
}

// ---- Credentials file -------------------------------------------------------

func TestCredentialsFileIsOwnerOnlyAndGoneAfterSuccess(t *testing.T) {
	h := newHarness(t, nil, smbStorage("st-2", "media"))
	h.st.secrets["st-2"] = "s3cret"
	if err := h.exec.Mount(context.Background(), h.st.rows[0]); err != nil {
		t.Fatalf("mount: %v", err)
	}
	if len(h.run.creds) != 1 {
		t.Fatalf("creds seen = %d, want 1", len(h.run.creds))
	}
	seen := h.run.creds[0]
	if seen.mode != 0o600 {
		t.Fatalf("credentials mode = %o, want 600", seen.mode)
	}
	wantContent := "username=bob\npassword=s3cret\ndomain=WORK\n"
	if seen.content != wantContent {
		t.Fatalf("credentials content = %q, want %q", seen.content, wantContent)
	}
	if _, err := os.Stat(seen.path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("credentials file still exists after a successful mount: %v", err)
	}
}

func TestCredentialsFileGoneAfterFailure(t *testing.T) {
	h := newHarness(t, nil, smbStorage("st-2", "media"))
	h.st.secrets["st-2"] = "s3cret"
	h.run.mountErrs[h.mountpoint("media")] = syscall.EACCES
	if err := h.exec.Mount(context.Background(), h.st.rows[0]); err == nil {
		t.Fatal("expected the mount to fail")
	}
	if len(h.run.creds) != 1 {
		t.Fatalf("creds seen = %d, want 1", len(h.run.creds))
	}
	if _, serr := os.Stat(h.run.creds[0].path); !errors.Is(serr, os.ErrNotExist) {
		t.Fatalf("credentials file still exists after a failed mount: %v", serr)
	}
}

func TestSecretReadFailureMountsNothing(t *testing.T) {
	h := newHarness(t, nil, smbStorage("st-2", "media"))
	h.st.secretErr = errors.New("storage key missing")
	if err := h.exec.Mount(context.Background(), h.st.rows[0]); err == nil {
		t.Fatal("expected a secret read failure to fail the mount")
	}
	if n := h.run.mountCount(); n != 0 {
		t.Fatalf("mount ran %d times with no secret", n)
	}
}

// ---- Failure is named; the server keeps going ------------------------------

func TestMountFailureIsNamedAndStartupContinues(t *testing.T) {
	bad := nfsStorage("st-bad", "broken")
	good := nfsStorage("st-good", "media")
	h := newHarness(t, nil, bad, good)
	h.run.mountErrs[h.mountpoint("broken")] = syscall.EPERM
	wantErr := "mount -t nfs nas:/export at " + h.mountpoint("broken") + ": not permitted; the container needs cap_add: SYS_ADMIN and security_opt: apparmor=unconfined"

	// Startup must not panic or return anything the caller could treat as a
	// server-stopping error. The good storage still comes up.
	h.exec.Startup(context.Background())

	if got := h.exec.Status(bad); got.Mounted || got.Error != wantErr {
		t.Fatalf("broken status = %+v, want unmounted with %q", got, wantErr)
	}
	if got := h.exec.Status(good); !got.Mounted || got.Error != "" {
		t.Fatalf("good status = %+v, want mounted and no error", got)
	}
}

// ---- Unmount ----------------------------------------------------------------

// EBUSY on the first attempt is retried once with MNT_DETACH.
func TestUnmountBusyRetriesOnceWithDetach(t *testing.T) {
	st := nfsStorage("st-1", "media")
	h := newHarness(t, nil, st)
	ctx := context.Background()
	if err := h.exec.Mount(ctx, st); err != nil {
		t.Fatalf("mount: %v", err)
	}
	h.run.umountHook = func(_ string, flags int) error {
		if flags == 0 {
			return syscall.EBUSY
		}
		return nil
	}
	if err := h.exec.Unmount(ctx, st); err != nil {
		t.Fatalf("unmount: %v", err)
	}
	var umounts []int
	for _, c := range h.run.allCalls() {
		if c.op == "umount" {
			umounts = append(umounts, c.flags)
		}
	}
	if !reflect.DeepEqual(umounts, []int{0, mntDetach}) {
		t.Fatalf("umount flags = %v, want [0 MNT_DETACH]", umounts)
	}
	if got := h.exec.Status(st); got.Mounted || got.Error != "" {
		t.Fatalf("status after detach = %+v", got)
	}
}

// The first attempt never forces a detach. A second busy answer is returned.
func TestUnmountNeverDetachesFirstAndReturnsSecondBusy(t *testing.T) {
	st := nfsStorage("st-1", "media")
	h := newHarness(t, nil, st)
	ctx := context.Background()
	if err := h.exec.Mount(ctx, st); err != nil {
		t.Fatalf("mount: %v", err)
	}
	h.run.umountHook = func(string, int) error { return syscall.EBUSY }
	err := h.exec.Unmount(ctx, st)
	want := "umount " + h.mountpoint("media") + ": " + syscall.EBUSY.Error()
	if err == nil || err.Error() != want {
		t.Fatalf("unmount error = %v, want %q", err, want)
	}
	var umounts []int
	for _, c := range h.run.allCalls() {
		if c.op == "umount" {
			umounts = append(umounts, c.flags)
		}
	}
	if !reflect.DeepEqual(umounts, []int{0, mntDetach}) {
		t.Fatalf("umount flags = %v, want [0 MNT_DETACH]", umounts)
	}
}

// Anything other than EBUSY is returned at once, with no retry.
func TestUnmountOtherErrorIsNotRetried(t *testing.T) {
	st := nfsStorage("st-1", "media")
	h := newHarness(t, nil, st)
	ctx := context.Background()
	if err := h.exec.Mount(ctx, st); err != nil {
		t.Fatalf("mount: %v", err)
	}
	h.run.umountHook = func(string, int) error { return syscall.EIO }
	err := h.exec.Unmount(ctx, st)
	want := "umount " + h.mountpoint("media") + ": " + syscall.EIO.Error()
	if err == nil || err.Error() != want {
		t.Fatalf("unmount error = %v, want %q", err, want)
	}
	if n := len(h.run.allCalls()) - 1; n != 1 {
		t.Fatalf("umount attempts = %d, want 1", n)
	}
}

func TestUnmountRunsUmountAndIsIdempotent(t *testing.T) {
	st := nfsStorage("st-1", "media")
	h := newHarness(t, nil, st)
	ctx := context.Background()
	if err := h.exec.Unmount(ctx, st); err != nil {
		t.Fatalf("unmount when not mounted: %v", err)
	}
	if n := len(h.run.allCalls()); n != 0 {
		t.Fatalf("unmount of an unmounted storage made %d kernel calls", n)
	}
	if err := h.exec.Mount(ctx, st); err != nil {
		t.Fatalf("mount: %v", err)
	}
	if err := h.exec.Unmount(ctx, st); err != nil {
		t.Fatalf("unmount: %v", err)
	}
	if got := h.exec.Status(st); got.Mounted {
		t.Fatal("still mounted after unmount")
	}
}

// ---- Timeouts and the busy flag --------------------------------------------

// A mount that outlives MountTimeout is reported, and no second call for the
// same storage starts until the first returns.
func TestTimedOutMountBlocksRetryUntilItReturns(t *testing.T) {
	st := nfsStorage("st-1", "media")
	h := newHarness(t, func(o *Options) { o.MountTimeout = 30 * time.Millisecond }, st)
	release := make(chan struct{})
	defer func() {
		select {
		case <-release:
		default:
			close(release)
		}
	}()
	h.run.hook = func(c call) {
		if c.op == "mount" {
			<-release
		}
	}
	ctx := context.Background()

	err := h.exec.Mount(ctx, st)
	if err == nil || !strings.Contains(err.Error(), "did not finish") {
		t.Fatalf("first mount error = %v, want the timeout", err)
	}
	if err := h.exec.Mount(ctx, st); !errors.Is(err, errBusy) {
		t.Fatalf("second mount error = %v, want the busy refusal", err)
	}
	if n := h.run.mountCount(); n != 1 {
		t.Fatalf("mount(2) calls while busy = %d, want 1", n)
	}

	close(release)
	deadline := time.Now().Add(3 * time.Second)
	for {
		if err := h.exec.Mount(ctx, st); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("storage never became mountable after the blocked call returned")
		}
		time.Sleep(5 * time.Millisecond)
	}
	if got := h.exec.Status(st); !got.Mounted {
		t.Fatalf("status = %+v, want mounted", got)
	}
}

func TestBoundedGivesUpOnAHungCall(t *testing.T) {
	block := make(chan struct{})
	defer close(block)
	start := time.Now()
	err := bounded(context.Background(), 30*time.Millisecond, func() error {
		<-block
		return nil
	})
	if err == nil || !strings.Contains(err.Error(), "did not answer") {
		t.Fatalf("err = %v, want a timeout", err)
	}
	if time.Since(start) > 2*time.Second {
		t.Fatalf("bounded took %s, which is far past its limit", time.Since(start))
	}
}

// ---- Capability check -------------------------------------------------------

func TestCapEffHasSysAdmin(t *testing.T) {
	cases := []struct {
		name    string
		status  string
		want    bool
		wantErr bool
	}{
		{"the deployed image", "Name:\tslserver\nCapEff:\t00000000a82425fb\n", true, false},
		{"bit 21 alone", "CapEff:\t0000000000200000\n", true, false},
		{"no capabilities", "CapEff:\t0000000000000000\n", false, false},
		{"bit 20 is not sys admin", "CapEff:\t0000000000100000\n", false, false},
		{"empty input", "", false, true},
		{"no CapEff line", "Name:\tx\n", false, true},
		{"not hex", "CapEff:\tnothex\n", false, true},
		{"too long for 64 bits", "CapEff:\tffffffffffffffffff\n", false, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := capEffHasSysAdmin(tc.status)
			if (err != nil) != tc.wantErr {
				t.Fatalf("error = %v, want error %v", err, tc.wantErr)
			}
			if got != tc.want {
				t.Fatalf("has SYS_ADMIN = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestCheckCapabilityLogsMissingSysAdminWithoutPanic(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("the capability check runs on Linux only")
	}
	dir := t.TempDir()
	cases := map[string]struct {
		body    string
		wantLog string
	}{
		"missing":    {"CapEff:\t0000000000000000\n", "cap_add: [SYS_ADMIN]"},
		"garbage":    {"not a status file", "cannot read process capabilities"},
		"present":    {"CapEff:\t00000000a82425fb\n", ""},
		"unreadable": {"", "cannot read process capabilities"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(dir, name)
			if name != "unreadable" {
				if err := os.WriteFile(path, []byte(tc.body), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			var buf bytes.Buffer
			e := New(&fakeStore{}, Options{
				ProcStatus: path,
				Log:        slog.New(slog.NewTextHandler(&buf, nil)),
			})
			e.checkCapability() // must not panic
			out := buf.String()
			if tc.wantLog == "" {
				if out != "" {
					t.Fatalf("expected no log, got %q", out)
				}
				return
			}
			if !strings.Contains(out, tc.wantLog) {
				t.Fatalf("log = %q, want it to contain %q", out, tc.wantLog)
			}
			if name == "missing" && !strings.Contains(out, "security_opt: [apparmor=unconfined]") {
				t.Fatalf("log = %q, want the apparmor option named", out)
			}
		})
	}
}

// ---- Non-Linux builds -------------------------------------------------------

// The non-Linux Mounter is compiled and tested on every platform: its calls
// return the Linux error and never reach the kernel.
func TestUnsupportedMounterSaysLinux(t *testing.T) {
	var m Mounter = unsupportedMounter{}
	if err := m.Mount("nas:/export", "/storage/x", "nfs", 0, ""); !errors.Is(err, ErrNeedsLinux) {
		t.Fatalf("mount error = %v, want ErrNeedsLinux", err)
	}
	if err := m.Unmount("/storage/x", 0); !errors.Is(err, ErrNeedsLinux) {
		t.Fatalf("umount error = %v, want ErrNeedsLinux", err)
	}
}

// ---- Watchdog ---------------------------------------------------------------

func TestWatchdogRemountsVanishedMount(t *testing.T) {
	st := nfsStorage("st-1", "media")
	h := newHarness(t, nil, st)
	if err := h.exec.Mount(context.Background(), st); err != nil {
		t.Fatalf("mount: %v", err)
	}
	h.run.vanish(h.mountpoint("media"))
	if got := h.exec.Status(st); got.Mounted {
		t.Fatal("status should show the vanished mount as missing")
	}

	before := h.run.mountCount()
	h.exec.Watch(context.Background())
	if n := h.run.mountCount(); n != before+1 {
		t.Fatalf("watchdog mount calls = %d, want %d", n, before+1)
	}
	if got := h.exec.Status(st); !got.Mounted || got.Error != "" {
		t.Fatalf("after watchdog status = %+v, want mounted and no error", got)
	}
}

func TestWatchdogBacksOffOnRepeatedFailure(t *testing.T) {
	st := nfsStorage("st-1", "media")
	h := newHarness(t, nil, st)
	h.run.mountErrs[h.mountpoint("media")] = syscall.ETIMEDOUT
	ctx := context.Background()

	step := func(at time.Duration, wantCalls int) {
		t.Helper()
		h.now = time.Date(2026, 11, 6, 12, 0, 0, 0, time.UTC).Add(at)
		h.exec.Watch(ctx)
		if n := h.run.mountCount(); n != wantCalls {
			t.Fatalf("at +%s: mount calls = %d, want %d", at, n, wantCalls)
		}
	}
	step(0, 1)              // first attempt, next due at +10s
	step(5*time.Second, 1)  // not due yet
	step(10*time.Second, 2) // due, next due at +10s +20s = +30s
	step(29*time.Second, 2) // not due yet
	step(30*time.Second, 3) // due, next due at +30s +40s = +70s
	step(69*time.Second, 3) // not due yet
	step(70*time.Second, 4) // due again

	got := h.exec.Status(st)
	if got.Mounted || !strings.Contains(got.Error, "did not answer") {
		t.Fatalf("status after failures = %+v, want the message kept", got)
	}
}

func TestBackoffDoublesAndCaps(t *testing.T) {
	e := New(&fakeStore{}, Options{BackoffBase: 10 * time.Second, BackoffMax: 10 * time.Minute})
	cases := map[int]time.Duration{
		1:  10 * time.Second,
		2:  20 * time.Second,
		3:  40 * time.Second,
		4:  80 * time.Second,
		5:  160 * time.Second,
		6:  320 * time.Second,
		7:  10 * time.Minute, // 640s capped
		30: 10 * time.Minute,
	}
	for n, want := range cases {
		if got := e.backoff(n); got != want {
			t.Fatalf("backoff(%d) = %s, want %s", n, got, want)
		}
	}
}

func TestWatchdogReportsStaleMountWithoutRemounting(t *testing.T) {
	st := nfsStorage("st-1", "media")
	h := newHarness(t, nil, st)
	// The mount table says mounted, but the mountpoint does not exist, so the
	// stat fails. A stale share looks like this from the outside.
	h.run.addMount("nas:/export", h.mountpoint("media"))

	h.exec.Watch(context.Background())

	got := h.exec.Status(st)
	if !got.Mounted {
		t.Fatalf("status = %+v, want mounted", got)
	}
	if !strings.Contains(got.Error, "does not respond") {
		t.Fatalf("status error = %q, want the stale report", got.Error)
	}
	if n := h.run.mountCount(); n != 0 {
		t.Fatalf("watchdog remounted a listed mount %d times", n)
	}
}

// ---- Test -------------------------------------------------------------------

func TestTestMountsProbesAndUnmounts(t *testing.T) {
	st := nfsStorage("st-1", "media")
	h := newHarness(t, nil, st)
	if err := h.exec.Test(context.Background(), st); err != nil {
		t.Fatalf("test: %v", err)
	}
	calls := h.run.allCalls()
	if len(calls) != 2 || calls[0].op != "mount" || calls[1].op != "umount" {
		t.Fatalf("calls = %+v, want mount then umount", calls)
	}
	if calls[1].target != h.mountpoint("media") {
		t.Fatalf("umount target = %q", calls[1].target)
	}
	entries, err := os.ReadDir(h.mountpoint("media"))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("probe file left behind: %v", entries)
	}
	if got := h.exec.Status(st); got.Mounted {
		t.Fatal("test should leave the storage unmounted when it mounted it")
	}
}

func TestTestReadOnlySkipsProbeAndStillUnmounts(t *testing.T) {
	st := nfsStorage("st-1", "media")
	st.ReadOnly = true
	h := newHarness(t, nil, st)
	if err := h.exec.Test(context.Background(), st); err != nil {
		t.Fatalf("test: %v", err)
	}
	if got := h.run.allCalls(); len(got) != 2 {
		t.Fatalf("calls = %+v, want mount and umount", got)
	}
}

func TestTestUnmountsEvenWhenTheCheckFails(t *testing.T) {
	st := nfsStorage("st-1", "media")
	h := newHarness(t, nil, st)
	h.run.hook = func(c call) {
		if c.op == "mount" {
			// The mount "succeeds" but the mountpoint is gone, so the stat fails.
			_ = os.RemoveAll(c.spec.Target)
		}
	}
	err := h.exec.Test(context.Background(), st)
	if err == nil || !strings.Contains(err.Error(), "stat") {
		t.Fatalf("test error = %v, want the stat failure", err)
	}
	calls := h.run.allCalls()
	if len(calls) != 2 || calls[1].op != "umount" {
		t.Fatalf("calls = %+v, want an unmount after the failed check", calls)
	}
	if got := h.exec.Status(st); !strings.Contains(got.Error, "stat") {
		t.Fatalf("status error = %q, want the test failure kept", got.Error)
	}
}

func TestTestOnAMountedStorageProbesInPlace(t *testing.T) {
	st := nfsStorage("st-1", "media")
	h := newHarness(t, nil, st)
	h.run.addMount("nas:/export", h.mountpoint("media"))
	if err := os.MkdirAll(h.mountpoint("media"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := h.exec.Test(context.Background(), st); err != nil {
		t.Fatalf("test: %v", err)
	}
	if got := h.run.allCalls(); len(got) != 0 {
		t.Fatalf("test touched a mounted storage: %+v", got)
	}
	if got := h.exec.Status(st); !got.Mounted {
		t.Fatal("test must leave an already mounted storage mounted")
	}
}

// ---- Mount idempotence, gate and validation --------------------------------

func TestMountIsIdempotentWhenAlreadyMounted(t *testing.T) {
	st := nfsStorage("st-1", "media")
	h := newHarness(t, nil, st)
	ctx := context.Background()
	if err := h.exec.Mount(ctx, st); err != nil {
		t.Fatalf("first mount: %v", err)
	}
	if err := h.exec.Mount(ctx, st); err != nil {
		t.Fatalf("second mount: %v", err)
	}
	if n := h.run.mountCount(); n != 1 {
		t.Fatalf("mount ran %d times, want 1", n)
	}
}

func TestHostGateBlocksEveryMountPath(t *testing.T) {
	blocked := nfsStorage("st-1", "media")
	h := newHarness(t, func(o *Options) {
		o.Allow = func(_ context.Context, st *store.Storage) error {
			return errors.New("host nas is not in storage_allowed_hosts")
		}
	}, blocked)
	ctx := context.Background()

	if err := h.exec.Mount(ctx, blocked); err == nil || !strings.Contains(err.Error(), "storage_allowed_hosts") {
		t.Fatalf("mount error = %v, want the allowlist refusal", err)
	}
	if err := h.exec.Test(ctx, blocked); err == nil {
		t.Fatal("test should be refused by the gate")
	}
	h.exec.Startup(ctx)
	h.exec.Watch(ctx)
	if n := len(h.run.allCalls()); n != 0 {
		t.Fatalf("gate refused, but %d kernel calls still ran", n)
	}
}

func TestStartupAndWatchdogSkipDisabledStorages(t *testing.T) {
	off := nfsStorage("st-off", "off")
	off.Enabled = false
	on := nfsStorage("st-on", "on")
	h := newHarness(t, nil, off, on)
	h.exec.Startup(context.Background())
	h.exec.Watch(context.Background())
	calls := h.run.allCalls()
	if len(calls) != 1 || calls[0].spec.Target != h.mountpoint("on") {
		t.Fatalf("calls = %+v, want one mount of the enabled storage", calls)
	}
}

func TestValidateRefusesUnsafeRows(t *testing.T) {
	base := func() *store.Storage { return nfsStorage("st-1", "media") }
	cases := map[string]func(*store.Storage){
		"name with a dot dot":   func(s *store.Storage) { s.Name = "../etc" },
		"id with a slash":       func(s *store.Storage) { s.ID = "a/b" },
		"unknown protocol":      func(s *store.Storage) { s.Protocol = "ftp" },
		"source with a space":   func(s *store.Storage) { s.Source = "nas:/a b" },
		"source with a dash":    func(s *store.Storage) { s.Source = "-o" },
		"options with a shell":  func(s *store.Storage) { s.Options = "vers=3;rm" },
		"options with a dollar": func(s *store.Storage) { s.Options = "vers=$3" },
		"credentials option":    func(s *store.Storage) { s.Options = "credentials=cifs" },
		"rw on read only":       func(s *store.Storage) { s.ReadOnly = true; s.Options = "rw" },
		"smb user line break": func(s *store.Storage) {
			s.Protocol = "smb"
			s.Source = "//nas/media"
			s.Username = "bob\npassword=x"
		},
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			h := newHarness(t, nil, base())
			st := base()
			mutate(st)
			if err := h.exec.Mount(context.Background(), st); err == nil {
				t.Fatal("expected the row to be refused")
			}
			if n := len(h.run.allCalls()); n != 0 {
				t.Fatalf("refused row still made %d kernel calls", n)
			}
		})
	}
}

func TestConcurrentMountsOfOneStorageSerialise(t *testing.T) {
	st := nfsStorage("st-1", "media")
	h := newHarness(t, nil, st)
	h.run.delay = 40 * time.Millisecond
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = h.exec.Mount(context.Background(), st)
		}()
	}
	wg.Wait()
	h.run.mu.Lock()
	peak := h.run.maxInflight
	h.run.mu.Unlock()
	if peak != 1 {
		t.Fatalf("kernel calls in flight at once = %d, want 1", peak)
	}
}

func TestStatusReadsTheMountTableOnly(t *testing.T) {
	st := nfsStorage("st-1", "media")
	h := newHarness(t, nil, st)
	if got := h.exec.Status(st); got.Mounted || got.Error != "" {
		t.Fatalf("fresh status = %+v", got)
	}
	h.run.addMount("nas:/export", h.mountpoint("media"))
	if got := h.exec.Status(st); !got.Mounted {
		t.Fatalf("status = %+v, want mounted", got)
	}
}

func TestMountTableEscapesAreUndone(t *testing.T) {
	if got := mountEscapes.Replace(`/storage/a\040b`); got != "/storage/a b" {
		t.Fatalf("unescaped = %q", got)
	}
}

// The scratch directory may not exist yet on a fresh install. The SMB mount
// creates it instead of failing.
func TestSMBMountCreatesMissingScratchDir(t *testing.T) {
	st := smbStorage("st-2", "media")
	h := newHarness(t, nil, st)
	h.st.secrets["st-2"] = "s3cret"
	if err := os.RemoveAll(h.scratch); err != nil {
		t.Fatal(err)
	}
	if err := h.exec.Mount(context.Background(), st); err != nil {
		t.Fatalf("mount with a missing scratch dir: %v", err)
	}
	if got := h.exec.Status(st); !got.Mounted {
		t.Fatalf("status = %+v, want mounted", got)
	}
}
