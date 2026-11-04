# Encode settings tiers: Basic, Moderate, Advanced

Status: specification for wave v1.10. Owner decisions recorded here, 2026-11-04.
Task files 54 to 63 implement it. `DECISIONS.md` is read only for every agent; the
five decisions in section 2 are recorded here and are the owner's answer.

## 1. What this wave builds

Three editors over one canonical flow graph, exposed per library:

| Tier | The person using it wants | Editor |
|------|---------------------------|--------|
| Basic | "shrink my files, do not make me think" | five stop quality slider, audio slider, remove subtitles checkbox, folder picker |
| Moderate | "show me the whole tree of decisions" | ordered tree of condition rows and actions, curated node set, no canvas |
| Advanced | "let me wire anything together" | drag and drop graph, curated node set plus a validated raw ffmpeg arguments node |

`DECISIONS.md` item 6 already requires all three on one engine, and `references` in the
repo (`ARCHITECTURE.md`) already declares the graph canonical. This wave makes the middle
tier real and widens the engine vocabulary the top tier needs.

The middle and top tiers are an engine job before they are a UI job. Section 4 lists what
the engine cannot express today; no amount of front end work substitutes for it.

## 2. Owner decisions (2026-11-04)

1. **The tier toggle is per library, with an app wide default that new libraries inherit.**
2. **Advanced exposes a curated node set covering the engine's real surface, plus exactly one
   validated raw arguments escape hatch node.** Not every ffmpeg flag as its own node.
3. **Moderate is a decision tree**: condition nodes (resolution, codec, bitrate, audio
   channels) with branches, on the same graph, curated node set, no canvas.
4. **Switching a library down to Basic keeps the flow intact.** Basic renders a read only
   summary plus a "not fully representable in Basic" banner. Only a tier that can express a
   step may edit it. Nothing is ever silently collapsed or dropped.
5. **One graph is the truth.** Basic writes quality plus audio, Moderate adds resolution,
   codec, container and rate control, Advanced edits the same graph. Hardware encoder becomes
   a per step choice whose default comes from the existing global policy.

Consequence of 4 for the build agent: downgrade is a display and permission question, never a
data migration. There must be no code path that rewrites or truncates a stored graph because a
library changed tier.

## 3. Verified current state (2026-11-04, main at e824b7d)

- `internal/flow/target.go`: `Target{video{codec, quality 1..5, reencode_if_same_codec},
  audio{mode, preferred_languages, compact_codec}, subtitles{mode, preferred_languages,
  extract_removed}, container}`. **No resolution, no rate control, no preset, no hardware field.**
- `internal/flow/easymode.go`: `EasyModeInput{quality, remove_subtitles, audio}` stored in the
  library `easy` column, `CompileEasyMode` produces a `Target`.
- `internal/flow/graph.go`: `Graph{schema, nodes[], edges[]}`, step types `probe`,
  `if_codec_not`, `if_bitrate_above`, `transcode_video`, `remux_container`, `convert_audio`,
  `subtitles`, `rename`, `skip_if_smaller`, `health_check`, `move`, `plugin`.
- `internal/flow/plan.go`: `Compile`, `ApplyStep`, `ResolvePlan`, `Plan.Target()`, and
  **`StepTypes()` which returns the palette the UI renders** (`StepTypeInfo{Type, Label,
  Description, Condition, Params []ParamField{Name, Label, Kind, Min, Max, Options, Required,
  Default, Help}}`). This is the single source of truth for node vocabulary.
- `internal/encoders/`: `cpu.go`, `qsv.go`, `quality.go` build the actual ffmpeg arguments from
  `encoders.Spec`; quality 1..5 maps to encoder specific rate settings there.
- `internal/server/libraryflow.go`: switching `flow_source` between `easy`, `steps` and
  `advanced`, with a representability question already implied but not implemented.
- `internal/server/dto.go`: `Library.FlowSource`; `web/src/api.ts` mirrors `Target`,
  `EasyInput`, `FlowGraph`.
- Migrations run to `0015_library_last_scan.sql`.
- Web: `EncodeSettingsPage.tsx`, `EasyLibraryForm.tsx`, `easyModel.ts` (QUALITY_STOPS,
  AUDIO_STOPS), `FlowCanvasPage.tsx`, `FlowCanvas.tsx`, `FlowInspector.tsx`,
  `flowCanvasModel.ts`.

## 4. What the engine cannot express today

Everything in this list is required by Moderate or Advanced and is missing:

1. Resolution. There is no scale step, so "4K in, 1080p out" cannot be written.
2. Rate control other than the quality scalar: no CRF, no target bitrate, no max bitrate.
3. Speed preset (`veryfast` .. `veryslow`), which is the single biggest CPU cost dial.
4. Per step hardware selection. `internal/encoders` picks QSV versus software from the global
   policy only, so one flow cannot mix a QSV encode with a software fallback.
5. Conditions on resolution, audio channel count, or HDR. The tree in Moderate has nothing to
   branch on beyond codec and bitrate.
6. Output naming beyond the existing `rename` template, and no explicit output path step.
7. Any escape hatch for an argument the engine has not modelled.
8. Tier storage and a representability check.

## 5. Canonical model changes

### 5.1 Target extension (backward compatible)

`Target.Video` gains optional fields. Old JSON keeps working: a missing field means "not set",
which resolves to today's behaviour.

```go
type VideoTarget struct {
    Codec               string       `json:"codec"`
    Quality             int          `json:"quality"`
    ReencodeIfSameCodec bool         `json:"reencode_if_same_codec"`
    Scale               *ScaleTarget `json:"scale,omitempty"`   // new
    Rate                *RateTarget  `json:"rate,omitempty"`    // new
    Preset              string       `json:"preset,omitempty"`  // new: veryfast..veryslow, "" = balanced
}

// ScaleTarget keeps aspect. Height is the long edge rule the owner asked for:
// scale only when the source is taller than Height; never upscale.
type ScaleTarget struct {
    Height      int  `json:"height"`        // 2160, 1440, 1080, 720, 480
    OnlyIfLarger bool `json:"only_if_larger"` // default true
    AllowUpscale bool `json:"allow_upscale"`  // default false
}

// RateTarget is one of three modes; quality keeps the existing scalar meaning.
type RateTarget struct {
    Mode      string  `json:"mode"`       // "quality" | "crf" | "bitrate"
    CRF       float64 `json:"crf"`        // 0..51, codec dependent default when zero
    BitrateKBPS int   `json:"bitrate_kbps"` // used when mode is bitrate
    MaxKBPS     int   `json:"max_kbps"`     // 0 = uncapped
}
```

`ARCHITECTURE.md` must be updated in the same commit, since the `Target` JSON shape is
documented there. That is a documentation change, not a decision change.

### 5.2 New step types

Added to `graph.go`, `StepTypes()`, `ApplyStep`, `Describe` and the TypeScript `FlowGraph`
types together, in one task each:

| Type | Params | Notes |
|------|--------|-------|
| `scale` | `height` (select 2160/1440/1080/720/480), `only_if_larger` (toggle, default true), `allow_upscale` (toggle, default false) | sets `Effective` scale; container and codec untouched |
| `transcode_video` (extend) | `codec`, `quality`, `rate_mode` (quality/crf/bitrate), `crf`, `bitrate_kbps`, `max_kbps`, `preset`, `hardware` (`auto`/`qsv`/`vaapi`/`videotoolbox`/`software`) | existing graphs keep the two field shape; new fields default empty |
| `if_resolution_above` | `height` (number) | condition, mirrors `if_bitrate_above` |
| `if_audio_channels_above` | `channels` (number 1..8) | condition |
| `if_hdr` | `is_hdr` (toggle) | condition; probe already reports HDR |
| `output_name` | `template` (string, same tokens as `rename`) | explicit naming step, kept separate from `rename` only if `rename` cannot express a full path; prefer extending `rename` if it can |
| `raw_args` | `stage` (`input`/`output`), `args` (string), `note` (string) | validated escape hatch, see 5.3 |

`hardware` on a step overrides the global policy for that step only. `auto` means "use the
global policy", which is today's behaviour and the default, so existing graphs are unchanged.

### 5.3 The `raw_args` contract

This is the only place a user can hand the engine something the validator has not modelled, so
the rules are explicit:

- The value is never passed to a shell. It is split with `shlex` equivalent quoting into
  argument tokens and appended to the ffmpeg argument vector for the chosen stage. No
  `sh -c`, ever.
- Rejected tokens and patterns, each with a 400 and a field error: any argument that changes
  the input or output path (`-i`, `-y` on the output), `-f` with a device, shell metacharacters
  (`;`, `|`, `&`, backtick, `$(`), and newlines.
- The step is labelled unsafe in the UI, renders a warning colour, and the library detail page
  lists every flow that contains one.
- `Describe` shows the raw string verbatim so an audit is possible from the UI.
- It is opt in per app setting `allow_raw_args` (default on, because the owner asked for it, but
  it is one switch to kill).

## 6. Tier model and representability

- `library.rule_tier` column: `basic` | `moderate` | `advanced`. Migration `0016_library_tier.sql`,
  default derived from the existing `flow_source` so every current library keeps its editor.
- App setting `default_rule_tier` (default `basic`) which new libraries inherit.
- `flow_source` keeps its current meaning (who owns the graph: `easy`, `steps`, `advanced`). The
  tier is what the UI offers and what may edit; they normally agree, and decision 4 is the case
  where they do not.
- New validator in `internal/flow`: `RepresentableAt(g Graph, tier Tier) []FieldError`, returning
  one error per step the tier cannot express, with the step id. Rules:
  - `basic`: only graphs that `CompileEasyMode` can regenerate, that is the easy step set with
    quality 1..5, audio mode, subtitle remove and container mkv. Any condition, scale, rate
    override, preset, hardware override, rename, move, plugin or raw args is not representable.
  - `moderate`: everything except `raw_args` and `plugin` (the two escape hatches).
  - `advanced`: everything.
- `PUT /api/v1/libraries/{id}/tier` `{"tier": "..."}` returns `{"tier": "...", "unrepresentable":
  [{path, message}]}`. A downgrade with a non empty list still succeeds (decision 4) and the UI
  renders the banner from that response.
- `GET /api/v1/libraries/{id}/flow?tier=basic` returns the graph plus the same
  `unrepresentable` list, so a downgraded library opens read only without a second call.

## 7. UI per tier

Follows `docs/design/ui-redesign-2026-11-04.md` (tokens, primitives, 44 px targets, toasts,
confirm dialogs, mobile first at 390 px).

- **Basic**: today's `EasyLibraryForm`, unchanged in behaviour. Add the plain language "what
  this does" disclosure the UI contract already asks for, listing the steps it generates.
- **Moderate**: new `FlowTreePage` at `/libraries/:id/tree`. Rows are ordered, each row is either
  a condition (with its children indented under it) or an action. Add, remove, reorder by
  keyboard and by drag handle. Fields render from `StepTypes()` params, never hand coded, so a
  new step type appears here for free. Server validation errors attach to the row that owns the
  field.
- **Advanced**: `FlowCanvasPage` keeps `@xyflow/react`, with the palette driven by `StepTypes()`,
  the `raw_args` node visually distinct, and server field errors on the node.
- **Tier switch**: on the library detail page, a three way control. Choosing a lower tier calls
  the tier endpoint and, when `unrepresentable` is non empty, shows a banner naming the steps and
  the tier that can edit them. The lower tier editor then renders read only.
- **Encode settings** (`/encode`): the app wide `default_rule_tier` and the `allow_raw_args`
  switch live here, in the existing rule tier section.

## 8. Sequencing

| Task | Scope |
|------|-------|
| 54 | Target extension (scale, rate, preset) plus `scale` step and `transcode_video` extension in the graph, validator and `Describe`; `ARCHITECTURE.md` |
| 55 | Encoders: resolution, CRF, bitrate and preset into the cpu and qsv argument builders, with tests proving arguments, not just no error |
| 56 | New condition steps: `if_resolution_above`, `if_audio_channels_above`, `if_hdr`, and their `StepTypes()` entries |
| 57 | Per step hardware selection with the global policy as the default; `Effective` carries it; runner honours it |
| 58 | `raw_args` step: tokenising, denylist, field errors, `describe`, `allow_raw_args` setting |
| 59 | Tier storage: migration 0016, `default_rule_tier`, `RepresentableAt`, tier endpoint, `?tier=` on the flow read |
| 60 | Moderate editor: `FlowTreePage`, condition rows with children, reorder, palette from `StepTypes()` |
| 61 | Advanced palette: every step type, `raw_args` node, server errors on nodes |
| 62 | Tier switch and downgrade banner on the library detail page, read only rendering |
| 63 | Verify end to end on real media: one flow per tier, prove output height, CRF or bitrate in the ffmpeg log, per step hardware, and a `raw_args` argument in the log; then docs, ROADMAP and STATUS |

Rules that apply to every task in this wave: branch `task/NN-slug` from `main`, one PR, CI green,
`make test`, `make lint`, `npm run build` and `npx playwright test` green before ticking the
checklist, `DECISIONS.md` untouched, every existing `data-testid` survives, and any test
assertion changed is listed in `tasks/NN-notes.md` with file, line, old, new and why.
