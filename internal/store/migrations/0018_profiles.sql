-- SPDX-License-Identifier: AGPL-3.0-only
-- Encode profiles (encode profiles and storage wave, spec 5.1): a named,
-- reusable encode graph with its tier and output policy. A library points at
-- one through profile_id. The graph, tier and result_policy are copied onto
-- profiles by the boot step BackfillProfiles (internal/store/profiles.go),
-- because a SQL migration cannot hash the canonical graph JSON.
--
-- libraries.flow, easy, rule_tier and result_policy stay in place, unused for
-- one release so a rollback loses nothing. Forward only.

CREATE TABLE profiles (
  id            TEXT PRIMARY KEY,
  name          TEXT NOT NULL UNIQUE,
  description   TEXT NOT NULL DEFAULT '',
  tier          TEXT NOT NULL,
  graph         TEXT NOT NULL,
  result_policy TEXT NOT NULL,
  created_at    TEXT NOT NULL,
  updated_at    TEXT NOT NULL
);

ALTER TABLE libraries ADD COLUMN profile_id TEXT REFERENCES profiles(id);
