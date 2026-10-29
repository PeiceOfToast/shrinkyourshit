# Task 07 notes: easy-mode UI, dashboard, library pages

## Plan

Order of work (small commits):

1. `internal/flow/easymode.go`: `EasyModeInput{Quality int, RemoveSubtitles bool, Audio int}`,
   `EasySettings{Codec, PreferredLanguages}` (settings live in `server`, so flow
   defines its own struct), `CompileEasyMode` per the ARCHITECTURE.md easy-mode
   mapping table, and the reverse `DescribeTarget(t) (EasyModeInput, ok)`.
   Signature note: the task file sketches `CompileEasyMode(...) flow.Target`;
   I return `(Target, error)` so bad slider values (quality outside 1..5,
   audio outside 1..4) surface as 400s at the API instead of silently
   producing a broken Target. Behavior for valid inputs follows the table
   exactly. Unit tests: all 40 slider/checkbox combos, round-trips, and
   hand-edited Targets rejected.
2. `internal/store`: migration `0003_library_easy.sql` (`ALTER TABLE libraries
   ADD COLUMN easy TEXT NOT NULL DEFAULT ''`), `Library.EasyJSON`, updated
   column lists/scan/insert/update. Store tests.
3. `internal/server`: `LibraryParams.EasyJSON`; `LibraryDTO.Easy`
   (json.RawMessage, null when none); new `stats.go` service (counts per
   status + bytes_saved, per library and total) with `GET /stats` in mind.
   `JobFilter.Order` ("desc") so the dashboard can ask for recent jobs.
4. `internal/api`: create/update accept `easy` or `target` (never both ->
   400); the server compiles `easy` with the current settings
   (default_video_codec + preferred_languages) and stores both; a `target`-only
   save clears the stored `easy` column. `GET /stats` endpoint. Empty `name`
   on create defaults to the folder's base name (the easy screen has no name
   field). API.md + ARCHITECTURE.md updated in the same commit (rule 5).
5. `web/src`: api client types/endpoints; SSE hook (one EventSource in the
   app shell, invalidates react-query caches); `easyModel.ts` (labels/hints);
   folder picker modal on `fs/browse`; `EasyLibraryForm`; Dashboard, Libraries
   list, Library detail (file table + filter + retry failed), library new/edit
   pages with the collapsed Advanced panel; routes.
6. Tests per acceptance item (see below). Web tests use vitest stubs of
   `fetch` and `EventSource` — no new dependencies (MSW not in STACK.md).
7. `tasks/STATUS.md`, PR.

## Decisions taken inside scope

- `CompileEasyMode` returns `(Target, error)` (see 1).
- Stored `easy` column only fills when a save carried `easy`; `LibraryDTO`
  falls back to `DescribeTarget(target)` when the column is empty, so
  hand-created (API-written) easy-shaped Targets still open in easy mode.
- `DescribeTarget` is strict about everything easy mode fixes: codec must be
  hevc/av1/h264 (not copy), reencode_if_same_codec false, subtitles
  keep_all/remove (not keep_preferred) with extract_removed false, container
  mkv, compact codec aac. Preferred languages are settings-driven and not
  part of the round-trip.
- Stats aggregates in the service over `ListLibraries`+`ListFiles` (home-lab
  scale) instead of adding SQL aggregation to the store interface.
- Web tests stub global `fetch`/`EventSource` via vitest instead of adding
  MSW (not a listed dependency; no new packages added).
- Dashboard shows the easy form inline when there are no libraries ("Add your
  first folder"), so the first visit after setup lands on it.
- An empty library `name` on create defaults to the folder's base name: the
  easy screen has no name field (the pre-existing CRUD test's
  empty-name-400 now tests an invalid target instead).
- The /stats acceptance item runs as a normal (non-`integration`-tagged) test
  at two levels: the server service and the HTTP endpoint, two libraries,
  files seeded straight into the store — it needs no ffmpeg, so running it
  under the integration build tag would only skip it in plain CI.
- The library edit page decides easy vs advanced by the `<details>` element's
  open state (ref), so what Save sends is always what the user sees; the
  Advanced panel defaults open for libraries with no easy representation.

## Blocked / questions

None so far.

## Summary (end of run)

- Built: `flow.CompileEasyMode`/`DescribeTarget` with the full mapping-table
  test; `easy` JSON column on libraries (migration 0003) wired through
  LibraryParams/LibraryDTO (stored easy wins, describe fallback, target-only
  save clears it); `GET /api/v1/stats` (totals + per-library breakdown) and
  `GET /api/v1/jobs?order=desc`; API.md + ARCHITECTURE.md updated. Web:
  api.ts client for all task-03/07 endpoints, one SSE subscription
  (`LiveEvents`) invalidating react-query caches, easy screen (folder picker
  modal, quality slider with plain-language stops, remove-subtitles checkbox,
  4-stop audio slider, watch toggle), dashboard (live progress via SSE, queue
  count, space saved, recent history, node placeholder, first-folder empty
  state), libraries list (counts, saved bytes, scan-now, delete confirm),
  library detail (file table, status filter, retry failed) and the edit page
  with the collapsed Advanced panel (the only place codec names appear).
- Skipped: step-stack editor, flow editor, nodes UI, webhooks (out of scope
  per task file). No new dependencies anywhere.
- Next task should know: `server.Server.Stats()` exists; LibraryDTO carries
  `easy` (null | {quality, remove_subtitles, audio}); library create/update
  accept `easy` XOR `target`; web tests stub global fetch via
  `web/src/test/fetchMock.ts` (no MSW) and EventSource via `stubEventSource`;
  HomePage was replaced by DashboardPage as the index route.
- Watch out: `go.mod` must stay at go 1.23.0; web tests that render pages
  using `useParams` need a real `<Route>` wrapper; the fetch stub must not
  put a body on 204 responses (Response constructor throws).
- Verified: `make test`, `make lint`, `make test-integration` (ffmpeg
  present) all green; live smoke: setup → easy create (name defaulted,
  exact compiled Target stored, easy kept) → target-only PUT clears easy →
  DELETE leaves the folder on disk → embedded UI served. Web production
  build and SPDX check also pass.
- CI: every CI step verified locally (vet, golangci-lint, spdx, go test unit
  + integration, eslint, prettier, vitest, web build); the repo token cannot
  read PR check rollups (`gh pr checks` → 403), same as task 05, so the
  owner should glance at the PR's checks once — the same commands pass here.
