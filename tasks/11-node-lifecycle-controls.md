# Task 11: Node lifecycle, explicit start and stop

## Goal
A node never encodes unless a human started it. Nodes get an explicit lifecycle with start, stop, drain and a pause everything switch, Tdarr style. Today the built in node is created enabled, `svc.Start(ctx)` starts the worker loop on every boot, there is no start or stop control, and there is no drain state.

## Read first
UI review at `docs/ui-review-2026-10-29.md` section 6. API.md (task 05 and 09 node endpoints). ARCHITECTURE.md (job lifecycle, worker state machine). Depends on tasks 05, 08, 09.

## Do
- Store: add `desired_state` (text, one of `stopped`, `running`, `draining`, default `stopped` for new installs) and `concurrency` (integer, default 1) to the nodes table. Add a migration; do not break existing rows, and migrate already created built in nodes to `running` only when a marker setting says the owner already had work running, otherwise `stopped`.
- Server: the heartbeat response carries `desired_state` and `concurrency` for that node, so a worker learns its instruction from the heartbeat it already sends. No new port, no long poll, no push channel.
- Worker loop: honour `desired_state`.
  - `stopped`: no claim calls at all. Heartbeat only.
  - `running`: claim up to `concurrency` jobs.
  - `draining`: finish jobs already claimed, claim nothing new, then report `stopped` once idle.
  - `error`: a transition that failed to apply, with the reason recorded for the UI.
- API (document in API.md):
  - `POST /api/v1/nodes/{id}/start`
  - `POST /api/v1/nodes/{id}/stop` with `{drain: bool}`
  - `POST /api/v1/nodes/{id}/drain`
  - `POST /api/v1/nodes/pause-all` and `POST /api/v1/nodes/resume-all`
  - `PUT /api/v1/nodes/{id}` gains `concurrency` (1 to 16, else `400 invalid_request`)
  - `GET /api/v1/nodes` returns `desired_state`, `effective_state`, `concurrency`, and `current_job_id`.
- Keep the existing `enabled` flag working: it still gates `nodeAuth`. `desired_state` is the operator intent; `enabled` is the hard off switch. Document the difference in API.md so nobody wonders why there are two.
- Setup wizard: end with an explicit choice, "Start the built in node now" or "Leave it stopped". Default is stopped. Never begin encoding because a wizard was completed.
- Nodes page: one card per node with a state pill (Running, Stopped, Draining, Error), a single primary toggle, a "finish the current job first" checkbox when stopping, a concurrency stepper, the current job with percent and ETA, last heartbeat, encoders, and ffmpeg version.
- Header: a global "Pause all encoding" switch that is visible on every page.
- Sanity: stopping must never delete a job. A claimed job that loses its worker returns to `queued` after the heartbeat timeout, which the runner already does.

## Out of scope
Scheduling policy beyond FIFO, node priority, per node schedules, remote start of the host process itself.

## Acceptance
- [x] A fresh install with a library and queued files claims nothing until the node is started. (Go test: worker with `desired_state=stopped` makes zero claim calls, heartbeat only)
- [x] `POST /api/v1/nodes/{id}/start` results in claims on the next heartbeat, without restarting the process. (`TestNodeStartTakesEffectOnHeartbeat`)
- [x] `POST /api/v1/nodes/{id}/stop` with `drain: true` finishes the in flight job, claims nothing new, and ends in `stopped`. (`TestNodeDrainFinishesCurrentJob`)
- [x] `stop` with `drain: false` stops claiming immediately and the in flight job returns to `queued` after the heartbeat timeout, with the original file untouched. (`TestNodeStopRequeuesJob`)
- [x] `pause-all` stops every node in one call, `resume-all` restores the previous per node intent. (`TestPauseAllRestoresIntent`)
- [x] `concurrency` above 16 returns `400 invalid_request`. (`TestNodeConcurrencyValidation`)
- [x] The setup wizard defaults to stopped and a completed wizard with no start choice leaves `desired_state=stopped`. (`SetupWizard.test.tsx`, `TestSetupStartNodeChoice`)
- [x] The Nodes page renders the state pill, the toggle and the concurrency stepper, and calls the right endpoint for each. (`NodesPage.test.tsx`)
- [x] API.md documents every new endpoint and the enabled versus desired_state distinction.
