-- SPDX-License-Identifier: AGPL-3.0-only
-- Easy-mode state (task 07). The raw EasyModeInput JSON the easy screen
-- submitted, stored beside the compiled target so an easy-mode library
-- reopens with its sliders exactly where they were. Empty when the library
-- was created from a full target.

ALTER TABLE libraries ADD COLUMN easy TEXT NOT NULL DEFAULT '';
