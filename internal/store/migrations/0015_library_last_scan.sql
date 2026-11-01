-- SPDX-License-Identifier: AGPL-3.0-only
-- Last scan summary (task 34): the scanner persists a JSON snapshot of the
-- most recent successful scan — its counts, including the distinct count of
-- files skipped because their kind is not what the library looks for — so
-- the library detail page can show why a photos library is doing nothing.
-- Opaque JSON to the store; the server owns the shape.

ALTER TABLE libraries ADD COLUMN last_scan TEXT NOT NULL DEFAULT '';
