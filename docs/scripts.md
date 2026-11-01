# Scripts: the sandboxed escape hatch

A plugin is a full program on the server with the app's access (see
[docs/plugins.md](plugins.md)). The escape hatch is for a smaller job: a
small piece of logic that runs when a job is claimed or a step needs a
transformation, and that must not touch anything the file it is handed
lives next to. Write it once, compile it to WebAssembly, and install it
like any other plugin.

The sandbox in one sentence: **the script can read the job's JSON on stdin
and touch the file it was handed, and nothing else.** No library, no
config, no database, no network, no subprocesses, no other directory —
the runtime gives it exactly one directory (the job's scratch directory),
a wall-clock deadline and a memory ceiling, and nothing else.

The runtime is [wazero](https://wazero.io), a pure-Go WebAssembly runtime:
a script is a dependency of the app, not a toolchain, and it behaves
identically on every platform.

## The manifest

```yaml
name: swap-audio
version: 1.0.0
type: step
description: Rewrites the working file with an audio swap
author: you
runtime: wasm
entry: main.wasm
memory_pages: 64
```

`runtime` is optional and defaults to `native` — a manifest without it
behaves exactly as before. With `runtime: wasm`:

- `entry` is the module file inside the plugin directory (`main.wasm` by
  convention). A `command` must not be set: a manifest that sets both is
  rejected at install time with a message naming both fields.
- `memory_pages` is the module's memory budget in 64 KiB pages. The
  default is 1024 pages (64 MiB), clamped to 16–4096 pages (1 MiB–
  256 MiB). The ceiling exists so a runaway allocation is a fast, loud
  trap instead of a server-wide OOM; the floor exists so a manifest typo
  does not silently starve a module that needs real work space.
- `env` is allowed, `timeout_seconds` clamps to 1–300 seconds as before,
  and `settings` are collected in the UI exactly like a native plugin's.

## The contract

The script receives:

- **stdin** — the job JSON as one line, the same document a native plugin
  gets (see [docs/plugins.md](plugins.md)), plus `phase` and, for a step,
  `file_path`. For a step the `file_path` is relative to the scratch
  directory, because the module sees the scratch directory as its root.
- **stdout** — captured into the job log with `source: plugin`. A step
  plugin may print one final line `{"file": "<path>"}` to hand the rest
  of the chain a different file; the path must be inside the scratch
  directory, or the step fails.
- **the scratch directory** — preopened read-write at the guest path
  `.`. This is the *only* directory the script sees. A hook phase gives
  it no directory at all: its stdin JSON is the whole contract.
- **the environment** — `SYS_PLUGIN_NAME`, `SYS_PHASE`, `SYS_DATA_DIR`,
  `SYS_STEP_INDEX` (for a step) and the names the manifest's `env:` lists,
  resolved from the plugin's settings first, else the server's own
  environment.

A non-zero WASI exit code fails the job for preflight and step plugins
and is logged as a warning for postprocess and notify, the same policy as
native plugins. A memory-limit trap, a missing export or a malformed
module fails the same way, with the wazero error text in the job log.
Exceeding the deadline kills the module and fails the job.

## A worked example

A step plugin that rewrites the working file in place and hands the
original path down the chain (printing nothing):

```go
// SPDX-License-Identifier: AGPL-3.0-only
package main

import (
	"encoding/json"
	"os"
	"path/filepath"
)

func main() {
	var job struct {
		FilePath string `json:"file_path"`
		Settings map[string]string `json:"settings"`
	}
	if err := json.NewDecoder(os.Stdin).Decode(&job); err != nil {
		os.Exit(1)
	}
	// The guest path is relative to the preopen (the scratch directory).
	raw, err := os.ReadFile(job.FilePath)
	if err != nil {
		os.Exit(1)
	}
	// ... the actual work ...
	os.WriteFile(filepath.Join(filepath.Dir(job.FilePath), "out.txt"),
		append(raw, '\n'), 0o600)
}
```

Build it:

```console
GOOS=wasip1 GOARCH=wasm go build -o main.wasm main.go
```

A plugin that hands a *different* file down the chain instead prints one
final JSON line (the file must be inside the scratch directory):

```go
import "fmt"
// ...
fmt.Printf(`{"file": "%s"}`+"\n", relativePath)
```

Install it from its directory like any other plugin. A module that is not
a valid WebAssembly binary is rejected at install time with the parse
error, never at the first job.

## What the sandbox does not do

An honest list:

- **No CPU instruction budget.** A module that loops forever is stopped
  by the deadline and nothing else. The deadline is the only thing that
  stops a spinning script.
- **No per-syscall auditing.** The sandbox constrains *what* the script
  can reach (one directory, no network, no processes), not *how often*.
- **No write-size quota.** A script can fill its scratch directory with a
  huge file. Scratch is the job's own space and is cleaned up when the
  job ends; the job's disk-usage accounting is unchanged.
- **No native scripts.** There is no native-script escape hatch in v1 —
  native scripts are arbitrary code with the server's access, which is
  exactly what the sandbox exists to avoid.
- Reading the clock or calling `random_get` inside the module works and
  is allowed; the deadline and memory ceiling are the guarantees, not
  clock precision.
