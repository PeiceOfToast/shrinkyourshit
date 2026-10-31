# Task 23: External integrations (library refresh, Sonarr/Radarr)

## Goal
After a file is shrunk, the media server notices without anyone clicking Scan. Optional integrations: Jellyfin, Emby, Plex (library refresh) and Sonarr, Radarr (rescan command), configured in the UI, with API keys that never come back out of the API.

## Read first
`internal/server/queue.go` (`Queue.Complete`, roughly line 246), `internal/server/events.go`, `internal/server/settings.go`, `internal/api/api.go` (route table), `internal/api/auth.go` (`settingsBody`, `getSettings`), `internal/store/migrations/0009_library_last_scan.sql`, `web/src/api.ts`, `web/src/PageContainer.tsx`, `web/src/uiControls.ts`, `web/src/RulesPage.tsx` (an existing settings-style page to copy the shape from).

## Why
This is the difference between "it encodes files" and "the library at home updates itself". The owner runs Sonarr/Radarr/Jellyfin, so this is the highest value item on the roadmap.

## Do

### 1. Migration `internal/store/migrations/0010_integrations.sql`

```sql
CREATE TABLE integrations (
    id              TEXT PRIMARY KEY,
    kind            TEXT NOT NULL,
    name            TEXT NOT NULL,
    base_url        TEXT NOT NULL,
    api_key         TEXT NOT NULL,
    enabled         INTEGER NOT NULL DEFAULT 1,
    refresh_on_done INTEGER NOT NULL DEFAULT 1,
    created_at      TEXT NOT NULL,
    updated_at      TEXT NOT NULL
);
```

`kind` is one of `jellyfin`, `emby`, `plex`, `sonarr`, `radarr`. Follow the existing migration conventions (see 0009) and add the store methods in `internal/store` next to the neighbouring entities: `ListIntegrations`, `GetIntegration`, `CreateIntegration`, `UpdateIntegration`, `DeleteIntegration`. Every store method gets an interface entry, because `store.Store` is an interface.

### 2. `internal/server/integrations.go`

```go
type Integration struct {
    ID, Kind, Name, BaseURL string
    APIKey                  string
    Enabled, RefreshOnDone  bool
}
```

- `func ValidateIntegration(in Integration) error` — rejects an unknown `kind`, a `base_url` that does not parse or is not `http`/`https`, an empty `base_url` or `api_key`, and trailing slashes are trimmed from `base_url`.
- `type Refresher struct` with `func NewRefresher(st store.Store, log *slog.Logger) *Refresher` and `func (r *Refresher) Notify(ctx context.Context)` — reads the enabled integrations with `refresh_on_done` and calls them.
- The per-kind calls, each through one small helper so the exact request is pinned in one place:

| kind | request |
|------|---------|
| jellyfin | `POST {base}/Library/Refresh`, header `X-Emby-Token: {key}` |
| emby | `POST {base}/Library/Refresh`, header `X-Emby-Token: {key}` |
| plex | `GET {base}/library/sections/all/refresh?X-Plex-Token={key}` |
| sonarr | `POST {base}/api/v3/command`, header `X-Api-Key: {key}`, body `{"name":"RescanSeries"}` |
| radarr | `POST {base}/api/v3/command`, header `X-Api-Key: {key}`, body `{"name":"RescanMovie"}` |

Confirm each against the vendor's own API docs while implementing, and if one differs, change it in that single helper and say so in the PR body. Success is any 2xx.

- **Coalescing is required.** A finished job must not mean a request storm: the refresher keeps one in-flight call per integration id and a 30 second floor between calls for the same id. A `Notify` that arrives inside the window is dropped, not queued. Test it.
- **Never fail a job.** `Notify` runs in its own goroutine with a 5 second per-request timeout, logs failures at warn with the integration name and status code only, and never returns an error to the caller. The API key is never logged.

### 3. Hook the queue
`internal/server/queue.go`: at the end of `Queue.Complete`, after the job row is written and the event published, call the refresher only when the job ended `done` **and** the output was actually used (bytes saved > 0). Take the refresher as a field on `Queue` set by a setter, or accept a nil-able `Notifier` interface — do not import a cycle, and keep the existing constructor working for tests that do not care.

### 4. API, in `internal/api/integrations.go`

```
GET    /api/v1/integrations
POST   /api/v1/integrations
PUT    /api/v1/integrations/{id}
DELETE /api/v1/integrations/{id}
POST   /api/v1/integrations/{id}/test
```

All behind `a.requireAdmin`. Rules:
- A response **never** contains `api_key`. It carries `api_key_set: true|false` instead. A `PUT` with an empty `api_key` keeps the stored key.
- `POST /{id}/test` performs that one integration's call once and answers `{"ok": bool, "status": int, "error": "..."}`, 200 even when the remote fails.
- Delete answers 204. Unknown id answers 404 in the existing error shape.

### 5. Web
- `web/src/api.ts`: `Integration`, `IntegrationBody`, `IntegrationTestResult` types plus `getIntegrations`, `createIntegration`, `updateIntegration`, `deleteIntegration`, `testIntegration`. Follow the existing function style exactly.
- `web/src/IntegrationsPage.tsx`: list of integrations as `DataList` rows (kind, name, base URL, enabled switch, a Test button, Edit, Delete), and an add/edit form with kind select, name, base URL, API key field that shows `(unchanged)` placeholder when `api_key_set`, an enabled `Switch`, and a `refresh_on_done` `Switch`. Use the shared control classes from `uiControls.ts`, so 44px tap targets come for free. Wire it into the route table in `web/src/App.tsx` and the navigation in `web/src/AppShell.tsx` beside the existing settings entries.
- Tests: `web/src/IntegrationsPage.test.tsx` — renders rows from a mocked `getIntegrations`, the API key is never rendered, saving a form calls `updateIntegration` with the typed values, and the Test button surfaces the returned error text.

## Out of scope
Per-library integration filters, Sonarr/Radarr queue deletion or file management, OAuth, notifications (task 25), retries beyond the one call, any change to `DECISIONS.md`.

## Acceptance
- [x] `internal/store/migrations/0010_integrations.sql` exists and `make test-go` migrates a fresh database to it. (`TestIntegrationsMigrationAndCRUD` opens a fresh DB, applies migrations, round trips a row; `make test` green.)
- [x] The five endpoints exist, are admin only, and no response body ever contains an api key. Proven by an API test that creates an integration with a distinctive key and asserts the string is absent from every GET body. (`TestIntegrationsRequireAdmin`, `TestIntegrationsCRUDViaAPI`)
- [x] `Refresher.Notify` hits the correct method, path, header and body for each of the five kinds, proven by `httptest` servers in `internal/server/integrations_test.go`. (`TestCallIntegrationRequestShapes`)
- [x] Two completions 1 second apart produce exactly one remote request per integration. Proven by a counter in the same test file. (`TestQueueTwoCompletionsOneRemoteRequest`, plus `TestRefresherCoalescing` for the floor itself)
- [x] A remote 500 or a dead host logs a warning and the job still ends `done`, proven by a queue test. (`TestQueueRemoteFailureWarnsAndJobStillDone`)
- [x] `IntegrationsPage` is reachable from the navigation, renders the list, hides the key, and passes its test file. `make lint` is clean. (`App.test.tsx` six nav entries, `IntegrationsPage.test.tsx` 8 tests)
