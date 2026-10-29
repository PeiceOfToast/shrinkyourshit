-- SPDX-License-Identifier: AGPL-3.0-only
-- File transfer mode (task 09). A job records the access mode the node
-- actually used (empty until reported: shared or transfer). A node can cap
-- its transfer throughput (MB/s; 0 = unlimited).

ALTER TABLE jobs ADD COLUMN access_mode TEXT NOT NULL DEFAULT '';
ALTER TABLE nodes ADD COLUMN throughput_limit_mbps INTEGER NOT NULL DEFAULT 0;
