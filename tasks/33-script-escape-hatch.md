# Task 33: The script escape hatch, sandboxed

## Goal
`DECISIONS.md` item 9 asks for "a sandboxed script escape hatch (runtime decided later)". This task decides the runtime and builds it. A user writes a small program, compiles it to WebAssembly with WASI, installs it like any other plugin, and runs it at a plugin point. The sandbox is the point: the script gets the job's scratch directory, a stdin contract, captured output, a wall-clock deadline and a memory ceiling, and nothing else. It cannot read the library, the config, the database or the network, and it cannot start a process.

The runtime is `github.com/tetratelabs/wazero`: pure Go, no cgo, no native toolchain at run time, identical behaviour on linux, macOS and Windows, and every capability is granted by us rather than inherited from the host. Add it to `STACK.md` in the same commit with the one-line justification task 31's rule 3 asks for (nothing in `STACK.md` duplicates it; `os/exec` is for ffmpeg, not for user code).

## Read first
`tasks/31-plugins-v1.md`, `tasks/32-plugins-as-flow-steps.md`, `internal/plugin/manifest.go`, `internal/plugin/runner.go`, `internal/plugin/install.go`, `internal/plugin/proc_unix.go`, `internal/plugin/proc_windows.go`, `internal/store/plugins.go`, `internal/worker/runner.go` (where task 32 runs step plugins), `docs/plugins.md`, `STACK.md`, `.github/workflows/ci.yml`.

## Do

### 1. Runtime in the manifest
`internal/plugin/manifest.go`: add

```go
// Runtime is how a plugin's entry point executes. Native is task 31's
// subprocess. WASM is task 33's sandbox: a WASI module run by wazero,
// with no filesystem outside the job scratch dir and no network.
RuntimeNative = "native"
RuntimeWASM   = "wasm"
```

`runtime` defaults to `native` so every existing manifest keeps working byte for byte. With `runtime: wasm`, `entry` names the module file inside the plugin directory (`main.wasm` by convention), `command` must be absent (a manifest that sets both is rejected with a message naming both fields), `env` is still allowed, and `timeout_seconds` still clamps to 1 to 300. Add `memory_pages` (optional, default 1024 pages = 64 MiB, clamped to 16 to 4096 pages) with the reasoning in a comment and in the docs.

### 2. The sandbox
New file `internal/plugin/wasm.go`:

```go
// RunWASM executes a WASI module with the plugin contract. The module gets
// stdin, stdout, stderr, the job scratch dir preopened read-write and the
// env names from the manifest. No other directory, no clock precision
// guarantees, no network, no subprocesses.
func RunWASM(ctx context.Context, plugin Installed, job JobContext) (Result, error)
```

wazero configuration, all of it explicit:
- `wazero.NewRuntimeConfig().WithMemoryLimitPages(pages)` from the manifest.
- `WithCloseOnContextDone(true)` so the timeout actually stops a spinning module. Test this with a module that loops forever; a plain `context.WithTimeout` is not enough on its own.
- Preopen **only** `job.ScratchDir`, read-write, at guest path `.`. Nothing else: no `.` from the host cwd, no data dir, no library path.
- `WithArgs` carries the plugin name only. `WithEnv` carries the `SYS_*` names plus the manifest `env` names resolved the task 31 way. `WithStdin` carries the same job JSON task 31 and task 32 write, plus `"phase"` and, for a step, `"file_path"`.
- stdout and stderr go into the job log with `source: "plugin"`, exactly like a native plugin.
- The result protocol is task 32's, unchanged: the module may rewrite the file whose path it was given, or print one final `{"file": "<path>"}` line. The same scratch-only path check applies, so a module printing `/etc/passwd` fails.
- A module that calls `random_get` or reads the clock works; that is allowed and documented.

Failure and timeout: a non-zero WASI exit code fails the job when the plugin is a step or preflight and logs a warning for postprocess and notify, matching task 31's rules. A memory-limit trap, a missing export or a malformed module fails the same way with the wazero error text in the job log. Exceeding the deadline kills the module and fails the job.

### 3. Install and storage
`internal/plugin/install.go`: for `runtime: wasm` the install copies the plugin directory and verifies `entry` exists and parses as a wasm binary (magic bytes plus a `CompileModule` in a throwaway runtime), so a broken module is rejected at install time with the parse error instead of at the first job. Nothing else about install, the settings table or the API changes; scripts are plugins with a runtime, not a second concept.

### 4. Docs
`docs/scripts.md`, linked from `docs/plugins.md` and the README:
- What the escape hatch is for, and the one-sentence version of the sandbox: it can touch the file it was handed and nothing else.
- A complete worked example: a Go program reading the job JSON from stdin, rewriting the scratch file, printing the result line, and the exact build command `GOOS=wasip1 GOARCH=wasm go build -o main.wasm main.go`.
- The manifest for it, the env names available, the scratch-only rule, the memory and timeout limits and their defaults.
- An explicit and honest list of what the sandbox does **not** do: no CPU instruction budget (a spinning module is stopped by the deadline only), no per-syscall auditing, no protection against a module that writes a huge file into scratch (scratch is the job's own space and is cleaned up), and no native scripts at all in v1. That last one is a decision, not an omission: native scripts are arbitrary code with the server's access and stay out.

### 5. Tests without a wasm toolchain
CI has no TinyGo and must not gain one. Commit a handful of small fixture modules under `internal/plugin/testdata/` and keep each one under 2 KB, the same way the existing fake-ffmpeg fixtures are committed:
- `hello.wasm`: reads stdin, writes one line to stdout, exits 0.
- `rewrite.wasm`: opens the file path from its stdin JSON and writes different bytes to it.
- `spin.wasm`: loops forever, for the timeout test.
- `oom.wasm`: allocates past the memory limit.
- `escape.wasm`: tries to open `/etc/passwd` and reports the result on stdout.
- `badfile.wasm`: prints `{"file": "/etc/passwd"}`.

Assembly is fine (assemble once with `wat2wasm` and commit the bytes). Note in the notes file how each fixture was produced so they can be regenerated.

## Out of scope
A wasm plugin SDK in a second language, WASI preview 2, host functions beyond WASI (no ffmpeg handle, no database, no HTTP client), native scripts under any "just this once" flag, signing or verifying plugin code, resource accounting per plugin across jobs, `DECISIONS.md` changes.

## Acceptance
- [ ] A manifest with `runtime: wasm` and a valid `entry` installs; one that sets both `runtime: wasm` and `command`, and one whose entry does not parse, are both rejected with a message naming the field. Proven by tests.
- [ ] `hello.wasm` runs and its stdout reaches the job log with `source: plugin`. Proven by a unit test.
- [ ] `rewrite.wasm` changes the file, and the change is visible downstream exactly as in task 32's in-place test.
- [ ] `escape.wasm` cannot open `/etc/passwd`: the module fails to open it and the job log shows the module's own report. Proven by a test.
- [ ] `badfile.wasm` printing an outside path fails the job with the path in the message.
- [ ] `spin.wasm` is stopped by `timeout_seconds` and the job fails; `oom.wasm` traps at the memory limit and the job fails with the wazero error text. Both proven by tests.
- [ ] Default behaviour is untouched: every task 31 native plugin test still passes unchanged, and a manifest without `runtime` behaves exactly as before.
- [ ] `docs/scripts.md` exists with the worked Go example, and `STACK.md` lists wazero with its justification.
- [ ] `make test`, `make test-integration` and `make lint` are green.
