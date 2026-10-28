// SPDX-License-Identifier: AGPL-3.0-only

// Package webui embeds the built web UI into the binary.
//
// The Vite build in web/ produces web/dist; `make build` copies that into
// this package's dist directory, which is compiled in with go:embed. The
// dist directory here is build output only (gitignored); a placeholder
// index.html is created by `make placeholder` so the Go build works before
// the first Vite build.
//
// Why an embed package separate from web/: web/ carries a stub go.mod so the
// Go toolchain never descends into web/node_modules (vendored Go files there
// would otherwise be picked up by ./... patterns). A package inside the main
// module must therefore not live under web/.
package webui

import "embed"

// Dist is the built web UI, rooted at the dist directory. Use fs.Sub to
// strip the prefix when handing it to the api package.
//
//go:embed all:dist
var Dist embed.FS
