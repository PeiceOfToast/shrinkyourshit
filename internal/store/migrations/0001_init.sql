-- SPDX-License-Identifier: AGPL-3.0-only
-- Initial schema. Enums are stored as text and validated in Go
-- (internal/store). Targets and probe results are opaque JSON.

CREATE TABLE IF NOT EXISTS libraries (
  id TEXT PRIMARY KEY,
  name TEXT NOT NULL,
  path TEXT NOT NULL,
  target TEXT NOT NULL,
  watch INTEGER NOT NULL DEFAULT 0,
  schedule TEXT NOT NULL DEFAULT '',
  result_policy TEXT NOT NULL DEFAULT 'replace_if_smaller',
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS files (
  id TEXT PRIMARY KEY,
  library_id TEXT NOT NULL REFERENCES libraries(id) ON DELETE CASCADE,
  path TEXT NOT NULL,
  size INTEGER NOT NULL DEFAULT 0,
  mtime INTEGER NOT NULL DEFAULT 0,
  fingerprint TEXT NOT NULL DEFAULT '',
  probe TEXT NOT NULL DEFAULT '',
  status TEXT NOT NULL DEFAULT 'new',
  satisfied_reason TEXT NOT NULL DEFAULT '',
  last_job_id TEXT NOT NULL DEFAULT '',
  bytes_saved INTEGER NOT NULL DEFAULT 0,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  UNIQUE (library_id, path)
);
CREATE INDEX IF NOT EXISTS idx_files_library_status ON files (library_id, status);

CREATE TABLE IF NOT EXISTS jobs (
  id TEXT PRIMARY KEY,
  file_id TEXT NOT NULL REFERENCES files(id) ON DELETE CASCADE,
  library_id TEXT NOT NULL REFERENCES libraries(id) ON DELETE CASCADE,
  node_id TEXT NOT NULL DEFAULT '',
  status TEXT NOT NULL DEFAULT 'queued',
  priority INTEGER NOT NULL DEFAULT 0,
  attempt INTEGER NOT NULL DEFAULT 0,
  encoder_used TEXT NOT NULL DEFAULT '',
  fallback_reasons TEXT NOT NULL DEFAULT '[]',
  percent REAL NOT NULL DEFAULT 0,
  speed REAL NOT NULL DEFAULT 0,
  eta_seconds INTEGER NOT NULL DEFAULT 0,
  bytes_before INTEGER NOT NULL DEFAULT 0,
  bytes_after INTEGER NOT NULL DEFAULT 0,
  log TEXT NOT NULL DEFAULT '',
  error TEXT NOT NULL DEFAULT '',
  claimed_at TEXT NOT NULL DEFAULT '',
  started_at TEXT NOT NULL DEFAULT '',
  finished_at TEXT NOT NULL DEFAULT '',
  heartbeat_at TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_jobs_status ON jobs (status);
CREATE INDEX IF NOT EXISTS idx_jobs_file ON jobs (file_id);

CREATE TABLE IF NOT EXISTS nodes (
  id TEXT PRIMARY KEY,
  name TEXT NOT NULL,
  key_hash TEXT NOT NULL DEFAULT '',
  os TEXT NOT NULL DEFAULT '',
  arch TEXT NOT NULL DEFAULT '',
  capabilities TEXT NOT NULL DEFAULT '{}',
  access_mode TEXT NOT NULL DEFAULT 'auto',
  path_mappings TEXT NOT NULL DEFAULT '[]',
  status TEXT NOT NULL DEFAULT 'pending',
  enabled INTEGER NOT NULL DEFAULT 1,
  last_heartbeat_at TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS pairing_tokens (
  id TEXT PRIMARY KEY,
  token_hash TEXT NOT NULL UNIQUE,
  expires_at TEXT NOT NULL,
  used_at TEXT NOT NULL DEFAULT ''
);

CREATE TABLE IF NOT EXISTS settings (
  key TEXT PRIMARY KEY,
  value TEXT NOT NULL DEFAULT ''
);
