# Task 11 notes: node lifecycle, explicit start and stop

## Plan (files in touch order)

1. `internal/store`: migration `0006_node_lifecycle.sql` (nodes gains
   `desired_state` TEXT default 'stopped', `concurrency` INTEGER default 1,
   `state_error` TEXT default ''); `Node` struct fields + validation;
   scan/create/update columns.
2. `internal/server`: `NodeRegistry` lifecycle methods (Start, Stop(drain),
   Drain via Stop, PauseAll/ResumeAll with a `paused_nodes` settings
   snapshot), heartbeat returns node concurrency + desired/effective state
   and completes a drain when the worker reports `stopped`, `EffectiveState`
   helper, DTO gains desired_state/effective_state/concurrency/state_error/
   current_job_id, Update gains concurrency (1..16).
3. `internal/api`: routes start/stop/drain/pause-all/resume-all, claim gate
   (409 `node_not_running` unless desired_state=running), heartbeat response
   shape, PUT concurrency, listNodes current_job_id, POST /setup gains
   `start_node` (+ `WithBuiltinNode` option wired by main).
4. `internal/worker`: client heartbeat gains `state` report and a parsed
   response struct; worker loop honours desired_state — no claim calls at
   all before the first heartbeat answers or while stopped/draining, aborts
   in-flight encodes (silent, sweeper requeues) on stop-now, reports
   `stopped` once a drain is idle; per-job cancel handles.
5. `cmd/shrinkyourshit/main.go`: wire builtin node id into the API for the
   wizard's start choice. No worker-start gate needed: the worker loop itself
   honours desired_state.
6. Docs: API.md task-11 details (heartbeat `state` report, claim 409,
   pause/resume bodies, setup `start_node`, state_error), ARCHITECTURE.md
   Node line + settings keys (rule 5).
7. Web: api.ts types/functions, NodesPage card (state pill, start/stop with
   drain checkbox, concurrency stepper, current job, ffmpeg version),
   AppShell global pause switch, SetupWizard step-3 start choice (default
   stopped), wizardModel, dashboardWarnings canRun, tests.

## Decisions taken inside the task's wording

- Migration marker: "owner already had work running" is read from the jobs
  table — nodes with claimed/running jobs at upgrade time stay `running`
  (their worker process is mid-job; stopping it would strand work). Everyone
  else wakes up `stopped`, including the built-in node of an idle install.
- `concurrency` is seeded from the existing `worker_concurrency` setting so
  an upgraded install keeps its throughput; new nodes default to 1. The
  heartbeat now returns the *node's* concurrency (API.md task 11), so the
  global setting is only a migration seed from here on.
- All newly created nodes (built-in, token-paired, pending) get
  `desired_state=stopped`: "a node never encodes unless a human started it".
- `error`/`state_error`: set on a node when a lifecycle transition fails to
  persist (e.g. a pause-all batch member), cleared by the next successful
  start/stop/drain. `effective_state` is `error` while set, otherwise it
  mirrors desired_state. Reason shown on the Nodes card.
- Stop-now abort: the worker cancels the encode contexts silently (no
  complete/fail report), so the job's heartbeat goes stale and the sweeper
  requeues it (attempt+1) — exactly the documented "returns to queued after
  the heartbeat timeout". The original file is never touched.
- Worker/back-compat: a heartbeat response with no `desired_state` (a
  pre-task-11 server) is treated as `running`, so old-server pairs still
  work; the claim gate lives in the API handler (nodeAuth already resolves
  the node), not in the store, so queue tests and third-party claims see the
  documented 409.
- pause-all snapshots each node's prior desired_state once; a repeat
  pause-all keeps the first snapshot so resume-all restores the true intent.

## Out of scope (per task file)

Scheduling beyond FIFO, node priority, per-node schedules, remote start of
the host process.

## Summary (written at the end)

- Built: node desired_state/concurrency/state_error columns + migration,
  lifecycle endpoints (start/stop/drain/pause-all/resume-all), heartbeat
  carries instructions and the worker reports drain completion, worker loop
  gates claims on desired_state and aborts on stop-now, setup wizard ends
  with an explicit start choice (default stopped), Nodes page cards with
  state pill/toggle/stepper and a global pause switch in the header.
- Skipped: PUT-based desired_state edits (state machine endpoints only),
  node priority/schedules (out of scope), dashboard banner text changes
  beyond the canRun predicate.
- Next task should know: heartbeat now returns `{concurrency,
  desired_state, effective_state}` and its body may carry `state`; the
  integration stack (`newStack`) starts the built-in node for the old
  end-to-end tests, so new tests that need a stopped node should stop it or
  pair a fresh (stopped) node; `TestWorkerConcurrencyFromSettings` became
  node-level.