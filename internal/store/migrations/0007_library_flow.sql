-- SPDX-License-Identifier: AGPL-3.0-only
-- Flow storage (task 14). flow_json is the canonical rule graph (the Graph
-- JSON from internal/flow) every tier -- easy mode, step stack, canvas --
-- generates and edits. flow_source records who owns it: easy, steps or
-- advanced. Both default empty: rows from before this task are backfilled
-- at startup by the server (a plain SQL migration cannot compile a Target
-- into a flow), and the library manager compiles lazily on read until then.

ALTER TABLE libraries ADD COLUMN flow_json TEXT NOT NULL DEFAULT '';
ALTER TABLE libraries ADD COLUMN flow_source TEXT NOT NULL DEFAULT '';
