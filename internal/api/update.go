// SPDX-License-Identifier: AGPL-3.0-only

package api

import (
	"errors"
	"net/http"

	"github.com/PeiceOfToast/shrinkyourshit/internal/server"
)

// updateStatus answers GET /api/v1/update/status (task 17): the current
// build against the last checked release, whether an update applies, and
// why self update is or is not supported. Never touches the network.
func (a *API) updateStatus(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, a.svc.Update().Snapshot())
}

// updateCheck answers POST /api/v1/update/check (task 17): forces a
// release refresh, rate limited to once a minute. A failed network check
// is a 200 whose status carries the quiet reason; only the rate limit and
// internal failures are errors.
func (a *API) updateCheck(w http.ResponseWriter, r *http.Request) {
	status, err := a.svc.Update().Check(r.Context())
	if errors.Is(err, server.ErrUpdateRateLimited) {
		writeError(w, http.StatusTooManyRequests, "rate_limited",
			"updates were checked less than a minute ago; try again shortly")
		return
	}
	if err != nil {
		a.internal(w, err)
		return
	}
	writeJSON(w, http.StatusOK, status)
}

// updateApply answers POST /api/v1/update/apply (task 17): starts the
// update for binary installs and reports progress as update.* events on
// the events channel; the 202 returns before any download starts. A
// container install is refused with the exact pull commands.
func (a *API) updateApply(w http.ResponseWriter, r *http.Request) {
	err := a.svc.Update().Apply(r.Context())
	var unsupported *server.SelfUpdateUnsupported
	switch {
	case errors.As(err, &unsupported):
		writeError(w, http.StatusConflict, "self_update_unsupported", unsupported.Reason)
		return
	case errors.Is(err, server.ErrAlreadyApplying):
		writeError(w, http.StatusConflict, "update_in_progress", "an update is already running")
		return
	case err != nil:
		a.internal(w, err)
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]bool{"started": true})
}
