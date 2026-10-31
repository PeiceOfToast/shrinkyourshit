-- SPDX-License-Identifier: AGPL-3.0-only
-- Plugins (task 31): one row per installed plugin. The manifest itself
-- lives on disk under <data dir>/plugins/<name>/ (the /config volume, so
-- it survives a container replace); this row is the index: the manifest's
-- identity fields copied at install time (name, version, type), whether
-- the operator enabled it (0 by default — nothing runs until it was
-- enabled on purpose), where it came from (source: a git URL or a local
-- path, shown in the UI), the collected settings values (settings, a JSON
-- object keyed by the manifest's setting keys) and when it was installed.
-- enabled is 0/1 like every other bool column.

CREATE TABLE plugins (
    id           TEXT PRIMARY KEY,
    name         TEXT UNIQUE NOT NULL,
    version      TEXT NOT NULL,
    type         TEXT NOT NULL,
    enabled      INTEGER NOT NULL DEFAULT 0,
    source       TEXT NOT NULL,
    settings     TEXT NOT NULL DEFAULT '{}',
    installed_at TEXT NOT NULL
);
