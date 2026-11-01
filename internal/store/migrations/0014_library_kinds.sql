-- SPDX-License-Identifier: AGPL-3.0-only
-- Library file kinds (task 34): a library declares what it holds, so the
-- scanner skips everything else before probing and the planner can avoid
-- building a media target for files that are not media. The column holds a
-- JSON array of Kind strings ("video", "audio", "subtitle", "image",
-- "other"). The default keeps every existing library video-only. One row
-- per kind is stored as the JSON array itself — validation (at least one
-- value, every value a known Kind, no duplicates) happens in Go.

ALTER TABLE libraries ADD COLUMN file_kinds TEXT NOT NULL DEFAULT '["video"]';
