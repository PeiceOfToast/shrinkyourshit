-- SPDX-License-Identifier: AGPL-3.0-only
-- Admin login sessions (task 06). Only the SHA-256 of the session token is
-- stored; expiry is checked on lookup and swept opportunistically.

CREATE TABLE IF NOT EXISTS sessions (
  token_hash TEXT PRIMARY KEY,
  created_at TEXT NOT NULL DEFAULT '',
  expires_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_sessions_expires ON sessions (expires_at);
