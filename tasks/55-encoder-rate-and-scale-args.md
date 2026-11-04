# Task 55: Encoder arguments for scale, rate control and preset

Part of the v1.10 encode settings tiers wave. The specification is `docs/design/encode-settings-tiers-2026-11-04.md`;
this file carries only the scope and the acceptance checklist for this one task. Read the spec
sections named below plus `AGENTS.md`, `DECISIONS.md`, `STACK.md` and `ARCHITECTURE.md`.

`DECISIONS.md` is read only. The owner decisions this wave implements are recorded in the spec,
section 2.

## Read first

Spec sections 4 items 1 to 3, 5.1, 5.2. `internal/encoders/` (`cpu.go`, `qsv.go`, `quality.go`,
`build_test.go`), `internal/worker/runner.go` (`BuildArgs`, `trackedEncoder`).

## Scope

`internal/encoders/spec.go` (or wherever `Spec` lives), `internal/encoders/cpu.go`,
`internal/encoders/qsv.go`, `internal/encoders/quality.go`, `internal/encoders/build_test.go`,
`internal/encoders/parity_test.go`, `internal/worker/runner.go`, `internal/worker/runner_test.go`.

## Do

1. Extend `encoders.Spec` with the scale target, the rate target and the preset, and populate it
   from the resolved `flow.Effective` in the runner.
2. Build the scale argument as `-vf scale=-2:<height>` (even width preserved) only when scaling
   applies: source taller than the target, or `allow_upscale` is set. No filter when the source
   already fits, so a compliant file is not silently re-encoded by a no-op filter chain.
3. Map rate control per encoder: software hevc to libx265 CRF, software h264 to libx264 CRF, QSV
   to its global quality argument scaled from the same 1 to 5 value, and to `-b:v`/`-maxrate`/
   `-bufsize` in bitrate mode. Quality 1 to 5 stays the mapping for mode quality, unchanged.
4. Pass the preset through where the encoder supports it (`-preset` for the software encoders,
   and the QSV equivalent), and skip it for encoders without the concept rather than erroring.
5. Tests assert the **exact argument vector** for: scale applied, scale skipped, crf mode,
   bitrate mode with a cap, preset, and one QSV case. Argument strings, not just "no error".

## Out of scope

No per step hardware selection (task 57). No new step types.

## Acceptance

- [ ] One test per encoder asserts the exact scale, rate and preset arguments.
- [ ] A source that already fits the scale target produces the same argument vector as before
      this task, proven by a golden test, so no library re-encodes after the upgrade.
- [ ] `make test` green, and `go test ./internal/encoders/... ./internal/worker/...` green.

## Notes

Write the plan and the exact argument vectors in `tasks/55-notes.md`.
