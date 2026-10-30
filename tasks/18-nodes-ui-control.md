# Task 18: Node switch, live job line and per node totals

## Goal
Each node card gets a real on/off switch, one condensed live line for the file it is encoding, and its own totals. The plan came from a UI review session; the decisions are the owner's: switch off is graceful (finish the current file), measured data on the card is file name, percent, speed, ETA and original against live output size, and totals are files done plus bytes saved per node.

Current state: `NodesPage.tsx` already renders Start and Stop buttons, a drain checkbox, a concurrency stepper and a lifecycle pill, and lifecycle endpoints exist. What is missing is the switch, the file identity, the live output size and the per node totals.

## Read first
`docs/ui-review-2026-10-29.md` section 6, `ARCHITECTURE.md` (worker lifecycle), `API.md` (task 11 and task 18). Depends on task 11 (done).

## Do
- `JobDTO` (`internal/server/dto.go`): add `file_name` (basename) and `file_path`, resolved from the job's `file_id`. Keep `file_id`.
- Live output size:
  - Migration: `jobs.bytes_so_far` INTEGER NOT NULL DEFAULT 0.
  - Worker: on every progress tick (every 2s per running job) `os.Stat` the scratch output and include `bytes_so_far` in the progress report. Cheap, one stat per tick.
  - Server: store it, expose `bytes_so_far` on `JobDTO`.
- Per node totals: fold an aggregate into `GET /api/v1/nodes` as `stats: {files_done, bytes_saved}`. One query for all nodes (`GROUP BY node_id`, sums of `MAX(bytes_before - bytes_after, 0)` over `status='done'`), never one query per node.
- UI `web/src/NodesPage.tsx`:
  - New `web/src/Switch.tsx`, accessible (`role="switch"`, `aria-checked`, 44px target). On means `POST /nodes/{id}/start`. Off means `POST /nodes/{id}/stop` with `drain: true`, so the current file finishes. A secondary "Stop now" keeps the existing confirm dialog and sends `drain: false`.
  - New `web/src/NodeJobLine.tsx`: file name truncated with the full path in `title`, percent with a bar (`aria-valuenow`), speed as `1.8x`, ETA as `1m 35s`, and size as `31.0 MB → 12.0 MB`. One line at `sm` and up, two lines under `sm`. Reuse `formatBytes` and `formatEta` from `easyModel.ts`.
  - Card footer: `N files, X GB saved` from the new stats field, plus the existing concurrency stepper.
  - Page header: Pause all and Resume all, disabled when they would be a no-op, calling `POST /api/v1/nodes/pause-all` and `resume-all`. The global app header stays clean; the switch is per node and pause all is page level.
- Freshness: drive card updates from the existing SSE stream with a 5 second poll fallback. Last write wins by `heartbeat_at` when two events land in the same tick.

## Out of scope
The flow editor, library page changes (task 19), scheduling policy, queue reordering.

## Acceptance
- [ ] `JobDTO` carries `file_name` and `file_path` for a job with a known file. (`TestJobDTOCarriesFileName`)
- [ ] Progress reported with `bytes_so_far` is stored and returned on the job. (`TestProgressStoresBytesSoFar`, `TestWorkerReportsBytesSoFar`)
- [ ] `GET /api/v1/nodes` includes per node `files_done` and `bytes_saved`, correct with a second node that has no finished jobs, and uses a single query. (`TestNodeStatsAggregate`)
- [ ] The card exposes a `role="switch"` whose `aria-checked` follows `desired_state`, calling start when off and stop with `drain: true` when on. (`NodesPage.test.tsx`)
- [ ] "Stop now" sends `drain: false` behind a confirmation. (`NodesPage.test.tsx`)
- [ ] The live line renders name, percent with `aria-valuenow`, speed, ETA, and `before → so far`. (`NodeJobLine.test.tsx`)
- [ ] Pause all and Resume all call the right endpoints and are disabled when redundant. (`NodesPage.test.tsx`)
- [ ] Playwright passes at 390, 768 and 1280 on `/nodes` with no horizontal overflow.
- [ ] `API.md` documents `file_name`, `file_path`, `bytes_so_far`, node `stats`, and that the switch maps to start and to stop with drain.
