// SPDX-License-Identifier: AGPL-3.0-only

package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strings"

	"github.com/PeiceOfToast/shrinkyourshit/internal/server"
)

// Pair endpoints (API.md, task 08). Open — an unpaired node has no key
// yet; this is how it asks to join. Token pairing approves immediately,
// tokenless pairing parks the node as pending until the admin approves.

// pairBody is the POST /pair request.
type pairBody struct {
	Token        string          `json:"token"`
	Name         string          `json:"name"`
	OS           string          `json:"os"`
	Arch         string          `json:"arch"`
	Capabilities json.RawMessage `json:"capabilities"`
}

// pairResponse is the POST /pair and GET /pair/{id} body (API.md task 08).
type pairResponse = server.PairResponse

// pair answers POST /pair.
func (a *API) pair(w http.ResponseWriter, r *http.Request) {
	body, err := decode[pairBody](r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	node, key, err := a.svc.Nodes().Pair(workerCtx(r), server.PairInput{
		Token:        body.Token,
		Name:         body.Name,
		OS:           body.OS,
		Arch:         body.Arch,
		Capabilities: strings.TrimSpace(string(body.Capabilities)),
	})
	switch {
	case errors.Is(err, server.ErrInvalidToken):
		writeError(w, http.StatusUnauthorized, "invalid_token", "pairing token is invalid, expired or already used")
		return
	case errors.Is(err, server.ErrPairInput):
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	case err != nil:
		a.internal(w, err)
		return
	}

	resp := pairResponse{NodeID: node.ID, Status: node.Status}
	if key != "" {
		resp.Status = "approved"
		resp.NodeKey = key
	}
	writeJSON(w, http.StatusCreated, resp)
}

// baseURL is this server's URL as the current request sees it: the request
// host plus the scheme (X-Forwarded-Proto wins, then TLS, then http). The
// install commands embed it so a node reaches the server the same way the
// admin's browser does — through a reverse proxy when there is one.
func baseURL(r *http.Request) string {
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	if p := r.Header.Get("X-Forwarded-Proto"); p != "" {
		scheme = p
	}
	return scheme + "://" + r.Host
}

// pairPoll answers GET /pair/{id}: what the worker polls while waiting for
// admin approval. The node key appears exactly once (then only the hash
// remains server-side).
func (a *API) pairPoll(w http.ResponseWriter, r *http.Request) {
	node, key, err := a.svc.Nodes().PollPair(workerCtx(r), r.PathValue("id"))
	if err != nil {
		a.nodeError(w, err)
		return
	}
	resp := pairResponse{NodeID: node.ID, Status: node.Status}
	if key != "" {
		resp.NodeKey = key
	}
	writeJSON(w, http.StatusOK, resp)
}

// setupServers answers GET /setup/servers: the servers discovered on the
// LAN for the wizard's "join" card. Once setup is complete the list is
// always empty — a configured install does not scan.
func (a *API) setupServers(w http.ResponseWriter, r *http.Request) {
	status, err := a.svc.Auth().Status(r.Context())
	if err != nil {
		a.internal(w, err)
		return
	}
	type serversResponse struct {
		Servers []server.DiscoveredServer `json:"servers"`
	}
	// Always an array in the JSON, never null (API.md list conventions).
	resp := serversResponse{Servers: []server.DiscoveredServer{}}
	if !status.SetupComplete {
		for _, info := range a.svc.Discovered(r.Context()) {
			resp.Servers = append(resp.Servers, server.DiscoveredServer{
				ID: info.ID, Name: info.Name, URL: info.URL, Version: info.Version,
			})
		}
	}
	writeJSON(w, http.StatusOK, resp)
}

// setupBody is the POST /setup request. Join adds the target server and an
// optional pairing token (task 08). start_node (task 11) is the wizard's
// explicit "start the built-in node now" choice: it defaults to false, so
// a completed wizard never begins encoding on its own.
type setupBody struct {
	Mode         string `json:"mode"`
	LoginEnabled bool   `json:"login_enabled"`
	Username     string `json:"username"`
	Password     string `json:"password"`
	Server       string `json:"server"`
	Token        string `json:"token"`
	StartNode    bool   `json:"start_node"`
}

// completeSetup answers POST /setup. mode "server" is the task-06 flow;
// mode "join" (task 08) records the choice (role=worker + server in the
// data dir, through the join callback main owns), marks setup complete and
// tells the process to switch to worker mode. Only accepted while setup is
// incomplete; afterwards 409.
func (a *API) completeSetup(w http.ResponseWriter, r *http.Request) {
	body, err := decode[setupBody](r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	if body.Mode == server.SetupModeJoin {
		a.completeSetupJoin(w, r, body)
		return
	}
	err = a.svc.Auth().CompleteSetup(r.Context(), server.SetupParams{
		Mode:         body.Mode,
		LoginEnabled: body.LoginEnabled,
		Username:     body.Username,
		Password:     body.Password,
	})
	if errors.Is(err, server.ErrSetupComplete) {
		writeError(w, http.StatusConflict, "setup_complete", "setup has already been completed")
		return
	}
	if errors.Is(err, server.ErrBadRequest) {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	if err != nil {
		a.internal(w, err)
		return
	}
	// The wizard's explicit start choice (task 11). The default is
	// stopped: finishing setup must never begin encoding on its own. A
	// failed start is logged, not fatal — setup is complete and the node
	// can be started from the Nodes page.
	if body.StartNode && a.builtinNode != "" {
		if _, err := a.svc.Nodes().Start(workerCtx(r), a.builtinNode); err != nil {
			a.log.Error("setup: start the built-in node", "err", err)
		}
	}
	status := server.Status{SetupComplete: true, LoginEnabled: body.LoginEnabled}
	if body.LoginEnabled {
		status.Username = body.Username
	}
	writeJSON(w, http.StatusCreated, status)
}

// completeSetupJoin runs the mode:"join" branch. Setup-completeness is
// checked before the callback fires (a second join must not re-write the
// config or re-trigger the role switch); the callback (wired by main)
// persists the decision before setup is marked complete, so a failure
// leaves setup retryable instead of half-joined.
func (a *API) completeSetupJoin(w http.ResponseWriter, r *http.Request, body setupBody) {
	if complete, err := a.svc.Auth().Status(r.Context()); err == nil && complete.SetupComplete {
		writeError(w, http.StatusConflict, "setup_complete", "setup has already been completed")
		return
	}
	target := strings.TrimSpace(body.Server)
	u, err := url.Parse(target)
	if target == "" || err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "a valid http(s) server URL is required to join")
		return
	}
	if a.join == nil {
		writeError(w, http.StatusInternalServerError, "internal", "joining is not wired into this process")
		return
	}
	if err := a.join(target, strings.TrimSpace(body.Token)); err != nil {
		a.internal(w, err)
		return
	}
	if err := a.svc.Auth().CompleteSetup(r.Context(), server.SetupParams{
		Mode:   server.SetupModeJoin,
		Server: target,
	}); err != nil {
		if errors.Is(err, server.ErrSetupComplete) {
			writeError(w, http.StatusConflict, "setup_complete", "setup has already been completed")
			return
		}
		a.internal(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, server.Status{
		SetupComplete: true,
		Mode:          server.SetupModeJoin,
		Server:        target,
	})
}
