-- SPDX-License-Identifier: AGPL-3.0-only
-- External integrations (task 23): the media servers and download clients
-- the refresher notifies after a job ends. api_key holds the remote's own
-- API key in plain text (the refresher must send it verbatim); it never
-- leaves the server through the API, which answers api_key_set instead.
-- enabled/refresh_on_done are 0/1 like every other bool column.

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
