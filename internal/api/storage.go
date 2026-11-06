// SPDX-License-Identifier: AGPL-3.0-only

package api

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/netip"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/PeiceOfToast/shrinkyourshit/internal/store"
)

// Storage API (task 65, spec 4.4 to 4.6). The API validates every field,
// gates writes behind login, enforces the optional host allowlist and
// serialises mount-side calls per storage. The mount executor itself is
// task 66: it plugs in through StorageMounter.

// ErrStorageMountUnavailable is what the default mounter answers. The
// server answers 501 until a real executor is wired.
var ErrStorageMountUnavailable = errors.New("mounting is not available on this server yet")

// StorageState is what the mounter reports about one storage. Task 66 fills
// it; until then every storage reads as not mounted with no error.
type StorageState struct {
	Mounted bool
	Error   string
}

// StorageMounter does the mount-side work for a storage (spec 4.1). The API
// calls Mount, Unmount and Test under the storage's lock, so an implementation
// never sees two of those calls for one storage at once. Status is a read and
// is not locked. An implementation receives the stored row; it obtains the
// SMB password with the store's StorageSecret, never from the API.
type StorageMounter interface {
	Status(st *store.Storage) StorageState
	Mount(ctx context.Context, st *store.Storage) error
	Unmount(ctx context.Context, st *store.Storage) error
	Test(ctx context.Context, st *store.Storage) error
}

// unavailableMounter is the default until task 66 wires an executor.
type unavailableMounter struct{}

func (unavailableMounter) Status(*store.Storage) StorageState { return StorageState{} }
func (unavailableMounter) Mount(context.Context, *store.Storage) error {
	return ErrStorageMountUnavailable
}
func (unavailableMounter) Unmount(context.Context, *store.Storage) error {
	return ErrStorageMountUnavailable
}
func (unavailableMounter) Test(context.Context, *store.Storage) error {
	return ErrStorageMountUnavailable
}

// WithStorageMounter wires the mount executor (task 66). A nil mounter keeps
// the default.
func WithStorageMounter(m StorageMounter) Option {
	return func(a *API) {
		if m != nil {
			a.mounter = m
		}
	}
}

// keyedLocks hands out one mutex per key (here: per storage id), so mount,
// unmount, test, edit and delete of one storage serialise while other storages
// proceed.
type keyedLocks struct {
	mu    sync.Mutex
	locks map[string]*sync.Mutex
}

func newKeyedLocks() *keyedLocks {
	return &keyedLocks{locks: map[string]*sync.Mutex{}}
}

// lock acquires the mutex for key and returns the unlock function.
func (l *keyedLocks) lock(key string) func() {
	l.mu.Lock()
	m, ok := l.locks[key]
	if !ok {
		m = &sync.Mutex{}
		l.locks[key] = m
	}
	l.mu.Unlock()
	m.Lock()
	return m.Unlock
}

// ---- Validation -----------------------------------------------------------

// storageFieldError is one refused field: path is the JSON field name.
type storageFieldError struct {
	Path    string `json:"path"`
	Message string `json:"message"`
}

// Length caps keep a request from carrying an unbounded value into the
// database or the mount command.
const (
	storageTextMax = 512
	storageUserMax = 128
)

// storageNameRE is the slug rule, the same one the store enforces
// (store.ValidStorageName). A test keeps the two in step.
var storageNameRE = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]*[a-z0-9])?$`)

// hostLabel is one DNS label; storageHostPat is a hostname or a bracketed IP.
const hostLabel = `[A-Za-z0-9](?:[A-Za-z0-9-]{0,61}[A-Za-z0-9])?`

var (
	hostnameRE  = regexp.MustCompile(`^` + hostLabel + `(?:\.` + hostLabel + `)*$`)
	storageHost = `(?:` + hostLabel + `(?:\.` + hostLabel + `)*|\[[0-9A-Fa-f:.]{2,45}\])`
	nfsSourceRE = regexp.MustCompile(`^(` + storageHost + `):(/[A-Za-z0-9._/-]*)$`)
	smbSourceRE = regexp.MustCompile(`^//(` + storageHost + `)/([A-Za-z0-9._-]+\$?)$`)
	// optionsCharsRE is what an options string may contain once the shell
	// characters have been named separately.
	optionsCharsRE = regexp.MustCompile(`^[A-Za-z0-9_.,=-]*$`)
	// shellChars are named in their own message so a user sees why a value
	// was refused.
	shellChars = ";&|$`<>()*?{}!~#'\"\\ \t"
)

// mountOption is one allowlisted mount option (spec 4.4). A value option
// takes "name=value" and must match pattern; a flag takes no value.
type mountOption struct {
	value   bool
	pattern *regexp.Regexp
}

var (
	numberRE   = regexp.MustCompile(`^[0-9]{1,9}$`)
	versionRE  = regexp.MustCompile(`^[0-9]{1,2}(?:\.[0-9]{1,2})?$`)
	modeRE     = regexp.MustCompile(`^[0-7]{3,4}$`)
	tokenValRE = regexp.MustCompile(`^[A-Za-z0-9_-]{1,32}$`)
)

// storageMountOptions is the spec 4.4 allowlist, with a value rule for each
// option that takes one.
var storageMountOptions = map[string]mountOption{
	"vers":      {true, versionRE},
	"nfsvers":   {true, versionRE},
	"timeo":     {true, numberRE},
	"retrans":   {true, numberRE},
	"port":      {true, numberRE},
	"actimeo":   {true, numberRE},
	"uid":       {true, numberRE},
	"gid":       {true, numberRE},
	"file_mode": {true, modeRE},
	"dir_mode":  {true, modeRE},
	"iocharset": {true, tokenValRE},
	"sec":       {true, tokenValRE},
	"soft":      {},
	"hard":      {},
	"rw":        {},
	"ro":        {},
	"nosuid":    {},
	"nodev":     {},
	"noexec":    {},
	"noatime":   {},
	"_netdev":   {},
}

// nfsDefaultOptions are the spec 4.4 NFS defaults: soft mounts, so a NAS
// hiccup returns an error instead of hanging the process (the 2026-11-05
// NZBGet incident).
const nfsDefaultOptions = "vers=3,soft,timeo=50,retrans=2"

// defaultStorageOptions fills in the NFS defaults when no options were given.
func defaultStorageOptions(protocol, options string) string {
	if options == "" && protocol == store.StorageProtocolNFS {
		return nfsDefaultOptions
	}
	return options
}

// storageInput is the validated view of a create or update body.
type storageInput struct {
	name     string
	protocol string
	source   string
	options  string
	username string
	domain   string
	password string
	readOnly bool
}

// checkStorageName enforces the slug rule and the length cap.
func checkStorageName(name string) []storageFieldError {
	switch {
	case name == "":
		return []storageFieldError{{"name", "a name is required"}}
	case len(name) > 64 || !storageNameRE.MatchString(name):
		return []storageFieldError{{"name", "use 1 to 64 lowercase letters, digits and dashes, with no dash at the start or end"}}
	}
	return nil
}

// checkStorageSource validates the source per protocol and returns the host
// it names, which the allowlist then checks.
func checkStorageSource(protocol, source string) (string, []storageFieldError) {
	if source == "" {
		return "", []storageFieldError{{"source", "a source is required"}}
	}
	if len(source) > storageTextMax {
		return "", []storageFieldError{{"source", "the source is too long"}}
	}
	switch protocol {
	case store.StorageProtocolNFS:
		m := nfsSourceRE.FindStringSubmatch(source)
		if m == nil {
			return "", []storageFieldError{{"source", "an NFS source looks like nas.lan:/mnt/tank, a host, a colon, then the export path, with no spaces"}}
		}
		if hasDotSegment(m[2]) {
			return "", []storageFieldError{{"source", "the export path cannot contain . or .. segments"}}
		}
		return strings.Trim(m[1], "[]"), nil
	case store.StorageProtocolSMB:
		m := smbSourceRE.FindStringSubmatch(source)
		if m == nil {
			return "", []storageFieldError{{"source", "an SMB source looks like //nas.lan/media, two slashes, the host, one slash, then the share name, with no spaces"}}
		}
		if isDotsOnly(m[2]) {
			return "", []storageFieldError{{"source", "the share name cannot be . or .."}}
		}
		return strings.Trim(m[1], "[]"), nil
	}
	return "", []storageFieldError{{"protocol", "the protocol must be nfs or smb"}}
}

// hasDotSegment reports whether any segment of an export path is . or ..
func hasDotSegment(p string) bool {
	for _, seg := range strings.Split(p, "/") {
		if isDotsOnly(seg) {
			return true
		}
	}
	return false
}

// isDotsOnly reports whether s is non-empty and made only of dots.
func isDotsOnly(s string) bool {
	return s != "" && strings.Trim(s, ".") == ""
}

// checkStorageOptions validates a mount options string against the spec 4.4
// allowlist. Every problem is a field error at path "options".
func checkStorageOptions(opts string, readOnly bool) []storageFieldError {
	if opts == "" {
		return nil
	}
	const path = "options"
	fail := func(msg string) []storageFieldError {
		return []storageFieldError{{path, msg}}
	}
	if len(opts) > storageTextMax {
		return fail("the options are too long")
	}
	if strings.ContainsAny(opts, "\r\n") {
		return fail("the options cannot contain a line break")
	}
	if strings.ContainsAny(opts, shellChars) {
		return fail("the options cannot contain spaces or shell characters such as ; | & $ or quotes; write them as a comma separated list like soft,ro")
	}
	if strings.Contains(opts, "-o") {
		return fail("the options cannot contain -o; write only the list, like soft,ro, because the server adds -o itself")
	}
	if !optionsCharsRE.MatchString(opts) {
		return fail("the options may only contain letters, digits and . _ - = ,")
	}

	seen := map[string]string{}
	for _, tok := range strings.Split(opts, ",") {
		if tok == "" {
			return fail("the options have an empty entry between commas")
		}
		if strings.HasPrefix(tok, "-") {
			return fail(fmt.Sprintf("%q cannot start with a dash; list the option name only", tok))
		}
		name, value, hasValue := strings.Cut(tok, "=")
		rule, known := storageMountOptions[name]
		if !known {
			return fail(fmt.Sprintf("%q is not an allowed option; allowed options are %s", name, allowedOptionNames()))
		}
		if _, dup := seen[name]; dup {
			return fail(fmt.Sprintf("%q appears more than once", name))
		}
		switch {
		case rule.value && !hasValue:
			return fail(fmt.Sprintf("%q needs a value, like %s=…", name, name))
		case !rule.value && hasValue:
			return fail(fmt.Sprintf("%q takes no value", name))
		case rule.value && !rule.pattern.MatchString(value):
			return fail(fmt.Sprintf("the value of %q is not allowed", name))
		}
		seen[name] = value
	}
	if _, soft := seen["soft"]; soft {
		if _, hard := seen["hard"]; hard {
			return fail("soft and hard cannot be used together")
		}
	}
	if _, ro := seen["ro"]; ro {
		if _, rw := seen["rw"]; rw {
			return fail("ro and rw cannot be used together")
		}
	}
	if _, rw := seen["rw"]; rw && readOnly {
		return fail("rw cannot be used on a read only storage; turn off read only or remove rw")
	}
	return nil
}

// allowedOptionNames lists the allowlist in a stable order for messages.
func allowedOptionNames() string {
	names := make([]string, 0, len(storageMountOptions))
	for name := range storageMountOptions {
		names = append(names, name)
	}
	sort.Strings(names)
	return strings.Join(names, ", ")
}

// checkStorageCredentials keeps the SMB credentials SMB-only and free of line
// breaks: the credentials file is line based (task 66).
func checkStorageCredentials(in storageInput) []storageFieldError {
	var errs []storageFieldError
	if in.protocol == store.StorageProtocolNFS {
		if in.username != "" {
			errs = append(errs, storageFieldError{"username", "a username only applies to SMB shares"})
		}
		if in.domain != "" {
			errs = append(errs, storageFieldError{"domain", "a domain only applies to SMB shares"})
		}
		if in.password != "" {
			errs = append(errs, storageFieldError{"password", "a password only applies to SMB shares"})
		}
	}
	for _, f := range []struct {
		path, value string
		max         int
	}{
		{"username", in.username, storageUserMax},
		{"domain", in.domain, storageUserMax},
		{"password", in.password, storageTextMax},
	} {
		if len(f.value) > f.max {
			errs = append(errs, storageFieldError{f.path, "that value is too long"})
		}
		if strings.ContainsAny(f.value, "\r\n\x00") {
			errs = append(errs, storageFieldError{f.path, "that value cannot contain a line break"})
		}
	}
	return errs
}

// checkStorageInput runs every field rule and the host allowlist. It returns
// all the problems it finds, so the user fixes them in one pass.
func checkStorageInput(in storageInput, policy hostPolicy) []storageFieldError {
	errs := checkStorageName(in.name)
	if in.protocol != store.StorageProtocolNFS && in.protocol != store.StorageProtocolSMB {
		errs = append(errs, storageFieldError{"protocol", "the protocol must be nfs or smb"})
	} else {
		host, srcErrs := checkStorageSource(in.protocol, in.source)
		errs = append(errs, srcErrs...)
		if len(srcErrs) == 0 && !policy.allows(host) {
			errs = append(errs, storageFieldError{"source", policy.refusal(host)})
		}
	}
	errs = append(errs, checkStorageOptions(in.options, in.readOnly)...)
	errs = append(errs, checkStorageCredentials(in)...)
	return errs
}

// ---- Host allowlist -------------------------------------------------------

// hostEntry is one line of storage_allowed_hosts: a hostname, an IP literal,
// or a CIDR.
type hostEntry struct {
	name   string
	addr   netip.Addr
	prefix netip.Prefix
}

// hostPolicy is the parsed storage_allowed_hosts setting (spec 4.6). Empty
// means unrestricted. An invalid entry sets err and refuses every host, so a
// typo fails closed.
type hostPolicy struct {
	restricted bool
	entries    []hostEntry
	allowed    []string
	err        error
}

// parseHostPolicy reads the comma separated setting.
func parseHostPolicy(raw string) hostPolicy {
	p := hostPolicy{allowed: []string{}}
	for _, part := range strings.Split(raw, ",") {
		entry := strings.TrimSpace(part)
		if entry == "" {
			continue
		}
		p.allowed = append(p.allowed, entry)
		if strings.Contains(entry, "/") {
			pre, err := netip.ParsePrefix(entry)
			if err != nil {
				p.err = fmt.Errorf("%q is not a valid CIDR", entry)
				break
			}
			p.entries = append(p.entries, hostEntry{prefix: pre.Masked()})
			continue
		}
		if addr, err := netip.ParseAddr(entry); err == nil {
			p.entries = append(p.entries, hostEntry{addr: addr.Unmap()})
			continue
		}
		if len(entry) > 253 || !hostnameRE.MatchString(entry) {
			p.err = fmt.Errorf("%q is not a host name, an IP address or a CIDR", entry)
			break
		}
		p.entries = append(p.entries, hostEntry{name: strings.ToLower(entry)})
	}
	p.restricted = len(p.entries) > 0 || p.err != nil
	return p
}

// allows reports whether host may be mounted. Hostnames match exactly, and a
// CIDR or IP entry matches only a literal IP address: the check does not
// resolve names, so a hostname outside the list is refused.
func (p hostPolicy) allows(host string) bool {
	if p.err != nil {
		return false
	}
	if !p.restricted {
		return true
	}
	addr, ipErr := netip.ParseAddr(host)
	if ipErr == nil {
		addr = addr.Unmap()
	}
	lower := strings.ToLower(host)
	for _, e := range p.entries {
		switch {
		case e.prefix.IsValid():
			if ipErr == nil && e.prefix.Contains(addr) {
				return true
			}
		case e.addr.IsValid():
			if ipErr == nil && e.addr == addr {
				return true
			}
		case e.name != "":
			if ipErr != nil && e.name == lower {
				return true
			}
		}
	}
	return false
}

// refusal is the plain message for a host that the policy does not allow.
func (p hostPolicy) refusal(host string) string {
	if p.err != nil {
		return fmt.Sprintf("storage_allowed_hosts has an invalid entry (%v), so no host can be mounted until it is fixed", p.err)
	}
	return fmt.Sprintf("host %q is not in storage_allowed_hosts; add it to the setting or pick another host", host)
}

// CheckStorageHost applies the storage_allowed_hosts rule to a stored source,
// for the mount executor's gate (task 66). It is the same rule the handlers
// use, so a host refused by the API is refused on startup and by the watchdog.
func CheckStorageHost(setting, protocol, source string) error {
	host, errs := checkStorageSource(protocol, source)
	if len(errs) > 0 {
		return errors.New(errs[0].Message)
	}
	policy := parseHostPolicy(setting)
	if !policy.allows(host) {
		return errors.New(policy.refusal(host))
	}
	return nil
}

// storageHostPolicy loads the current allowlist.
func (a *API) storageHostPolicy(r *http.Request) (hostPolicy, error) {
	raw, _, err := a.st.GetSetting(r.Context(), store.SettingStorageAllowedHosts)
	if err != nil {
		return hostPolicy{}, err
	}
	return parseHostPolicy(raw), nil
}

// ---- DTOs -----------------------------------------------------------------

// storageDTO is the API shape of one storage. The password never leaves the
// server: a response carries password_set instead. The status fields are
// filled by the mounter (task 66).
type storageDTO struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Protocol    string    `json:"protocol"`
	Source      string    `json:"source"`
	Options     string    `json:"options"`
	Username    string    `json:"username"`
	Domain      string    `json:"domain"`
	PasswordSet bool      `json:"password_set"`
	ReadOnly    bool      `json:"read_only"`
	Enabled     bool      `json:"enabled"`
	Mountpoint  string    `json:"mountpoint"`
	Mounted     bool      `json:"mounted"`
	Error       string    `json:"error"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// toStorageDTO maps a stored row and its mount status to the API shape.
func toStorageDTO(st *store.Storage, state StorageState) storageDTO {
	return storageDTO{
		ID:          st.ID,
		Name:        st.Name,
		Protocol:    st.Protocol,
		Source:      st.Source,
		Options:     st.Options,
		Username:    st.Username,
		Domain:      st.Domain,
		PasswordSet: st.HasSecret,
		ReadOnly:    st.ReadOnly,
		Enabled:     st.Enabled,
		Mountpoint:  "/storage/" + st.Name,
		Mounted:     state.Mounted,
		Error:       state.Error,
		CreatedAt:   st.CreatedAt,
		UpdatedAt:   st.UpdatedAt,
	}
}

// hostPolicyDTO tells the UI what the allowlist currently does.
type hostPolicyDTO struct {
	Restricted bool     `json:"restricted"`
	Allowed    []string `json:"allowed"`
	Message    string   `json:"message"`
	Error      string   `json:"error,omitempty"`
}

// toHostPolicyDTO states plainly whether any host may be mounted.
func toHostPolicyDTO(p hostPolicy) hostPolicyDTO {
	dto := hostPolicyDTO{Restricted: p.restricted, Allowed: p.allowed}
	switch {
	case p.err != nil:
		dto.Error = p.err.Error()
		dto.Message = "storage_allowed_hosts has an invalid entry, so no storage can be mounted until it is fixed."
	case p.restricted:
		dto.Message = "Only the hosts listed in storage_allowed_hosts can be mounted."
	default:
		dto.Message = "Any host can be mounted. Set storage_allowed_hosts to limit which hosts this server may mount; setting it is recommended."
	}
	return dto
}

// storageListResponse is GET /api/v1/storages.
type storageListResponse struct {
	Items      []storageDTO  `json:"items"`
	Total      int           `json:"total"`
	HostPolicy hostPolicyDTO `json:"host_policy"`
}

// storageBody is the create and update request. Update is a full replacement,
// with two differences: an empty password keeps the stored one, and an absent
// enabled keeps the stored value.
type storageBody struct {
	Name     string `json:"name"`
	Protocol string `json:"protocol"`
	Source   string `json:"source"`
	Options  string `json:"options"`
	Username string `json:"username"`
	Domain   string `json:"domain"`
	Password string `json:"password"`
	ReadOnly bool   `json:"read_only"`
	Enabled  *bool  `json:"enabled"`
}

// input applies the NFS option default and returns the validated view.
func (b storageBody) input() storageInput {
	return storageInput{
		name:     b.Name,
		protocol: b.Protocol,
		source:   b.Source,
		options:  defaultStorageOptions(b.Protocol, b.Options),
		username: b.Username,
		domain:   b.Domain,
		password: b.Password,
		readOnly: b.ReadOnly,
	}
}

// writeStorageErrors answers 400 with every field error.
func writeStorageErrors(w http.ResponseWriter, errs []storageFieldError) {
	writeJSON(w, http.StatusBadRequest, struct {
		Error  string              `json:"error"`
		Code   string              `json:"code"`
		Errors []storageFieldError `json:"errors"`
	}{Error: "the storage settings are not valid", Code: "invalid_request", Errors: errs})
}

// ---- Gates and lookups ----------------------------------------------------

// storageWriteAllowed answers 403 when login is off (spec 4.6). It returns
// false after writing the reply. It runs before the body is read, so an
// unauthenticated install learns the reason and nothing else.
func (a *API) storageWriteAllowed(w http.ResponseWriter, r *http.Request) bool {
	enabled, err := a.loginEnabled(r)
	if err != nil {
		a.internal(w, err)
		return false
	}
	if !enabled {
		writeError(w, http.StatusForbidden, "login_required",
			"Turn on login in App settings before you manage storage. Storage can reach your network, so it stays locked until login is on.")
		return false
	}
	return true
}

// storedStorage loads the storage named by the path, or writes the 404.
func (a *API) storedStorage(w http.ResponseWriter, r *http.Request) (*store.Storage, bool) {
	st, err := a.st.GetStorage(r.Context(), r.PathValue("id"))
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "no such storage")
		return nil, false
	}
	if err != nil {
		a.internal(w, err)
		return nil, false
	}
	return st, true
}

// storageHostAllowed re-checks a stored source against the current allowlist
// before any mount-side call. A host removed from the list after the row was
// saved is refused here (403).
func (a *API) storageHostAllowed(w http.ResponseWriter, r *http.Request, st *store.Storage) bool {
	policy, err := a.storageHostPolicy(r)
	if err != nil {
		a.internal(w, err)
		return false
	}
	host, errs := checkStorageSource(st.Protocol, st.Source)
	if len(errs) > 0 {
		writeError(w, http.StatusConflict, "invalid_storage", "the saved source is not valid: "+errs[0].Message)
		return false
	}
	if !policy.allows(host) {
		writeError(w, http.StatusForbidden, "host_not_allowed", policy.refusal(host))
		return false
	}
	return true
}

// ---- Handlers -------------------------------------------------------------

// listStorages returns every storage with its status and the host policy.
// Reads stay open to any admin session, and with login off they are open to
// everyone, so the UI can explain why writes are locked.
func (a *API) listStorages(w http.ResponseWriter, r *http.Request) {
	rows, err := a.st.ListStorages(r.Context())
	if err != nil {
		a.internal(w, err)
		return
	}
	policy, err := a.storageHostPolicy(r)
	if err != nil {
		a.internal(w, err)
		return
	}
	items := make([]storageDTO, 0, len(rows))
	for _, st := range rows {
		items = append(items, toStorageDTO(st, a.mounter.Status(st)))
	}
	writeJSON(w, http.StatusOK, storageListResponse{Items: items, Total: len(items), HostPolicy: toHostPolicyDTO(policy)})
}

// getStorage returns one storage.
func (a *API) getStorage(w http.ResponseWriter, r *http.Request) {
	st, ok := a.storedStorage(w, r)
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, toStorageDTO(st, a.mounter.Status(st)))
}

// createStorage validates and stores a new storage (201).
func (a *API) createStorage(w http.ResponseWriter, r *http.Request) {
	if !a.storageWriteAllowed(w, r) {
		return
	}
	body, err := decode[storageBody](r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	policy, err := a.storageHostPolicy(r)
	if err != nil {
		a.internal(w, err)
		return
	}
	in := body.input()
	if errs := checkStorageInput(in, policy); len(errs) > 0 {
		writeStorageErrors(w, errs)
		return
	}
	if _, err := a.st.GetStorageByName(r.Context(), in.name); err == nil {
		writeError(w, http.StatusConflict, "conflict", fmt.Sprintf("a storage named %q already exists", in.name))
		return
	} else if !errors.Is(err, store.ErrNotFound) {
		a.internal(w, err)
		return
	}
	enabled := true
	if body.Enabled != nil {
		enabled = *body.Enabled
	}
	st := &store.Storage{
		Name: in.name, Protocol: in.protocol, Source: in.source, Options: in.options,
		Username: in.username, Domain: in.domain, Password: in.password,
		ReadOnly: in.readOnly, Enabled: enabled,
	}
	if err := a.st.CreateStorage(r.Context(), st); err != nil {
		a.internal(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, toStorageDTO(st, a.mounter.Status(st)))
}

// updateStorage replaces an existing storage. The name and the protocol are
// fixed: the name is the mountpoint that libraries point at, and changing the
// protocol would leave a sealed password on a row that no longer uses one.
func (a *API) updateStorage(w http.ResponseWriter, r *http.Request) {
	if !a.storageWriteAllowed(w, r) {
		return
	}
	unlock := a.storageLocks.lock(r.PathValue("id"))
	defer unlock()
	stored, ok := a.storedStorage(w, r)
	if !ok {
		return
	}
	body, err := decode[storageBody](r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	policy, err := a.storageHostPolicy(r)
	if err != nil {
		a.internal(w, err)
		return
	}
	in := body.input()
	errs := checkStorageInput(in, policy)
	if in.name != stored.Name {
		errs = append(errs, storageFieldError{"name", "the name cannot change, because it is the mountpoint that libraries use; create a new storage for another name"})
	}
	if in.protocol != stored.Protocol {
		errs = append(errs, storageFieldError{"protocol", "the protocol cannot change; create a new storage instead"})
	}
	if len(errs) > 0 {
		writeStorageErrors(w, errs)
		return
	}
	enabled := stored.Enabled
	if body.Enabled != nil {
		enabled = *body.Enabled
	}
	st := &store.Storage{
		ID: stored.ID, Name: in.name, Protocol: in.protocol, Source: in.source, Options: in.options,
		Username: in.username, Domain: in.domain, Password: in.password,
		ReadOnly: in.readOnly, Enabled: enabled,
	}
	if err := a.st.UpdateStorage(r.Context(), st); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "not_found", "no such storage")
			return
		}
		a.internal(w, err)
		return
	}
	writeJSON(w, http.StatusOK, toStorageDTO(st, a.mounter.Status(st)))
}

// deleteStorage removes a storage row. It refuses while the storage is mounted,
// so a row cannot vanish while its mount is still live.
func (a *API) deleteStorage(w http.ResponseWriter, r *http.Request) {
	if !a.storageWriteAllowed(w, r) {
		return
	}
	unlock := a.storageLocks.lock(r.PathValue("id"))
	defer unlock()
	st, ok := a.storedStorage(w, r)
	if !ok {
		return
	}
	if a.mounter.Status(st).Mounted {
		writeError(w, http.StatusConflict, "conflict", "unmount the storage before you delete it")
		return
	}
	if err := a.st.DeleteStorage(r.Context(), st.ID); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "not_found", "no such storage")
			return
		}
		a.internal(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// storageMountError answers a failed mount-side call. The default mounter
// answers 501; anything else is a failed attempt, with the executor's message.
func (a *API) storageMountError(w http.ResponseWriter, code string, err error) {
	if errors.Is(err, ErrStorageMountUnavailable) {
		writeError(w, http.StatusNotImplemented, "not_implemented", err.Error())
		return
	}
	writeError(w, http.StatusConflict, code, err.Error())
}

// mountStorage mounts a storage (task 66 does the work) under its lock.
func (a *API) mountStorage(w http.ResponseWriter, r *http.Request) {
	if !a.storageWriteAllowed(w, r) {
		return
	}
	unlock := a.storageLocks.lock(r.PathValue("id"))
	defer unlock()
	st, ok := a.storedStorage(w, r)
	if !ok || !a.storageHostAllowed(w, r, st) {
		return
	}
	if err := a.mounter.Mount(r.Context(), st); err != nil {
		a.storageMountError(w, "mount_failed", err)
		return
	}
	writeJSON(w, http.StatusOK, toStorageDTO(st, a.mounter.Status(st)))
}

// unmountStorage unmounts a storage under its lock.
func (a *API) unmountStorage(w http.ResponseWriter, r *http.Request) {
	if !a.storageWriteAllowed(w, r) {
		return
	}
	unlock := a.storageLocks.lock(r.PathValue("id"))
	defer unlock()
	st, ok := a.storedStorage(w, r)
	if !ok {
		return
	}
	if err := a.mounter.Unmount(r.Context(), st); err != nil {
		a.storageMountError(w, "unmount_failed", err)
		return
	}
	writeJSON(w, http.StatusOK, toStorageDTO(st, a.mounter.Status(st)))
}

// storageTestResponse is the POST /{id}/test body. A failed check is 200 with
// ok false, as the integration test does; the executor's message says why.
type storageTestResponse struct {
	OK    bool   `json:"ok"`
	Error string `json:"error,omitempty"`
}

// testStorage mounts, checks and unmounts a storage under its lock (task 66).
func (a *API) testStorage(w http.ResponseWriter, r *http.Request) {
	if !a.storageWriteAllowed(w, r) {
		return
	}
	unlock := a.storageLocks.lock(r.PathValue("id"))
	defer unlock()
	st, ok := a.storedStorage(w, r)
	if !ok || !a.storageHostAllowed(w, r, st) {
		return
	}
	if err := a.mounter.Test(r.Context(), st); err != nil {
		if errors.Is(err, ErrStorageMountUnavailable) {
			a.storageMountError(w, "test_failed", err)
			return
		}
		writeJSON(w, http.StatusOK, storageTestResponse{OK: false, Error: err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, storageTestResponse{OK: true})
}
