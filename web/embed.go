// SPDX-License-Identifier: AGPL-3.0-only

// Package web embeds the built web UI. The Makefile and CI run the Vite
// build in web/ first, producing web/dist, which is compiled into the binary
// with go:embed. web/dist/index.html is a placeholder so the Go build works
// before the first Vite build.
package web

import "embed"

// Dist is the built web UI, rooted at the dist directory. Use fs.Sub to
// strip the prefix when handing it to the api package.
//
//go:embed all:dist
var Dist embed.FS
