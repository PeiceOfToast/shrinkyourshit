// SPDX-License-Identifier: AGPL-3.0-only

package server

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"math"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"time"

	"github.com/PeiceOfToast/shrinkyourshit/internal/plugin"
	"github.com/PeiceOfToast/shrinkyourshit/internal/store"
)

// Webhook event names (task 25; documented in API.md — do not add values
// without updating it). They mirror the store constants; the server layer
// owns the wire contract.
const (
	EventWebhookJobDone    = store.WebhookEventJobDone
	EventWebhookJobFailed  = store.WebhookEventJobFailed
	EventWebhookLibScanned = store.WebhookEventLibScanned
)

// WebhookEventValues lists the events a webhook can subscribe to.
func WebhookEventValues() []string {
	return store.WebhookEventValues()
}

// WebhookEvent is the payload of every webhook delivery (task 25). Its
// JSON shape is a public contract: every field carries an explicit tag,
// and the nested values are the DTOs the rest of the API already publishes
// (JobDTO / FileDTO / LibraryDTO), so consumers read one documented shape
// instead of raw database rows. job and file are set on the job events,
// scan only on library.scanned, library whenever it is known.
type WebhookEvent struct {
	Event   string      `json:"event"`
	Job     *JobDTO     `json:"job,omitempty"`
	File    *FileDTO    `json:"file,omitempty"`
	Library *LibraryDTO `json:"library,omitempty"`
	Scan    *ScanResult `json:"scan,omitempty"`
	At      time.Time   `json:"at"`
}

// Webhook is the service shape of one configured hook. The secret exists
// only to sign outbound deliveries; it never comes back out through the
// API after the create response.
type Webhook struct {
	ID      string
	Name    string
	URL     string
	Secret  string
	Events  []string
	Enabled bool
}

// WebhookFromRow maps a stored row to the service shape.
func WebhookFromRow(row *store.Webhook) Webhook {
	return Webhook{
		ID:      row.ID,
		Name:    row.Name,
		URL:     row.URL,
		Secret:  row.Secret,
		Events:  row.EventList(),
		Enabled: row.Enabled,
	}
}

// Row maps back to the stored shape for persistence.
func (w Webhook) Row() *store.Webhook {
	return &store.Webhook{
		ID:      w.ID,
		Name:    w.Name,
		URL:     w.URL,
		Secret:  w.Secret,
		Events:  strings.Join(w.Events, ","),
		Enabled: w.Enabled,
	}
}

// subscribedTo reports whether the hook listens for event.
func (w Webhook) subscribedTo(event string) bool {
	for _, name := range w.Events {
		if name == event {
			return true
		}
	}
	return false
}

// ValidateWebhook checks one webhook and normalizes it in place: name and
// URL are trimmed and the event list is rebuilt from its trimmed,
// de-duplicated names. It rejects an unknown event name, a URL that does
// not parse or is not http/https (or has no host), and an empty name or
// event list. The pointer is deliberate: the normalization must be visible
// to the caller so the trimmed values are what get stored.
func ValidateWebhook(w *Webhook) error {
	w.Name = strings.TrimSpace(w.Name)
	w.URL = strings.TrimSpace(w.URL)

	if w.Name == "" {
		return fmt.Errorf("webhook: name is required")
	}
	u, err := url.Parse(w.URL)
	if err != nil {
		return fmt.Errorf("webhook: url does not parse: %v", err)
	}
	if (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return fmt.Errorf("webhook: url must be an http or https URL with a host")
	}
	seen := map[string]bool{}
	var events []string
	for _, name := range w.Events {
		name = strings.TrimSpace(name)
		if name == "" || seen[name] {
			continue
		}
		known := false
		for _, ok := range WebhookEventValues() {
			if name == ok {
				known = true
				break
			}
		}
		if !known {
			return fmt.Errorf("webhook: unknown event %q: want one of %v", name, WebhookEventValues())
		}
		seen[name] = true
		events = append(events, name)
	}
	if len(events) == 0 {
		return fmt.Errorf("webhook: at least one event is required: want one of %v", WebhookEventValues())
	}
	w.Events = events
	return nil
}

// NewWebhookSecret generates the signing secret of a new hook: 32 random
// bytes, hex encoded. It is returned once on create and never again.
func NewWebhookSecret() (string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("webhook: generate secret: %w", err)
	}
	return hex.EncodeToString(raw), nil
}

// deliveryTimeout bounds one outbound webhook attempt (task 25: 5 seconds).
const deliveryTimeout = 5 * time.Second

// deliveryAttempts is how many times one event is tried before the
// dispatcher gives up and records the outcome (task 25: 3 attempts).
const deliveryAttempts = 3

// webhookUserAgent is the User-Agent of every delivery.
func webhookUserAgent(version string) string {
	if version == "" {
		version = "v0.0.0"
	}
	return "shrinkyourshit/" + version
}

// SignBody is the X-SYS-Signature value for one delivery: the hex
// HMAC-SHA256 of the raw body under the hook's secret.
func SignBody(secret string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

// Dispatcher delivers webhook events to the configured hooks (task 25).
// Delivery is fire and forget: Notify lists the subscribers and hands each
// one to its own goroutine, so a slow or dead hook can never slow a
// completion, a failure report or a scan. A final failure is a warn line
// plus a recorded delivery row — nothing here can fail a job.
type Dispatcher struct {
	st      store.Store
	log     *slog.Logger
	client  *http.Client
	version string
	// backoff is the wait before attempt 2 and attempt 3. The default is
	// 1s then 4s; tests shrink it without a clock seam.
	backoff []time.Duration
}

// NewWebhookDispatcher builds the dispatcher over a store. version feeds
// the User-Agent.
func NewWebhookDispatcher(st store.Store, log *slog.Logger, version string) *Dispatcher {
	if log == nil {
		log = slog.Default()
	}
	return &Dispatcher{
		st:      st,
		log:     log,
		client:  &http.Client{},
		version: version,
		backoff: []time.Duration{time.Second, 4 * time.Second},
	}
}

// notifierRouter fans one lifecycle event out to every notification
// channel (task 23 integrations, task 25 webhooks, task 31 plugin
// phases). The per-channel filters live in the receivers; the router is a
// plain fan-out so the Queue keeps exactly one hook (the task 23 Notifier
// shape, reused).
type notifierRouter struct {
	integrations *Refresher
	webhooks     *Dispatcher
	plugins      *PluginService
}

func (r *notifierRouter) Notify(ctx context.Context, ev WebhookEvent) {
	if r.integrations != nil {
		r.integrations.Notify(ctx, ev)
	}
	if r.webhooks != nil {
		r.webhooks.Notify(ctx, ev)
	}
}

// PostJob hands a finished job to the plugin phases (task 31): the
// postprocess and notify hooks. The job context carries what the event
// shape does not — the original source path and the applied output path —
// so it is built by the queue and passed through here. The plugins
// service runs the phases detached (bounded by each plugin's own
// timeout), so a slow plugin can never slow a completion.
func (r *notifierRouter) PostJob(ctx context.Context, pc plugin.JobContext) {
	if r.plugins != nil {
		r.plugins.PostJob(ctx, pc)
	}
}

// Notify hands one lifecycle event to every enabled hook subscribed to it.
// It returns as soon as the rows are listed: every delivery runs in its
// own goroutine (acceptance: delivery never blocks a job, and a dead hook
// only logs a warning).
func (d *Dispatcher) Notify(ctx context.Context, ev WebhookEvent) {
	rows, err := d.st.ListWebhooks(ctx)
	if err != nil {
		d.log.Error("webhooks: list", "err", err)
		return
	}
	raw, err := json.Marshal(ev)
	if err != nil {
		d.log.Error("webhooks: marshal event", "event", ev.Event, "err", err)
		return
	}
	for _, row := range rows {
		if !row.Enabled || !WebhookFromRow(row).subscribedTo(ev.Event) {
			continue
		}
		go d.deliver(row, ev.Event, raw)
	}
}

// outcome is one delivery's final recorded result.
type outcome struct {
	status int
	error  string
}

// deliver performs one hook's delivery with retries and records the final
// outcome — one row per event, holding the last attempt's result. The
// body is the raw event JSON, except for Discord and Slack incoming
// webhooks, which only render the one-line summary; the signature covers
// whichever bytes are actually sent.
func (d *Dispatcher) deliver(row *store.Webhook, event string, rawEvent []byte) {
	w := WebhookFromRow(row)

	var last outcome
	for attempt := 0; attempt < deliveryAttempts; attempt++ {
		if attempt > 0 && attempt-1 < len(d.backoff) {
			// A background goroutine owns its own time; a plain sleep is
			// the honest backoff (1s, then 4s, task 25).
			time.Sleep(d.backoff[attempt-1])
		}
		last = d.send(w, event, rawEvent)
		if last.ok() {
			break
		}
	}

	d.record(w, event, last)
	switch {
	case last.ok():
		d.log.Debug("webhook delivered", "webhook", w.Name, "event", event, "status", last.status)
	case last.status > 0:
		d.log.Warn("webhook delivery failed", "webhook", w.Name, "event", event, "status", last.status)
	default:
		d.log.Warn("webhook delivery failed", "webhook", w.Name, "event", event, "err", last.error)
	}
}

// ok reports a delivered outcome: any 2xx.
func (o outcome) ok() bool { return o.status >= 200 && o.status < 300 }

// send performs one attempt with its own 5 second timeout. A non-2xx
// records the status and the first 200 characters of the body as the
// error; a transport failure records status 0 and an error text that
// carries no URL (a webhook URL can embed a token in its query string).
func (d *Dispatcher) send(w Webhook, event string, rawEvent []byte) outcome {
	ctx, cancel := context.WithTimeout(context.Background(), deliveryTimeout)
	defer cancel()

	body, _ := wireBody(w, rawEvent)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, w.URL, bytes.NewReader(body))
	if err != nil {
		return outcome{error: "could not build the request"}
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", webhookUserAgent(d.version))
	req.Header.Set("X-SYS-Event", event)
	req.Header.Set("X-SYS-Signature", SignBody(w.Secret, body))

	resp, err := d.client.Do(req)
	if err != nil {
		// *url.Error embeds the full URL — which can carry a token — so
		// only the underlying cause survives into the error text.
		if ue, ok := err.(*url.Error); ok {
			err = ue.Err
		}
		return outcome{error: "delivery failed: " + err.Error()}
	}
	defer func() { _ = resp.Body.Close() }()
	head, _ := io.ReadAll(io.LimitReader(resp.Body, 200))
	_, _ = io.Copy(io.Discard, resp.Body)
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return outcome{status: resp.StatusCode}
	}
	return outcome{status: resp.StatusCode, error: string(head)}
}

// record stores the delivery outcome and keeps the history bounded
// (newest 50 rows per hook, task 25).
func (d *Dispatcher) record(w Webhook, event string, last outcome) {
	delivery := &store.WebhookDelivery{
		WebhookID: w.ID,
		Event:     event,
		Status:    last.status,
		Error:     last.error,
	}
	if err := d.st.AddWebhookDelivery(context.Background(), delivery); err != nil {
		d.log.Error("webhooks: record delivery", "webhook", w.Name, "err", err)
	}
	if _, err := d.st.PruneWebhookDeliveries(context.Background(), store.WebhookDeliveriesKeep); err != nil {
		d.log.Error("webhooks: prune deliveries", "webhook", w.Name, "err", err)
	}
}

// Test performs one delivery of ev to w — the admin's Test button. It
// skips the retries on purpose (an explicit diagnostic should answer
// quickly), still records the outcome, and is answered with the remote's
// status and error so the UI can show what happened.
func (d *Dispatcher) Test(ctx context.Context, w Webhook, ev WebhookEvent) (int, error) {
	ctx, cancel := context.WithTimeout(ctx, deliveryTimeout)
	defer cancel()

	raw, err := json.Marshal(ev)
	if err != nil {
		return 0, fmt.Errorf("webhook %q: marshal event: %w", w.Name, err)
	}
	body, _ := wireBody(w, raw)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, w.URL, bytes.NewReader(body))
	if err != nil {
		return 0, fmt.Errorf("webhook %q: build request", w.Name)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", webhookUserAgent(d.version))
	req.Header.Set("X-SYS-Event", ev.Event)
	req.Header.Set("X-SYS-Signature", SignBody(w.Secret, body))

	resp, err := d.client.Do(req)
	if err != nil {
		if ue, ok := err.(*url.Error); ok {
			err = ue.Err
		}
		d.record(w, ev.Event, outcome{error: "delivery failed: " + err.Error()})
		return 0, fmt.Errorf("webhook %q: request failed: %v", w.Name, err)
	}
	defer func() { _ = resp.Body.Close() }()
	head, _ := io.ReadAll(io.LimitReader(resp.Body, 200))
	_, _ = io.Copy(io.Discard, resp.Body)
	last := outcome{status: resp.StatusCode}
	if resp.StatusCode >= 300 {
		last.error = string(head)
	}
	d.record(w, ev.Event, last)
	if resp.StatusCode >= 300 {
		return resp.StatusCode, fmt.Errorf("webhook %q: unexpected status", w.Name)
	}
	return resp.StatusCode, nil
}

// wireBody picks the wire body for one hook: Discord and Slack incoming
// webhooks only render the one-line summary, everything else receives the
// raw event JSON. The boolean reports that the body was wrapped.
func wireBody(w Webhook, rawEvent []byte) (body []byte, wrapped bool) {
	var ev WebhookEvent
	if err := json.Unmarshal(rawEvent, &ev); err != nil {
		return rawEvent, false // undecorable: send the raw event
	}
	summary := webhookSummary(ev)
	switch chatShapeOf(w.URL) {
	case shapeDiscord:
		if out, err := json.Marshal(map[string]string{"content": summary}); err == nil {
			return out, true
		}
	case shapeSlack:
		if out, err := json.Marshal(map[string]string{"text": summary}); err == nil {
			return out, true
		}
	}
	return rawEvent, false
}

// chat shape of a webhook URL's host (task 25). The host must be the
// platform itself or a subdomain of it — a bare suffix match alone would
// let "evil-discord.com" pass.
type chatShape int

const (
	shapeOther chatShape = iota
	shapeDiscord
	shapeSlack
)

func chatShapeOf(raw string) chatShape {
	u, err := url.Parse(raw)
	if err != nil {
		return shapeOther
	}
	host := strings.ToLower(u.Hostname())
	for _, base := range []string{"discord.com", "discordapp.com"} {
		if host == base || strings.HasSuffix(host, "."+base) {
			return shapeDiscord
		}
	}
	if host == "slack.com" || strings.HasSuffix(host, ".slack.com") {
		return shapeSlack
	}
	return shapeOther
}

// webhookSummary builds the one-line human summary of an event (task 25).
// It is the single place the wording lives. Examples:
//
//	job.done: The Matrix (1999) 4.1 GB to 1.2 GB, 71% smaller
//	job.failed: Heat failed: encoder exited 1
//	library.scanned: Films: 412 files, 9 new
func webhookSummary(ev WebhookEvent) string {
	switch ev.Event {
	case EventWebhookJobDone:
		name := eventName(ev)
		if ev.File != nil && ev.File.Status == store.FileStatusDone {
			// The result policy kept the original: the output was not
			// applied (a skip), so the numbers would overstate it.
			return fmt.Sprintf("job.done: %s kept the original (not smaller)", name)
		}
		if ev.Job != nil && ev.Job.BytesBefore > 0 && ev.Job.BytesAfter > 0 {
			pct := math.Round(100 - float64(ev.Job.BytesAfter)/float64(ev.Job.BytesBefore)*100)
			before, after := humanBytes(ev.Job.BytesBefore), humanBytes(ev.Job.BytesAfter)
			if pct > 0 {
				return fmt.Sprintf("job.done: %s %s to %s, %.0f%% smaller", name, before, after, pct)
			}
			return fmt.Sprintf("job.done: %s %s to %s (no space saved)", name, before, after)
		}
		return fmt.Sprintf("job.done: %s finished", name)
	case EventWebhookJobFailed:
		err := ""
		if ev.Job != nil {
			err = strings.TrimSpace(ev.Job.Error)
		}
		if err == "" {
			return fmt.Sprintf("job.failed: %s failed", eventName(ev))
		}
		return fmt.Sprintf("job.failed: %s failed: %s", eventName(ev), err)
	case EventWebhookLibScanned:
		lib := "a library"
		if ev.Library != nil && ev.Library.Name != "" {
			lib = ev.Library.Name
		}
		if ev.Scan != nil {
			return fmt.Sprintf("library.scanned: %s: %d files, %d new", lib, ev.Scan.Seen, ev.Scan.Queued)
		}
		return fmt.Sprintf("library.scanned: %s", lib)
	}
	return ev.Event
}

// eventName picks the human file name of a job event: the resolved base
// name from the job DTO, else the file row's base name, else "a file".
func eventName(ev WebhookEvent) string {
	if ev.Job != nil && ev.Job.FileName != "" {
		return ev.Job.FileName
	}
	if ev.File != nil && ev.File.Path != "" {
		return filepath.Base(ev.File.Path)
	}
	return "a file"
}

// humanBytes renders a size the way the summary examples spell it: decimal
// units with one decimal place (4.1 GB).
func humanBytes(n int64) string {
	const kb, mb, gb = 1e3, 1e6, 1e9
	f := float64(n)
	switch {
	case n >= gb:
		return fmt.Sprintf("%.1f GB", f/gb)
	case n >= mb:
		return fmt.Sprintf("%.1f MB", f/mb)
	case n >= kb:
		return fmt.Sprintf("%.1f KB", f/kb)
	default:
		return strconvI64(n) + " B"
	}
}

func strconvI64(n int64) string {
	return fmt.Sprintf("%d", n)
}

// SampleEvent is the fixed sample payload the Test button sends (task 25):
// a synthetic job.done with example numbers.
func SampleEvent() WebhookEvent {
	return WebhookEvent{
		Event: EventWebhookJobDone,
		Job: &JobDTO{
			ID:          "sample-job",
			Status:      store.JobStatusDone,
			FileName:    "The Matrix (1999).mkv",
			FilePath:    "/media/movies/The Matrix (1999).mkv",
			BytesBefore: 4_100_000_000,
			BytesAfter:  1_200_000_000,
			EncoderUsed: "sample",
			Percent:     100,
		},
		File: &FileDTO{
			ID:      "sample-file",
			Path:    "/media/movies/The Matrix (1999).mkv",
			Size:    1_200_000_000,
			Status:  store.FileStatusSatisfied,
			LibraryID: "sample-library",
		},
		Library: &LibraryDTO{
			ID:   "sample-library",
			Name: "Sample Library",
		},
		At: time.Now().UTC(),
	}
}
