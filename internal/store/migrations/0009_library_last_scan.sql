-- SPDX-License-Identifier: AGPL-3.0-only
-- Last scan time (task 19). The scanner stamps it when a scan of the
-- library finishes without error, so the library detail header can show
-- when the folder was last looked at. Empty means "never scanned".

ALTER TABLE libraries ADD COLUMN last_scan_at TEXT NOT NULL DEFAULT '';
