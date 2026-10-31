# Task 31: Plugins, version one

## Goal
A user can install a plugin from a git URL or a directory, enable it, and have it run at defined moments in a job's life. Installed plugins are visible, disableable and removable. Nothing runs unless it was installed and enabled on purpose.

This is roadmap item 2 (plugin repos and a bundled plugin format) and the groundwork for the script escape hatch. Version one deliberately stops at **hooks**, not flow steps: `preflight`, `postprocess` and `notify` run around a job.

## Read first
`internal/server/queue.go` (where a job is claimed, completed and failed), `internal/server/resultpolicy.go`, `internal/server/settings.go`, `internal/server/joblog.go` (task 27, the log the plugin output goes into), `internal/store/migrations/0012_job_logs.sql`, `internal/api/api.go`, `internal/api/webhooks.go` (the closest existing shape: a table, a dispatcher, an admin API, a test endpoint), `web/src/IntegrationsPage.tsx`, `docker/Dockerfile` (check whether `git` is present in the runtime image).

## Do

### 1. Manifest format
`plugin.yaml` at the root of a plugin directory:

```yaml
name: notify-telegram
version: 1.0.0
description: Send a Telegram message when a file finishes.
author: optional
type: postprocess          # preflight | postprocess | notify
command: ["python3", "main.py"]
timeout_seconds: 30        # default 30, max 300
env:                       # names only, values come from settings or the job
  - TELEGRAM_BOT_TOKEN
settings:                  # collected on install and shown in the UI
  - key: chat_id
    label: Chat id
    required: true
```

Rules: unknown top level fields are an error (so a typo fails loudly), `name` must match `^[a-z0-9][a-z0-9-]{1,40}$`, `type` must be one of the three, `command` must be a non-empty array with no shell metacharacters (it is `exec`, not a shell), and `timeout_seconds` is clamped to 1 to 300. Validation lives in `internal/plugin/manifest.go` with tests for each rejection.

### 2. Storage and execution
- Migration `0013_plugins.sql`: `plugins (id TEXT PRIMARY KEY, name TEXT UNIQUE NOT NULL, version TEXT NOT NULL, type TEXT NOT NULL, enabled INTEGER NOT NULL DEFAULT 0, source TEXT NOT NULL, settings TEXT NOT NULL DEFAULT '{}', installed_at TEXT NOT NULL)`.
- Plugins unpack under `<data dir>/plugins/<name>/`, which is on the `/config` volume so they survive a container replace.
- `internal/plugin/runner.go`: `Run(ctx, plugin, job, phase) Result`. The process gets:
  - the job context as JSON on **stdin** (`job_id`, `library`, `source_path`, `output_path`, `bytes_before`, `bytes_after`, `encoder`, `status`, `phase`),
  - env: `SYS_PLUGIN_NAME`, `SYS_PHASE`, `SYS_DATA_DIR`, plus every name in `env` resolved from the plugin settings and the server environment. A required setting that is missing means the plugin does not run and the reason is logged; it never falls back to a guess.
  - stdout and stderr both captured into the job log with `source: "plugin"` and the plugin name in the message, so task 27's viewer shows plugin output next to ffmpeg.
  - `context.WithTimeout` from the manifest, and the whole process group killed on timeout.
- Failure rules, which matter: a failing `preflight` fails the job with the plugin's name and exit code in the error; a failing `postprocess` or `notify` logs a warning and **never** fails a job that encoded fine. A plugin cannot block a job forever.
- Run in the same place webhooks and library refresh already hook in, so there is one notifier path, not three.

### 3. Install sources
- `POST /api/v1/plugins/install` with either `{"git_url": "...", "ref": "main"}` or `{"path": "/abs/dir"}`. Git installs clone to a temp dir with `git clone --depth 1`, read and validate `plugin.yaml`, then move into place. A path install copies a directory that must already be visible to the server.
- Verify `git` exists in the runtime image; if it does not, add it in the Dockerfile and say so in the PR body.
- Installed plugins are **disabled by default**. Enabling is a separate, explicit action, and the UI says plainly that a plugin runs as the server user with the same access to the media folders.
- Removal deletes the directory and the row, and refuses while the plugin is enabled.

### 4. API and UI
```
GET    /api/v1/plugins
POST   /api/v1/plugins/install
PUT    /api/v1/plugins/{id}          # enabled, settings values
POST   /api/v1/plugins/{id}/test     # run once against a synthetic job, return its output
DELETE /api/v1/plugins/{id}
```
Admin only, existing error shape. The test endpoint returns the captured output and exit code, 200 even when the plugin fails.

UI: `web/src/PluginsPage.tsx`, a tab beside Servers and Webhooks on the integrations page (task 23/25), showing each plugin with name, version, type, enabled `Switch`, an Install form (git URL plus ref, or a path), a settings form generated from the manifest, a Test button that shows output, and Remove. 44 px controls, `DataList` cards below `sm`.

### 5. Docs
`docs/plugins.md`: the manifest reference, the three phases with the exact JSON a plugin receives, the env contract, the timeout and failure rules, a worked example (a `notify` plugin that posts to a URL), and the security statement in plain words. Link it from the README and from the plugins tab.

## Out of scope
A plugin as a flow step, a plugin marketplace or index, signing or sandboxing beyond the process timeout (say explicitly in the docs that there is no sandbox yet), plugin-to-plugin dependencies, a plugin SDK in another language.

## Acceptance
- [x] A plugin directory with a valid manifest installs, appears disabled, and does nothing until enabled. Proven end to end in a test.
- [x] Installing from a git URL works against a local `file://` repository inside the test, and an invalid manifest is rejected with a message naming the field.
- [x] `preflight` output appears in the job log with `source: plugin`; `postprocess` failure leaves the job `done` with a warning; `preflight` failure fails the job with the plugin name in the error.
- [x] A plugin that sleeps past its timeout is killed and the job continues, proven by a test with a 1 second timeout.
- [x] A plugin requiring a missing setting does not run, and the job log says why.
- [x] Unknown manifest fields, a bad name and a bad command shape are each rejected with a test.
- [x] The plugins tab lists, enables, tests and removes a plugin, with tests; `docs/plugins.md` exists and is linked.
- [x] `make test`, `make test-integration` and `make lint` are green.
