// SPDX-License-Identifier: AGPL-3.0-only

package api

import (
	"bytes"
	"context"
	"encoding/json"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/PeiceOfToast/shrinkyourshit/internal/secret"
	"github.com/PeiceOfToast/shrinkyourshit/internal/server"
	"github.com/PeiceOfToast/shrinkyourshit/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeMounter records mount-side calls. It can block Mount on release, so a
// test can prove that a second call waits for the first.
type fakeMounter struct {
	mu          sync.Mutex
	inFlight    int
	maxInFlight int
	calls       map[string]int
	mounted     map[string]bool
	err         error
	entered     chan struct{}
	release     chan struct{}
}

func newFakeMounter() *fakeMounter {
	return &fakeMounter{calls: map[string]int{}, mounted: map[string]bool{}}
}

func (f *fakeMounter) Status(st *store.Storage) StorageState {
	f.mu.Lock()
	defer f.mu.Unlock()
	return StorageState{Mounted: f.mounted[st.ID]}
}

// op runs one locked call: it counts it, tracks overlap, and optionally blocks.
func (f *fakeMounter) op(name string, block bool) error {
	f.mu.Lock()
	f.inFlight++
	if f.inFlight > f.maxInFlight {
		f.maxInFlight = f.inFlight
	}
	f.calls[name]++
	err := f.err
	f.mu.Unlock()
	defer func() {
		f.mu.Lock()
		f.inFlight--
		f.mu.Unlock()
	}()
	if block && f.entered != nil {
		f.entered <- struct{}{}
	}
	if block && f.release != nil {
		<-f.release
	}
	return err
}

func (f *fakeMounter) Mount(_ context.Context, st *store.Storage) error {
	if err := f.op("mount", true); err != nil {
		return err
	}
	f.mu.Lock()
	f.mounted[st.ID] = true
	f.mu.Unlock()
	return nil
}

func (f *fakeMounter) Unmount(_ context.Context, st *store.Storage) error {
	if err := f.op("unmount", false); err != nil {
		return err
	}
	f.mu.Lock()
	delete(f.mounted, st.ID)
	f.mu.Unlock()
	return nil
}

func (f *fakeMounter) Test(context.Context, *store.Storage) error {
	return f.op("test", false)
}

func (f *fakeMounter) count(op string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls[op]
}

func (f *fakeMounter) overlap() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.maxInFlight
}

// newStorageHarness builds the store with a storage key source (so passwords
// can be sealed) and the API with the given mounter. The default mounter is
// used when mounter is nil.
func newStorageHarness(t *testing.T, mounter StorageMounter) *apiHarness {
	t.Helper()
	return newStorageHarnessRoot(t, mounter, "")
}

// newStorageHarnessRoot is newStorageHarness with the mountpoint root set
// (task 69 browse tests point it at a temporary directory). Empty keeps /storage.
func newStorageHarnessRoot(t *testing.T, mounter StorageMounter, root string) *apiHarness {
	t.Helper()
	keyPath := filepath.Join(t.TempDir(), "storage.key")
	st, err := store.New(filepath.Join(t.TempDir(), "data.db"), store.WithSecrets(secret.NewSource("", keyPath)))
	require.NoError(t, err)
	t.Cleanup(func() { _ = st.Close() })
	svc := server.New(server.Config{
		Store:             st,
		SweepInterval:     time.Hour,
		HeartbeatTimeout:  time.Hour,
		WatchPollInterval: time.Hour,
		ScratchDir:        filepath.Join(t.TempDir(), "scratch"),
	})
	h := &apiHarness{t: t, svc: svc, st: st}
	var opts []Option
	if mounter != nil {
		opts = append(opts, WithStorageMounter(mounter))
	}
	if root != "" {
		opts = append(opts, WithStorageRoot(root))
	}
	h.mux = New("v0.0.0", testStatic(), svc, opts...)
	return h
}

// loginAs turns login on through setup and returns a session cookie.
func loginAs(t *testing.T, h *apiHarness) *http.Cookie {
	t.Helper()
	rec := h.do(http.MethodPost, "/api/v1/setup", map[string]any{
		"mode": "server", "login_enabled": true, "username": "admin", "password": "secret",
	})
	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())
	rec = h.do(http.MethodPost, "/api/v1/auth/login", map[string]any{"username": "admin", "password": "secret"})
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	return cookieOf(t, rec)
}

// seedStorage writes a row straight to the store, for tests where the API
// gate would refuse the write (login off).
func seedStorage(t *testing.T, h *apiHarness, st *store.Storage) *store.Storage {
	t.Helper()
	if st.Protocol == "" {
		st.Protocol = store.StorageProtocolNFS
	}
	if st.Source == "" {
		st.Source = "nas.lan:/mnt/" + st.Name
	}
	if st.Options == "" {
		st.Options = nfsDefaultOptions
	}
	st.Enabled = true
	require.NoError(t, h.st.CreateStorage(context.Background(), st))
	return st
}

// storageRequest builds a request, with an optional session, that is safe to
// use from a goroutine (no require calls).
func storageRequest(method, path string, body any, sess *http.Cookie) *http.Request {
	var req *http.Request
	if body != nil {
		raw, _ := json.Marshal(body)
		req = httptest.NewRequest(method, path, bytes.NewReader(raw))
	} else {
		req = httptest.NewRequest(method, path, nil)
	}
	if sess != nil {
		req.AddCookie(sess)
	}
	return req
}

// storageBodyFor returns a valid NFS create body.
func storageBodyFor(name string) map[string]any {
	return map[string]any{
		"name": name, "protocol": "nfs", "source": "nas.lan:/mnt/" + name,
	}
}

// errorPaths returns the field paths of a 400 storage error body.
func errorPaths(t *testing.T, rec *httptest.ResponseRecorder) []string {
	t.Helper()
	var body struct {
		Code   string `json:"code"`
		Errors []struct {
			Path string `json:"path"`
		} `json:"errors"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body), rec.Body.String())
	require.Equal(t, "invalid_request", body.Code)
	var out []string
	for _, e := range body.Errors {
		out = append(out, e.Path)
	}
	return out
}

// ---- Validators (pure) ----------------------------------------------------

func TestStorageOptionsAllowlist(t *testing.T) {
	valid := []string{
		"",
		"vers=3,soft,timeo=50,retrans=2",
		"ro,nosuid,nodev,noexec,noatime",
		"uid=1000,gid=1000,file_mode=0644,dir_mode=0755",
		"vers=3.0,sec=ntlmssp,iocharset=utf8,_netdev",
		"nfsvers=4.1,hard,port=2049,actimeo=1",
	}
	for _, opts := range valid {
		assert.Empty(t, checkStorageOptions(opts, false), "want valid: %q", opts)
	}

	// Shell metacharacters, line breaks, a second -o, unknown options, and
	// every rule the spec or the design adds.
	invalid := []string{
		"soft;rm -rf /",
		"soft\nhard",
		"soft\rro",
		"soft -o ro",
		"-o soft",
		"soft,-o,ro",
		"ro`id`",
		"$(id)",
		"soft|ro",
		"unknown",
		"soft,soft",
		"soft,hard",
		"ro,rw",
		"timeo",
		"timeo=abc",
		"soft=1",
		"uid=../x",
		"file_mode=999",
		"soft,,ro",
		"soft,",
		"-soft",
		"vers=3:4",
		"iocharset=utf 8",
	}
	for _, opts := range invalid {
		errs := checkStorageOptions(opts, false)
		require.NotEmpty(t, errs, "want refused: %q", opts)
		assert.Equal(t, "options", errs[0].Path, opts)
		assert.NotEmpty(t, errs[0].Message, opts)
	}
}

func TestStorageOptionsReadOnlyConflict(t *testing.T) {
	assert.NotEmpty(t, checkStorageOptions("rw", true), "rw on a read only storage is refused")
	assert.Empty(t, checkStorageOptions("rw", false))
	assert.Empty(t, checkStorageOptions("ro", true))
}

func TestStorageSourceRules(t *testing.T) {
	valid := map[string][]string{
		store.StorageProtocolNFS: {"nas.lan:/mnt/tank", "192.168.1.10:/export/media", "[fd00::1]:/data", "nas:/"},
		store.StorageProtocolSMB: {"//nas.lan/media", "//192.168.1.10/backup$", "//nas/Media-2"},
	}
	for proto, sources := range valid {
		for _, src := range sources {
			host, errs := checkStorageSource(proto, src)
			assert.Empty(t, errs, "%s want valid: %q", proto, src)
			assert.NotEmpty(t, host, "%s host for %q", proto, src)
		}
	}
	invalid := map[string][]string{
		store.StorageProtocolNFS: {"nas.lan:/mnt/tank x", "nas.lan", "//nas/share", "nas.lan:/a/../b", "nas.lan:/a/./b", "-bad:/x", "", "nas lan:/x"},
		store.StorageProtocolSMB: {"//nas.lan/media share", "nas.lan:/media", "//nas.lan/..", "//nas.lan/", "///x/y", "//nas.lan/media/sub"},
	}
	for proto, sources := range invalid {
		for _, src := range sources {
			_, errs := checkStorageSource(proto, src)
			require.NotEmpty(t, errs, "%s want refused: %q", proto, src)
			assert.Equal(t, "source", errs[0].Path)
		}
	}
	_, errs := checkStorageSource("webdav", "nas.lan:/x")
	require.NotEmpty(t, errs)
	assert.Equal(t, "protocol", errs[0].Path)
}

func TestStorageNameRuleMatchesStore(t *testing.T) {
	names := []string{"movies", "nas-1", "a", "", "Movies", "-x", "x-", "a/b", "a b", "a_b",
		strings.Repeat("a", 64), strings.Repeat("a", 65), "ünï"}
	for _, n := range names {
		apiOK := len(checkStorageName(n)) == 0
		storeOK := store.ValidStorageName(n) == nil
		assert.Equal(t, storeOK, apiOK, "name %q: api and store must agree", n)
	}
}

func TestStorageCredentialRules(t *testing.T) {
	base := storageInput{name: "nas", protocol: store.StorageProtocolNFS, source: "nas.lan:/x", options: nfsDefaultOptions}
	nfsUser := base
	nfsUser.username = "bob"
	paths := errorPaths2(checkStorageCredentials(nfsUser))
	assert.Contains(t, paths, "username", "NFS takes no username")

	smb := storageInput{name: "nas", protocol: store.StorageProtocolSMB, source: "//nas.lan/x"}
	smb.username = "bob\nsecret=1"
	assert.Contains(t, errorPaths2(checkStorageCredentials(smb)), "username", "a line break in the username is refused")
	smb.username = "bob"
	smb.password = "pw\x00"
	assert.Contains(t, errorPaths2(checkStorageCredentials(smb)), "password", "a NUL in the password is refused")
	smb.password = "fine-password"
	smb.username = strings.Repeat("u", storageUserMax+1)
	assert.Contains(t, errorPaths2(checkStorageCredentials(smb)), "username", "an overlong username is refused")
	smb.username = "bob"
	assert.Empty(t, checkStorageCredentials(smb))
}

// errorPaths2 lists the paths of a field error slice (no HTTP).
func errorPaths2(errs []storageFieldError) []string {
	var out []string
	for _, e := range errs {
		out = append(out, e.Path)
	}
	return out
}

func TestStorageHostPolicy(t *testing.T) {
	cases := []struct {
		setting string
		host    string
		want    bool
	}{
		{"", "anything.lan", true},
		{" , ", "anything.lan", true},
		{"nas.lan", "nas.lan", true},
		{"nas.lan", "NAS.lan", true},
		{"nas.lan", "evil.lan", false},
		{"nas.lan", "10.0.0.5", false},
		{"10.0.0.0/24", "10.0.0.5", true},
		{"10.0.0.0/24", "10.0.1.5", false},
		{"10.0.0.0/24", "::ffff:10.0.0.5", true},
		{"192.168.1.10", "192.168.1.10", true},
		{"192.168.1.10", "192.168.1.11", false},
		{"fd00::/64", "fd00::5", true},
		{" nas.lan , 10.0.0.0/24 ,", "10.0.0.9", true},
		{"nas.lan, bad entry!", "nas.lan", false},
		{"10.0.0.0/33", "10.0.0.5", false},
	}
	for _, c := range cases {
		p := parseHostPolicy(c.setting)
		assert.Equal(t, c.want, p.allows(c.host), "setting %q host %q", c.setting, c.host)
	}
	assert.Error(t, parseHostPolicy("nas.lan, bad entry!").err, "an invalid entry is reported")
	assert.False(t, parseHostPolicy("").restricted, "empty means unrestricted")
	assert.True(t, parseHostPolicy("nas.lan").restricted)
}

// TestStorageValidationNeverPanics feeds deterministic random bytes and odd
// strings through the whole validator. The test passes by not panicking.
func TestStorageValidationNeverPanics(t *testing.T) {
	odd := []string{"", "\x00", "\xff\xfe", strings.Repeat("a=", 3000), strings.Repeat("/", 600), "//", "[", "]", ":", "%s", "\u202e"}
	rng := rand.New(rand.NewSource(65))
	alphabet := []byte("abcxyz0123-_.,=:/[]@ \t\r\n;$`-oR")
	inputs := append([]string{}, odd...)
	for i := 0; i < 400; i++ {
		buf := make([]byte, rng.Intn(80))
		for j := range buf {
			buf[j] = alphabet[rng.Intn(len(alphabet))]
		}
		inputs = append(inputs, string(buf))
	}
	policy := parseHostPolicy("nas.lan,10.0.0.0/24")
	for _, s := range inputs {
		for _, proto := range []string{store.StorageProtocolNFS, store.StorageProtocolSMB, "bogus"} {
			_ = checkStorageInput(storageInput{name: s, protocol: proto, source: s, options: s, username: s, domain: s, password: s}, policy)
			_ = checkStorageInput(storageInput{name: "nas", protocol: proto, source: s, options: s, readOnly: true}, hostPolicy{})
		}
	}
}

// ---- HTTP -----------------------------------------------------------------

// Acceptance 1: a shell metacharacter, a newline, an unknown option or a
// second -o is rejected with 400, and the field error names the options.
func TestStorageHTTPRejectsBadOptions(t *testing.T) {
	h := newStorageHarness(t, nil)
	sess := loginAs(t, h)
	for _, opts := range []string{"soft;id", "soft\nro", "unknown", "soft,-o,ro", "soft -o ro"} {
		body := storageBodyFor("nas")
		body["options"] = opts
		rec := h.doWith(t, sess, http.MethodPost, "/api/v1/storages", body)
		require.Equal(t, http.StatusBadRequest, rec.Code, "%q: %s", opts, rec.Body.String())
		assert.Equal(t, []string{"options"}, errorPaths(t, rec), opts)
	}
	// Nothing was stored.
	rows, err := h.st.ListStorages(context.Background())
	require.NoError(t, err)
	assert.Empty(t, rows)
}

// Acceptance 2: with login disabled, every write endpoint returns 403, and
// nothing changes. Browse is gated the same way.
func TestStorageWritesForbiddenWithLoginOff(t *testing.T) {
	m := newFakeMounter()
	h := newStorageHarness(t, m)
	st := seedStorage(t, h, &store.Storage{Name: "nas"})

	writes := []struct {
		method, path string
		body         any
	}{
		{http.MethodPost, "/api/v1/storages", storageBodyFor("other")},
		{http.MethodPut, "/api/v1/storages/" + st.ID, storageBodyFor("nas")},
		{http.MethodDelete, "/api/v1/storages/" + st.ID, nil},
		{http.MethodPost, "/api/v1/storages/" + st.ID + "/test", nil},
		{http.MethodPost, "/api/v1/storages/" + st.ID + "/mount", nil},
		{http.MethodPost, "/api/v1/storages/" + st.ID + "/unmount", nil},
		{http.MethodGet, "/api/v1/storages/" + st.ID + "/browse", nil},
	}
	for _, w := range writes {
		rec := h.do(w.method, w.path, w.body)
		require.Equal(t, http.StatusForbidden, rec.Code, "%s %s: %s", w.method, w.path, rec.Body.String())
		var body struct {
			Code  string `json:"code"`
			Error string `json:"error"`
		}
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
		assert.Equal(t, "login_required", body.Code, w.method+" "+w.path)
		assert.Contains(t, body.Error, "App settings", "the message says where to fix it")
	}
	// Nothing ran: the row is intact and the mounter was never called.
	got, err := h.st.GetStorage(context.Background(), st.ID)
	require.NoError(t, err)
	assert.Equal(t, "nas", got.Name)
	assert.Zero(t, m.count("mount")+m.count("unmount")+m.count("test"))
}

// With login on and a session, the same writes work, and reads are open.
func TestStorageCRUDWithLogin(t *testing.T) {
	h := newStorageHarness(t, nil)
	sess := loginAs(t, h)

	rec := h.doWith(t, sess, http.MethodPost, "/api/v1/storages", storageBodyFor("movies"))
	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())
	var created storageDTO
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &created))
	assert.Equal(t, "movies", created.Name)
	assert.Equal(t, nfsDefaultOptions, created.Options, "NFS gets the soft default when no options are given")
	assert.Equal(t, "/storage/movies", created.Mountpoint)
	assert.True(t, created.Enabled)
	assert.False(t, created.Mounted)
	assert.Empty(t, created.Error)

	rec = h.doWith(t, sess, http.MethodGet, "/api/v1/storages", nil)
	require.Equal(t, http.StatusOK, rec.Code)
	var list storageListResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &list))
	assert.Equal(t, 1, list.Total)
	assert.False(t, list.HostPolicy.Restricted)
	assert.Contains(t, list.HostPolicy.Message, "Any host can be mounted", "the response says an empty list means any host")

	rec = h.doWith(t, sess, http.MethodGet, "/api/v1/storages/"+created.ID, nil)
	require.Equal(t, http.StatusOK, rec.Code)

	// PUT changes the options and leaves the name alone.
	rec = h.doWith(t, sess, http.MethodPut, "/api/v1/storages/"+created.ID, map[string]any{
		"name": "movies", "protocol": "nfs", "source": "nas.lan:/mnt/movies", "options": "vers=4.1,hard",
		"read_only": true, "enabled": false,
	})
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	var updated storageDTO
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &updated))
	assert.Equal(t, "vers=4.1,hard", updated.Options)
	assert.True(t, updated.ReadOnly)
	assert.False(t, updated.Enabled)

	rec = h.doWith(t, sess, http.MethodDelete, "/api/v1/storages/"+created.ID, nil)
	require.Equal(t, http.StatusNoContent, rec.Code, rec.Body.String())
	rec = h.doWith(t, sess, http.MethodGet, "/api/v1/storages/"+created.ID, nil)
	assert.Equal(t, http.StatusNotFound, rec.Code)
}

// Acceptance 3 (host allowlist): when storage_allowed_hosts is set, a source
// outside it is refused on create, on edit, and again at mount time.
func TestStorageHostAllowlistRefusesDisallowedHosts(t *testing.T) {
	m := newFakeMounter()
	h := newStorageHarness(t, m)
	sess := loginAs(t, h)
	ctx := context.Background()
	require.NoError(t, h.st.SetSetting(ctx, store.SettingStorageAllowedHosts, "nas.lan, 10.0.0.0/24"))

	// Allowed: by name and by CIDR.
	rec := h.doWith(t, sess, http.MethodPost, "/api/v1/storages", storageBodyFor("by-name"))
	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())
	body := map[string]any{"name": "by-cidr", "protocol": "nfs", "source": "10.0.0.7:/mnt/x"}
	rec = h.doWith(t, sess, http.MethodPost, "/api/v1/storages", body)
	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())

	// Refused on create: a host outside the list, and an address outside the CIDR.
	for i, src := range []string{"evil.lan:/mnt/x", "10.0.1.5:/mnt/x"} {
		rec = h.doWith(t, sess, http.MethodPost, "/api/v1/storages", map[string]any{
			"name": "refused" + string(rune('a'+i)), "protocol": "nfs", "source": src,
		})
		require.Equal(t, http.StatusBadRequest, rec.Code, src)
		assert.Equal(t, []string{"source"}, errorPaths(t, rec), src)
	}

	// The list says the policy is in force and names the allowed hosts.
	rec = h.doWith(t, sess, http.MethodGet, "/api/v1/storages", nil)
	require.Equal(t, http.StatusOK, rec.Code)
	var list storageListResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &list))
	assert.True(t, list.HostPolicy.Restricted)
	assert.Equal(t, []string{"nas.lan", "10.0.0.0/24"}, list.HostPolicy.Allowed)

	// Tighten the list. The existing row now points at a host that is not
	// allowed, so mount and test refuse it before the executor runs.
	require.NoError(t, h.st.SetSetting(ctx, store.SettingStorageAllowedHosts, "nas.lan"))
	stored, err := h.st.GetStorageByName(ctx, "by-cidr")
	require.NoError(t, err)
	for _, path := range []string{"/mount", "/test"} {
		rec = h.doWith(t, sess, http.MethodPost, "/api/v1/storages/"+stored.ID+path, nil)
		require.Equal(t, http.StatusForbidden, rec.Code, path+": "+rec.Body.String())
		var e struct {
			Code string `json:"code"`
		}
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &e))
		assert.Equal(t, "host_not_allowed", e.Code)
	}
	assert.Zero(t, m.count("mount")+m.count("test"), "the executor never ran for a refused host")
}

// An invalid entry in the allowlist fails closed and says which entry.
func TestStorageHostAllowlistInvalidEntryFailsClosed(t *testing.T) {
	h := newStorageHarness(t, nil)
	sess := loginAs(t, h)
	require.NoError(t, h.st.SetSetting(context.Background(), store.SettingStorageAllowedHosts, "nas.lan, bad entry!"))

	rec := h.doWith(t, sess, http.MethodPost, "/api/v1/storages", storageBodyFor("nas"))
	require.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Equal(t, []string{"source"}, errorPaths(t, rec))

	rec = h.doWith(t, sess, http.MethodGet, "/api/v1/storages", nil)
	var list storageListResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &list))
	assert.True(t, list.HostPolicy.Restricted)
	assert.Contains(t, list.HostPolicy.Error, "bad entry!")
}

// The SMB password is accepted, is never echoed back, and is sealed at rest.
func TestStoragePasswordNeverReturned(t *testing.T) {
	h := newStorageHarness(t, nil)
	sess := loginAs(t, h)
	const pw = "distinctive-smb-password-7c1e"

	rec := h.doWith(t, sess, http.MethodPost, "/api/v1/storages", map[string]any{
		"name": "office", "protocol": "smb", "source": "//nas.lan/office",
		"username": "bob", "domain": "HOME", "password": pw,
	})
	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())
	assert.NotContains(t, rec.Body.String(), pw)
	assert.Contains(t, rec.Body.String(), `"password_set":true`)

	var created storageDTO
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &created))
	rec = h.doWith(t, sess, http.MethodGet, "/api/v1/storages", nil)
	assert.NotContains(t, rec.Body.String(), pw)

	// An edit with an empty password keeps the stored one.
	rec = h.doWith(t, sess, http.MethodPut, "/api/v1/storages/"+created.ID, map[string]any{
		"name": "office", "protocol": "smb", "source": "//nas.lan/office", "username": "bob", "domain": "HOME", "password": "",
	})
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.Contains(t, rec.Body.String(), `"password_set":true`)
	got, err := h.st.StorageSecret(context.Background(), created.ID)
	require.NoError(t, err)
	assert.Equal(t, pw, got)
}

// NFS refuses SMB credentials, and the name and protocol cannot change.
func TestStorageUpdateRules(t *testing.T) {
	h := newStorageHarness(t, nil)
	sess := loginAs(t, h)
	st := seedStorage(t, h, &store.Storage{Name: "nas"})

	rec := h.doWith(t, sess, http.MethodPut, "/api/v1/storages/"+st.ID, map[string]any{
		"name": "renamed", "protocol": "smb", "source": "//nas.lan/x", "username": "bob",
	})
	require.Equal(t, http.StatusBadRequest, rec.Code)
	paths := errorPaths(t, rec)
	assert.Contains(t, paths, "name", "the name cannot change")
	assert.Contains(t, paths, "protocol", "the protocol cannot change")

	rec = h.doWith(t, sess, http.MethodPut, "/api/v1/storages/"+st.ID, map[string]any{
		"name": "nas", "protocol": "nfs", "source": "nas.lan:/mnt/nas", "username": "bob",
	})
	require.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Equal(t, []string{"username"}, errorPaths(t, rec))
}

func TestStorageDuplicateNameConflicts(t *testing.T) {
	h := newStorageHarness(t, nil)
	sess := loginAs(t, h)
	rec := h.doWith(t, sess, http.MethodPost, "/api/v1/storages", storageBodyFor("movies"))
	require.Equal(t, http.StatusCreated, rec.Code)
	rec = h.doWith(t, sess, http.MethodPost, "/api/v1/storages", storageBodyFor("movies"))
	require.Equal(t, http.StatusConflict, rec.Code, rec.Body.String())
}

// Acceptance 4: two concurrent mount calls on one storage serialise. The first
// blocks inside the executor; the second must not enter until it is released.
func TestStorageConcurrentMountsSerialise(t *testing.T) {
	m := newFakeMounter()
	m.entered = make(chan struct{}, 4)
	m.release = make(chan struct{})
	h := newStorageHarness(t, m)
	sess := loginAs(t, h)
	st := seedStorage(t, h, &store.Storage{Name: "nas"})

	var wg sync.WaitGroup
	statuses := make([]int, 2)
	for i := range statuses {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			rec := httptest.NewRecorder()
			h.mux.ServeHTTP(rec, storageRequest(http.MethodPost, "/api/v1/storages/"+st.ID+"/mount", nil, sess))
			statuses[i] = rec.Code
		}(i)
	}

	<-m.entered // the first call is inside the executor
	time.Sleep(150 * time.Millisecond)
	assert.Equal(t, 1, m.count("mount"), "the second mount must wait for the first")
	close(m.release)
	wg.Wait()

	assert.Equal(t, 2, m.count("mount"), "both calls ran, one after the other")
	assert.Equal(t, 1, m.overlap(), "at most one mount call was in flight at a time")
	assert.Equal(t, []int{http.StatusOK, http.StatusOK}, statuses)
}

// The default mounter (task 66 not wired) answers 501 for every mount-side
// call, and browse says the storage is not mounted.
func TestStorageDefaultMounterAnswersNotImplemented(t *testing.T) {
	h := newStorageHarness(t, nil)
	sess := loginAs(t, h)
	st := seedStorage(t, h, &store.Storage{Name: "nas"})

	for _, path := range []string{"/mount", "/unmount", "/test"} {
		rec := h.doWith(t, sess, http.MethodPost, "/api/v1/storages/"+st.ID+path, nil)
		assert.Equal(t, http.StatusNotImplemented, rec.Code, path+": "+rec.Body.String())
	}
	rec := h.doWith(t, sess, http.MethodGet, "/api/v1/storages/"+st.ID+"/browse", nil)
	require.Equal(t, http.StatusConflict, rec.Code)
	assert.Contains(t, rec.Body.String(), "not_mounted")
}

// A failed mount is 409 with the executor's message; a failed test is 200 with
// ok false; a mounted storage cannot be deleted.
func TestStorageMounterOutcomes(t *testing.T) {
	m := newFakeMounter()
	h := newStorageHarness(t, m)
	sess := loginAs(t, h)
	st := seedStorage(t, h, &store.Storage{Name: "nas"})

	m.err = errString("mount.nfs: access denied")
	rec := h.doWith(t, sess, http.MethodPost, "/api/v1/storages/"+st.ID+"/mount", nil)
	require.Equal(t, http.StatusConflict, rec.Code)
	assert.Contains(t, rec.Body.String(), "access denied")

	rec = h.doWith(t, sess, http.MethodPost, "/api/v1/storages/"+st.ID+"/test", nil)
	require.Equal(t, http.StatusOK, rec.Code)
	var tr storageTestResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &tr))
	assert.False(t, tr.OK)
	assert.Contains(t, tr.Error, "access denied")

	m.err = nil
	rec = h.doWith(t, sess, http.MethodPost, "/api/v1/storages/"+st.ID+"/mount", nil)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	var mounted storageDTO
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &mounted))
	assert.True(t, mounted.Mounted)

	rec = h.doWith(t, sess, http.MethodDelete, "/api/v1/storages/"+st.ID, nil)
	assert.Equal(t, http.StatusConflict, rec.Code, "a mounted storage cannot be deleted")

	rec = h.doWith(t, sess, http.MethodPost, "/api/v1/storages/"+st.ID+"/unmount", nil)
	require.Equal(t, http.StatusOK, rec.Code)
	rec = h.doWith(t, sess, http.MethodDelete, "/api/v1/storages/"+st.ID, nil)
	assert.Equal(t, http.StatusNoContent, rec.Code)
}

type errString string

func (e errString) Error() string { return string(e) }

// A missing id is 404 on every per-storage route.
func TestStorageUnknownIDIsNotFound(t *testing.T) {
	h := newStorageHarness(t, nil)
	sess := loginAs(t, h)
	for _, route := range []struct{ method, path string }{
		{http.MethodGet, "/api/v1/storages/nope"},
		{http.MethodPut, "/api/v1/storages/nope"},
		{http.MethodDelete, "/api/v1/storages/nope"},
		{http.MethodPost, "/api/v1/storages/nope/mount"},
		{http.MethodGet, "/api/v1/storages/nope/browse"},
	} {
		rec := h.doWith(t, sess, route.method, route.path, storageBodyFor("x"))
		assert.Equal(t, http.StatusNotFound, rec.Code, route.method+" "+route.path)
	}
}

// Storage routes stay behind the session when login is on.
func TestStorageRequiresSessionWhenLoginOn(t *testing.T) {
	h := newStorageHarness(t, nil)
	_ = loginAs(t, h)
	rec := h.do(http.MethodGet, "/api/v1/storages", nil)
	assert.Equal(t, http.StatusUnauthorized, rec.Code)
	rec = h.do(http.MethodPost, "/api/v1/storages", storageBodyFor("nas"))
	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}
