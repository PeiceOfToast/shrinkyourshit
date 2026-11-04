# Task 56 notes: condition steps for resolution, audio channels and HDR

## Plan (written before implementation)

Files, in order:

1. `internal/flow/graph.go`: the three constants, typed params (`ifResolutionAboveParams`,
   `ifAudioChannelsAboveParams`, `ifHDRParams`), bounds constants, `validateStepParams` cases,
   `isGuard`, `StepNeedsMedia`.
2. `internal/flow/plan.go`: `Condition` gains `Height`, `Channels`, `IsHDR`; `pass` evaluates the
   three from `probe.MediaInfo`; `Compile` and `ParseParams` read the params; `describeStep` and
   `describeConditions` render each as a clause; `StepTypes()` gains three palette entries.
3. Tests in `internal/flow/plan_test.go` and `internal/flow/graph_test.go` only (task scope).
4. Docs: `ARCHITECTURE.md` and `API.md` list the step types (AGENTS rule 5 and the API convention);
   they are outside the task's scope list, so they are called out below.
5. `web/src/api.ts`: checked, no change needed. `FlowStep.type` is a plain string and `StepTypeInfo`
   is generic, so the new types need no type edit.

## Probe check (acceptance item 2)

`probe.MediaInfo` already reports every field the guards need, so the probe is unchanged:

- height: `VideoStream.Height` (`internal/probe/ffprobe.go` reads `s.Height`)
- audio channels: `AudioStream.Channels` (`ffprobe.go` reads `s.Channels`, with a fallback)
- HDR: `VideoStream.HDR`, true for `smpte2084` and `arib-std-b67` (`ffprobe.go` `hdrTransfers`)

Nothing was guessed and no field was added.

## Owner calls and deviations (read these)

- **Audio channels: any track counts.** `if_audio_channels_above` passes when any audio stream has
  more channels than the threshold, not only the first one. A file with a 5.1 track passes even when
  its first track is stereo. The task file did not say which track; this is my reading.
- **Audio guard does not need video.** The video guards (`if_resolution_above`, `if_hdr`, and the
  existing two) still fail on a file with no video stream. `if_audio_channels_above` reads only audio,
  so an audio-only file can pass it. `Condition.pass` was restructured so the video check applies
  per kind.
- **`is_hdr` is required.** It is a `*bool` in the params, so a missing toggle is a validation error
  (`params.is_hdr`) rather than a silent "not HDR". The palette default is `true`.
- **Strict comparisons.** "taller than" and "more channels than" are exclusive, like
  `if_bitrate_above`. A 1080p video does not pass `if_resolution_above` with height 1080.
- **Docs outside the scope list.** `ARCHITECTURE.md` (step list, per-type params, the non-media guard
  list) and `API.md` (the `condition: true` list) were updated. AGENTS rule 5 requires it.
- **Not changed: `web/src/flowModel.ts`.** It is UI and out of scope. Its `gatesFor` and
  `conditionText` only recognise `if_codec_not` and `if_bitrate_above`. Effect: in the step stack, a
  new guard row shows its palette description (correct, but generic), and the action row below it
  does not get the "only when ..." prefix. **Follow-up for the UI task (60 or the step stack owner).**
- **Not changed: `internal/flow/targetflow.go`.** It was already not gofmt-clean at HEAD
  (`git show HEAD:internal/flow/targetflow.go | gofmt -l` lists it). Not touched here.

## Findings (exact)

Toolchain, PATH exported as the task requires: `go1.27.3 linux/amd64`, `make` at
`/home/mb/.local/bin/make`, `golangci-lint` at `/home/mb/.local/bin/golangci-lint`, `ffmpeg` present.

- Baseline before any edit: `make test-go` exit 0, every package `ok`.
- `go build ./...` after the edits: exit 0.
- `gofmt -l internal/flow/graph.go internal/flow/plan.go internal/flow/*_test.go`: clean after `gofmt -w`.
- `go test ./internal/flow/ -run 'Guard|Resolution|Audio|HDR|NewGuards'`: all new tests pass.
- `make test`: exit 0. Go: every package `ok`. Web: `Test Files 62 passed (62)`, `Tests 444 passed (444)`.
- `make lint`: exit 0. `go vet ./...` clean, `golangci-lint run` reports `0 issues.`, `check-spdx.sh`
  clean (no new files), `eslint --max-warnings 0` clean, `prettier --check` clean.
- `make test-integration` (ffmpeg present): exit 0, 13 packages `ok`.

New tests and what they prove:

| Acceptance | Test (file) | Evidence |
|---|---|---|
| passing and failing fixture per condition | `TestResolutionGuardPassAndFail`, `TestAudioChannelsGuardPassAndFail`, `TestHDRGuardPassAndFail` (plan_test.go) | pass and fail both asserted through `Compile` plus `ResolvePlan` |
| child action skipped when false, rest of chain runs | `TestGuardChildSkippedKeepsRestOfChain` (plan_test.go) | `tv` not run, `remux` runs, video stays copy |
| child-with-guard shape equals `if_bitrate_above` | `TestGuardedChildShapeMatchesBitrateGuard` (plan_test.go) | identical `id/type/when` shape for all four guard types |
| palette exposes all three, with help | `TestStepTypesIncludeGuardPalette` (graph_test.go) | `Condition`, `Description`, params, bounds, defaults, `Help` asserted |
| bounds and validation | `TestValidateGuardParamBounds`, `TestValidateGuardMustHaveFollowingStep` (graph_test.go) | 13 cases incl. boundaries 480/4320, 1/8, missing `is_hdr` |
| plain-language describe | `TestGuardClausesDescribeInPlainLanguage` (plan_test.go) | exact clause text for each guard |
| media gating | `TestNewGuardsNeedMediaAndSkipNonMedia` (plan_test.go) | `StepNeedsMedia`, `isGuard`, skip on a non-media kind |

## Summary

1. Built: `if_resolution_above` (height 480..4320), `if_audio_channels_above` (channels 1..8) and
   `if_hdr` (is_hdr, required). Each is a typed, validated, palette-listed condition that evaluates
   from `probe.MediaInfo` and renders a plain-language clause. Probe unchanged; every field existed.
2. Skipped: UI (`flowModel.ts` still knows only the two old guards, see above), and any change to how
   conditions are stored (the `Condition` JSON gains three omitempty fields only).
3. Tests: 10 new test functions across `plan_test.go` and `graph_test.go`. Existing tests unchanged.
   Verified by `make test` (444 web, all Go ok), `make lint` (0 issues), `make test-integration` (ok).
4. Decided: any-track semantics for channels, the audio guard works without video, `is_hdr` required,
   strict "more than". The docs in `ARCHITECTURE.md` and `API.md` were updated for the new types.
5. Next task (57) should know: the guard `Condition` now has seven kinds (`codec_not`, `bitrate_above`,
   `resolution_above`, `audio_channels_above`, `hdr`), so any new switch over `Condition.Kind` must
   handle all of them; `flowModel.ts` `gatesFor` and `conditionText` need the three new types before the
   step stack shows them correctly.
