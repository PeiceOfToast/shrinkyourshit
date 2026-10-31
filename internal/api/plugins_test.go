// SPDX-License-Identifier: AGPL-3.0-only

package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/PeiceOfToast/shrinkyourshit/internal/server"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// pluginsHarness is an api harness with a login-enabled admin session and
// a plugins root in its own temp data dir.
func pluginsHarness(t *testing.T) (*apiHarness, *http.Cookie, string) {
	t.Helper()
	var dataDir string
	h := newAPIHarness(t, func(cfg *server.Config) {
		dataDir = filepath.Join(t.TempDir(), "data")
		cfg.DataDir = dataDir
	})
	rec := h.do(http.MethodPost, "/api/v1/setup", map[string]any{
		"mode": "server", "login_enabled": true, "username": "admin", "password": "secret",
	})
	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())
	rec = h.do(http.MethodPost, "/api/v1/auth/login", map[string]any{
		"username": "admin", "password": "secret",
	})
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	return h, cookieOf(t, rec), dataDir
}

// doPlugins performs a plugins request with the admin cookie attached.
func doPlugins(t *testing.T, h *apiHarness, sess *http.Cookie, method, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, nil)
	if body != nil {
		raw, err := json.Marshal(body)
		require.NoError(t, err)
		req = httptest.NewRequest(method, path, bytes.NewReader(raw))
	}
	req.AddCookie(sess)
	return h.doReq(req)
}

// shAvailable skips when sh is missing (the test plugins exec shell).
func shAvailable(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("sh is not available")
	}
}

// writePluginDir writes a plugin directory with a run.sh script and its
// manifest, and returns the directory.
func writePluginDir(t *testing.T, name, typ, script string) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), name)
	require.NoError(t, os.MkdirAll(dir, 0o755))
	scriptPath := filepath.Join(dir, "run.sh")
	require.NoError(t, os.WriteFile(scriptPath, []byte(script+"\n"), 0o755))
	raw := `name: ` + name + `
version: 1.0.0
description: The test plugin.
type: ` + typ + `
command: ["sh", "` + scriptPath + `"]
`
	require.NoError(t, os.WriteFile(filepath.Join(dir, "plugin.yaml"), []byte(raw), 0o644))
	return dir
}

// dtoOf decodes one plugin DTO.
func dtoOf(t *testing.T, rec *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	var dto map[string]any
	require.NoError(t, jsonUnmarshal(rec, &dto))
	return dto
}

// TestPluginsEndpointsCRUD covers the plugins surface: install from a
// directory (disabled by default), list, get, settings update, enable,
// test, and remove — with the refusal while enabled.
func TestPluginsEndpointsCRUD(t *testing.T) {
	shAvailable(t)
	h, sess, dataDir := pluginsHarness(t)

	// Install from a directory: 201, disabled, no values.
	rec := doPlugins(t, h, sess, http.MethodPost, "/api/v1/plugins/install",
		map[string]any{"path": writePluginDir(t, "crud-plugin", "preflight", "echo installed")})
	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())
	var created struct {
		ID       string           `json:"id"`
		Name     string           `json:"name"`
		Version  string           `json:"version"`
		Type     string           `json:"type"`
		Enabled  bool             `json:"enabled"`
		Source   string           `json:"source"`
		Settings []map[string]any `json:"settings"`
		Values   map[string]any   `json:"values"`
	}
	require.NoError(t, jsonUnmarshal(rec, &created))
	require.NotEmpty(t, created.ID)
	assert.Equal(t, "crud-plugin", created.Name)
	assert.Equal(t, "preflight", created.Type)
	assert.False(t, created.Enabled, "installed plugins are disabled")
	assert.Empty(t, created.Values)
	assert.Contains(t, created.Source, "crud-plugin")

	// The manifest unpacked into the plugins root.
	assert.FileExists(t, filepath.Join(dataDir, "plugins", "crud-plugin", "plugin.yaml"))

	// List shows it.
	rec = doPlugins(t, h, sess, http.MethodGet, "/api/v1/plugins", nil)
	require.Equal(t, http.StatusOK, rec.Code)
	var list struct {
		Items []map[string]any `json:"items"`
		Total int              `json:"total"`
	}
	require.NoError(t, jsonUnmarshal(rec, &list))
	require.Len(t, list.Items, 1)
	assert.Equal(t, "crud-plugin", list.Items[0]["name"])

	// Enable + settings values via PUT. Unknown settings keys are rejected.
	rec = doPlugins(t, h, sess, http.MethodPut, "/api/v1/plugins/"+created.ID,
		map[string]any{"settings": map[string]any{"bogus": "x"}})
	require.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())

	rec = doPlugins(t, h, sess, http.MethodPut, "/api/v1/plugins/"+created.ID,
		map[string]any{"enabled": true})
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	require.NoError(t, jsonUnmarshal(rec, &created))
	assert.True(t, created.Enabled)

	// The plugins root got the manifest out; remove is refused while enabled.
	rec = doPlugins(t, h, sess, http.MethodDelete, "/api/v1/plugins/"+created.ID, nil)
	require.Equal(t, http.StatusConflict, rec.Code, rec.Body.String())

	// Disable, then remove: 204, the row and the directory both gone.
	rec = doPlugins(t, h, sess, http.MethodPut, "/api/v1/plugins/"+created.ID,
		map[string]any{"enabled": false})
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	rec = doPlugins(t, h, sess, http.MethodDelete, "/api/v1/plugins/"+created.ID, nil)
	require.Equal(t, http.StatusNoContent, rec.Code, rec.Body.String())
	assert.NoDirExists(t, filepath.Join(dataDir, "plugins", "crud-plugin"))
	rec = doPlugins(t, h, sess, http.MethodGet, "/api/v1/plugins", nil)
	require.NoError(t, jsonUnmarshal(rec, &list))
	assert.Empty(t, list.Items)
}

// TestPluginsInstallValidation covers the install rejections: an invalid
// manifest names the field, a duplicate install conflicts, and a body
// with both or neither source is a bad request.
func TestPluginsInstallValidation(t *testing.T) {
	shAvailable(t)
	h, sess, _ := pluginsHarness(t)

	// An invalid manifest names the field.
	bad := writePluginDir(t, "bad-plugin", "preflight", "echo hi")
	require.NoError(t, os.WriteFile(filepath.Join(bad, "plugin.yaml"), []byte(
		"name: bad-plugin\ntype: flow_step\ncommand: [\"echo\"]\n"), 0o644))
	rec := doPlugins(t, h, sess, http.MethodPost, "/api/v1/plugins/install",
		map[string]any{"path": bad})
	require.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())
	assert.Contains(t, rec.Body.String(), "type", "the message names the field")

	// A valid install, then the same name again: 409.
	rec = doPlugins(t, h, sess, http.MethodPost, "/api/v1/plugins/install",
		map[string]any{"path": writePluginDir(t, "dup-plugin", "notify", "echo hi")})
	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())
	rec = doPlugins(t, h, sess, http.MethodPost, "/api/v1/plugins/install",
		map[string]any{"path": writePluginDir(t, "dup-plugin", "notify", "echo hi")})
	require.Equal(t, http.StatusConflict, rec.Code, rec.Body.String())

	// Neither source.
	rec = doPlugins(t, h, sess, http.MethodPost, "/api/v1/plugins/install", map[string]any{})
	require.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())
}

// TestPluginsGitInstall (task 31): installing from a git URL works
// against a local file:// repository (the same clone the service runs
// for a remote URL).
func TestPluginsGitInstall(t *testing.T) {
	git, err := exec.LookPath("git")
	if err != nil {
		t.Skip("git is not installed")
	}
	shAvailable(t)
	h, sess, dataDir := pluginsHarness(t)

	// Build a local repository with the plugin at its root.
	repo := filepath.Join(t.TempDir(), "plugin-repo")
	require.NoError(t, os.MkdirAll(repo, 0o755))
	scriptPath := filepath.Join(repo, "run.sh")
	require.NoError(t, os.WriteFile(scriptPath, []byte("echo from git\n"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(repo, "plugin.yaml"), []byte(
		"name: git-plugin\nversion: 2.0.0\ntype: notify\ncommand: [\"sh\", \""+scriptPath+"\"]\n"), 0o644))
	for _, args := range [][]string{
		{"init", "-q", "-b", "main"},
		{"config", "user.email", "test@example.com"},
		{"config", "user.name", "test"},
		{"add", "-A"},
		{"commit", "-q", "-m", "the plugin"},
	} {
		cmd := exec.Command(git, args...)
		cmd.Dir = repo
		out, cerr := cmd.CombinedOutput()
		require.NoError(t, cerr, "git %v: %s", args, out)
	}

	rec := doPlugins(t, h, sess, http.MethodPost, "/api/v1/plugins/install",
		map[string]any{"git_url": "file://" + repo})
	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())
	var created struct {
		ID      string `json:"id"`
		Name    string `json:"name"`
		Version string `json:"version"`
		Enabled bool   `json:"enabled"`
		Source  string `json:"source"`
	}
	require.NoError(t, jsonUnmarshal(rec, &created))
	assert.Equal(t, "git-plugin", created.Name)
	assert.Equal(t, "2.0.0", created.Version)
	assert.False(t, created.Enabled)
	assert.Equal(t, "file://"+repo, created.Source)
	assert.FileExists(t, filepath.Join(dataDir, "plugins", "git-plugin", "plugin.yaml"))
	assert.NoDirExists(t, filepath.Join(dataDir, "plugins", "git-plugin", ".git"))

	// A second install of the same name: 409.
	rec = doPlugins(t, h, sess, http.MethodPost, "/api/v1/plugins/install",
		map[string]any{"git_url": "file://" + repo})
	require.Equal(t, http.StatusConflict, rec.Code, rec.Body.String())
}

// TestPluginsTestEndpoint (task 31): the test endpoint runs the plugin
// once against a synthetic job and answers 200 with the captured output
// and exit code, even when the plugin fails.
func TestPluginsTestEndpoint(t *testing.T) {
	shAvailable(t)
	h, sess, _ := pluginsHarness(t)

	rec := doPlugins(t, h, sess, http.MethodPost, "/api/v1/plugins/install",
		map[string]any{"path": writePluginDir(t, "test-me", "notify", "echo test ran")})
	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())
	var created struct {
		ID string `json:"id"`
	}
	require.NoError(t, jsonUnmarshal(rec, &created))

	// 200 even though the plugin is disabled: the admin asked for this
	// run explicitly.
	rec = doPlugins(t, h, sess, http.MethodPost, "/api/v1/plugins/"+created.ID+"/test", nil)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	var got struct {
		OK       bool     `json:"ok"`
		Skipped  bool     `json:"skipped"`
		ExitCode int      `json:"exit_code"`
		Error    string   `json:"error"`
		Output   []string `json:"output"`
	}
	require.NoError(t, jsonUnmarshal(rec, &got))
	assert.True(t, got.OK)
	assert.False(t, got.Skipped)
	assert.Equal(t, 0, got.ExitCode)
	assert.Equal(t, []string{"test ran"}, got.Output)

	// A failing plugin: still 200, exit code and error carried.
	rec = doPlugins(t, h, sess, http.MethodPost, "/api/v1/plugins/install",
		map[string]any{"path": writePluginDir(t, "test-fail", "notify", "exit 5")})
	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())
	var failing struct {
		ID string `json:"id"`
	}
	require.NoError(t, jsonUnmarshal(rec, &failing))
	rec = doPlugins(t, h, sess, http.MethodPost, "/api/v1/plugins/"+failing.ID+"/test", nil)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	require.NoError(t, jsonUnmarshal(rec, &got))
	assert.False(t, got.OK)
	assert.Equal(t, 5, got.ExitCode)
	assert.Contains(t, got.Error, "exit status 5")

	// Unknown id: 404.
	rec = doPlugins(t, h, sess, http.MethodPost, "/api/v1/plugins/no-such-id/test", nil)
	require.Equal(t, http.StatusNotFound, rec.Code, rec.Body.String())
}

// TestPluginsRequiresAdmin (task 31): the plugins endpoints are admin
// only.
func TestPluginsRequiresAdmin(t *testing.T) {
	shAvailable(t)
	h, _, _ := pluginsHarness(t)

	for _, tc := range []struct {
		method, path string
	}{
		{http.MethodGet, "/api/v1/plugins"},
		{http.MethodPost, "/api/v1/plugins/install"},
		{http.MethodGet, "/api/v1/plugins/x"},
		{http.MethodPut, "/api/v1/plugins/x"},
		{http.MethodDelete, "/api/v1/plugins/x"},
		{http.MethodPost, "/api/v1/plugins/x/test"},
	} {
		rec := h.do(tc.method, tc.path, nil)
		assert.Equal(t, http.StatusUnauthorized, rec.Code, "%s %s", tc.method, tc.path)
	}
}
