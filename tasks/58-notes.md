# Task 58 notes: raw_args escape hatch

Branch: `task/58-raw-args-escape-hatch`

## Plan

1. `internal/flow/rawargs.go` (new): `ParseRawArgs` (denylist + hand written shell-quoting
   tokeniser), `RawArgsRefusals` (the `allow_raw_args` gate), `RawArg` type.
2. `internal/flow/graph.go`: `StepRawArgs` constant, params validation, `StepNeedsMedia`.
3. `internal/flow/plan.go`: `ApplyStep` collects `Effective.RawArgs` per stage, `Describe`
   line marked UNSAFE with the raw string verbatim, `ParseParams`, `StepTypes()` entry,
   `StepTypeInfo.Unsafe` marker.
4. `internal/store/store.go`: `SettingAllowRawArgs`.
5. `internal/server/settings.go`: `AllowRawArgs` on `Values` (default true) and `Update`.
6. `internal/api/auth.go`: `allow_raw_args` on the settings body.
7. Gate: `LibraryManager` (server) refuses a saved graph holding raw_args when the setting is
   off (400 invalid_flow). `POST /flows/validate` and `POST /flows/preview` refuse the same way.
8. Tests in `rawargs_test.go`, `settings_test.go`, `internal/api/flow_test.go`.
9. Docs: ARCHITECTURE.md (step type list, settings keys), API.md (task 58 section).

## Denylist and the reason for each entry

Checked on the raw string before tokenising, in this order (first failure wins):

| Check | Why |
|-------|-----|
| empty or longer than 2000 bytes | a bounded, non empty argument string |
| newline, carriage return, NUL | one line per argument string; NUL cannot appear in an argv anyway |
| `;` `\|` `&` backtick `$(` | shell metacharacters. Nothing runs a shell, but the rule is kept as specified so a reader can audit the string at a glance. Consequence: ffmpeg filter graphs that use `;` are refused. |
| unbalanced `'` or `"`, trailing `\` | a tokeniser error, not a silent guess |
| token `-i` | sets the input path. The engine owns inputs. |
| token `-y` or `-n` | control overwriting of the output. The engine owns the output path. |
| `-f` followed by a device format (`lavfi`, `v4l2`, `alsa`, `x11grab`, ... or a `/dev/` path) | reads from a device instead of a file |
| bare word (no leading `-`) that does not follow an option expecting a value | a bare output file name, or a stray positional. Options listed as boolean (`-an`, `-vn`, `-sn`, `-shortest`, ...) do not take a value, so a word after them is refused. |

Every rejection returns one `FieldError` at `nodes[i].params.args` (or `params.stage`,
`params.note`, `params` for unknown fields).

## Known limits (recorded, not built)

- Only the listed shell metacharacters are refused. `>`, `<`, `$VAR` and quotes around a
  metacharacter are not examined beyond the checks above.
- Attached forms (`-fv4l2`, `-i<path>`) are not matched. ffmpeg accepts the option only as a
  separate token in normal use; this is a known gap of the token check.
- An unknown option followed by a path (`-some_unknown_flag /dev/video0`) is not caught.
  The denylist is an allowlist of intent, not a sandbox (task brief: no sandbox).
- Options that read other files (`-filter_complex movie=...`, `-protocol_whitelist`) are not
  refused. This is what the owner accepted with `allow_raw_args` on by default.

## Choices made where the task was silent

- `stage` default in the palette is `output`. Required.
- More than one `raw_args` step per graph is allowed; each appends to its stage in chain
  order. Task 61 may restrict the advanced palette to one (spec 2.2) without an engine change.
- `note` is capped at 200 characters; `args` at 2000 bytes. Both are my numbers.
- `allow_raw_args` is an app setting in the settings table (like the QSV opt-ins of task 35),
  not a process `config` flag. The spec (section 5.3, section 7) describes it as an app setting,
  so `internal/config` is untouched.
- The setting is enforced at save time and on the validate and preview endpoints. Stored graphs
  are not re-checked at dispatch (see the wiring section below for why).
- `StepTypeInfo.Unsafe` is `json:"unsafe,omitempty"`, so the other palette entries keep their
  current JSON.
- No TypeScript change: no UI in this task.

## Dependency

None added. The tokeniser is hand written; nothing in `go.mod` covers POSIX word splitting
with the rules above (`github.com/google/shlex` is not in STACK.md and was not needed).

## Wiring: raw_args reach ffmpeg (follow-up on this branch)

`ApplyStep` collects `Effective.RawArgs`; the worker now applies them to the encode.

- `internal/encoders/encoders.go`: `Spec.InputArgs` and `Spec.OutputArgs` ([]string).
- `internal/encoders/build.go` `buildCommand`: `InputArgs` goes right before `-i` (after the
  backend's pre-input flags such as `-vaapi_device`); `OutputArgs` goes right before the output
  path (last argument). Every backend builds through `buildCommand`, so cpu, nvenc, qsv and vaapi
  get them with no per encoder change. The video copy path (remux) gets them too. videotoolbox
  off darwin returns no arguments at all, so it never reaches an encode (unchanged).
- `internal/worker/runner.go`: the flow walk keeps `eff.RawArgs` in a local `rawArgs`;
  `splitRawArgsByStage` splits it by stage into the spec, in chain order, and `logRawArgs`
  writes the log lines before the encode.
- Log line format, one per raw_args step, in chain order:

  ```
  raw_args step <step id> (<input|output> stage): "<token 1>" "<token 2>" ...
  ```

  Each token is Go quoted (`strconv.Quote`), so a token with a space stays one unambiguous
  item, e.g. `raw_args step out_arg (output stage): "-metadata" "comment=shrink me"`.
  It is an info level sys line in the job log. Task 63 should assert on this text.

Tests:
- `internal/encoders/rawargs_build_test.go`: in the real argv for cpu, nvenc, qsv and vaapi, an
  input token is before `-i` and an output token is before the output path, which stays last;
  the copy (remux) path too; an empty spec adds nothing.
- `internal/worker/rawargs_test.go`: a compiled flow's raw_args reach the encoder spec
  (`InputArgs`, `OutputArgs`) and the ffmpeg argv they run; one log line per step with the exact
  tokens; no raw_args means no line and no tokens.

Limits (recorded, not built):
- The stored-graph dispatch refusal is NOT built. The worker never reads the settings table: it
  only gets a `JobSpec` from the server. Reading `allow_raw_args` would need a new `JobSpec`
  field (a schema or API change) or a settings call from the worker, so it is skipped. Today the
  switch is enforced at save, validate and preview only, so nothing checks a stored graph at
  dispatch: a graph saved while the switch was on still runs after the switch is turned off.
  Owner call: add a `JobSpec` field, or a server side check in the dispatch path.
- Subtitle extract commands (`ExtractCommands`) and the health check decode do not get raw
  tokens. Only the main encode does.
- Stage placement is fixed by the engine: input tokens go before `-i`, output tokens go after the
  maps, codec and container options, right before the output path. A token that needs to sit
  before a specific map or codec option cannot be placed there by the user.
- A token that is not a valid option for the chosen encoder makes ffmpeg fail the encode; the
  job then reports `encode failed` and the fallback chain tries the next encoder with the same
  tokens. The tokens are not validated against the encoder.
- `gofmt -l` already lists several files on `main` (for example `internal/worker/runner.go`
  struct field alignment); it is not part of `make lint` and was not touched here.

## Test evidence

- Wiring follow-up: `make test` and `make lint` exit 0 (Go packages `encoders`, `worker`
  and `flow` ok; web 62 files, 444 tests; lint 0 issues).

- `go test ./...` passes (all packages). `make lint` passes (go vet, golangci-lint 0 issues,
  SPDX check, eslint, prettier). `npm run test` passes (62 files, 444 tests).
- `internal/flow/rawargs_test.go`: tokeniser words, quotes, escapes, unbalanced quote and
  trailing backslash; every denylist entry through `Validate` at `nodes[1].params.args`;
  stage, note, length and unknown-field paths; plan stage and verbatim `Describe`; media gate;
  palette `unsafe` marker.
- `internal/api/flow_test.go`: with `allow_raw_args` off, save, validate and preview answer 400
  `invalid_flow` at `nodes[2].type`, the stored graph is unchanged, and the switch back on
  saves again. Forbidden words answer 400 at `nodes[2].params.args`.
- `internal/server/settings_test.go`: default on, round trip, `RefusedRawArgs`.

## Summary

1. Built: `raw_args` step (stage, args, note), hand written shell-quote tokeniser, denylist
   with a field error per rule, `Effective.RawArgs`, verbatim `UNSAFE` line in `Describe`,
   `StepTypeInfo.Unsafe`, and the `allow_raw_args` app setting (default on) enforced on save,
   validate and preview with 400 `invalid_flow`.
2. Wired (follow-up): `Spec.InputArgs`/`OutputArgs` in `buildCommand`, the worker splits
   the raw_args by stage into the spec, one log line per step. The arguments reach ffmpeg.
   No UI, no TypeScript change, no `internal/config` change.
3. Skipped: a dispatch-time check for stored graphs while the switch is off (the worker has no
   access to the setting; see the wiring section).
4. Task 63 can assert on the `raw_args step ...` log line and on the argv.
5. Owner calls: `;` is refused even inside quotes (spec wording), more than one `raw_args`
   step is allowed per graph, and the 2000-byte and 200-character limits are my numbers.

