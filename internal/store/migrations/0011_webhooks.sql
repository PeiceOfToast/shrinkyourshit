-- SPDX-License-Identifier: AGPL-3.0-only
-- Webhooks (task 25): the outbound HTTP hooks the server notifies when a
-- job ends or a scan finishes. The secret signs every delivery (HMAC-SHA256
-- over the raw body) and is generated server side on create; the API
-- returns it once and answers secret_set afterwards. events is a comma
-- separated list of the subscribed event names (job.done, job.failed,
-- library.scanned). enabled is 0/1 like every other bool column.
-- webhook_deliveries records the last outcome per hook (one row per
-- delivered event, pruned to the newest 50 per hook); a non-2xx answer
-- stores the response status and the head of its body, a transport failure
-- stores status 0.

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
