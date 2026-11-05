-- SPDX-License-Identifier: AGPL-3.0-only
-- Rule tier (task 59, spec 6): the editor a library offers, one of basic,
-- moderate or advanced. It is a property of the library, not of its graph:
-- changing it never rewrites flow_json (decision 4). The backfill below
-- derives each existing library's tier from its flow_source so every library
-- keeps the editor it had: easy becomes basic, steps becomes moderate and
-- advanced stays advanced. A row with no flow_source yet (not backfilled at
-- startup) follows the same inference LibraryFlow uses: easy input without a
-- stored flow is easy, anything else is advanced.

ALTER TABLE libraries ADD COLUMN rule_tier TEXT NOT NULL DEFAULT '';

UPDATE libraries SET rule_tier = CASE flow_source
  WHEN 'easy' THEN 'basic'
  WHEN 'steps' THEN 'moderate'
  WHEN 'advanced' THEN 'advanced'
  ELSE CASE WHEN flow_json = '' AND easy <> '' THEN 'basic' ELSE 'advanced' END
END;
