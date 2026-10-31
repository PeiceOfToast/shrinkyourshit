-- SPDX-License-Identifier: AGPL-3.0-only
-- Job log rows (task 27): every job keeps its complete log as structured
-- rows, not as a 64 KiB tail or files on disk. One row per distinct message;
-- `repeated` collapses consecutive identical lines (an ffmpeg warning loop
-- writes one row, not thousands).
--
-- The FK cascade takes the log rows with the job: deleting a job, a file or
-- a library (which cascades to its jobs) removes its log rows in the same
-- place the job rows go, and is a no-op when they are already gone.
--
-- Note: the database is switched to incremental auto vacuum in store.New
-- (before migrations apply), because a PRAGMA inside this transactional
-- migration runner would be silently ignored and a mode change on a
-- non-empty database needs one VACUUM to stick.

CREATE TABLE IF NOT EXISTS job_log (
    id       INTEGER PRIMARY KEY AUTOINCREMENT,
    job_id   TEXT NOT NULL REFERENCES jobs(id) ON DELETE CASCADE,
    seq      INTEGER NOT NULL,
    at       TEXT NOT NULL,
    level    TEXT NOT NULL,               -- info | warn | error
    source   TEXT NOT NULL,               -- ffmpeg | sys
    message  TEXT NOT NULL,
    repeated INTEGER NOT NULL DEFAULT 1
);

CREATE INDEX IF NOT EXISTS job_log_job ON job_log(job_id, id);
CREATE INDEX IF NOT EXISTS job_log_at  ON job_log(at);
