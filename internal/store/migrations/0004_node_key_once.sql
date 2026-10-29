-- SPDX-License-Identifier: AGPL-3.0-only
-- Node pairing (task 08). A pending node receives its key when the admin
-- approves it; the raw key is kept here until the node retrieves it once,
-- so a server restart between approval and the first poll does not lose
-- it. After the first retrieval only the hash remains (the column is
-- cleared; revoke and delete clear it too).

ALTER TABLE nodes ADD COLUMN key_once TEXT NOT NULL DEFAULT '';
