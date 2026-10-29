// SPDX-License-Identifier: AGPL-3.0-only

package api

import (
	"errors"
	"net/http"
	"time"

	"github.com/PeiceOfToast/shrinkyourshit/internal/server"
	"github.com/PeiceOfToast/shrinkyourshit/internal/store"
)

// Admin node endpoints (API.md, task 08): list, pairing token creation,
// approve, revoke, update, delete. The built-in node appears here too —
// the admin can rename, disable or revoke it like any other node.

// listNodes answers GET /nodes.
func (a *API) listNodes(w http.ResponseWriter, r *http.Request) {
	nodes, err := a.st.ListNodes(r.Context())
	if err != nil {
		a.internal(w, err)
		return
	}
	items := make([]server.NodeDTO, 0, len(nodes))
	for _, n := range nodes {
		items = append(items, server.NewNodeDTO(n))
	}
	writeJSON(w, http.StatusOK, server.ListResponse[server.NodeDTO]{Items: items, Total: len(items)})
}

// installCommands is the ready-to-copy install command set returned by
// POST /nodes/tokens. The install script endpoints themselves arrive with
// task 09; here they are strings the admin can already copy.
type installCommands struct {
	Docker     string `json:"docker"`
	Shell      string `json:"shell"`
	PowerShell string `json:"powershell"`
}

// tokenResponse is the POST /nodes/tokens body: the raw token (shown once)
// and the commands embedding it.
type tokenResponse struct {
	Token     string          `json:"token"`
	ExpiresAt string          `json:"expires_at"`
	Commands  installCommands `json:"commands"`
}

// createNodeToken answers POST /nodes/tokens with a fresh one-time token
// and the three install commands built from this request's origin.
func (a *API) createNodeToken(w http.ResponseWriter, r *http.Request) {
	raw, expires, err := a.svc.Nodes().CreateToken(r.Context())
	if err != nil {
		a.internal(w, err)
		return
	}
	base := baseURL(r)
	writeJSON(w, http.StatusCreated, tokenResponse{
		Token:     raw,
		ExpiresAt: expires.UTC().Format(time.RFC3339),
		Commands: installCommands{
			Docker: "docker run -d --name shrinkyourshit-node " +
				"-e SHRINK_ROLE=worker " +
				"-e SHRINK_SERVER=" + base + " " +
				"-e SHRINK_TOKEN=" + raw + " " +
				"-e SHRINK_NODE_NAME=\"$(hostname)\" " +
				"-v /config:/config -v /media:/media -v /scratch:/scratch " +
				"ghcr.io/peiceoftoast/shrinkyourshit:latest",
			Shell: "curl -fsSL " + base + "/install.sh | sh -s -- --server " + base + " --token " + raw,
			PowerShell: "$env:SHRINK_SERVER='" + base + "'; " +
				"$env:SHRINK_TOKEN='" + raw + "'; " +
				"irm " + base + "/install.ps1 | iex",
		},
	})
}

// approveNode answers POST /nodes/{id}/approve: pending → approved with a
// fresh key waiting in key_once for the node's first poll.
func (a *API) approveNode(w http.ResponseWriter, r *http.Request) {
	node, err := a.svc.Nodes().Approve(r.Context(), r.PathValue("id"))
	if errors.Is(err, server.ErrNotPending) {
		writeError(w, http.StatusConflict, "not_pending", "node is not pending approval")
		return
	}
	if err != nil {
		a.nodeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, server.NewNodeDTO(node))
}

// revokeNode answers POST /nodes/{id}/revoke: the key stops working, the
// node's in-flight jobs return to the queue.
func (a *API) revokeNode(w http.ResponseWriter, r *http.Request) {
	node, err := a.svc.Nodes().Revoke(r.Context(), r.PathValue("id"))
	if err != nil {
		a.nodeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, server.NewNodeDTO(node))
}

// nodeBody is the PUT /nodes/{id} request. Pointers distinguish "not sent"
// from a zero value.
type nodeBody struct {
	Name            *string `json:"name"`
	Enabled         *bool   `json:"enabled"`
	AccessMode      *string `json:"access_mode"`
	PathMappings    *string `json:"path_mappings"`
	ThroughputLimit *int64  `json:"throughput_limit_mbps"`
}

// updateNode answers PUT /nodes/{id}: rename, enable/disable, access mode,
// path mappings and the transfer throughput limit.
func (a *API) updateNode(w http.ResponseWriter, r *http.Request) {
	body, err := decode[nodeBody](r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	node, err := a.svc.Nodes().Update(r.Context(), r.PathValue("id"), server.NodeUpdateParams{
		Name:            body.Name,
		Enabled:         body.Enabled,
		AccessMode:      body.AccessMode,
		PathMappings:    body.PathMappings,
		ThroughputLimit: body.ThroughputLimit,
	})
	if errors.Is(err, server.ErrPairInput) {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	if err != nil {
		a.nodeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, server.NewNodeDTO(node))
}

// deleteNode answers DELETE /nodes/{id} (the Reject action for a pending
// node, and cleanup for revoked ones).
func (a *API) deleteNode(w http.ResponseWriter, r *http.Request) {
	err := a.svc.Nodes().Delete(r.Context(), r.PathValue("id"))
	if err != nil {
		a.nodeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// nodeError writes the HTTP error for a node service result.
func (a *API) nodeError(w http.ResponseWriter, err error) {
	switch {
	case err == nil:
	case errors.Is(err, server.ErrPairInput):
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
	case errors.Is(err, server.ErrNotPending):
		writeError(w, http.StatusConflict, "not_pending", "node is not pending approval")
	case errors.Is(err, store.ErrNotFound):
		writeError(w, http.StatusNotFound, "not_found", "no such node")
	default:
		a.internal(w, err)
	}
}
