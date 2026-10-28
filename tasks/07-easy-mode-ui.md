# Task 07: Easy-mode UI, dashboard, library pages

## Goal
A non-technical user picks a folder, moves a slider, ticks a box, and presses Start. Then they watch it work on the dashboard.

## Read first
ARCHITECTURE.md (easy-mode mapping, Target schema), API.md (task 03 and task 07 endpoints), DECISIONS.md (6, 15, B1, B3), AGENTS.md rule 14. Depends on tasks 05 and 06.

## Do
- `internal/flow/easymode.go`: `CompileEasyMode(input EasyModeInput, settings) flow.Target` exactly per the easy-mode mapping table. `EasyModeInput{Quality int 1..5, RemoveSubtitles bool, Audio int 1..4}`. Also the reverse: `DescribeTarget(t) (EasyModeInput, ok bool)` so an existing library can be reopened in easy mode when its Target was produced by it.
- `internal/api`: `GET /stats`. Library create/update accept either a full `target` or an `easy` object; the server compiles `easy` to a Target and stores both (`easy` as a JSON column on the library for round-tripping).
- `/web`:
  - **New library (easy mode)** page: folder picker modal using `fs/browse` (breadcrumbs, directories only); quality slider with 5 labelled stops ("Smallest files", "Smaller", "Balanced", "Higher quality", "Best quality") and a one-line plain-language hint per stop; "Remove subtitles" checkbox; audio slider with the 4 labels from the mapping table; "Watch this folder for new files" toggle (default on); Start button. No codec names anywhere on this page.
  - **Libraries** list: name, path, counts (satisfied, queued, done, failed), space saved, scan-now button, edit, delete with confirmation that explains files on disk are untouched.
  - **Library detail**: file table with status, reasons, bytes saved, filter by status; retry failed.
  - **Dashboard**: running jobs with progress bars, speed and ETA (live via SSE); queue count; recent history; total space saved; per-node activity placeholder (task 08 fills it).
  - **Advanced** section on the library edit page: a collapsed "Advanced" panel exposing the raw Target fields (codec, reencode_if_same_codec, container, languages). This is the only place codec names appear.
- Empty states: first visit after setup lands on "Add your first folder" with the easy-mode form.

## Out of scope
Step-stack editor, visual flow editor, node management UI, webhooks.

## Acceptance
- [ ] Unit tests: `CompileEasyMode` produces the exact Target for every slider and checkbox combination in the mapping table; `DescribeTarget` round-trips them and returns `ok=false` for a hand-edited Target.
- [ ] Browser test (Playwright or Vitest with MSW): choose a folder, slider at 3, remove subs, audio 2, Start creates a library whose stored Target matches the expected JSON.
- [ ] Dashboard shows a running job's progress updating from SSE events (test with a fake event stream).
- [ ] `GET /stats` totals match the sum of library counts in an integration test with two libraries.
- [ ] The easy-mode page source contains no occurrences of "hevc", "h265", "h264", "av1", "crf", or "ffmpeg" (test greps the built page component).
- [ ] Deleting a library from the UI leaves the fixture files on disk.
