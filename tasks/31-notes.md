# Task 31 notes — Plugins, version one

## Plan

Branch `task/31-plugins-v1` from `main`. Commit order:

1. `internal/plugin` package: `manifest.go` (YAML parse with unknown-field
   rejection via `yaml.v3` `KnownFields`, name regex, type check, command
   shape with shell-metacharacter rejection, timeout clamp 1..300, default
   30) + tests for every rejection.
2. `internal/plugin/runner.go`: `Run(ctx, plugin, job, phase) Result` —
   job JSON on stdin, env `SYS_PLUGIN_NAME`/`SYS_PHASE`/`SYS_DATA_DIR` plus
   each declared env name resolved from plugin settings then the server
   environment, stdout/stderr captured line-wise, `context.WithTimeout`
   from the manifest, process group killed on timeout (unix: `Setpgid` +
   group SIGKILL via `cmd.Cancel`; windows: plain kill). Platform files
   follow the `internal/discovery` split.
3. Migration `0013_plugins.sql` (exact columns from the task), store CRUD
   in `internal/store/plugins.go`, interface additions, tests.
4. `internal/plugin/install.go`: git clone `--depth 1 [--branch ref]` to a
   temp dir, path copy, both validate `plugin.yaml` then copy the tree
   (minus `.git`) into `<data dir>/plugins/<name>/`, refusing an existing
   target. Tests: local `file://` git repo (skip when git is absent),
   path install, invalid manifest names the field.
5. Server-side `internal/server/plugins.go`: `PluginService` (list/install/
   update/test/delete + the three phases). Job-log lines get a new source
   value `plugin` (store validation + ARCHITECTURE.md updated in the same
   commit). Queue wiring: preflight gates `Claim` (failure fails the job
   with the plugin name and exit code; a plugin that cannot produce a
   verdict — missing required setting, timeout — is skipped with a warn
   and the job proceeds); postprocess/notify fan out through the existing
   notifier router (one path: `notifierRouter` gains the plugin receiver,
   `emitJobEvent` hands it the plugin context built where the original
   source path is still known).
6. Admin API `internal/api/plugins.go`: the five endpoints, registered in
   `api.go`, documented in `docs/openapi.yaml` (the anti-drift test gates
   this), tests in `internal/api/plugins_test.go`.
7. UI: `web/src/PluginsPage.tsx` (the Plugins tab), third tab on
   IntegrationsPage, `api.ts` client, component tests.
8. `docs/plugins.md` (manifest reference, phases with the exact stdin JSON,
   env contract, timeout and failure rules, a notify-plugin worked example,
   security statement incl. "no sandbox yet"), README link, `git` added to
   the Docker runtime image (it is absent today — noted in the PR body).

## Decisions made inside the task's freedom

- `status` in the plugin stdin JSON is the job's final status (`done` /
  `failed`; `claimed` for preflight). `phase` equals the plugin's manifest
  type. `output_path` is empty when no output was applied (skip, kept
  original, unverified output) — a postprocess plugin decides for itself.
- postprocess runs for `job.done` only; notify runs for `done` and
  `failed`. A preflight plugin that times out or cannot run does NOT fail
  the job (it produced no verdict — the warn line in the job log says why);
  a preflight plugin that exits non-zero fails the job.
- The plugins fan-out hangs off `notifierRouter` (the same object the
  queue already holds), so there is one notifier path; the queue reaches
  it through an optional interface that test fakes simply do not implement.
- The runtime Docker image ships neither curl nor python; the worked
  example in docs uses curl and says so.

## Summary (filled at the end)

- Built:
  - `internal/plugin`: the manifest (`plugin.yaml`, strict unknown fields, name regex `^[a-z0-9][a-z0-9-]{1,40}$`, three types, exec'd command with a shell-metacharacter blacklist, timeout clamped 1..300, settings entries), the runner (stdin JSON `plugin.JobContext`, env exactly `SYS_PLUGIN_NAME`/`SYS_PHASE`/`SYS_DATA_DIR` plus the manifest's `env` names resolved from settings first then the server env, whole process group killed on timeout, output capped at 1000 lines), and installs from a git URL (`git clone --depth 1`, optional ref) or a directory copy.
  - Store: migration `0013_plugins.sql` + `store.Plugin` CRUD (`CreatePlugin`, `GetPlugin`, `GetPluginByName`, `ListPlugins`, `ListEnabledPlugins`, `UpdatePlugin`, `DeletePlugin`).
  - Server: `PluginService` with the three phases — `Preflight` (synchronous, in `Queue.Claim`; a non-zero exit fails the job with the plugin name and exit code, a timeout does not), `PostJob` (detached goroutines off the notifier router: postprocess for done, notify for done+failed, failures are warn lines and never fail a job) — output appended to the job log with `source: plugin` and the plugin name leading the message. `ErrPreflightRejected` is mapped to 204 in the worker claim handler (the job is already failed; the node polls again).
  - Admin API: `GET /api/v1/plugins`, `POST /api/v1/plugins/install`, `GET/PUT/DELETE /api/v1/plugins/{id}`, `POST /api/v1/plugins/{id}/test` (200 always, with output/exit code/skip reason); OpenAPI updated; endpoint tests.
  - UI: `web/src/PluginsPage.tsx` as a third tab on the integrations page — DataList rows, install form (git URL + ref / folder mode), enable switch (explicit PUT), manifest-driven settings form, Test with captured output, Remove with a confirm dialog.
  - Docs: `docs/plugins.md` (manifest reference, stdin JSON, env contract, timeout/failure rules, a worked python `notify` example, security statement) linked from README and the plugins tab; the Dockerfile creates `/config/plugins` and ships `git` (clone runs inside the runtime container, task 31 requirement — noted in the PR body).
  - Doc sync (AGENTS.md rules 4/5): the endpoints are in `API.md` (task 31 section); `ARCHITECTURE.md` gains the `Plugin` data-model row, the `plugin` job-log source and a "Plugins (v1, task 31)" section replacing the post-MVP placeholder.
  - Runner bug fixed after the first full gates run: `Run` used `cmd.Run()` while two goroutines still read the `StdoutPipe`/`StderrPipe` — `Wait` closes those descriptors when the command exits, so lines still buffered in the pipe could vanish from the job log under load. That was the real cause of the intermittent `TestNotifyPhaseRunsForDoneAndFailed` integration failure. `Run` now starts, drains both pipes to EOF, then Waits; a regression test runs 3000 lines (past the 64 KiB pipe buffer) and requires all 1000 captured lines, which fails against the old ordering.
- Skipped: nothing in scope. Out of scope per the task file stayed out (no flow-step plugin, no marketplace, no sandbox, no SDK).
- Gates: `make test` (go + web, 214 web tests), a forced fresh `go test -count=1 -tags=integration ./...` and `make lint` all green after the fix. One earlier integration run failed `TestNotifyPhaseRunsForDoneAndFailed` under full parallel load — root-caused and fixed as above, not waved through.
- Session note: a second agent session working this branch committed the finished UI/docs work as `fd54c1a` ("task 31: docs/plugins.md, Dockerfile plugins dir, UI tests, lint fixes"), filled an earlier version of this summary and ticked the acceptance boxes mid-run (its record claimed green gates without noting the flake's cause, and its Dockerfile change did not yet add `git`). This continuation verified that commit against the task file, fixed the gaps (runner pipes, git, API.md/ARCHITECTURE.md), re-ran every gate and set the final record. `fd54c1a` is kept as-is; the follow-ups are separate commits.
- Next task should know: the plugin env contract is exact (nothing else inherited, no PATH/HOME in the child env — scripts spawn other programs with absolute paths); `Test` endpoint runs on disabled plugins too; the fetchMock test helper now exposes `routes` for mid-test route pushes; `plugin.Result` distinguishes "did not run" (`Ran`/`SkipReason`) from "ran and failed" — a preflight timeout is NOT a verdict and the job proceeds.
