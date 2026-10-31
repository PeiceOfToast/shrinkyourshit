# Plugins, version one

This document explains the hook plugins: what one is, how to write one, and
what the app guarantees. The plugin contract is versioned; this document is
version one.

## What a plugin is

A plugin is a directory on the server's disk with a `plugin.yaml` manifest at
its root and a program the manifest declares. When the app does something a
plugin cares about, it runs the plugin's program and hands it a JSON summary
of the event on stdin.

There are three phases:

- **preflight** — runs on the server right after a job is claimed, before the
  node sees it. The plugin's exit code decides whether the job proceeds:
  exit 0 lets it run, any other exit code fails the job immediately with the
  plugin's name in the error. A timeout is not a verdict: the job proceeds
  and the timeout is recorded in the job log.
- **postprocess** — runs after a job finishes **done** and its output was
  actually applied. It never runs for a failed job, and never for a
  completion where the result policy kept the original.
- **notify** — runs after every job ends, done or failed, with the job's
  final status in the input. It never runs for skipped completions, because
  no job ran.

Plugins are installed from a git URL (shallow-cloned) or from a directory
path, and are **disabled** until you enable them on purpose.

## The security statement

A plugin is a program on the server and the app runs it **as the server
user**. There is no sandbox: a plugin can read anything the app can read and
write anything the app can write. Only install plugins you trust. The
plugins page says this plainly.

## The manifest

```yaml
name: notify-telegram
version: 1.0.0
type: notify
description: Sends a Telegram message when a file finishes
author: you
command: ["python3", "main.py"]
timeout_seconds: 30
env:
  - TG_TOKEN
settings:
  - key: chat_id
    label: Chat id
    required: true
```

| Field | Required | Meaning |
| --- | --- | --- |
| `name` | yes | Lowercase letters, digits and dashes. A directory name and the identity in the UI. |
| `version` | yes | Shown next to the name. |
| `type` | yes | `preflight`, `postprocess` or `notify`. |
| `description` | no | Shown under the name. |
| `author` | no | Shown in the UI. |
| `command` | yes | The program's argv elements. Each element must be free of shell metacharacters — `| & ; < > ( ) $ \` \ " ' * ? [ ] # ~` and newlines — since the command is `exec`'d directly, never through a shell. |
| `timeout_seconds` | no | 1–300. The default is 30 seconds. |
| `env` | no | The names of extra environment variables to pass through (the app's other settings are not read). |
| `settings` | no | The collected values the UI asks for. |

## The stdin JSON

The plugin receives a JSON object on stdin:

```json
{
  "job_id": "…",
  "phase": "postprocess",
  "library": "Movies",
  "source_path": "/media/movies/Movie.mkv",
  "output_path": "/media/movies/Movie.mkv",
  "bytes_before": 4100000000,
  "bytes_after": 1200000000,
  "encoder": "sample",
  "status": "done"
}
```

- `phase` is the phase the plugin is running in (`preflight`, `postprocess`
  or `notify`).
- `source_path` is the file's path before the result policy applied the
  output, and `output_path` is the final path of an output that was applied
  (or `""` when none was).
- `status` is the job's final status: `done` for postprocess and notify, and
  `claimed` for preflight (the job has not run yet).
- `bytes_before` / `bytes_after` are the sizes the worker reported. A
  preflight plugin sees them at 0.

## The environment

The plugin's environment is minimal by design — exactly:

- `SYS_PLUGIN_NAME`, `SYS_PHASE`, `SYS_DATA_DIR` — the plugin's name, the
  phase it runs in and the server's data directory.
- Anything the manifest's `env:` names — resolved from the plugin's settings
  first (the values collected in the UI), else from the server's own
  environment.

Nothing else is inherited: no `PATH`, no `HOME`. A plugin's own program is
resolved by the system; if a plugin script needs other programs, use
absolute paths. The JSON summary on stdin is the only other channel.

## The settings

A plugin can declare `settings:` entries. The UI collects a value for each
and the server stores them (never logged, never echoed). Each becomes an
environment variable named by its key when the plugin runs. A required
setting with no value stops the plugin from running (skipped, with the
reason recorded in the job log).

## Output and exit codes

- **stdout lines** — captured as the plugin's output and written into the
  job's log. Exit 0 is success; non-zero is failure with the exit code
  recorded.
- **stderr lines** — captured as warnings in the job's log, prefixed with
  the plugin's name.
- **preflight** — exit 0 lets the job run; any other exit code fails it
  before the node sees it. stdout is still recorded in the job log.
- **timeout** — the whole process group is killed; the phase records a
  timeout. A preflight timeout does **not** fail the job.
- Output is capped at 1000 lines per run (the job log's own cap of 2000
  rows still applies on append).

## The plugins page

The Integrations page has a **Plugins** tab:

- **Install** — from a git URL (with an optional ref) or from a directory
  path the server can see. The new plugin is **disabled**: nothing runs
  until you enable it.
- **The switch** — enables or disables the plugin. Off means the plugin
  never runs.
- **Settings** — the manifest's declared keys.
- **Test** — runs the plugin once against a synthetic job. The answer is
  always 200: the body carries the run's captured output, the exit code and
  whether it was skipped.
- **Remove** — deletes a plugin's row and its directory. A plugin that is
  still on has to be turned off first; removal refuses while enabled.

## Worked example: a notify plugin that posts to a URL

A `notify` plugin receives the job summary on stdin and can post it
anywhere. This one reads the target URL from a setting named `HOOK_URL`
(the manifest's `env:` lists it, so the server feeds the collected value as
an environment variable) and posts the summary as JSON:

`plugin.yaml`:

```yaml
name: notify-webhook
version: 1.0.0
type: notify
description: Posts the job summary to a URL when a job ends.
author: you
command: ["/usr/bin/python3", "main.py"]
timeout_seconds: 15
env:
  - HOOK_URL
settings:
  - key: HOOK_URL
    label: The URL to post to
    required: true
```

`main.py`:

```python
import json, os, sys, urllib.request

summary = json.load(sys.stdin)          # the job summary on stdin
body = json.dumps({"hook_url": os.environ["HOOK_URL"], "job": summary})
req = urllib.request.Request(
    os.environ["HOOK_URL"],
    data=body.encode(),
    headers={"Content-Type": "application/json"},
)
print(urllib.request.urlopen(req).status)
```

The exit code is 0 when the post succeeds, so a failure is logged as a
warning and the job stays `done` — notify never fails a job that encoded
fine. The setting value is collected in the UI and stored in the row;
never logged, never echoed.
