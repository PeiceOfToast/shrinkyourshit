# Task 33 notes: the script escape hatch, sandboxed

## Plan

1. `internal/plugin/manifest.go`: add `Runtime` (`native` | `wasm`, default
   `native`), `Entry`, `MemoryPages` (default 1024, clamp 16..4096). With
   `runtime: wasm`, `command` must be absent and `entry` must be a plain
   relative file name; `entry`/`memory_pages` on a native manifest are
   rejected (naming the field) so a typo fails at install time.
2. `internal/plugin/wasm.go`: `RunWASM(ctx, Plugin, JobContext, scratch)` —
   wazero runtime with `WithMemoryLimitPages`, `WithCloseOnContextDone(true)`,
   exactly one dir preopen (the job scratch dir, read-write; hook phases get
   a fresh temp dir removed after the run), stdin = the job JSON, args =
   plugin name, env = SYS_* + manifest env names, stdout/stderr captured
   into `Result.Output`.
3. Route inside `Runner.Run` / `Runner.RunStep` when `Manifest.Runtime ==
   wasm`, so no caller changes. Step protocol (in-place rewrite or
   `{"file": ...}` handover) and the scratch-only path check are reused.
4. `internal/plugin/install.go`: wasm installs verify `entry` exists and
   parses (magic bytes + `CompileModule` in a throwaway runtime).
5. `internal/server` `ResolveSteps` and `server.PluginStep` gain
   Runtime/Entry/MemoryPages so the node can run wasm steps from its own
   install dir. No store migration (the manifest on disk stays the truth).
6. `internal/plugin/testdata/gen/main.go` (testdata is ignored by the go
   tool): a hand-rolled wasm binary emitter producing the six fixture
   modules; regenerate with
   `go run ./internal/plugin/testdata/gen/main.go`.
7. Tests: manifest rejections, install-time parse rejection, hello stdout
   into the job log, rewrite visible downstream, escape/badfile, spin
   timeout, oom trap, and the task 31 native suite untouched.
8. Docs: `docs/scripts.md` (worked Go example, honest sandbox limits),
   linked from `docs/plugins.md` and the README; `STACK.md` gains wazero
   with its justification.

## Fixture production

No wabt (`wat2wasm`) on the build box. The fixtures are emitted by
`internal/plugin/testdata/gen/main.go`, which hand-encodes the wasm binary
format (LEB128, sections, code bodies) and writes the six modules under
`internal/plugin/testdata/`. Each is a few hundred bytes (< 2 KB). All
imports are `wasi_snapshot_preview1` (`fd_read`, `fd_write`, `fd_close`,
`path_open`, `proc_exit`); `spin.wasm` has no imports at all (a pure
infinite loop) and `oom.wasm` grows memory past the runtime limit.

Regenerate: `go run ./internal/plugin/testdata/gen/main.go`.

## Pending / questions

- wazero preopen guest path: the task says "guest path `.`"; wazero's
  `FSConfig.WithDirMount` takes the guest path as a mount point — resolved
  empirically once the module compiles (see the wasm.go comment). The
  sandbox rule is unchanged either way: exactly one directory (the job
  scratch dir), nothing else, no host cwd.

## Summary

- Built: the wasm runtime (wazero) in the manifest (`runtime: wasm`,
  `entry`, `memory_pages`), the sandbox (`internal/plugin/wasm.go`: one
  preopen at guest `.`, stdin JSON, captured output, deadline and memory
  ceiling), install-time module validation (`CompileModule` + magic
  bytes), the six committed fixture modules with a hand-rolled wasm
  emitter (`internal/plugin/testdata/gen/main.go` — no assembler toolchain
  in CI), and docs/scripts.md with the worked Go example.
- Fixed en route: the fixture emitter's double-run bug (the `_start` export
  plus a start section made wazero execute the body twice — export only
  now), a swapped i32.store in escape.wasm's errno digits, a missing scan
  increment in rewrite.wasm (it spun to the deadline), and the runner's
  exit-code path reporting a trap as "exit status -1" instead of the wazero
  error text.
- Skipped: nothing from the task file. Native scripts stay out (decision,
  not an omission).
- Next task should know: fixture modules are generated, not committed
  media; regenerate with `go run ./internal/plugin/testdata/gen/main.go
  internal/plugin/testdata`. wazero's memory limit is clamped, not
  rejected, so a module declaring a max above the runtime ceiling is
  accepted with the clamped limit — oom.wasm declares 65537 pages to trip
  the compile-time rejection. `RunWASM` is a Runner method, so the native
  and wasm paths share required-settings gating and the step protocol.
