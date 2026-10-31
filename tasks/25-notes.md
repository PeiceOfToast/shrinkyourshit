# Task 25 notes — Webhooks and an OpenAPI document

## Plan

Files to touch, in order:

1. `tasks/STATUS.md` — in_progress. Branch `task/25-webhooks-and-openapi`.
2. `internal/store/migrations/0011_webhooks.sql` — the two tables exactly as the task spells them.
3. `internal/store/webhooks.go` — `Webhook` + `WebhookDelivery` rows, CRUD, `AddWebhookDelivery`,
   `ListWebhookDeliveries(webhookID, limit)` (newest first), `PruneWebhookDeliveries(keep)`;
   `DeleteWebhook` also removes that hook's deliveries. Interface entries in `interface.go`.
4. `internal/store/webhooks_test.go` — CRUD round trip, delivery add/list/prune (row 51 drops the
   oldest), delete removes deliveries, unknown ids → ErrNotFound, fresh-DB migration check naming 0011.
5. `internal/server/webhooks.go` — event names (`job.done`, `job.failed`, `library.scanned`),
   `WebhookEvent`, `ValidateWebhook`, `NewWebhookSecret` (crypto/rand, 32 bytes hex),
   `webhookSummary(ev)` (one wording place), and the `Dispatcher`: POST the configured URL with
   `X-SYS-Event`, `X-SYS-Signature: sha256=<hmac>`, `User-Agent: shrinkyourshit/<version>`,
   5 s timeout, 3 attempts (1 s then 4 s backoff), one delivery row recorded after the final attempt
   (status; first 200 chars of body as error; status 0 on transport failure), never an error out.
   Discord (`discord.com`/`discordapp.com`) gets `{"content": …}`, Slack gets `{"text": …}`, the
   signature covers the bytes actually sent.
6. Hook refactor: `Notifier` grows the event (`Notify(ctx, ev WebhookEvent)`) so the Queue and the
   scanner keep ONE outbound hook (task 23's `SetNotifier` is reused, no parallel hook). `Queue`
   fires `job.done` from `Complete` (every done outcome) and `job.failed` from `Fail`; the scanner
   fires `library.scanned` from `ScanLibrary`'s success path. The refresher filters internally on
   done-with-real-savings so its task 23 behavior is unchanged; the dispatcher filters on the three
   webhook events.
7. `internal/server/webhooks_test.go` — acceptance tests: delivery shape + headers + verifiable
   signature for all three events, 500 retried 3 times with backoff and recorded, dead hook warns
   only and the job stays done, Discord/Slack wrapping, summary wording, scanner event.
8. `internal/api/webhooks.go` — the six endpoints (admin only), DTO with `secret_set`, the create
   response carrying the secret once, `POST /{id}/test` with a fixed sample `job.done` payload
   (single attempt), `GET /{id}/deliveries?limit=`.
9. `internal/api/webhooks_test.go` — CRUD, secret returned exactly once and absent from later GETs,
   admin-only 401, test endpoint against an httptest remote, deliveries list.
10. `docs/openapi.yaml` + `docs/docs.go` (go:embed — the file must sit beside the embedding
    package) + `internal/api/openapi.go` serving `GET /api/v1/openapi.json` (YAML converted to JSON
    with the already-listed `gopkg.in/yaml.v3`) and `GET /openapi.yaml` verbatim, no auth.
11. `internal/api/openapi_test.go` — the anti-drift test: `go/parser` walks `api.go`, collects every
    `"<METHOD> /path"` passed to `HandleFunc`, normalizes `{id}` params, and diffs both directions
    against the paths/methods in `docs/openapi.yaml` (parsed via the embed).
12. `web/src/api.ts` clients, `web/src/IntegrationsPage.tsx` gains Servers/Webhooks tabs (secret
    shown once with a copy button and the "you will not see this again" note, deliveries dialog),
    `web/src/IntegrationsPage.test.tsx` grows the three new assertions.
13. `API.md` (Task 25 section), `ARCHITECTURE.md` (webhook data model, notifier hook, events).
14. `make test`, `make lint`, `make test-integration`; STATUS.md → done with the PR link.

## Deviations to flag

- The task's `WebhookEvent` sketch (`Job *store.Job; Library *store.Library`) cannot produce the
  prescribed summaries: `store.Job` has no file name (only `file_id`) and no scan counts, and raw
  store rows marshal without json tags (the task itself says every field gets an explicit tag) and
  would leak the 64 KiB ffmpeg log tail. The event therefore carries the DTOs the rest of the API
  already publishes (`*JobDTO`, `*FileDTO`, `*LibraryDTO`) plus `*ScanResult` for `library.scanned`,
  all with explicit tags — same four fields the task names (`event`, `job`, `library`, `at`) plus
  `file` and `scan`.
- `webhookSummary`'s `library.scanned` example ("412 files, 9 new") needs the scan counts, so the
  event carries `scan` (see above). `ScanResult` gains json tags (it was never marshaled before).
- The task says both `job.done` and `job.failed` "fire from `Queue.Complete`"; `Complete` only ever
  ends `done`, so `job.failed` fires from `Queue.Fail` (where the encoder error text lives).
- The task's "Read first" list names `docs/API.md`; the API contract lives at the repo root
  (`API.md`), and `docs/` held only review notes. `docs/openapi.yaml` is created as asked.

## Out of scope (untouched)

Signing-rotation UI, per-library webhooks, delivery replay, generated clients, publishing the doc
outside the app, any change to DECISIONS.md.
