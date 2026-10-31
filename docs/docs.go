// SPDX-License-Identifier: AGPL-3.0-only

// Package docs holds the shipped OpenAPI document (task 25). The file
// lives beside this package because go:embed cannot reach a parent
// directory; internal/api imports it and serves the document.
package docs

import _ "embed"

// OpenAPIYAML is docs/openapi.yaml verbatim.
//
//go:embed openapi.yaml
var OpenAPIYAML string
