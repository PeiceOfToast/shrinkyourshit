-- SPDX-License-Identifier: AGPL-3.0-only
-- Live output size (task 18). The worker stats its scratch output on every
-- progress tick (one cheap stat per ~2s tick) and reports the size; the
-- server stores it so the node card can show the original against the
-- growing output. 0 means "nothing reported yet".

ALTER TABLE jobs ADD COLUMN bytes_so_far INTEGER NOT NULL DEFAULT 0;
