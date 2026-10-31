// SPDX-License-Identifier: AGPL-3.0-only

package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"testing"

	"github.com/PeiceOfToast/shrinkyourshit/internal/server"
	"github.com/PeiceOfToast/shrinkyourshit/internal/store"
	"github.com/stretchr/testify/require"
)

// logJobFixture queues a job and claims it (the claimed job is what a
// worker reports logs for). Returns the node key and the claimed job.
func (h *apiHarness) claimedJob(t *testing.T) (string, *store.Job) {
	t.Helper()
	key, _ := h.addNode(t, "w1", store.NodeStatusApproved, true)
	_, _, _ = h.queueOne(t)
	resp := h.doNode(http.MethodPost, "/api/v1/worker/claim", nil, key)
	require.Equal(t, http.StatusOK, resp.Code)
	var job server.JobDTO
	require.NoError(t, json.Unmarshal(resp.Body.Bytes(), &job))
	return key, h.jobRow(t, job.ID)
}

// jobRow fetches the raw job row.
func (h *apiHarness) jobRow(t *testing.T, id string) *store.Job {
	t.Helper()
	job, err := h.st.GetJob(context.Background(), id)
	require.NoError(t, err)
	return job
}

// appendLog stores one line for a job directly through the store.
func (h *apiHarness) appendLog(t *testing.T, jobID string, lines []store.JobLogLine) {
	t.Helper()
	require.NoError(t, h.st.AppendJobLog(context.Background(), jobID, lines))
}

func TestJobLogReadBackInOrder(t *testing.T) {
	h := newAPIHarness(t)
	key, job := h.claimedJob(t)

	// The owner appends a batch; the API stores and serves it in order.
	batch := map[string]any{
		"seq": 0,
		"lines": []map[string]any{
			{"seq": 0, "level": "info", "source": "sys", "message": "started"},
			{"seq": 1, "level": "warn", "source": "ffmpeg", "message": "slow", "repeated": 4},
			{"seq": 2, "level": "error", "source": "ffmpeg", "message": "boom"},
		},
	}
	resp := h.doNode(http.MethodPost, "/api/v1/worker/jobs/"+job.ID+"/log", batch, key)
	require.Equal(t, http.StatusOK, resp.Code)

	resp = h.do(http.MethodGet, "/api/v1/jobs/"+job.ID+"/log", nil)
	require.Equal(t, http.StatusOK, resp.Code)
	var out struct {
		Lines []struct {
			ID       int64  `json:"id"`
			Seq      int64  `json:"seq"`
			Level    string `json:"level"`
			Source   string `json:"source"`
			Message  string `json:"message"`
			Repeated int64  `json:"repeated"`
		} `json:"lines"`
		Total   int64  `json:"total"`
		HasMore bool   `json:"has_more"`
		Status  string `json:"status"`
	}
	require.NoError(t, json.Unmarshal(resp.Body.Bytes(), &out))
	require.Len(t, out.Lines, 3)
	require.Equal(t, store.JobStatusClaimed, out.Status,
		"the response carries the job's current status for the viewer")
	for i, line := range out.Lines {
		if i > 0 {
			require.Greater(t, line.ID, out.Lines[i-1].ID, "rows are ordered by id")
		}
	}
	require.Equal(t, "started", out.Lines[0].Message)
	require.Equal(t, int64(4), out.Lines[1].Repeated)
	require.Equal(t, "boom", out.Lines[2].Message)
	require.Equal(t, int64(3), out.Total)
	require.False(t, out.HasMore)

	// A job with no log rows answers an empty list, 200.
	resp = h.do(http.MethodGet, "/api/v1/jobs/"+job.ID+"/log/download", nil)
	require.Equal(t, http.StatusOK, resp.Code)
	require.Contains(t, resp.Header().Get("Content-Type"), "text/plain")
	require.Equal(t, `attachment; filename="job-`+job.ID+`.log"`,
		resp.Header().Get("Content-Disposition"))
}

func TestJobLogAfterIDAndLevelAndHasMore(t *testing.T) {
	h := newAPIHarness(t)
	_, job := h.claimedJob(t)

	h.appendLog(t, job.ID, []store.JobLogLine{
		{Seq: 0, Level: store.JobLogInfo, Source: store.JobLogSourceSys, Message: "one"},
		{Seq: 1, Level: store.JobLogWarn, Source: store.JobLogSourceFFmpeg, Message: "two"},
		{Seq: 2, Level: store.JobLogInfo, Source: store.JobLogSourceFFmpeg, Message: "three"},
		{Seq: 3, Level: store.JobLogError, Source: store.JobLogSourceSys, Message: "four"},
	})

	// after_id returns only newer rows.
	resp := h.do(http.MethodGet, "/api/v1/jobs/"+job.ID+"/log?after_id=2", nil)
	require.Equal(t, http.StatusOK, resp.Code)
	var out jobLogResponse
	require.NoError(t, json.Unmarshal(resp.Body.Bytes(), &out))
	require.Len(t, out.Lines, 2)
	require.Equal(t, "three", out.Lines[0].Message)
	require.Equal(t, "four", out.Lines[1].Message)
	require.Equal(t, int64(4), out.Total)
	require.False(t, out.HasMore)

	// The level filter narrows and the total stays the job's row count.
	resp = h.do(http.MethodGet, "/api/v1/jobs/"+job.ID+"/log?level=info", nil)
	require.NoError(t, json.Unmarshal(resp.Body.Bytes(), &out))
	require.Len(t, out.Lines, 2)
	require.Equal(t, int64(4), out.Total)

	// has_more is right at the limit boundary: 4 rows, limit 3 → 3 rows
	// with has_more; the last one via after_id has none.
	resp = h.do(http.MethodGet, "/api/v1/jobs/"+job.ID+"/log?limit=3", nil)
	require.NoError(t, json.Unmarshal(resp.Body.Bytes(), &out))
	require.Len(t, out.Lines, 3)
	require.True(t, out.HasMore)
	last := out.Lines[2].ID
	resp = h.do(http.MethodGet, "/api/v1/jobs/"+job.ID+"/log?limit=3&after_id="+
		strconvI64(last), nil)
	require.NoError(t, json.Unmarshal(resp.Body.Bytes(), &out))
	require.Len(t, out.Lines, 1)
	require.False(t, out.HasMore)

	// Bad parameters are 400 invalid_request.
	for _, path := range []string{
		"/api/v1/jobs/" + job.ID + "/log?after_id=abc",
		"/api/v1/jobs/" + job.ID + "/log?level=verbose",
		"/api/v1/jobs/" + job.ID + "/log?limit=-1",
		"/api/v1/jobs/" + job.ID + "/log?limit=0",
	} {
		resp = h.do(http.MethodGet, path, nil)
		require.Equal(t, http.StatusBadRequest, resp.Code, path)
	}

	// An unknown job is 404 on both routes.
	resp = h.do(http.MethodGet, "/api/v1/jobs/nope/log", nil)
	require.Equal(t, http.StatusNotFound, resp.Code)
	resp = h.do(http.MethodGet, "/api/v1/jobs/nope/log/download", nil)
	require.Equal(t, http.StatusNotFound, resp.Code)
}

// TestJobLogDownloadRendersRows: the download route is text/plain, one row
// per line as `<at> <level> <source> <message>`.
func TestJobLogDownloadRendersRows(t *testing.T) {
	h := newAPIHarness(t)
	_, job := h.claimedJob(t)

	h.appendLog(t, job.ID, []store.JobLogLine{
		{Seq: 0, Level: store.JobLogInfo, Source: store.JobLogSourceSys, Message: "started"},
		{Seq: 1, Level: store.JobLogWarn, Source: store.JobLogSourceFFmpeg, Message: "a warning"},
	})
	resp := h.do(http.MethodGet, "/api/v1/jobs/"+job.ID+"/log/download", nil)
	require.Equal(t, http.StatusOK, resp.Code)
	require.Contains(t, resp.Header().Get("Content-Type"), "text/plain")
	require.Equal(t, `attachment; filename="job-`+job.ID+`.log"`,
		resp.Header().Get("Content-Disposition"))

	lines := strings.Split(strings.TrimSpace(resp.Body.String()), "\n")
	require.Len(t, lines, 2)
	// "<at> <level> <source> <message>" — the at field is RFC 3339.
	fields := strings.Split(lines[0], " ")
	require.Len(t, fields, 4)
	require.Equal(t, "info", fields[1])
	require.Equal(t, "sys", fields[2])
	require.Equal(t, "started", fields[3])
	require.Equal(t, "a warning", strings.SplitN(lines[1], " ", 4)[3])
}

// TestJobLogHasMoreBeyondPage: exactly at the page boundary there is no
// more; one row beyond flips has_more.
func TestJobLogHasMoreBeyondPage(t *testing.T) {
	h := newAPIHarness(t)
	_, job := h.claimedJob(t)
	lines := make([]store.JobLogLine, 0, 4)
	for i := 0; i < 4; i++ {
		lines = append(lines, store.JobLogLine{Seq: int64(i), Level: store.JobLogInfo,
			Source: store.JobLogSourceSys, Message: "line"})
	}
	h.appendLog(t, job.ID, lines)

	var out jobLogResponse
	resp := h.do(http.MethodGet, "/api/v1/jobs/"+job.ID+"/log?limit=4", nil)
	require.NoError(t, json.Unmarshal(resp.Body.Bytes(), &out))
	require.False(t, out.HasMore)

	resp = h.do(http.MethodGet, "/api/v1/jobs/"+job.ID+"/log?limit=3", nil)
	require.NoError(t, json.Unmarshal(resp.Body.Bytes(), &out))
	require.True(t, out.HasMore)
}

// TestJobLogCapOnAppend: the per-job cap stops a flood (task 27).
func TestJobLogCapOnAppend(t *testing.T) {
	h := newAPIHarness(t)
	key, job := h.claimedJob(t)

	flood := make([]map[string]any, 0, 1200)
	for i := 0; i < 1200; i++ {
		flood = append(flood, map[string]any{
			"seq": i, "level": "info", "source": "ffmpeg", "message": "frame",
		})
	}
	batch := map[string]any{"seq": 0, "lines": flood}
	resp := h.doNode(http.MethodPost, "/api/v1/worker/jobs/"+job.ID+"/log", batch, key)
	require.Equal(t, http.StatusOK, resp.Code)

	total, err := h.st.CountJobLog(context.Background(), job.ID)
	require.NoError(t, err)
	require.LessOrEqual(t, total, int64(store.JobLogMaxLinesPerJob))
}

// TestJobDTOCarriesLogCounts: job responses carry log_lines and has_log
// (task 27).
func TestJobDTOCarriesLogCounts(t *testing.T) {
	h := newAPIHarness(t)
	_, job := h.claimedJob(t)

	resp := h.do(http.MethodGet, "/api/v1/jobs/"+job.ID, nil)
	require.Equal(t, http.StatusOK, resp.Code)
	var dto server.JobDTO
	require.NoError(t, json.Unmarshal(resp.Body.Bytes(), &dto))
	require.False(t, dto.HasLog)
	require.Zero(t, dto.LogLines)

	h.appendLog(t, job.ID, []store.JobLogLine{
		{Seq: 0, Level: store.JobLogInfo, Source: store.JobLogSourceSys, Message: "row"},
	})
	resp = h.do(http.MethodGet, "/api/v1/jobs/"+job.ID, nil)
	require.NoError(t, json.Unmarshal(resp.Body.Bytes(), &dto))
	require.True(t, dto.HasLog)
	require.Equal(t, int64(1), dto.LogLines)
}

// strconvI64 formats an int64 for a query string.
func strconvI64(v int64) string {
	return strconv.FormatInt(v, 10)
}
