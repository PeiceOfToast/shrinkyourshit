// SPDX-License-Identifier: AGPL-3.0-only

// Package shrinkyourshit is the module root package. It embeds the
// installer scripts that live in /scripts and serves them templated
// (task 09): the copy an admin pastes from the web UI already knows the
// server URL and the release version to download.
package shrinkyourshit

import (
	_ "embed"
	"strings"
)

//go:embed scripts/installer.sh
var installerShSource string

//go:embed scripts/installer.ps1
var installerPs1Source string

// Server URL and version markers inside the scripts; the served copy has
// them replaced, the raw source keeps them (and asks for flags instead).
const (
	serverURLMarker = "__SHRINK_SERVER_URL__"
	versionMarker   = "__SHRINK_VERSION__"
)

// InstallShell renders the POSIX installer for a server URL and version.
func InstallShell(serverURL, version string) string {
	return render(installerShSource, serverURL, version)
}

// InstallPowerShell renders the Windows installer for a server URL and
// version.
func InstallPowerShell(serverURL, version string) string {
	return render(installerPs1Source, serverURL, version)
}

func render(source, serverURL, version string) string {
	return strings.ReplaceAll(
		strings.ReplaceAll(source, versionMarker, version),
		serverURLMarker, serverURL,
	)
}
