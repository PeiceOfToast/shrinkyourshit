-- SPDX-License-Identifier: AGPL-3.0-only
-- Node lifecycle (task 11). Operator intent lives on the node: desired_state
-- is what the human asked for (stopped | running | draining), concurrency caps
-- the node's parallel jobs, state_error records a transition that failed to
-- apply. New nodes default to stopped: nothing encodes until a human starts
-- it. Existing nodes that had work in flight at upgrade time keep running
-- (the job rows are the marker — stopping mid-encode would strand it);
-- everyone else, including an idle built-in node, wakes up stopped.
-- concurrency is seeded from the old global worker_concurrency setting so an
-- upgraded install keeps its throughput (a missing or broken value falls
-- back to 1).

ALTER TABLE nodes ADD COLUMN desired_state TEXT NOT NULL DEFAULT 'stopped';
ALTER TABLE nodes ADD COLUMN concurrency INTEGER NOT NULL DEFAULT 1;
ALTER TABLE nodes ADD COLUMN state_error TEXT NOT NULL DEFAULT '';

UPDATE nodes SET desired_state = 'running'
  WHERE status = 'approved'
    AND EXISTS (SELECT 1 FROM jobs WHERE jobs.node_id = nodes.id
                AND jobs.status IN ('claimed', 'running'));

UPDATE nodes SET concurrency = MAX(1, CAST(COALESCE(
  (SELECT value FROM settings WHERE key = 'worker_concurrency'), '1'
) AS INTEGER));
