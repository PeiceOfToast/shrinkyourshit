# Task 35 notes — Intel QSV tuning

## Plan (files, in order)

1. `internal/encoders/encoders.go` — `EncoderInfo` gains an optional
   `qsv_features` record (`QSVFeatures`: look_ahead, extbrc, async_depth,
   low_power), filled only by the qsv backend.
2. `internal/encoders/qsv.go` — per-codec feature probe during detection
   (one-second `testsrc` self-test with the candidate flag set, exit code
   decides), cached behind the backend mutex; `SetQSVPrefs`/`QSVRequested`
   for the opt-ins; warn-once per requested-but-unsupported feature; flag
   assembly in `BuildArgs` (qsv variant only).
3. `internal/encoders/quality.go` — ICQ mapping documented on the table.
4. `internal/store/store.go` — `qsv_extra_quality`, `qsv_low_power` keys.
5. `internal/server/settings.go` + `internal/api/auth.go` — the two bools
   (default false) through `Values`/`Update`/`settingsBody`.
6. `internal/server/node.go` — heartbeat response carries the two prefs
   (read from settings). `internal/worker/client.go` parses them;
   `internal/worker/worker.go` `applyHeartbeat` forwards to the encoder.
7. Tests: `detect_test.go` (fake ffmpeg with flag rejection), `build_test.go`
   (flag table + async depth range ends + vaapi immunity + ICQ/fallback
   ends), `parity_test.go` (byte-identical regression), `settings_test.go`,
   `node_test.go`, `worker_test.go` (glue via `encoders.QSVRequested`).
8. Web: `api.ts` (capabilities + settings types), `NodesPage.tsx`
   (plain-language feature strip), `EncodeSettingsPage.tsx` (two switches
   with help text), tests for both.
9. Docs: `docs/intel-qsv.md` (ICQ vs bitrate, look-ahead cost, low-power
   wrong choice, Deep Link non-goals in the task's exact words), linked from
   `internal/encoders/README.md` and `README.md`. `API.md` (heartbeat +
   settings gains), `ARCHITECTURE.md` (capabilities shape + settings keys),
   `CHANGELOG.md`, `docs/openapi.yaml` (Settings schema fields).
10. `internal/encoders/integration_test.go` — real QSV look-ahead encode,
    guarded like task 30's VAAPI test; skips naming what is missing.

## Interpretations (owner can override)

- **Byte-identical forces the opt-in gate for async depth.** The task says
  `-async_depth` "stays internal" (no setting) but also that with the new
  settings off the argument vector is byte-identical to today. So async
  depth is emitted only once the user turns on either QSV setting, with the
  depth picked from the quality step (1-2 → 4, 3 → 3, 4-5 → 2; higher
  quality, shallower pipeline). Documented in the builder comment and the
  doc.
- **The ICQ mapping keeps today's numbers.** The QSV column of the quality
  table (30/28/26/24/22) is already passed as `-global_quality`, which
  ffmpeg maps to Intel's ICQ rate control when no bitrate cap is set. New
  numbers would change every QSV encode without any user ask, which the
  task forbids. The task's ICQ work is implemented as: the table documents
  the ICQ mapping (range 1..51, what each end means), hevc_qsv/av1_qsv take
  it, and h264_qsv keeps the same existing mapping as its fallback path
  (h264 ICQ support varies by Intel generation; switching it would change
  encodes nobody asked to change). Table tests pin both paths at steps 1
  and 5.
- **Feature probing is per codec.** ffmpeg's `look_ahead` option exists only
  on `h264_qsv` (verified in ffmpeg 6.1/7.0/7.1 and master source);
  `hevc_qsv`/`av1_qsv` have `extbrc`, `async_depth` and `low_power`. The
  probe therefore runs per codec whose qsv variant was chosen; BuildArgs
  consults the current codec's result, and the capabilities report the
  union. The node page and docs say look-ahead applies to the H.264 format.
- **Probes mirror the runtime argument shape** (low-power probed together
  with `-global_quality`, look-ahead probed with `-look_ahead 1 -extbrc 1
  -global_quality`, since ffmpeg errors on look-ahead without a bitrate or
  quality): "probed true" means the runtime flag set is accepted.
- The `*_vaapi` variant of the qsv backend never carries these flags (the
  vaapi encoders do not have them) and the standalone vaapi backend is
  untouched (out of scope).

## Summary (written at the end)

- Built: capability probing for look-ahead/extbrc/async depth/low power per
  codec with a fake-ffmpeg test; `qsv_extra_quality` and `qsv_low_power`
  settings flowing server → heartbeat → worker → encoder; opt-in-only flag
  assembly with warn-once for unsupported requests; async depth derived
  from quality; node-page feature strip and encode-settings switches with
  tests; docs/intel-qsv.md with the Deep Link non-goals.
- Skipped: Hyper Encode / Deep Link (not implemented, not planned, stated
  in the doc), AV1 tuning beyond ICQ mapping, NVIDIA/VAAPI changes,
  per-library encoder settings.
- Next task should know: the capabilities JSON gained an optional
  `qsv_features` object on the qsv encoder entry; the heartbeat response
  gained `qsv_extra_quality`/`qsv_low_power`; nothing else moved.
