// SPDX-License-Identifier: AGPL-3.0-only

package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	shrink "github.com/PeiceOfToast/shrinkyourshit"
	"github.com/stretchr/testify/require"
)

func TestInstallShellContainsServerURL(t *testing.T) {
	h := newAPIHarness(t)

	resp := h.do(http.MethodGet, "/install.sh", nil)
	require.Equal(t, http.StatusOK, resp.Code)
	body := resp.Body.String()

	// Acceptance: the served script carries this server's actual URL and
	// version, and no unreplaced markers.
require.Contains(t, body, `SERVER="http://example.com"`)
	require.Contains(t, body, `SCRIPT_VERSION="v0.0.0"`)
	require.NotContains(t, body, "__SHRINK_SERVER_URL__")
	require.NotContains(t, body, "__SHRINK_VERSION__")
	require.True(t, strings.HasPrefix(body, "#!/bin/sh"), "the script must start with its shebang")
	require.Equal(t, "text/x-shellscript; charset=utf-8", resp.Header().Get("Content-Type"))
}

func TestInstallPowerShellContainsServerURL(t *testing.T) {
	h := newAPIHarness(t)

	resp := h.do(http.MethodGet, "/install.ps1", nil)
	require.Equal(t, http.StatusOK, resp.Code)
	body := resp.Body.String()
	require.Contains(t, body, "http://example.com")
	require.Contains(t, body, "v0.0.0")
	require.NotContains(t, body, "__SHRINK_SERVER_URL__")
	require.NotContains(t, body, "__SHRINK_VERSION__")
}

func TestInstallShellBehindReverseProxy(t *testing.T) {
	h := newAPIHarness(t)

	req := httptest.NewRequest(http.MethodGet, "/install.sh", nil)
	req.Header.Set("X-Forwarded-Proto", "https")
	req.Host = "media.example.com"
	rec := httptest.NewRecorder()
	h.mux.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)
	require.Contains(t, rec.Body.String(), "https://media.example.com")
}

func TestInstallerRenderReplacesMarkers(t *testing.T) {
	sh := shrink.InstallShell("http://m:8765", "v1.2.3")
	require.Contains(t, sh, "http://m:8765")
	require.Contains(t, sh, "v1.2.3")
	require.NotContains(t, sh, "__SHRINK_")

	ps := shrink.InstallPowerShell("http://m:8765", "v1.2.3")
	require.Contains(t, ps, "http://m:8765")
	require.Contains(t, ps, "v1.2.3")
	require.NotContains(t, ps, "__SHRINK_")
}
