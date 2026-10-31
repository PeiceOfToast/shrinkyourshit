# Task 25: Webhooks and an OpenAPI document

## Goal
The app can tell something else what happened, and a stranger can read the whole API from a machine-readable file that cannot drift from the real routes.

## Read first
`internal/server/queue.go` (`Queue.Complete`), `internal/server/events.go`, `internal/api/api.go` (every route), `internal/server/integrations.go` and `web/src/IntegrationsPage.tsx` (from task 23 — this task adds a tab to that page), `internal/store/migrations/0010_integrations.sql`, `docs/API.md`, `web/src/api.ts`.

Depends on task 23: the page, the client helpers and the `Notifier` hook already exist.

## Do

### 1. Migration `internal/store/migrations/0011_webhooks.sql`

```sql
CREATE TABLE webhooks (
    id          TEXT PRIMARY KEY,
    name        TEXT NOT NULL,
    url         TEXT NOT NULL,
    secret      TEXT NOT NULL,
    events      TEXT NOT NULL,          -- comma separated: job.done, job.failed, library.scanned
    enabled     INTEGER NOT NULL DEFAULT 1,
    created_at  TEXT NOT NULL,
    updated_at  TEXT NOT NULL
);

CREATE TABLE webhook_deliveries (
    id          TEXT PRIMARY KEY,
    webhook_id  TEXT NOT NULL,
    event       TEXT NOT NULL,
    status      INTEGER NOT NULL,
    error       TEXT NOT NULL DEFAULT '',
    created_at  TEXT NOT NULL
);
```
Store methods and interface entries alongside the integration ones, plus `AddWebhookDelivery`, `ListWebhookDeliveries(webhookID, limit)` and a prune that keeps the newest 50 rows per hook.

### 2. `internal/server/webhooks.go`

- `type WebhookEvent struct { Event string; Job *store.Job; Library *store.Library; At time.Time }` — JSON shape is the contract, so every field gets an explicit tag.
- Events: `job.done`, `job.failed`, `library.scanned`. The first two fire from `Queue.Complete`, the third from the scanner's finish path. Reuse whatever `Notifier` shape task 23 introduced rather than adding a second parallel hook in `Queue`.
- Request:
  - `POST` to the configured URL, `Content-Type: application/json`, `User-Agent: shrinkyourshit/<version>`.
  - Header `X-SYS-Event: <event>` and `X-SYS-Signature: sha256=<hex hmac of the raw body with the webhook secret>`.
  - 5 second timeout.
  - A 2xx records status and no error. Anything else records the status and the first 200 characters of the body as the error. Transport failures record status 0.
- Retries: 3 attempts total, 1s then 4s backoff, then give up and record. Never block the job, never fail the job.
- **Discord and Slack shaping**: keep the raw event JSON as the body always, but when the URL host ends in `discord.com` or `discordapp.com` wrap it as `{"content": "<one line summary>"}` and when it ends in `slack.com` wrap it as `{"text": "<one line summary>"}`. The summary is built by one function, `webhookSummary(ev WebhookEvent) string`, so there is exactly one place to change the wording. Examples: `job.done: The Matrix (1999) 4.1 GB to 1.2 GB, 71% smaller`, `job.failed: Heat failed: encoder exited 1`, `library.scanned: Films: 412 files, 9 new`.
- Secret handling: the secret is generated server side when a webhook is created (`crypto/rand`, 32 bytes hex) and is returned **once** on create. After that the API reports `secret_set: true` and never the value again. The UI shows it once with a copy button and a "you will not see this again" note.

### 3. API, in `internal/api/webhooks.go`

```
GET    /api/v1/webhooks
POST   /api/v1/webhooks
PUT    /api/v1/webhooks/{id}
DELETE /api/v1/webhooks/{id}
POST   /api/v1/webhooks/{id}/test
GET    /api/v1/webhooks/{id}/deliveries
```
Admin only, existing error shape, 204 on delete. `POST /{id}/test` sends a synthetic `job.done` with a fixed sample payload and returns the same shape as the integration test endpoint.

### 4. UI
`web/src/IntegrationsPage.tsx` gains tabs: **Servers** (task 23 content) and **Webhooks**. The webhooks tab is a `DataList` of hooks with enabled switch, Test, Deliveries, Delete, and an add form (name, URL, event checkboxes). A deliveries dialog shows the last 50 attempts with time, event, status and error. New clients in `web/src/api.ts` following the existing style. Tests in `web/src/IntegrationsPage.test.tsx`: the secret is shown once on create and never after a reload, a delivery row renders its status, and the Test button reports the result.

### 5. OpenAPI
- `docs/openapi.yaml`, OpenAPI 3.1, hand written, covering every route in `internal/api/api.go`: path, method, admin requirement, request body schema for the write routes, response schema for the read routes, and the two security modes (cookie session, worker key).
- Served at `GET /api/v1/openapi.json` and `GET /openapi.yaml` from the embedded copy (`go:embed`), no auth on the YAML, because it documents the API rather than exposing it.
- **The anti-drift test is the point of this section.** `internal/api/openapi_test.go` walks `api.go` with `go/parser`, collects every `"<METHOD> /path"` string passed to `HandleFunc`, normalises `{id}` style parameters, and fails with the missing list when any of them is absent from `docs/openapi.yaml`. It also fails when the document lists a route that no longer exists. Check the document in with every route present, so the test passes on the first run.

## Out of scope
Webhook signing rotation UI, per-library webhooks, replay of a stored delivery, generated clients, publishing the document anywhere outside the app, any change to `DECISIONS.md`.

## Acceptance
- [ ] `job.done`, `job.failed` and `library.scanned` each deliver to an `httptest` server with the right `X-SYS-Event`, a signature that verifies with the secret, and a body matching the documented shape. Proven in `internal/server/webhooks_test.go`.
- [ ] A 500 response is retried 3 times with backoff and recorded in `webhook_deliveries`.
- [ ] Delivery never blocks or fails a job, and a dead hook logs a warning only.
- [ ] Discord and Slack URLs receive the wrapped body; everything else receives the raw event.
- [ ] The secret is returned exactly once, and never appears in any later response. Proven by an API test asserting the string is absent from a subsequent GET.
- [ ] `GET /api/v1/openapi.json` returns the document, and `openapi_test.go` proves the document and the route table agree in both directions.
- [ ] The webhooks tab works, shows deliveries, and its tests pass. `make test` and `make lint` are green.
