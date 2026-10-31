// SPDX-License-Identifier: AGPL-3.0-only

package store

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// jobLogCtx is the shared context for the job log tests.
func jobLogCtx() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), 30*time.Second)
}

// jobLogFixture creates a library and a file to hang jobs on (the jobs
// table has FKs to both).
func jobLogFixture(t *testing.T, s *SQLite) (libID, fileID string) {
	t.Helper()
	ctx, cancel := jobLogCtx()
	defer cancel()
	lib := validLibrary()
	require.NoError(t, s.CreateLibrary(ctx, lib))
	f := &File{LibraryID: lib.ID, Path: "/media/movies/movie.mkv", Size: 1000, Status: FileStatusQueued}
	require.NoError(t, s.UpsertFile(ctx, f))
	return lib.ID, f.ID
}

// jobLogJob creates a job row with the given status.
func jobLogJob(t *testing.T, s *SQLite, jobID, libID, fileID, status string) {
	t.Helper()
	ctx, cancel := jobLogCtx()
	defer cancel()
	require.NoError(t, s.CreateJob(ctx, &Job{ID: jobID, FileID: fileID, LibraryID: libID,
		Status: status, BytesBefore: 100}))
}

// logLine builds a JobLogLine for tests.
func logLine(seq int64, level, source, message string) JobLogLine {
	return JobLogLine{Seq: seq, Level: level, Source: source, Message: message, Repeated: 1}
}

// TestJobLogAppendAndReadBack round-trips one batch and checks the sort,
// the filters and the count.
func TestJobLogAppendAndReadBack(t *testing.T) {
	ctx, cancel := jobLogCtx()
	defer cancel()
	s := openTemp(t)

	libID, fileID := jobLogFixture(t, s)
	jobLogJob(t, s, "job-1", libID, fileID, JobStatusDone)
	jobLogJob(t, s, "job-2", libID, fileID, JobStatusDone)

	batch := []JobLogLine{
		{Seq: 0, Level: JobLogInfo, Source: "sys", Message: "started", Repeated: 1},
		{Seq: 1, Level: JobLogInfo, Source: "ffmpeg", Message: "frame= 1", Repeated: 1},
		{Seq: 2, Level: JobLogWarn, Source: "ffmpeg", Message: "rate limit", Repeated: 3},
	}
	require.NoError(t, s.AppendJobLog(ctx, "job-1", batch))

	// A job with no rows answers empty.
	rows, err := s.ListJobLog(ctx, "job-2", 0, "", 0)
	require.NoError(t, err)
	require.Empty(t, rows)
	total, err := s.CountJobLog(ctx, "job-2")
	require.NoError(t, err)
	require.Equal(t, int64(0), total)

	rows, err = s.ListJobLog(ctx, "job-1", 0, "", 0)
	require.NoError(t, err)
	require.Len(t, rows, 3)
	for i, row := range rows {
		require.Positive(t, row.ID)
		if i > 0 {
			require.Greater(t, row.ID, rows[i-1].ID, "rows must be ordered by id")
		}
		require.Equal(t, "job-1", row.JobID)
		require.Equal(t, batch[i].Seq, row.Seq)
		require.Equal(t, batch[i].Level, row.Level)
		require.Equal(t, batch[i].Source, row.Source)
		require.Equal(t, batch[i].Message, row.Message)
		require.Equal(t, batch[i].Repeated, row.Repeated)
		require.False(t, row.At.IsZero(), "the server stamps the row time")
	}

	total, err = s.CountJobLog(ctx, "job-1")
	require.NoError(t, err)
	require.Equal(t, int64(3), total)
}

func TestJobLogAfterIDAndLevelFilters(t *testing.T) {
	ctx, cancel := jobLogCtx()
	defer cancel()
	s := openTemp(t)

	libID, fileID := jobLogFixture(t, s)
	jobLogJob(t, s, "job-1", libID, fileID, JobStatusDone)
	require.NoError(t, s.AppendJobLog(ctx, "job-1", []JobLogLine{
		{Seq: 0, Level: JobLogInfo, Source: "sys", Message: "first"},
		{Seq: 1, Level: JobLogWarn, Source: "ffmpeg", Message: "warn"},
		{Seq: 2, Level: JobLogInfo, Source: "ffmpeg", Message: "second"},
		{Seq: 3, Level: JobLogError, Source: "sys", Message: "boom"},
	}))

	rows, err := s.ListJobLog(ctx, "job-1", 0, "", 0)
	require.NoError(t, err)
	require.Len(t, rows, 4)

	// after_id returns only rows newer than it.
	after := rows[1].ID
	rest, err := s.ListJobLog(ctx, "job-1", after, "", 0)
	require.NoError(t, err)
	require.Len(t, rest, 2)
	require.Equal(t, "second", rest[0].Message)
	require.Equal(t, "boom", rest[1].Message)

	// after_id with no newer rows is empty.
	rest, err = s.ListJobLog(ctx, "job-1", rows[3].ID, "", 0)
	require.NoError(t, err)
	require.Empty(t, rest)

	// The level filter narrows.
	rows, err = s.ListJobLog(ctx, "job-1", 0, JobLogWarn, 0)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.Equal(t, "warn", rows[0].Message)

	rows, err = s.ListJobLog(ctx, "job-1", 0, JobLogInfo, 0)
	require.NoError(t, err)
	require.Len(t, rows, 2)

	// A limit caps the page.
	rows, err = s.ListJobLog(ctx, "job-1", 0, "", 2)
	require.NoError(t, err)
	require.Len(t, rows, 2)
	require.Equal(t, "first", rows[0].Message)
}

func TestJobLogPerJobCap(t *testing.T) {
	ctx, cancel := jobLogCtx()
	defer cancel()
	s := openTemp(t)

	libID, fileID := jobLogFixture(t, s)
	jobLogJob(t, s, "job-1", libID, fileID, JobStatusDone)

	full := make([]JobLogLine, 0, JobLogMaxLinesPerJob)
	for i := 0; i < JobLogMaxLinesPerJob; i++ {
		full = append(full, logLine(int64(i), JobLogInfo, "ffmpeg", "stderr line"))
	}
	require.NoError(t, s.AppendJobLog(ctx, "job-1", full))

	total, err := s.CountJobLog(ctx, "job-1")
	require.NoError(t, err)
	require.Equal(t, int64(JobLogMaxLinesPerJob), total)

	// More rows are dropped: the cap is enforced on append.
	require.NoError(t, s.AppendJobLog(ctx, "job-1", []JobLogLine{
		{Seq: 2000, Level: JobLogError, Source: "sys", Message: "late"},
	}))
	total, err = s.CountJobLog(ctx, "job-1")
	require.NoError(t, err)
	require.Equal(t, int64(JobLogMaxLinesPerJob), total)

	// The first lines of the batch are the ones that landed.
	rows, err := s.ListJobLog(ctx, "job-1", 0, "", 0)
	require.NoError(t, err)
	require.Equal(t, int64(0), rows[0].Seq)
	require.Equal(t, int64(JobLogMaxLinesPerJob-1), rows[len(rows)-1].Seq)
}

func TestJobLogPruneByAgeAndCount(t *testing.T) {
	ctx, cancel := jobLogCtx()
	defer cancel()
	s := openTemp(t)

	now := time.Now().UTC()
	old := now.Add(-30 * 24 * time.Hour)
	libID, fileID := jobLogFixture(t, s)
	jobLogJob(t, s, "old", libID, fileID, JobStatusDone)
	jobLogJob(t, s, "running", libID, fileID, JobStatusRunning)
	jobLogJob(t, s, "queued", libID, fileID, JobStatusQueued)

	// Three jobs with old rows: a done job, a running job, a queued job.
	for _, jobID := range []string{"old", "running", "queued"} {
		lines := []JobLogLine{
			{At: old, Seq: 0, Level: JobLogInfo, Source: "sys", Message: "old line"},
			{At: old, Seq: 1, Level: JobLogInfo, Source: "sys", Message: "old line 2"},
		}
		require.NoError(t, s.AppendJobLog(ctx, jobID, lines))
	}
	// A fresh row on the done job survives the age rule.
	require.NoError(t, s.AppendJobLog(ctx, "old", []JobLogLine{
		{Seq: 2, Level: JobLogInfo, Source: "sys", Message: "fresh"},
	}))

	// The age rule deletes the done job's old rows but never touches a
	// running or queued job's rows, even when they are older.
	removed, err := s.PruneJobLog(ctx, now.Add(-14*24*time.Hour), JobLogMaxTotal)
	require.NoError(t, err)
	require.Equal(t, int64(2), removed)

	for _, jobID := range []string{"running", "queued"} {
		total, err := s.CountJobLog(ctx, jobID)
		require.NoError(t, err)
		require.Equal(t, int64(2), total, "the %s job's rows must survive", jobID)
	}
	total, err := s.CountJobLog(ctx, "old")
	require.NoError(t, err)
	require.Equal(t, int64(1), total) // the fresh row

	// The count rule keeps the newest maxTotal unprotected rows, oldest
	// first. Unprotected rows: 1 on "old"; maxTotal=0 keeps none of them,
	// but the running and queued jobs' rows stay protected.
	removed, err = s.PruneJobLog(ctx, now.Add(-14*24*time.Hour), 0)
	require.NoError(t, err)
	require.Equal(t, int64(1), removed)
	total, err = s.CountJobLog(ctx, "old")
	require.NoError(t, err)
	require.Equal(t, int64(0), total)
	for _, jobID := range []string{"running", "queued"} {
		total, err := s.CountJobLog(ctx, jobID)
		require.NoError(t, err)
		require.Equal(t, int64(2), total, "the %s job's rows must survive", jobID)
	}
}

func TestJobLogPruneCountKeepsNewest(t *testing.T) {
	ctx, cancel := jobLogCtx()
	defer cancel()
	s := openTemp(t)

	libID, fileID := jobLogFixture(t, s)
	jobLogJob(t, s, "job-1", libID, fileID, JobStatusDone)
	// 10 rows, older than the cutoff: maxTotal=4 keeps the newest 4.
	require.NoError(t, s.AppendJobLog(ctx, "job-1", []JobLogLine{
		{At: time.Now().UTC().Add(-30 * 24 * time.Hour), Seq: 0, Level: JobLogInfo, Source: "sys", Message: "line 0"},
	}))
	require.NoError(t, s.AppendJobLog(ctx, "job-1", []JobLogLine{
		{At: time.Now().UTC().Add(-30 * 24 * time.Hour), Seq: 1, Level: JobLogInfo, Source: "sys", Message: "line 1"},
	}))
	require.NoError(t, s.AppendJobLog(ctx, "job-1", []JobLogLine{
		{At: time.Now().UTC().Add(-30 * 24 * time.Hour), Seq: 2, Level: JobLogInfo, Source: "sys", Message: "line 2"},
	}))
	require.NoError(t, s.AppendJobLog(ctx, "job-1", []JobLogLine{
		{At: time.Now().UTC().Add(-30 * 24 * time.Hour), Seq: 3, Level: JobLogInfo, Source: "sys", Message: "line 3"},
	}))
	require.NoError(t, s.AppendJobLog(ctx, "job-1", []JobLogLine{
		{At: time.Now().UTC().Add(-30 * 24 * time.Hour), Seq: 4, Level: JobLogInfo, Source: "sys", Message: "line 4"},
	}))

	removed, err := s.PruneJobLog(ctx, time.Now().UTC().Add(-14*24*time.Hour), 4)
	require.NoError(t, err)
	require.Equal(t, int64(5), removed) // the age rule deletes all 5 old rows

	// Refill and prune by count only (everything is fresh now).
	for i := 0; i < 10; i++ {
		require.NoError(t, s.AppendJobLog(ctx, "job-1", []JobLogLine{
			{Seq: int64(i), Level: JobLogInfo, Source: "sys", Message: "fresh line"},
		}))
	}
	removed, err = s.PruneJobLog(ctx, time.Now().UTC().Add(-14*24*time.Hour), 4)
	require.NoError(t, err)
	require.Equal(t, int64(6), removed)
	rows, err := s.ListJobLog(ctx, "job-1", 0, "", 0)
	require.NoError(t, err)
	require.Len(t, rows, 4)
	require.Equal(t, "fresh line", rows[0].Message)
}

func TestJobLogRowsDisappearWithTheirJob(t *testing.T) {
	ctx, cancel := jobLogCtx()
	defer cancel()
	s := openTemp(t)

	libID, fileID := jobLogFixture(t, s)
	jobLogJob(t, s, "job-1", libID, fileID, JobStatusDone)
	jobLogJob(t, s, "job-2", libID, fileID, JobStatusDone)
	require.NoError(t, s.AppendJobLog(ctx, "job-1", []JobLogLine{
		{Seq: 0, Level: JobLogInfo, Source: "sys", Message: "one"},
	}))
	require.NoError(t, s.AppendJobLog(ctx, "job-2", []JobLogLine{
		{Seq: 0, Level: JobLogInfo, Source: "sys", Message: "two"},
	}))

	// Deleting a job removes its log rows.
	require.NoError(t, s.DeleteJob(ctx, "job-1"))
	total, err := s.CountJobLog(ctx, "job-1")
	require.NoError(t, err)
	require.Equal(t, int64(0), total)

	// Deleting a file removes its jobs' log rows too (the cascade).
	require.NoError(t, s.DeleteFile(ctx, fileID))
	total, err = s.CountJobLog(ctx, "job-2")
	require.NoError(t, err)
	require.Equal(t, int64(0), total)

	// The file cascade removed job-2 as well (jobs.file_id REFERENCES files
	// ON DELETE CASCADE), which took its log rows with it; deleting log
	// rows for a job that is already gone is safe.
	total, err = s.CountJobLog(ctx, "job-2")
	require.NoError(t, err)
	require.Equal(t, int64(0), total)
	require.NoError(t, s.DeleteJobLog(ctx, "job-2"))
}

func TestJobLogValidate(t *testing.T) {
	ctx, cancel := jobLogCtx()
	defer cancel()
	s := openTemp(t)
	libID, fileID := jobLogFixture(t, s)
	jobLogJob(t, s, "job-1", libID, fileID, JobStatusDone)

	require.Error(t, s.AppendJobLog(ctx, "job-1", []JobLogLine{
		{Seq: 0, Level: "verbose", Source: "sys", Message: "bad level"},
	}))
	require.Error(t, s.AppendJobLog(ctx, "job-1", []JobLogLine{
		{Seq: 0, Level: JobLogInfo, Source: "stderr", Message: "bad source"},
	}))
	require.Error(t, s.AppendJobLog(ctx, "job-1", []JobLogLine{
		{Seq: 0, Level: JobLogInfo, Source: "sys", Message: ""},
	}))
	require.NoError(t, s.AppendJobLog(ctx, "job-1", []JobLogLine{
		{Seq: 0, Level: JobLogInfo, Source: "sys", Message: "no repeat count"},
	})) // a zero repeat count normalizes to one
	rows, err := s.ListJobLog(ctx, "job-1", 0, "", 0)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.Equal(t, int64(1), rows[0].Repeated)
}

func TestJobLogPruneReclaimsSpace(t *testing.T) {
	// Acceptance item 8: the file size before and after a prune test, and
	// the incremental vacuum ran.
	ctx, cancel := jobLogCtx()
	defer cancel()
	s := openTemp(t)
	libID, fileID := jobLogFixture(t, s)
	jobLogJob(t, s, "job-1", libID, fileID, JobStatusDone)

	// The job_log rows are capped at 2000 per job; two jobs give 4000
	// 4 KiB messages ≈ 16 MB.
	big := string(make([]byte, 4096))
	appendBig := func(jobID string) {
		t.Helper()
		lines := make([]JobLogLine, 0, JobLogMaxLinesPerJob)
		for i := 0; i < JobLogMaxLinesPerJob; i++ {
			lines = append(lines, JobLogLine{Seq: int64(i), Level: JobLogInfo,
				Source: "ffmpeg", Message: big})
		}
		require.NoError(t, s.AppendJobLog(ctx, jobID, lines))
	}
	jobLogJob(t, s, "job-2", libID, fileID, JobStatusDone)
	appendBig("job-1")
	appendBig("job-2")

	jobLogJob(t, s, "job-3", libID, fileID, JobStatusRunning)
	require.NoError(t, s.AppendJobLog(ctx, "job-3", []JobLogLine{
		{Seq: 0, Level: JobLogInfo, Source: "sys", Message: "still running"},
	}))

	size := func() int64 {
		// The file's byte size: PRAGMA page_count * page_size.
		var pages int64
		require.NoError(t, s.db.QueryRow(`PRAGMA page_count`).Scan(&pages))
		var pageSize int64
		require.NoError(t, s.db.QueryRow(`PRAGMA page_size`).Scan(&pageSize))
		return pages * pageSize
	}

	before := size()
	require.Greater(t, before, int64(10_000_000), "the log rows must be in the file")
	t.Logf("database file size before the prune: %d bytes", before)

	// Backdate the done jobs' rows and prune by age.
	_, err := s.db.Exec(`UPDATE job_log SET at = ? WHERE job_id IN ('job-1', 'job-2')`,
		fmtTime(time.Now().UTC().Add(-30*24*time.Hour)))
	require.NoError(t, err)
	removed, err := s.PruneJobLog(ctx, time.Now().UTC().Add(-14*24*time.Hour), JobLogMaxTotal)
	require.NoError(t, err)
	require.Equal(t, int64(2*JobLogMaxLinesPerJob), removed)

	after := size()
	require.Less(t, after, before, "incremental vacuum must give the space back")
	require.NotZero(t, after)
	t.Logf("database file size after the prune: %d bytes", after)

	// The running job's row survives.
	total, err := s.CountJobLog(ctx, "job-3")
	require.NoError(t, err)
	require.Equal(t, int64(1), total)
}
