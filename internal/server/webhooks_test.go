// SPDX-License-Identifier: AGPL-3.0-only

package server

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/PeiceOfToast/shrinkyourshit/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// shrinkBackoff makes a dispatcher's retries fast enough for tests without
// a clock seam.
func shrinkBackoff(d *Dispatcher) {
	d.backoff = []time.Duration{5 * time.Millisecond, 5 * time.Millisecond}
}

// sampleDone builds a job.done event with the numbers of the task's
// example summary.
func sampleDone() WebhookEvent {
	job := JobDTO{
		ID: "job-1", Status: store.JobStatusDone,
		FileName: "The Matrix (1999).mkv", FilePath: "/media/movies/The Matrix (1999).mkv",
		BytesBefore: 4_100_000_000, BytesAfter: 1_200_000_000, Percent: 100,
	}
	file := FileDTO{ID: "file-1", Status: store.FileStatusSatisfied, BytesSaved: 2_900_000_000}
	lib := LibraryDTO{ID: "lib-1", Name: "Movies"}
	return WebhookEvent{
		Event:   EventWebhookJobDone,
		Job:     &job,
		File:    &file,
		Library: &lib,
		At:      time.Now().UTC(),
	}
}

// newWebhook stores a hook pointing at url and returns its secret.
func newWebhook(t *testing.T, h *harness, name, url string, events ...string) string {
	t.Helper()
	secret, err := NewWebhookSecret()
	require.NoError(t, err)
	w := Webhook{Name: name, URL: url, Secret: secret, Events: events, Enabled: true}
	require.NoError(t, ValidateWebhook(&w))
	require.NoError(t, h.st.CreateWebhook(context.Background(), w.Row()))
	return secret
}

// webhookRow returns the stored shape of the only hook (each test stores
// one unless stated).
func webhookRow(t *testing.T, h *harness) *store.Webhook {
	t.Helper()
	rows, err := h.st.ListWebhooks(context.Background())
	require.NoError(t, err)
	require.Len(t, rows, 1)
	return rows[0]
}

// deliveries returns a hook's recorded deliveries (the tests wait on the
// remote or sleep before calling, so the goroutine has settled).
func deliveries(t *testing.T, h *harness, webhookID string) []*store.WebhookDelivery {
	t.Helper()
	rows, err := h.st.ListWebhookDeliveries(context.Background(), webhookID, 0)
	require.NoError(t, err)
	return rows
}

// TestValidateWebhook pins the create/update validation and the in-place
// normalization.
func TestValidateWebhook(t *testing.T) {
	w := Webhook{Name: " Hook ", URL: " https://discord.com/api/webhooks/1/abc ",
		Events: []string{" job.done ", "job.done", "job.failed"}}
	require.NoError(t, ValidateWebhook(&w))
	assert.Equal(t, "Hook", w.Name)
	assert.Equal(t, "https://discord.com/api/webhooks/1/abc", w.URL)
	assert.Equal(t, []string{EventWebhookJobDone, EventWebhookJobFailed}, w.Events)

	// Unknown event name.
	w.Events = []string{"job.finished"}
	require.ErrorContains(t, ValidateWebhook(&w), "unknown event")

	// URL that does not parse.
	w = Webhook{Name: "x", URL: "http://host name/", Events: []string{EventWebhookJobDone}}
	require.Error(t, ValidateWebhook(&w))

	// URL without http/https.
	w = Webhook{Name: "x", URL: "ftp://host/", Events: []string{EventWebhookJobDone}}
	require.ErrorContains(t, ValidateWebhook(&w), "http or https")
	w = Webhook{Name: "x", URL: "no-scheme", Events: []string{EventWebhookJobDone}}
	require.Error(t, ValidateWebhook(&w))

	// Missing name.
	w = Webhook{Name: "  ", URL: "http://host/", Events: []string{EventWebhookJobDone}}
	require.ErrorContains(t, ValidateWebhook(&w), "name is required")

	// Empty subscription.
	w = Webhook{Name: "x", URL: "http://host/", Events: nil}
	require.ErrorContains(t, ValidateWebhook(&w), "at least one event")
}

// TestNewWebhookSecret proves the generated secret is 32 bytes of hex and
// unique per call.
func TestNewWebhookSecret(t *testing.T) {
	a, err := NewWebhookSecret()
	require.NoError(t, err)
	b, err := NewWebhookSecret()
	require.NoError(t, err)
	require.Len(t, a, 64)
	require.Len(t, b, 64)
	_, err = hex.DecodeString(a)
	require.NoError(t, err)
	assert.NotEqual(t, a, b)
}

// TestWebhookDeliveryContract covers acceptance item 1: all three events
// deliver to an httptest server with the right X-SYS-Event, a signature
// that verifies with the secret, and a body matching the documented shape.
func TestWebhookDeliveryContract(t *testing.T) {
	for _, tc := range []struct {
		name     string
		event    string
		build    func() WebhookEvent
		bodyKeys []string
	}{
		{
			name:     "job.done",
			event:    EventWebhookJobDone,
			build:    sampleDone,
			bodyKeys: []string{"event", "job", "file", "library", "at"},
		},
		{
			name:  "job.failed",
			event: EventWebhookJobFailed,
			build: func() WebhookEvent {
				ev := sampleDone()
				ev.Event = EventWebhookJobFailed
				ev.Job.Status = store.JobStatusFailed
				ev.Job.Error = "encoder exited 1"
				return ev
			},
			bodyKeys: []string{"event", "job", "file", "library", "at"},
		},
		{
			name:  "library.scanned",
			event: EventWebhookLibScanned,
			build: func() WebhookEvent {
				lib := LibraryDTO{ID: "lib-1", Name: "Films"}
				scan := ScanResult{Seen: 412, Queued: 9}
				return WebhookEvent{Event: EventWebhookLibScanned, Library: &lib, Scan: &scan, At: time.Now().UTC()}
			},
			bodyKeys: []string{"event", "library", "scan", "at"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := newHarness(t)
			remote := newCountingRemote(t, http.StatusNoContent)
			secret := newWebhook(t, h, "hook", remote.srv.URL, tc.event)
			d := NewWebhookDispatcher(h.st, nil, "v1.2.3")
			shrinkBackoff(d)

			d.Notify(context.Background(), tc.build())
			waitForCount(t, remote, 1)

			_, path, _, body, header := remote.last()
			assert.Equal(t, "/", path)
			assert.Equal(t, "application/json", header.Get("Content-Type"))
			assert.Equal(t, "shrinkyourshit/v1.2.3", header.Get("User-Agent"))
			assert.Equal(t, tc.event, header.Get("X-SYS-Event"))

			// The signature verifies over the exact bytes received.
			mac := hmac.New(sha256.New, []byte(secret))
			mac.Write([]byte(body))
			assert.Equal(t, "sha256="+hex.EncodeToString(mac.Sum(nil)), header.Get("X-SYS-Signature"))

			// The body is the documented shape.
			var payload map[string]any
			require.NoError(t, json.Unmarshal([]byte(body), &payload))
			require.Len(t, payload, len(tc.bodyKeys), "body keys: %v", payload)
			for _, key := range tc.bodyKeys {
				require.Contains(t, payload, key)
			}
			assert.Equal(t, tc.event, payload["event"])

			// The delivery was recorded as a success.
			rows := deliveries(t, h, webhookRow(t, h).ID)
			require.Len(t, rows, 1)
			assert.Equal(t, http.StatusNoContent, rows[0].Status)
			assert.Empty(t, rows[0].Error)
			assert.Equal(t, tc.event, rows[0].Event)
		})
	}
}

// A 500 is retried 3 times with backoff and recorded in
// webhook_deliveries (acceptance item 2).
func TestWebhookRetriesAndRecordsFailure(t *testing.T) {
	h := newHarness(t)
	remote := newCountingRemote(t, http.StatusInternalServerError)
	newWebhook(t, h, "hook", remote.srv.URL, EventWebhookJobDone)
	d := NewWebhookDispatcher(h.st, nil, "v0.0.0")
	shrinkBackoff(d)

	d.Notify(context.Background(), sampleDone())
	waitForCount(t, remote, deliveryAttempts)
	assert.Equal(t, deliveryAttempts, remote.requests())

	// Let the delivery goroutine record the row.
	deadline := time.Now().Add(5 * time.Second)
	for {
		if len(deliveries(t, h, webhookRow(t, h).ID)) == 1 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("no delivery row was recorded")
		}
		time.Sleep(10 * time.Millisecond)
	}
	rows := deliveries(t, h, webhookRow(t, h).ID)
	require.Len(t, rows, 1, "one row holds the final outcome")
	assert.Equal(t, http.StatusInternalServerError, rows[0].Status)
	assert.Equal(t, EventWebhookJobDone, rows[0].Event)
}

// Delivery never blocks or fails a job, and a dead hook logs a warning
// only (acceptance item 3).
func TestWebhookDeadHookWarnsAndJobStillDone(t *testing.T) {
	h := newHarness(t)
	newWebhook(t, h, "dead", "http://127.0.0.1:1", EventWebhookJobDone)
	shrinkBackoff(h.svc.Webhooks())
	logs := &captureBuffer{}
	h.svc.Webhooks().log = slog.New(slog.NewTextHandler(logs, &slog.HandlerOptions{Level: slog.LevelDebug}))
	h.svc.Queue().SetInspector(func(context.Context, string, string) (ResultInfo, error) {
		return inspectedOutput, nil
	}, "")

	_, _, job := claimJob(t, h, "node-1")
	got, err := h.svc.Queue().Complete(context.Background(), job.ID, "node-1", CompleteParams{
		OutputPath: filepath.Join(h.root, "movie.mkv"), BytesAfter: 100,
	})
	require.NoError(t, err)
	require.Equal(t, store.JobStatusDone, got.Status)

	deadline := time.Now().Add(5 * time.Second)
	for {
		if len(deliveries(t, h, webhookRow(t, h).ID)) == 1 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("the dead-hook delivery was never recorded")
		}
		time.Sleep(10 * time.Millisecond)
	}
	rows := deliveries(t, h, webhookRow(t, h).ID)
	require.Len(t, rows, 1)
	assert.Equal(t, 0, rows[0].Status)
	assert.NotEmpty(t, rows[0].Error)

	deadline = time.Now().Add(5 * time.Second)
	for {
		if strings.Contains(logs.text(), "webhook delivery failed") {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("dead-hook warning missing; log:\n%s", logs.text())
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// Discord and Slack hosts receive the wrapped one-line summary; everything
// else receives the raw event (acceptance item 4). The chat URLs cannot
// answer a live request, so the body choice is proven on wireBody plus a
// live delivery to a plain hook.
func TestWebhookChatWrapping(t *testing.T) {
	// One event for the whole test: the raw bytes must be identical to
	// what the live delivery sends, timestamp included.
	ev := sampleDone()
	summary := webhookSummary(ev)
	raw := mustJSON(t, ev)

	for _, tc := range []struct {
		host    string
		wrapped bool
		key     string
	}{
		{"discord.com", true, "content"},
		{"discordapp.com", true, "content"},
		{"hooks.discord.com", true, "content"},
		{"hooks.slack.com", true, "text"},
		{"slack.com", true, "text"},
		{"example.com", false, ""},
		{"evil-discord.com", false, ""},
	} {
		t.Run(tc.host, func(t *testing.T) {
			body, wrapped := wireBody(Webhook{URL: "https://" + tc.host + "/hook"}, []byte(raw))
			if !tc.wrapped {
				require.False(t, wrapped)
				assert.Equal(t, raw, string(body), "a plain hook receives the raw event JSON")
				return
			}
			require.True(t, wrapped)
			var payload map[string]string
			require.NoError(t, json.Unmarshal(body, &payload))
			require.Len(t, payload, 1)
			assert.Equal(t, summary, payload[tc.key])
		})
	}

	// Live: a plain hook receives the raw event bytes.
	h := newHarness(t)
	remote := newCountingRemote(t, http.StatusNoContent)
	newWebhook(t, h, "plain", remote.srv.URL, EventWebhookJobDone)
	d := NewWebhookDispatcher(h.st, nil, "v0.0.0")
	shrinkBackoff(d)
	d.Notify(context.Background(), ev)
	waitForCount(t, remote, 1)
	_, _, _, body, _ := remote.last()
	assert.Equal(t, raw, body)
	// Let the delivery goroutine record its row before the harness closes
	// the store.
	deadline := time.Now().Add(5 * time.Second)
	for {
		if len(deliveries(t, h, webhookRow(t, h).ID)) == 1 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("the delivery was never recorded")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func mustJSON(t *testing.T, v any) string {
	t.Helper()
	raw, err := json.Marshal(v)
	require.NoError(t, err)
	return string(raw)
}

// TestWebhookSummary pins the one wording place (task 25).
func TestWebhookSummary(t *testing.T) {
	ev := sampleDone()
	assert.Equal(t, "job.done: The Matrix (1999).mkv 4.1 GB to 1.2 GB, 71% smaller", webhookSummary(ev))

	// A skip keeps the original: the numbers must not claim savings.
	file := FileDTO{Status: store.FileStatusDone}
	ev.File = &file
	assert.Equal(t, "job.done: The Matrix (1999).mkv kept the original (not smaller)", webhookSummary(ev))

	// A failed job names the file and the encoder's error.
	failed := sampleDone()
	failed.Event = EventWebhookJobFailed
	failed.Job.Status = store.JobStatusFailed
	failed.Job.Error = "encoder exited 1"
	assert.Equal(t, "job.failed: The Matrix (1999).mkv failed: encoder exited 1", webhookSummary(failed))

	// A scanned library names the library and the counts.
	lib := LibraryDTO{Name: "Films"}
	scanned := WebhookEvent{
		Event:   EventWebhookLibScanned,
		Library: &lib,
		Scan:    &ScanResult{Seen: 412, Queued: 9},
	}
	assert.Equal(t, "library.scanned: Films: 412 files, 9 new", webhookSummary(scanned))

	// Missing pieces degrade to honest placeholders, never panic.
	assert.Equal(t, "job.failed: a file failed", webhookSummary(WebhookEvent{Event: EventWebhookJobFailed}))
	assert.Equal(t, "library.scanned: a library", webhookSummary(WebhookEvent{Event: EventWebhookLibScanned}))
}

// TestQueueEmitsWebhookEvents proves the queue path end to end: a done
// completion delivers job.done, a failed job delivers job.failed.
func TestQueueEmitsWebhookEvents(t *testing.T) {
	h := newHarness(t)
	remote := newCountingRemote(t, http.StatusNoContent)
	newWebhook(t, h, "hook", remote.srv.URL, EventWebhookJobDone, EventWebhookJobFailed)
	shrinkBackoff(h.svc.Webhooks())
	h.svc.Queue().SetInspector(func(context.Context, string, string) (ResultInfo, error) {
		return inspectedOutput, nil
	}, "")

	_, _, job := claimJob(t, h, "node-1")
	_, err := h.svc.Queue().Complete(context.Background(), job.ID, "node-1", CompleteParams{
		OutputPath: filepath.Join(h.root, "movie.mkv"), BytesAfter: 100,
	})
	require.NoError(t, err)
	waitForCount(t, remote, 1)
	_, _, _, body, header := remote.last()
	assert.Equal(t, EventWebhookJobDone, header.Get("X-SYS-Event"))
	assert.Contains(t, body, `"event":"job.done"`)

	_, _, failJob := claimJob(t, h, "node-1")
	_, err = h.svc.Queue().Fail(context.Background(), failJob.ID, "node-1", FailParams{Error: "encoder exited 1"})
	require.NoError(t, err)
	waitForCount(t, remote, 2)
	_, _, _, body, header = remote.last()
	assert.Equal(t, EventWebhookJobFailed, header.Get("X-SYS-Event"))
	assert.Contains(t, body, `"event":"job.failed"`)
	assert.Contains(t, body, "encoder exited 1")

	// Both deliveries recorded, both success.
	rows := deliveries(t, h, webhookRow(t, h).ID)
	require.Len(t, rows, 2)
	assert.Equal(t, http.StatusNoContent, rows[0].Status)
	assert.Equal(t, http.StatusNoContent, rows[1].Status)
}

// TestScannerEmitsLibraryScanned proves the scanner's finish path fires
// the third event, carrying the library and the scan counts.
func TestScannerEmitsLibraryScanned(t *testing.T) {
	h := newHarness(t)
	remote := newCountingRemote(t, http.StatusNoContent)
	newWebhook(t, h, "hook", remote.srv.URL, EventWebhookLibScanned)
	shrinkBackoff(h.svc.Webhooks())

	lib := h.createLibrary(false)
	require.NoError(t, os.WriteFile(filepath.Join(h.root, "movie.mkv"), make([]byte, 2000), 0o600))
	h.scan(lib.ID)

	waitForCount(t, remote, 1)
	_, _, _, body, header := remote.last()
	assert.Equal(t, EventWebhookLibScanned, header.Get("X-SYS-Event"))
	var payload WebhookEvent
	require.NoError(t, json.Unmarshal([]byte(body), &payload))
	require.NotNil(t, payload.Scan)
	require.NotNil(t, payload.Library)
	assert.Equal(t, "Movies", payload.Library.Name)
	assert.Equal(t, 1, payload.Scan.Seen)
}

// TestWebhookSubscriptions proves a hook only hears the events it asked
// for, and a disabled hook hears nothing.
func TestWebhookSubscriptions(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	remote := newCountingRemote(t, http.StatusNoContent)

	onlyDone := validWebhookRow("only-done", remote.srv.URL, EventWebhookJobDone)
	require.NoError(t, h.st.CreateWebhook(ctx, onlyDone))
	off := validWebhookRow("off", remote.srv.URL, EventWebhookJobDone, EventWebhookJobFailed)
	off.Enabled = false
	require.NoError(t, h.st.CreateWebhook(ctx, off))
	d := NewWebhookDispatcher(h.st, nil, "v0.0.0")
	shrinkBackoff(d)

	d.Notify(ctx, sampleDone())
	waitForCount(t, remote, 1)

	// job.failed reaches nobody: onlyDone is not subscribed, off is off.
	d.Notify(ctx, func() WebhookEvent {
		ev := sampleDone()
		ev.Event = EventWebhookJobFailed
		return ev
	}())
	time.Sleep(100 * time.Millisecond)
	assert.Equal(t, 1, remote.requests())
}

func validWebhookRow(name, url string, events ...string) *store.Webhook {
	return &store.Webhook{
		Name: name, URL: url, Secret: "0123456789abcdef0123456789abcdef",
		Events: strings.Join(events, ","), Enabled: true,
	}
}

// TestDispatcherTestSingleAttempt covers the Test button's one-shot call:
// one attempt (no retries), recorded, answered with the remote's outcome.
func TestDispatcherTestSingleAttempt(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()

	failing := newCountingRemote(t, http.StatusInternalServerError)
	row := validWebhookRow("hook", failing.srv.URL, EventWebhookJobDone)
	require.NoError(t, h.st.CreateWebhook(ctx, row))
	d := NewWebhookDispatcher(h.st, nil, "v0.0.0")
	shrinkBackoff(d)

	status, err := d.Test(ctx, WebhookFromRow(row), sampleDone())
	require.Error(t, err)
	assert.Equal(t, http.StatusInternalServerError, status)
	assert.Equal(t, 1, failing.requests(), "the test call never retries")

	rows := deliveries(t, h, row.ID)
	require.Len(t, rows, 1)
	assert.Equal(t, http.StatusInternalServerError, rows[0].Status)

	// A healthy remote answers 2xx with no error.
	okay := newCountingRemote(t, http.StatusNoContent)
	row2 := validWebhookRow("hook2", okay.srv.URL, EventWebhookJobDone)
	require.NoError(t, h.st.CreateWebhook(ctx, row2))
	status, err = d.Test(ctx, WebhookFromRow(row2), sampleDone())
	require.NoError(t, err)
	assert.Equal(t, http.StatusNoContent, status)
	waitForCount(t, okay, 1)
}

// The refresher must stay quiet for events that are not a done completion
// with real savings (the task 23 behavior, now expressed as a filter).
func TestRefresherIgnoresOtherEvents(t *testing.T) {
	h := newHarness(t)
	remote := newCountingRemote(t, http.StatusNoContent)
	require.NoError(t, h.st.CreateIntegration(context.Background(), testIntegration(t, store.IntegrationKindJellyfin, remote.srv.URL)))
	r := h.svc.Integrations()
	ctx := context.Background()

	failed := sampleDone()
	failed.Event = EventWebhookJobFailed
	r.Notify(ctx, failed)
	skipped := sampleDone()
	skippedFile := FileDTO{Status: store.FileStatusDone}
	skipped.File = &skippedFile
	r.Notify(ctx, skipped)
	r.Notify(ctx, WebhookEvent{Event: EventWebhookLibScanned})
	time.Sleep(100 * time.Millisecond)
	assert.Zero(t, remote.requests())

	// Keep-both (satisfied but zero savings) stays quiet too.
	keepBoth := sampleDone()
	kb := FileDTO{Status: store.FileStatusSatisfied, BytesSaved: 0}
	keepBoth.File = &kb
	r.Notify(ctx, keepBoth)
	time.Sleep(100 * time.Millisecond)
	assert.Zero(t, remote.requests())

	// The real thing still notifies.
	r.Notify(ctx, doneWithSavings())
	waitForCount(t, remote, 1)
}
