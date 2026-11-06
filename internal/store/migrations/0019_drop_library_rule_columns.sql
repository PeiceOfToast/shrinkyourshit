-- SPDX-License-Identifier: AGPL-3.0-only
-- v1.12 (task 78): drop the library rule columns that v1.11 deprecated. The
-- rule lives on the profile (0018, libraries.profile_id). Nothing has read or
-- written these six columns since task 76 (code) and task 77 (API).
--
--   flow_json, flow_source, target, easy, rule_tier, result_policy
--
-- Forward only and not reversible. After this migration a v1.10 binary cannot
-- run a library: it expects these columns to carry the rule. v1.11 and later
-- do not read them (CHANGELOG.md, v1.12 rollback warning).
--
-- ALTER TABLE ... DROP COLUMN needs SQLite 3.35 or newer; modernc.org/sqlite
-- v1.34.5 bundles a newer one. Each column is unindexed and unreferenced by
-- any view, trigger, foreign key or CHECK, so the drops are plain. The runner
-- applies this file once, inside its transaction.

ALTER TABLE libraries DROP COLUMN flow_json;
ALTER TABLE libraries DROP COLUMN flow_source;
ALTER TABLE libraries DROP COLUMN target;
ALTER TABLE libraries DROP COLUMN easy;
ALTER TABLE libraries DROP COLUMN rule_tier;
ALTER TABLE libraries DROP COLUMN result_policy;
