-- SPDX-License-Identifier: AGPL-3.0-only
-- Storages (encode profiles and in-app storage wave, spec 4.3): network shares
-- the server mounts inside the container. name is a slug and also the
-- mountpoint (/storage/<name>). secret_enc holds the SMB password sealed with
-- the storage key (internal/secret); it is never returned by any read path.
-- Forward only.

CREATE TABLE storages (
  id          TEXT PRIMARY KEY,
  name        TEXT NOT NULL UNIQUE,
  protocol    TEXT NOT NULL,
  source      TEXT NOT NULL,
  options     TEXT NOT NULL DEFAULT '',
  username    TEXT NOT NULL DEFAULT '',
  domain      TEXT NOT NULL DEFAULT '',
  secret_enc  TEXT NOT NULL DEFAULT '',
  read_only   INTEGER NOT NULL DEFAULT 0,
  enabled     INTEGER NOT NULL DEFAULT 1,
  created_at  TEXT NOT NULL,
  updated_at  TEXT NOT NULL
);
