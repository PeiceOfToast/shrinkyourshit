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
- Skipped:
- Next task should know:
