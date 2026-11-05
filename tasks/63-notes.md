# Task 63 notes: verify the v1.10 wave on real media and document it

Evidence file for the whole encode settings tiers wave (54 to 63). Every claim below comes from a
command run on 2026-11-05 on this box. Raw outputs are in `/tmp/sys63/logs/` (scratch, not committed).

## Plan (in the order it was carried out)

1. Build with `make build`; start the built binary against a scratch data dir on port 8763 with
   LAN discovery off; generate three 4 s 1920x1080 H.264 clips with ffmpeg `lavfi` sources.
2. Create three libraries with `result_policy: keep_both`, so the originals are never replaced.
3. Basic: create from the easy input. Moderate: save a graph with a condition and a scale. Advanced:
   save a graph with a `raw_args` step. Set each tier through `PUT /libraries/{id}/tier`.
4. Start the built-in node, scan, poll `/api/v1/jobs` to terminal, download each job log.
5. ffprobe the outputs and originals; run the downgrade round trip against the API and the SQLite row.
6. `make test`, `make lint`, `make test-integration`.
7. Docs: ARCHITECTURE (job log lines the evidence relies on), API (tier reply keys), ROADMAP (wave
   marked delivered), STATUS.

## Environment

- ffmpeg and ffprobe: `7.0.2-static` (johnvansickle build) from `/home/mb/.local/opt/ffmpeg-static/`.
  Not on the default PATH, so the runs used a PATH that adds that directory. Nothing was installed.
- Go `go1.27.3` from `/home/mb/.local/go`, Node `v20.20.2`.
- Encoders in that ffmpeg: `libx264`, `libx265`. No `hevc_qsv`, `hevc_vaapi` or `hevc_nvenc`.
  `/dev/dri/card0` exists on this box, but this ffmpeg has no Intel or VAAPI encoder, so no GPU encode
  ran. The built-in node reported `encoders=[cpu]`.
- Server: `./bin/shrinkyourshit serve --role both`, `SHRINK_DATA_DIR=/tmp/sys63/data`,
  `SHRINK_SCRATCH_DIR=/tmp/sys63/scratch`, `SHRINK_PORT=8763`, mDNS and broadcast discovery off.
  No container, no prod service, no file outside the repo and `/tmp/sys63` was touched.
- Media: `/tmp/sys63/media/{basic,moderate,advanced}/clip.mkv`, generated with
  `ffmpeg -f lavfi -i testsrc=size=1920x1080:rate=25 -f lavfi -i sine=frequency=440:sample_rate=48000 -t 4 -c:v libx264 -pix_fmt yuv420p -c:a aac -ac 2 -shortest`.

## The three flows

| Library | id | Tier (after run) | Flow saved as | Graph |
|---------|----|------------------|---------------|-------|
| basic | `81e02c57-2483-48cc-9e5c-fe85c43ab7b2` | basic | easy (`{quality 3, remove_subtitles false, audio 1}`) | generated from easy mode |
| moderate | `a2c7ffe3-9841-4b5a-8378-b346f60e1f0c` | moderate | steps | `probe` → `if_resolution_above {height 1000}` → `scale {height 720, only_if_larger true, allow_upscale false}` → `transcode_video {hevc, quality 3, rate_mode crf, crf 24, preset medium, hardware software}` → `remux_container {mkv}` |
| advanced | `971ee9ce-e737-44b7-a6bd-7464682bcc86` | advanced | advanced | `probe` → `transcode_video {hevc, quality 3, rate_mode bitrate, bitrate_kbps 1500, max_kbps 2500, preset medium}` → `remux_container {mkv}` → `raw_args {stage output, args "-threads 2 -metadata title=sys63verbatim"}` |

Graph bodies: `/tmp/sys63/logs/graph-moderate.json`, `/tmp/sys63/logs/graph-advanced.json`.
`POST /api/v1/flows/validate` returned `{"valid":true}` for both before saving.

Job ids: basic `46cdaec9-ddf7-4aca-8d57-2656a4b1cf76`, moderate `a1c75821-56bd-4226-9cc4-fde38d8fd738`,
advanced `96ee020e-c268-465d-90ca-c36bf32f0db8`. All three ended `done` (`/api/v1/jobs`).
Full logs: `/tmp/sys63/logs/job-<id>.log`, from `GET /api/v1/jobs/{id}/log/download`.

## Job log evidence (the relevant lines, timestamp prefix removed)

Each line below is copied from the saved job log. `sys` is the worker's own line, `ffmpeg` is
ffmpeg's stderr, which the worker captures into the job log.

### Basic: job 46cdaec9 (`job-46cdaec9-ddf7-4aca-8d57-2656a4b1cf76.log`, 60 lines)

```
info sys target: mkv container, hevc video, keep_all audio
info ffmpeg Stream #0:0 -> #0:0 (h264 (native) -> hevc (libx265))
info ffmpeg x265 [info]: Rate Control / qCompress            : CRF-24.0 / 0.60
info ffmpeg Output #0, matroska, to '/tmp/sys63/scratch/job-46cdaec9-ddf7-4aca-8d57-2656a4b1cf76/output.mkv':
info ffmpeg   Stream #0:0: Video: hevc, yuv420p(progressive), 1920x1080 [SAR 1:1 DAR 16:9], q=2-31, 25 fps, 1k tbn
info sys encoder chosen: cpu
info sys bytes before: 153.6 KiB; after: 118.7 KiB
```

### Moderate: job a1c75821 (`job-a1c75821-56bd-4226-9cc4-fde38d8fd738.log`, 60 lines)

```
info sys target: mkv container, hevc video, keep_all audio
info ffmpeg Stream #0:0 -> #0:0 (h264 (native) -> hevc (libx265))
info ffmpeg x265 [info]: Rate Control / qCompress            : CRF-24.0 / 0.60
info ffmpeg Output #0, matroska, to '/tmp/sys63/scratch/job-a1c75821-56bd-4226-9cc4-fde38d8fd738/output.mkv':
info ffmpeg   Stream #0:0: Video: hevc, yuv420p(progressive), 1280x720 [SAR 1:1 DAR 16:9], q=2-31, 25 fps, 1k tbn
info ffmpeg [out#0/matroska @ 0x1c95b480] video:34KiB audio:63KiB subtitle:0KiB other streams:0KiB global headers:2KiB muxing overhead: 5.215306%
info sys encoder chosen: cpu (step transcode_video set hardware to software)
info sys result policy: applied, output at /tmp/sys63/media/moderate/clip.shrunk.mkv
info sys bytes before: 153.6 KiB; after: 102.4 KiB
```

### Advanced: job 96ee020e (`job-96ee020e-c268-465d-90ca-c36bf32f0db8.log`, 63 lines)

```
info sys raw_args step raw_args (output stage): "-threads" "2" "-metadata" "title=sys63verbatim"
info ffmpeg Stream #0:0 -> #0:0 (h264 (native) -> hevc (libx265))
info ffmpeg x265 [info]: Rate Control / qCompress            : ABR-1500 kbps / 0.60
info ffmpeg x265 [info]: VBV/HRD buffer / max-rate / init    : 5000 / 2500 / 0.750
info ffmpeg Output #0, matroska, to '/tmp/sys63/scratch/job-96ee020e-c268-465d-90ca-c36bf32f0db8/output.mkv':
info ffmpeg     title           : sys63verbatim
info ffmpeg   Stream #0:0: Video: hevc, yuv420p(progressive), 1920x1080 [SAR 1:1 DAR 16:9], q=2-31, 1500 kb/s, 25 fps, 1k tbn
info ffmpeg         cpb: bitrate max/min/avg: 2500000/0/1500000 buffer size: 5000000 vbv_delay: N/A
info sys encoder chosen: cpu
info sys bytes before: 153.6 KiB; after: 346.9 KiB
```

What each line proves:

- **Basic and Moderate**: the rate control line says `CRF-24.0`, which is the argued CRF.
- **Moderate**: the output stream line says `1280x720`, which is the scale target applied (see the ffprobe table).
- **Moderate per-step hardware**: `encoder chosen: cpu (step transcode_video set hardware to software)`. The step's choice is named in the job log.
- **Advanced raw_args**: the `sys` line lists the tokens verbatim, Go-quoted. ffmpeg's own output metadata then shows `title : sys63verbatim`, so the `-metadata` token reached ffmpeg as an output option.
- **Advanced rate control**: `ABR-1500 kbps` with `max-rate 2500`, and the `cpb` line `2500000/0/1500000`, match the bitrate step (`bitrate_kbps 1500`, `max_kbps 2500`).

## Output heights (ffprobe, not by eye)

From `/tmp/sys63/logs/ffprobe-summary.txt`. Each line is `codec,width,height`, then the format name, duration and format bit rate. It was printed by `ffprobe -v error -select_streams v:0 -show_entries stream=codec_name,width,height -of csv=p=0 <file>` and `ffprobe -v error -show_entries format=format_name,bit_rate,duration -of csv=p=0 <file>`.

```
basic/clip.mkv          h264,1920,1080 "matroska,webm",4.021000,312847
basic/clip.shrunk.mkv   hevc,1920,1080 "matroska,webm",4.052000,239897
moderate/clip.mkv       h264,1920,1080 "matroska,webm",4.021000,312847
moderate/clip.shrunk.mkv hevc,1280,720 "matroska,webm",4.052000,207000
advanced/clip.mkv       h264,1920,1080 "matroska,webm",4.021000,312847
advanced/clip.shrunk.mkv hevc,1920,1080 "matroska,webm",4.052000,701233
```

| Flow | Expected output height | ffprobe | Result |
|------|------------------------|---------|--------|
| basic (no scale in Basic) | 1920 | 1920 | pass |
| moderate (scale 720, source 1080, condition `if_resolution_above 1000` true) | 720 | 720 | pass |
| advanced (no scale in graph) | 1920 | 1920 | pass |

The moderate condition: the source is 1080 and the threshold is 1000, so the condition passed and the scale ran. No
negative case (a threshold above 1080) was run, so the condition is proved only in the passing direction.

Originals: `ls -l` after the runs shows `clip.mkv` at 157245 bytes in all three folders, mtime 02:47:39 to 02:47:41,
before the jobs started at 02:48. `keep_both` wrote `clip.shrunk.mkv` beside each one.

## Rate control

- Moderate CRF: the log shows `CRF-24.0`.
- Advanced bitrate: the log shows `ABR-1500 kbps` and max-rate 2500.
- Achieved bitrate is not claimed to match the target. The advanced ffprobe format bit rate is 701233 (container, video and audio together). ffmpeg's own summary line for the video stream is `encoded 100 frames in 2.29s (43.68 fps), 569.99 kb/s`. This 4 s test pattern is very compressible, so ABR undershoots 1500. That is a property of the test clip, not a result this task checked for accuracy.

## Downgrade round trip (advanced library, `971ee9ce`)

Every command below was run; the output is quoted.

1. Before. `GET /api/v1/libraries/971ee9ce.../flow` reported `tier advanced source advanced unrepresentable []`.

2. Downgrade. `PUT /api/v1/libraries/971ee9ce.../tier` with `{"tier":"basic"}` returned:

   ```
   {"tier":"basic","unrepresentable":[{"path":"nodes[1].type","message":"step \"transcode_video\" (transcode_video) is not representable in the basic tier; the moderate tier can edit it"},{"path":"nodes[3].type","message":"step \"raw_args\" (raw_args) is not representable in the basic tier; the advanced tier can edit it"},{"path":"graph","message":"the basic tier skips files already in the target codec: the flow needs an if_codec_not step on the same codec as the encode"}]}
   ```

3. After. `GET .../flow` and `GET .../flow?tier=basic` both returned `tier basic source advanced` with the same three `unrepresentable` entries.

4. Graph byte compare, through the API. `rt-1-before-graph.json` and `rt-3-after-graph.json` hold the `graph` member
   of the two reads, written by `json.dumps(..., indent=2)`:

   ```
   $ diff rt-1-before-graph.json rt-3-after-graph.json; echo exit=$?
   exit=0
   $ cmp rt-1-before-graph.json rt-3-after-graph.json && echo byte-identical
   byte-identical
   ```

5. Stored row, read straight from SQLite with a read-only connection (`file:/tmp/sys63/data/shrinkyourshit.db?mode=ro`).
   The round trip was repeated with the row captured on each side: first set `tier advanced`, then snapshot,
   then downgrade, then snapshot.

   ```
   before: rule_tier=advanced flow_source=advanced
   after:  rule_tier=basic    flow_source=advanced
   $ diff -u db-1-flow.json db-2-flow.json; echo exit=$?
   exit=0
   $ cmp db-1-flow.json db-2-flow.json; echo exit=$?
   exit=0
   $ sha256sum db-1-flow.json db-2-flow.json
   8da0b57cb3707143b0a90920e16dcc996b44cea072a30b3dc6d8e32ca9a707cc  db-1-flow.json
   8da0b57cb3707143b0a90920e16dcc996b44cea072a30b3dc6d8e32ca9a707cc  db-2-flow.json
   ```

   The stored `flow_json` is 583 bytes both times. Its content is the advanced graph with the `raw_args` step.

6. The API graph equals the stored graph: `stored == api graph (semantic): True`.

Result: the downgrade changed `rule_tier` and nothing else. The graph is byte-identical in the database and
through the API.

## Gates

- `make test`: exit 0. Go: 13 packages `ok`, none failed (`internal/api`, `internal/server` and `internal/worker` included). Web: `Test Files 65 passed (65)`, `Tests 474 passed (474)`. Log: `/tmp/sys63/logs/make-test.log`.
- `make lint`: exit 0. `golangci`/`go vet`, the SPDX check, `eslint --max-warnings 0`, `prettier --check`. Log: `/tmp/sys63/logs/make-lint.log`.
- `make test-integration` (ffmpeg on PATH, build tag `integration`):
  - Run 1 exited `2`. Every package passed except `internal/worker`, which failed one test. The exact output
    (`/tmp/sys63/logs/make-test-integration-run1.log`, lines 24 to 26):

    ```
    --- FAIL: TestNodeDrainFinishesCurrentJob (66.90s)
        lifecycle_integration_test.go:100: state while waiting for long job running: long job done, attempt 0, error
        lifecycle_integration_test.go:100: timed out waiting for long job running
    FAIL	github.com/PeiceOfToast/shrinkyourshit/internal/worker	311.163s
    make: *** [Makefile:66: test-integration] Error 1
    ```

  - Isolation: `go test -tags=integration -count=3 -run 'TestNodeDrainFinishesCurrentJob$' ./internal/worker/`
    exited 0 (`ok ... 21.865s`). `go test -tags=integration -count=1 -timeout 30m ./internal/worker/` exited 0
    (`ok ... 183.276s`).
  - Reading: the test waits for a 15 s libx264 fixture (`makeLongFixture`) to be seen `running`. In the full run,
    every integration package ran at once on 6 cores, and the encode had already finished when the first poll
    landed. The test's own comment (`lifecycle_integration_test.go`, the `MaxSlots = 1` note) records this
    timing class as known flaky. This is not a proven product defect: it passed 4 of 4 runs outside the parallel
    load. It is recorded here, not fixed (no code change in this task).
  - Run 2 (`make test-integration`, full, `/tmp/sys63/logs/make-test-integration-run2.log`) exited `0`: every
    package `ok`, `internal/worker` `ok 180.156s`. Some packages report `(cached)`: Go reused their passing
    results from run 1 because their inputs had not changed. Only `internal/worker` ran fresh in run 2.
  - Net: the integration target is green on a second full run. The run 1 failure is an intermittent timing failure
    of one test under parallel load. Owner call: whether to make that test's "long job" window load-proof.

## Gaps and open questions

These are where the evidence falls short of what the task file and the spec asked for. None was fixed here; the
task forbids code changes, and the owner should decide.

1. **The job log does not contain the ffmpeg command line.** Do item 1 asks to capture "the actual ffmpeg
   command line" from the job log. The code never writes the argv: `execFFmpeg` in `internal/worker/runner.go`
   runs `exec.Command` without logging `args`. Only the `raw_args step ...` token line is written by
   `logRawArgs`. The evidence above uses ffmpeg's own echo lines instead. `tasks/58-notes.md` said task 63 would
   assert "on the argv", so this may be an unmet expectation rather than a regression. Owner call: add an
   `info sys ffmpeg argv: ...` line in the worker.

2. **`-threads 2` is not shown to take effect.** The token is in the `raw_args` line, but the advanced job still
   reports `x265 [info]: Thread pool created using 6 threads`. The `-metadata` token is proven in ffmpeg's output
   (`title : sys63verbatim`). Only the token's presence in the job is proved for `-threads 2`.

3. **`PUT /api/v1/libraries/{id}/flow` is not tier gated (observed).** The advanced graph with `raw_args` was saved
   while the library was on the basic tier. The response was `"source":"advanced","tier":"basic"` and the save
   succeeded. This is the open owner question recorded in `tasks/59-notes.md` and `tasks/62-notes.md`. It is
   recorded, not changed.

4. **No per-step hardware proof beyond `software`.** This ffmpeg build has no `hevc_qsv`, `hevc_vaapi` or
   `hevc_nvenc`. The per-step proof is the `software` choice. A `qsv`, `vaapi` or `nvenc` choice was not run.

5. **Condition only tested in the passing direction** (see above).

6. **`TestNodeDrainFinishesCurrentJob` failed once under the parallel integration run** (run 1, output in the
   Gates section). It passed 4 of 4 runs outside that load. Not reproduced as a product defect; recorded for the
   owner.

## Docs updated

- `ARCHITECTURE.md` (flow schema section): records the two job-log lines the evidence relies on, the
  per-step `raw_args` token line and the `encoder chosen` hardware note, and states that the argv is not logged
  (gap 1).
- `API.md` (task 59): the `GET .../flow` reply keys as observed (`library_id`, `source`, `tier`, `graph`,
  `unrepresentable`), and the observed `PUT .../flow` tier gap (gap 3).
- `ROADMAP.md`: the v1.10 header is marked delivered, with the open items listed.
- `tasks/STATUS.md`: row 63 stays `in_progress` with the branch name (AGENTS rule 13: the supervisor opens the PR).

## Summary

1. Built the binary and ran three real encodes on the built-in CPU node: Basic (CRF 24, 1920x1080), Moderate (condition, scale to 720, CRF 24, per-step `software`, output 1280x720), Advanced (ABR 1500 kb/s with a verbatim `raw_args` title token).
2. ffprobe confirmed output heights 1920, 720 and 1920. The originals were untouched under `keep_both`.
3. The downgrade round trip (advanced to basic) left the stored graph byte-identical in the database (`cmp` exit 0, sha256 8da0b57c...) and through the API (`diff` exit 0).
4. Gaps: the job log does not carry the ffmpeg argv, `-threads 2` has no visible effect, and `PUT .../flow` is not tier gated. See the Gaps section.
5. The next task is the owner's call: the argv log line and the tier gate on flow save are both open. `make test` and `make lint` are green.
