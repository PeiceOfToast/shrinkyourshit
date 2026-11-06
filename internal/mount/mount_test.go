// SPDX-License-Identifier: AGPL-3.0-only

package mount

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
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

type call struct {
	program string
	args    []string
}

// credsSeen is what the runner saw in the credentials file while mount ran.
type credsSeen struct {
	path    string
	mode    os.FileMode
	content string
}

// fakeRunner records every call. It keeps a fake mount table in procFile, so a
// successful mount shows up in Status and a umount removes it again.
type fakeRunner struct {
	procFile string

	mu          sync.Mutex
	calls       []call
	failures    map[string]string // mountpoint -> stderr for a failed mount
	hook        func(program string, args []string)
	delay       time.Duration
	inflight    int
	maxInflight int
	creds       []credsSeen
}

func (r *fakeRunner) Run(_ context.Context, program string, args ...string) error {
	r.mu.Lock()
	r.calls = append(r.calls, call{program: program, args: append([]string(nil), args...)})
	r.inflight++
	if r.inflight > r.maxInflight {
		r.maxInflight = r.inflight
	}
	delay, hook := r.delay, r.hook
	r.mu.Unlock()
	defer func() {
		r.mu.Lock()
		r.inflight--
		r.mu.Unlock()
	}()

	if delay > 0 {
		time.Sleep(delay)
	}
	if hook != nil {
		hook(program, args)
	}

	switch program {
	case "mount":
		mp := args[len(args)-1]
		if path := credsPathOf(args); path != "" {
			r.recordCreds(path)
		}
		r.mu.Lock()
		stderr, failed := r.failures[mp]
		r.mu.Unlock()
		if failed {
			return &CommandError{Program: "mount", Stderr: stderr, Err: errors.New("exit status 32")}
		}
		r.addMount(args[len(args)-2], mp)
		return nil
	case "umount":
		r.removeMount(args[0])
		return nil
	}
	return fmt.Errorf("unexpected program %q", program)
}

// credsPathOf returns the credentials= path from a mount argument vector.
func credsPathOf(args []string) string {
	for i, a := range args {
		if a == "-o" && i+1 < len(args) {
			for _, opt := range strings.Split(args[i+1], ",") {
				if p, ok := strings.CutPrefix(opt, "credentials="); ok {
					return p
				}
			}
		}
	}
	return ""
}

func (r *fakeRunner) recordCreds(path string) {
	seen := credsSeen{path: path}
	if info, err := os.Stat(path); err == nil {
		seen.mode = info.Mode().Perm()
		if data, err := os.ReadFile(path); err == nil {
			seen.content = string(data)
		}
	}
	r.mu.Lock()
	r.creds = append(r.creds, seen)
	r.mu.Unlock()
}

func (r *fakeRunner) addMount(src, mp string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	data, _ := os.ReadFile(r.procFile)
	line := fmt.Sprintf("%s %s nfs rw 0 0\n", src, mp)
	_ = os.WriteFile(r.procFile, append(data, []byte(line)...), 0o644)
}

func (r *fakeRunner) removeMount(mp string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	data, _ := os.ReadFile(r.procFile)
	var keep []string
	for _, line := range strings.Split(string(data), "\n") {
		f := strings.Fields(line)
		if len(f) >= 2 && f[1] == mp {
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
	_ = os.WriteFile(r.procFile, []byte(out), 0o644)
}

// vanish drops a mount from the table without a umount, as a stale or lost
// share looks from the outside.
func (r *fakeRunner) vanish(mp string) { r.removeMount(mp) }

func (r *fakeRunner) programCalls() []call {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]call(nil), r.calls...)
}

func (r *fakeRunner) mountCount() int {
	n := 0
	for _, c := range r.programCalls() {
		if c.program == "mount" {
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
	run     *fakeRunner
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
	h.run = &fakeRunner{procFile: h.proc, failures: map[string]string{}}
	o := Options{
		Root:         h.root,
		ScratchDir:   h.scratch,
		ProcMounts:   h.proc,
		Runner:       h.run,
		Log:          slog.New(slog.NewTextHandler(io.Discard, nil)),
		Now:          func() time.Time { return h.now },
		MountTimeout: 5 * time.Second,
		StatTimeout:  200 * time.Millisecond,
		BackoffBase:  10 * time.Second,
		BackoffMax:   10 * time.Minute,
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
		Options: "vers=3,soft,timeo=50,retrans=2", Enabled: true,
	}
}

func smbStorage(id, name string) *store.Storage {
	return &store.Storage{
		ID: id, Name: name, Protocol: "smb", Source: "//nas/media",
		Options: "vers=3.0", Username: "bob", Domain: "WORK", HasSecret: true, Enabled: true,
	}
}

// ---- Argument vectors -------------------------------------------------------

func TestMountArgsNFSExact(t *testing.T) {
	st := nfsStorage("st-1", "media")
	got := MountArgs(st, "/storage/media", "")
	want := []string{"-t", "nfs", "-o", "vers=3,soft,timeo=50,retrans=2", "nas:/export", "/storage/media"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("nfs args:\n got %q\nwant %q", got, want)
	}
}

func TestMountArgsNFSReadOnlyAddsRo(t *testing.T) {
	st := nfsStorage("st-1", "media")
	st.Options = "vers=3"
	st.ReadOnly = true
	got := MountArgs(st, "/storage/media", "")
	want := []string{"-t", "nfs", "-o", "vers=3,ro", "nas:/export", "/storage/media"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("nfs read only args:\n got %q\nwant %q", got, want)
	}
}

func TestMountArgsNFSNoOptionsOmitsDashO(t *testing.T) {
	st := nfsStorage("st-1", "media")
	st.Options = ""
	got := MountArgs(st, "/storage/media", "")
	want := []string{"-t", "nfs", "nas:/export", "/storage/media"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("nfs no options:\n got %q\nwant %q", got, want)
	}
}

func TestMountArgsSMBExactWithCredentialsPath(t *testing.T) {
	st := smbStorage("st-2", "media")
	creds := "/scratch/cifs-st-2.cred"
	got := MountArgs(st, "/storage/media", creds)
	want := []string{"-t", "cifs", "-o", "credentials=/scratch/cifs-st-2.cred,vers=3.0", "//nas/media", "/storage/media"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("smb args:\n got %q\nwant %q", got, want)
	}
}

func TestMountArgsSMBReadOnlyExact(t *testing.T) {
	st := smbStorage("st-2", "media")
	st.ReadOnly = true
	got := MountArgs(st, "/storage/media", "/scratch/cifs-st-2.cred")
	want := []string{"-t", "cifs", "-o", "credentials=/scratch/cifs-st-2.cred,vers=3.0,ro", "//nas/media", "/storage/media"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("smb read only args:\n got %q\nwant %q", got, want)
	}
}

// The argument vector the runner receives, for both protocols, with the real
// credentials path the executor chose.
func TestMountRunsExactVectorNFS(t *testing.T) {
	h := newHarness(t, nil, nfsStorage("st-1", "media"))
	if err := h.exec.Mount(context.Background(), h.st.rows[0]); err != nil {
		t.Fatalf("mount: %v", err)
	}
	calls := h.run.programCalls()
	if len(calls) != 1 {
		t.Fatalf("calls = %d, want 1: %+v", len(calls), calls)
	}
	want := call{program: "mount", args: []string{"-t", "nfs", "-o", "vers=3,soft,timeo=50,retrans=2", "nas:/export", h.mountpoint("media")}}
	if !reflect.DeepEqual(calls[0], want) {
		t.Fatalf("mount call:\n got %+v\nwant %+v", calls[0], want)
	}
}

func TestMountRunsExactVectorSMBWithCredentialsPath(t *testing.T) {
	h := newHarness(t, nil, smbStorage("st-2", "media"))
	h.st.secrets["st-2"] = "s3cret"
	if err := h.exec.Mount(context.Background(), h.st.rows[0]); err != nil {
		t.Fatalf("mount: %v", err)
	}
	creds := filepath.Join(h.scratch, "cifs-st-2.cred")
	want := call{program: "mount", args: []string{
		"-t", "cifs", "-o", "credentials=" + creds + ",vers=3.0", "//nas/media", h.mountpoint("media"),
	}}
	calls := h.run.programCalls()
	if len(calls) != 1 || !reflect.DeepEqual(calls[0], want) {
		t.Fatalf("mount call:\n got %+v\nwant %+v", calls, want)
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
	h.run.failures[h.mountpoint("media")] = "mount error(13): Permission denied"
	err := h.exec.Mount(context.Background(), h.st.rows[0])
	if err == nil {
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

// ---- Failure surfaces stderr; the server keeps going -----------------------

func TestMountFailureSurfacesStderrAndStartupContinues(t *testing.T) {
	bad := nfsStorage("st-bad", "broken")
	good := nfsStorage("st-good", "media")
	h := newHarness(t, nil, bad, good)
	stderr := "mount.nfs: access denied by server while mounting nas:/export"
	h.run.failures[h.mountpoint("broken")] = stderr

	// Startup must not panic or return anything the caller could treat as a
	// server-stopping error. The good storage still comes up.
	h.exec.Startup(context.Background())

	if got := h.exec.Status(bad); got.Mounted || got.Error != stderr {
		t.Fatalf("broken status = %+v, want unmounted with stderr %q", got, stderr)
	}
	if got := h.exec.Status(good); !got.Mounted || got.Error != "" {
		t.Fatalf("good status = %+v, want mounted and no error", got)
	}
}

func TestCommandErrorKeepsStderrVerbatim(t *testing.T) {
	err := &CommandError{Program: "mount", Stderr: "mount.cifs: bad credentials\n", Err: errors.New("exit status 32")}
	if got := err.Error(); got != "mount.cifs: bad credentials\n" {
		t.Fatalf("Error() = %q", got)
	}
	empty := &CommandError{Program: "mount", Err: errors.New("exit status 1")}
	if got := empty.Error(); !strings.Contains(got, "mount failed") {
		t.Fatalf("empty stderr message = %q", got)
	}
}

func TestExecRunnerReturnsStderrFromTheProgram(t *testing.T) {
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("sh not available")
	}
	err := execRunner{}.Run(context.Background(), "sh", "-c", "echo 'mount.cifs: no route' >&2; exit 3")
	var ce *CommandError
	if !errors.As(err, &ce) {
		t.Fatalf("error %v is not a *CommandError", err)
	}
	if ce.Error() != "mount.cifs: no route" {
		t.Fatalf("stderr = %q", ce.Error())
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
	h.run.failures[h.mountpoint("media")] = "mount.nfs: Connection timed out"
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
	if got.Mounted || !strings.Contains(got.Error, "Connection timed out") {
		t.Fatalf("status after failures = %+v, want the stderr kept", got)
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

// ---- Test -------------------------------------------------------------------

func TestTestMountsProbesAndUnmounts(t *testing.T) {
	st := nfsStorage("st-1", "media")
	h := newHarness(t, nil, st)
	if err := h.exec.Test(context.Background(), st); err != nil {
		t.Fatalf("test: %v", err)
	}
	calls := h.run.programCalls()
	if len(calls) != 2 || calls[0].program != "mount" || calls[1].program != "umount" {
		t.Fatalf("calls = %+v, want mount then umount", calls)
	}
	if calls[1].args[0] != h.mountpoint("media") {
		t.Fatalf("umount target = %q", calls[1].args[0])
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
	if got := h.run.programCalls(); len(got) != 2 {
		t.Fatalf("calls = %+v, want mount and umount", got)
	}
}

func TestTestUnmountsEvenWhenTheCheckFails(t *testing.T) {
	st := nfsStorage("st-1", "media")
	h := newHarness(t, nil, st)
	h.run.hook = func(program string, args []string) {
		if program == "mount" {
			// The mount "succeeds" but the mountpoint is gone, so the stat fails.
			_ = os.RemoveAll(args[len(args)-1])
		}
	}
	err := h.exec.Test(context.Background(), st)
	if err == nil || !strings.Contains(err.Error(), "stat") {
		t.Fatalf("test error = %v, want the stat failure", err)
	}
	calls := h.run.programCalls()
	if len(calls) != 2 || calls[1].program != "umount" {
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
	if got := h.run.programCalls(); len(got) != 0 {
		t.Fatalf("test touched a mounted storage: %+v", got)
	}
	if got := h.exec.Status(st); !got.Mounted {
		t.Fatal("test must leave an already mounted storage mounted")
	}
}

// ---- Unmount, gate and validation ------------------------------------------

func TestUnmountRunsUmountAndIsIdempotent(t *testing.T) {
	st := nfsStorage("st-1", "media")
	h := newHarness(t, nil, st)
	ctx := context.Background()
	if err := h.exec.Unmount(ctx, st); err != nil {
		t.Fatalf("unmount when not mounted: %v", err)
	}
	if n := len(h.run.programCalls()); n != 0 {
		t.Fatalf("unmount of an unmounted storage ran %d commands", n)
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
	if n := len(h.run.programCalls()); n != 0 {
		t.Fatalf("gate refused, but %d commands still ran", n)
	}
}

func TestStartupAndWatchdogSkipDisabledStorages(t *testing.T) {
	off := nfsStorage("st-off", "off")
	off.Enabled = false
	on := nfsStorage("st-on", "on")
	h := newHarness(t, nil, off, on)
	h.exec.Startup(context.Background())
	h.exec.Watch(context.Background())
	calls := h.run.programCalls()
	if len(calls) != 1 || calls[0].args[len(calls[0].args)-1] != h.mountpoint("on") {
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
			if n := len(h.run.programCalls()); n != 0 {
				t.Fatalf("refused row still ran %d commands", n)
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
		t.Fatalf("mounts in flight at once = %d, want 1", peak)
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
