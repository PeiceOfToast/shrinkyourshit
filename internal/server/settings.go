// SPDX-License-Identifier: AGPL-3.0-only

package server

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"strconv"
	"strings"

	"github.com/PeiceOfToast/shrinkyourshit/internal/store"
	"github.com/PeiceOfToast/shrinkyourshit/internal/update"
)

// Settings is the typed view of the settings key/value table. It knows the
// defaults so nothing else has to guess what an empty setting means.
type Settings struct {
	st  store.Store
	log *slog.Logger
}

// NewSettings builds the settings service.
func NewSettings(st store.Store, log *slog.Logger) *Settings {
	if log == nil {
		log = slog.Default()
	}
	return &Settings{st: st, log: log}
}

// Values is the GET /settings body (no secrets: the password hash is never
// part of this struct).
type Values struct {
	WorkerConcurrency   int      `json:"worker_concurrency"`
	ScratchDir          string   `json:"scratch_dir"`
	DefaultResultPolicy string   `json:"default_result_policy"`
	LoginEnabled        bool     `json:"login_enabled"`
	Username            string   `json:"admin_username"`
	PreferredLanguages  []string `json:"preferred_languages"`
	DefaultVideoCodec   string   `json:"default_video_codec"`
	UpdateCheck         bool     `json:"update_check"`
	UpdateChannel       string   `json:"update_channel"`
	BackupBeforeUpdate  bool     `json:"backup_before_update"`
}

// Video codec choices for the settings API (ARCHITECTURE.md easy-mode
// mapping: AV1 and H.264 are selectable in settings, not on the easy screen).
const (
	VideoCodecHEVC = "hevc"
	VideoCodecAV1  = "av1"
	VideoCodecH264 = "h264"
)

// VideoCodecValues lists the valid default_video_codec values.
func VideoCodecValues() []string {
	return []string{VideoCodecHEVC, VideoCodecAV1, VideoCodecH264}
}

// Defaults are used for unset or invalid settings.
const (
	DefaultWorkerConcurrency  = 1
	DefaultResultPolicy       = store.ResultPolicyReplaceIfSmaller
	DefaultPreferredLanguages = "eng"
	DefaultVideoCodec         = VideoCodecHEVC
	DefaultScratchDir         = "" // empty: the process config decides
	MinWorkerConcurrency      = 1
	MaxWorkerConcurrency      = 16
	// Self update (task 16): checks are on, the channel is stable and
	// database backups before an update are on. Nothing applies an
	// update by default — `shrink update` is always a human command.
	DefaultUpdateCheck        = true
	DefaultUpdateChannel      = update.ChannelStable
	DefaultBackupBeforeUpdate = true
)

// read parses one string setting with a fallback.
func read(settings map[string]string, key, fallback string) string {
	v, ok := settings[key]
	if !ok || strings.TrimSpace(v) == "" {
		return fallback
	}
	return strings.TrimSpace(v)
}

// Get reads every setting with defaults applied.
func (s *Settings) Get(ctx context.Context) (Values, error) {
	raw, err := s.st.ListSettings(ctx)
	if err != nil {
		return Values{}, fmt.Errorf("settings: read: %w", err)
	}
	out := Values{
		WorkerConcurrency:   DefaultWorkerConcurrency,
		ScratchDir:          read(raw, store.SettingScratchDir, DefaultScratchDir),
		DefaultResultPolicy: DefaultResultPolicy,
		LoginEnabled:        raw[store.SettingLoginEnabled] == "true",
		Username:            raw[store.SettingAdminUsername],
		PreferredLanguages:  []string{read(raw, store.SettingPreferredLanguages, DefaultPreferredLanguages)},
		DefaultVideoCodec:   DefaultVideoCodec,
		UpdateCheck:         DefaultUpdateCheck,
		UpdateChannel:       DefaultUpdateChannel,
		BackupBeforeUpdate:  DefaultBackupBeforeUpdate,
	}
	if n, err := strconv.Atoi(read(raw, store.SettingWorkerConcurrency, "1")); err == nil && n >= MinWorkerConcurrency && n <= MaxWorkerConcurrency {
		out.WorkerConcurrency = n
	}
	if p := read(raw, store.SettingDefaultResultPol, ""); validResultPolicy(p) {
		out.DefaultResultPolicy = p
	}
	if c := read(raw, store.SettingDefaultVideoCodec, ""); validVideoCodec(c) {
		out.DefaultVideoCodec = c
	}
	if raw[store.SettingUpdateCheck] == "false" {
		out.UpdateCheck = false
	}
	if c := read(raw, store.SettingUpdateChannel, ""); validUpdateChannel(c) {
		out.UpdateChannel = c
	}
	if raw[store.SettingBackupBeforeUpdate] == "false" {
		out.BackupBeforeUpdate = false
	}
	if langs := read(raw, store.SettingPreferredLanguages, ""); langs != "" {
		out.PreferredLanguages = splitLanguages(langs)
	}
	return out, nil
}

// validResultPolicy mirrors store validation without importing an error.
func validResultPolicy(p string) bool {
	for _, v := range store.ResultPolicyValues() {
		if v == p {
			return true
		}
	}
	return false
}

// validVideoCodec reports whether c is a known video codec choice.
func validVideoCodec(c string) bool {
	for _, v := range VideoCodecValues() {
		if v == c {
			return true
		}
	}
	return false
}

// validUpdateChannel reports whether c is a known update feed.
func validUpdateChannel(c string) bool {
	for _, v := range update.ChannelValues() {
		if v == c {
			return true
		}
	}
	return false
}

// splitLanguages parses the stored comma-separated language list.
func splitLanguages(raw string) []string {
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	sort.Strings(out)
	return out
}

// Update is one PUT /settings request. Field pointers distinguish "not
// sent" from a zero value so the UI can PUT a full document without
// accidentally clearing things it did not render.
type Update struct {
	WorkerConcurrency   *int
	ScratchDir          *string
	DefaultResultPolicy *string
	PreferredLanguages  *[]string
	DefaultVideoCodec   *string

	// Self update (task 16): whether the app checks for updates, which
	// release feed to follow, and whether the database is backed up
	// before an update. Nothing here ever applies an update.
	UpdateCheck        *bool
	UpdateChannel      *string
	BackupBeforeUpdate *bool

	// Login toggle. nil means "leave as is". Turning login on requires a
	// username (when none is stored) and a password in the same request.
	LoginEnabled *bool
	Username     string

	// Password: the initial password when enabling login, or a password
	// change when login is already on. A change requires CurrentPassword
	// to verify (login enabled) and never passes through here in clear
	// text further than to the auth service.
	Password        string
	CurrentPassword string
}

// Update validates and applies a settings change. The auth service carries
// the account parts (passwords never pass through this service).
func (s *Settings) Update(ctx context.Context, auth *Auth, u Update) error {
	current, err := s.Get(ctx)
	if err != nil {
		return err
	}

	if u.WorkerConcurrency != nil {
		if *u.WorkerConcurrency < MinWorkerConcurrency || *u.WorkerConcurrency > MaxWorkerConcurrency {
			return badRequest("worker_concurrency must be %d to %d", MinWorkerConcurrency, MaxWorkerConcurrency)
		}
		if err := s.st.SetSetting(ctx, store.SettingWorkerConcurrency, strconv.Itoa(*u.WorkerConcurrency)); err != nil {
			return fmt.Errorf("settings: concurrency: %w", err)
		}
	}
	if u.ScratchDir != nil {
		if err := s.st.SetSetting(ctx, store.SettingScratchDir, strings.TrimSpace(*u.ScratchDir)); err != nil {
			return fmt.Errorf("settings: scratch dir: %w", err)
		}
	}
	if u.DefaultResultPolicy != nil {
		if !validResultPolicy(*u.DefaultResultPolicy) {
			return badRequest("default_result_policy must be one of %v", store.ResultPolicyValues())
		}
		if err := s.st.SetSetting(ctx, store.SettingDefaultResultPol, *u.DefaultResultPolicy); err != nil {
			return fmt.Errorf("settings: result policy: %w", err)
		}
	}
	if u.PreferredLanguages != nil {
		if err := s.st.SetSetting(ctx, store.SettingPreferredLanguages, strings.Join(*u.PreferredLanguages, ",")); err != nil {
			return fmt.Errorf("settings: preferred languages: %w", err)
		}
	}
	if u.DefaultVideoCodec != nil {
		if !validVideoCodec(*u.DefaultVideoCodec) {
			return badRequest("default_video_codec must be one of %v", VideoCodecValues())
		}
		if err := s.st.SetSetting(ctx, store.SettingDefaultVideoCodec, *u.DefaultVideoCodec); err != nil {
			return fmt.Errorf("settings: video codec: %w", err)
		}
	}
	if u.UpdateCheck != nil {
		if err := s.st.SetSetting(ctx, store.SettingUpdateCheck, strconv.FormatBool(*u.UpdateCheck)); err != nil {
			return fmt.Errorf("settings: update check: %w", err)
		}
	}
	if u.UpdateChannel != nil {
		if !validUpdateChannel(*u.UpdateChannel) {
			return badRequest("update_channel must be one of %v", update.ChannelValues())
		}
		if err := s.st.SetSetting(ctx, store.SettingUpdateChannel, *u.UpdateChannel); err != nil {
			return fmt.Errorf("settings: update channel: %w", err)
		}
	}
	if u.BackupBeforeUpdate != nil {
		if err := s.st.SetSetting(ctx, store.SettingBackupBeforeUpdate, strconv.FormatBool(*u.BackupBeforeUpdate)); err != nil {
			return fmt.Errorf("settings: backup before update: %w", err)
		}
	}

	if u.LoginEnabled == nil {
		// No login change; a password change may still be requested.
		if u.Password != "" {
			return auth.ChangePassword(ctx, ChangePasswordParams{
				CurrentPassword: u.CurrentPassword,
				NewPassword:     u.Password,
			})
		}
		return nil
	}

	switch {
	case *u.LoginEnabled && !current.LoginEnabled:
		// Turning login on needs a working account in the same request.
		username := u.Username
		if username == "" {
			username = current.Username
		}
		if strings.TrimSpace(username) == "" {
			return badRequest("username is required when enabling login")
		}
		if u.Password == "" {
			return badRequest("password is required when enabling login")
		}
		return auth.EnableLogin(ctx, username, u.Password)
	case !*u.LoginEnabled && current.LoginEnabled:
		if err := auth.DisableLogin(ctx); err != nil {
			return err
		}
	default:
		// Login state unchanged; still apply a password change if asked.
		if u.Password != "" {
			return auth.ChangePassword(ctx, ChangePasswordParams{
				CurrentPassword: u.CurrentPassword,
				NewPassword:     u.Password,
			})
		}
	}
	return nil
}
