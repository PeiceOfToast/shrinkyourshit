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
  are not re-checked at dispatch here (see gap below).
- `StepTypeInfo.Unsafe` is `json:"unsafe,omitempty"`, so the other palette entries keep their
  current JSON.
- No TypeScript change: no UI in this task.

## Dependency

None added. The tokeniser is hand written; nothing in `go.mod` covers POSIX word splitting
with the rules above (`github.com/google/shlex` is not in STACK.md and was not needed).

## Gap for the next tasks (important)

`ApplyStep` now puts the tokens in `Effective.RawArgs`, but nothing feeds them into the
ffmpeg argument vector. `internal/encoders` builds args from `encoders.Spec`, which has no
raw-args field, and the worker (`internal/worker/runner.go`) never reads `eff.RawArgs`. So
today a saved `raw_args` step is accepted, shown in the preview, and silently not applied to
the encode. Task 63 requires a raw_args argument in the ffmpeg log, so the wiring is needed
before that. It was out of this task's scope ("Modify" list), so it is not built here.
Before the step is reachable by users, the worker or the encoder spec must consume
`Effective.RawArgs`, and the dispatch path should refuse stored graphs with raw_args while the
setting is off (today only save-time is enforced).

## Test evidence

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
2. Skipped: any change to the encoder spec or worker, so the arguments are NOT applied to
   ffmpeg yet (see the gap above). No UI, no TypeScript change, no `internal/config` change.
3. Skipped: a dispatch-time check for stored graphs while the switch is off.
4. Next task (59) should not assume raw arguments reach the encode. Task 63's "raw_args
   argument in the log" needs the wiring first; it is the most important open item here.
5. Owner calls: `;` is refused even inside quotes (spec wording), more than one `raw_args`
   step is allowed per graph, and the 2000-byte and 200-character limits are my numbers.

