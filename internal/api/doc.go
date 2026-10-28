// SPDX-License-Identifier: AGPL-3.0-only

// Package api implements the HTTP surface of the server: handlers, SSE events
// and auth middleware.
//
// API.md defines the contract: base path /api/v1, JSON request and response
// bodies, and errors as {"error": "...", "code": "snake_case"}. Task 01 only
// exposes /api/v1/health and serves the embedded web UI; later tasks register
// their routes here.
package api
