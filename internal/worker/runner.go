// SPDX-License-Identifier: AGPL-3.0-only

package worker

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"github.com/PeiceOfToast/shrinkyourshit/internal/encoders"
	"github.com/PeiceOfToast/shrinkyourshit/internal/flow"
	"github.com/PeiceOfToast/shrinkyourshit/internal/plugin"
	"github.com/PeiceOfToast/shrinkyourshit/internal/probe"
	"github.com/PeiceOfToast/shrinkyourshit/internal/server"
	"github.com/PeiceOfToast/shrinkyourshit/internal/store"
)

// Prober probes one file. It is a function type so tests can replace
// ffprobe; production uses probe.Probe (same shape as server.Prober).
type Prober func(ctx context.Context, ffprobePath, path string) (probe.MediaInfo, error)

// runner processes one claimed job end to end (task 05): copy the source
// into scratch, encode with GPU-to-CPU fallback while streaming progress,
// run subtitle extracts, verify the output, apply the result policy to the
// library, and report complete or fail. The original is only touched after
// the output verified. Transfer mode (task 09) swaps the copy for a
// resumable download and the policy apply for a checksum-verified upload;
// the server then does the verify-and-replace. Since task 32 the job's
// flow is executed as a step walk: plugin steps run at their chain
// position on the working copy, and every guard after one evaluates
// against the re-probed result.
type runner struct {
	cfg     Config
	client  *Client
	caps    encoders.Capability
	encs    []encoders.Encoder // test override; nil → Registry(target, caps)
	prober  Prober
	runFn   encoders.RunFn   // test override; nil → ffmpeg subprocess
	plugs   *plugin.Runner   // step plugins (task 32); nil → none
	sc      *scratch
	access  *accessCache
	log     *slog.Logger
	tracker *encoderTracker
}

// run executes one job. It reports the outcome itself; a returned error
// means even the report could not be delivered (logged by the slot loop).
// cancelled flips when the server says the job was cancelled or the worker
// is shutting down: the encode is killed, scratch is cleaned, and nothing
// is reported (the server already moved the job, or the sweeper will).
func (r *runner) run(ctx context.Context, spec *server.JobSpec, cancelled *atomic.Bool) error {
	defer r.sc.remove(spec.ID)

	tail := newTailBuffer(server.MaxLogTail)
	// Task 27: the structured job log streams to the server in batches
	// while the job runs; the tail stays for the failure message.
	sink := newJobLogSink(r.client, spec.ID, r.log)
	defer sink.finish()
	fail := func(format string, args ...any) error {
		msg := fmt.Sprintf(format, args...)
		r.log.Warn("job failed", "job", spec.ID, "error", msg)
		sink.errorf("%s", msg)
		sink.finish()
		if err := r.client.Fail(ctx, spec.ID, msg, tail.Tail()); err != nil && !isAPIStatus(err, 409) {
			r.log.Error("worker: fail report", "job", spec.ID, "err", err)
		}
		return nil
	}

	dir, err := r.sc.ensure(spec.ID)
	if err != nil {
		return fail("prepare scratch: %v", err)
	}

	// How does this node reach the source? Configured modes pass through;
	// auto runs the shared-storage check (cached per library, task 09).
	mode, err := resolveAccessMode(ctx, r.client, spec, r.access, r.log)
	if err != nil {
		return fail("resolve access mode: %v", err)
	}

	// Bring the source into scratch, keeping its mtime so the fingerprint
	// matches the original row.
	srcExt := filepath.Ext(spec.SourcePath)
	local := filepath.Join(dir, "source"+srcExt)
	if mode == store.NodeAccessTransfer {
		local, err = downloadSource(ctx, r.client, spec, dir, spec.ThroughputLimitMBs, r.log)
		if err != nil {
			if cancelled.Load() || ctx.Err() != nil {
				return nil
			}
			return fail("download source: %v", err)
		}
	} else {
		if err := copyWithContext(ctx, spec.SourcePath, local); err != nil {
			if cancelled.Load() || ctx.Err() != nil {
				return nil
			}
			return fail("copy source: %v", err)
		}
		mtime := time.Unix(spec.SourceMtime, 0)
		_ = os.Chtimes(local, mtime, mtime) // best effort; the check below is authoritative
		if fp, err := probe.Fingerprint(local); err != nil {
			return fail("fingerprint scratch copy: %v", err)
		} else if fp != spec.Fingerprint {
			return fail("source changed since the job was queued; the next scan will re-evaluate it")
		}
	}

	srcInfo, err := r.prober(ctx, r.cfg.FFprobePath, local)
	if err != nil {
		return fail("probe source: %v", err)
	}
	sink.sysf("probed source: %s container, %s, %d video, %d audio, %d subtitle stream(s)",
		srcInfo.Container, server.HumanBytes(srcInfo.SizeBytes),
		len(srcInfo.Video), len(srcInfo.Audio), len(srcInfo.Subtitle))

	// The flow is the rule (task 14): compile the graph the spec carries
	// and walk it step by step. The resolved Target drives the encode; the
	// resolved policy and rename override the spec's library-level values.
	// A spec without a flow (pre-task-14 server) falls back to the spec's
	// Target.
	//
	// The walk (task 32) evaluates each step's conditions against the
	// *current* probe of the working file. A plugin step runs there and
	// then, on the working copy inside this job's scratch directory, and
	// the file is re-probed before the next step — so a guard after a
	// plugin (if_codec_not, if_bitrate_above, skip_if_smaller) sees what
	// the plugin actually produced. Plugin steps run before the encode:
	// the v1 encode is one combined command built from the resolved
	// Target, so a plugin that must act on the finished output stays a
	// postprocess/notify plugin in v1.
	target := spec.Target
	policy := spec.ResultPolicy
	rename := ""
	var hc *flow.HealthCheckSpec
	working := local // the working file the chain operates on
	cur := srcInfo   // the current probe of the working file
	if spec.Flow != nil {
		plan, err := flow.Compile(*spec.Flow)
		if err != nil {
			return fail("invalid flow: %v", err)
		}
		eff := flow.FreshEffective()
		for i, step := range plan.Steps {
			ran, err := flow.ApplyStep(&eff, step, cur)
			if err != nil {
				return fail("resolve flow: %v", err)
			}
			if !ran || step.Type != flow.StepPlugin {
				continue
			}
			if cancelled.Load() || ctx.Err() != nil {
				return nil
			}
			next, err := r.execPluginStep(ctx, spec, step, i, working, dir, sink)
			if err != nil {
				if cancelled.Load() || ctx.Err() != nil {
					return nil
				}
				return fail("%v", err)
			}
			// Re-probe the working file so the next step — guards and the
			// encode alike — sees what the plugin produced. A plugin that
			// leaves an unreadable file fails the step with the probe
			// error.
			info, perr := r.prober(ctx, r.cfg.FFprobePath, next)
			if perr != nil {
				return fail("plugin %s: probe the working file after the step: %v",
					pluginNameOf(step), perr)
			}
			working = next
			cur = info
		}
		target = eff.Target
		if eff.SkipIfSmaller {
			policy = store.ResultPolicyReplaceIfSmaller
		}
		rename = eff.Rename
		hc = eff.HealthCheck
	}
	sink.sysf("target: %s container, %s video, %s audio", target.Container,
		target.Video.Codec, target.Audio.Mode)
	sink.sysf("result policy: %s", policy)

	// Encode with fallback while a ticker reports -progress every interval.
	// The encode consumes the working file as the chain left it (a plugin
	// step may have replaced it, task 32) with the re-probed view.
	output := filepath.Join(dir, "output."+containerExt(target.Container))
	sp := encoders.NewSpec(working, output, cur, target)
	state := newProgressState()

	runCtx, cancelRun := context.WithCancel(ctx)
	defer cancelRun()
	reportDone := make(chan struct{})
	stopReport := make(chan struct{})
	go func() {
		defer close(reportDone)
		r.reportLoop(stopReport, runCtx, spec, state, cur.DurationSeconds, cancelled, cancelRun, mode, output)
	}()

	res, encErr := encoders.RunWithFallback(runCtx, r.registryFor(target), sp,
		func(ctx context.Context, args []string) error {
			return r.execEncode(ctx, args, state, tail, sink)
		})
	close(stopReport)
	<-reportDone

	if cancelled.Load() || ctx.Err() != nil {
		return nil
	}
	if encErr != nil {
		return fail("encode failed: %v", encErr)
	}
	sink.sysf("encoder chosen: %s", res.EncoderUsed)
	for _, fb := range res.Fallbacks {
		sink.warnf("encoder %s failed, falling back: %s", fb.Encoder, fb.Error)
	}

	// Removed text subtitles become .srt files beside the (scratch) output;
	// they move into the library only when the result is applied.
	extractCmds := encoders.ExtractCommands(sp)
	if mode == store.NodeAccessTransfer && len(extractCmds) > 0 {
		// Transfer mode has no extract channel yet (task 09): the removed
		// tracks stay in the source; the engine still strips them from the
		// uploaded output. Logged so the behavior is visible.
		r.log.Warn("worker: subtitle extracts are not uploaded in transfer mode; skipping extract commands",
			"job", spec.ID, "extracts", len(extractCmds))
		extractCmds = nil
	}
	for _, ec := range extractCmds {
		if err := r.execExtract(runCtx, ec.Args, tail, sink); err != nil {
			if cancelled.Load() || ctx.Err() != nil {
				return nil
			}
			return fail("extract subtitles: %v", err)
		}
	}

	outInfo, err := r.prober(ctx, r.cfg.FFprobePath, output)
	if err != nil {
		return fail("probe output: %v", err)
	}
	st, err := os.Stat(output)
	if err != nil {
		return fail("stat output: %v", err)
	}
	if err := encoders.VerifyOutput(sp, cur.DurationSeconds, outInfo, st.Size()); err != nil {
		return fail("output verification failed: %v", err)
	}

	// The health_check step (task 24): verify the produced file before
	// anything is reported. A failed check must not apply the result
	// policy — the source stays untouched and the job ends failed, exactly
	// like an encode failure.
	if hc != nil {
		if err := r.runHealthCheck(runCtx, hc, output, cur, outInfo, tail, sink); err != nil {
			if cancelled.Load() || ctx.Err() != nil {
				return nil
			}
			return fail("health check failed: %v", err)
		}
		sink.sysf("health check: passed")
	}

	// Transfer mode (task 09): the output is uploaded and the server does
	// the verify-and-replace with the library's result policy.
	if mode == store.NodeAccessTransfer {
		return r.completeTransfer(ctx, spec, output, st.Size(), res, tail, sink, cancelled)
	}

	applied, err := server.ApplyResultPolicy(ctx, policy, output, spec.SourcePath,
		st.Size(), spec.BytesBefore, r.sc.extractFiles(dir, "output"), rename, server.MoveDirective{
			Destination: spec.MoveTo,
			KeepTree:    spec.MoveKeepTree,
			RelDir:      spec.MoveRelDir,
		})
	if err != nil {
		if cancelled.Load() || ctx.Err() != nil {
			return nil
		}
		return fail("apply result policy: %v", err)
	}

	in := CompleteInput{
		BytesAfter:      st.Size(),
		Encoder:         res.EncoderUsed,
		FallbackReasons: fallbackStrings(res),
		LogTail:         tail.Tail(),
		AccessMode:      mode,
	}
	if applied.SkipReason != "" {
		in.SkippedReason = applied.SkipReason
		in.BytesAfter = spec.BytesBefore
		sink.sysf("result policy: skipped, original kept (%s)", applied.SkipReason)
		sink.sysf("bytes before: %s; after: %s", server.HumanBytes(spec.BytesBefore),
			server.HumanBytes(spec.BytesBefore))
	} else {
		in.OutputPath = applied.FinalPath
		sink.sysf("result policy: applied, output at %s", applied.FinalPath)
		sink.sysf("bytes before: %s; after: %s", server.HumanBytes(spec.BytesBefore),
			server.HumanBytes(st.Size()))
	}
	sink.finish()
	if err := r.client.Complete(ctx, spec.ID, in); err != nil {
		if isAPIStatus(err, 409) {
			// The job left the active state while we finished (a cancel
			// raced the last second). Disk state self-heals on the next
			// scan; the server's decision stands.
			r.log.Warn("worker: complete rejected", "job", spec.ID, "err", err)
			return nil
		}
		r.log.Error("worker: complete report", "job", spec.ID, "err", err)
		return err
	}

	r.log.Info("job complete", "job", spec.ID, "output", applied.FinalPath,
		"encoder", res.EncoderUsed, "bytes_after", st.Size())
	return nil
}

// completeTransfer uploads the verified output and reports the job done;
// the server applies the result policy from the upload (its
// verify-and-replace) and answers with the recorded job. A replace_if_smaller
// output that is not smaller is never uploaded: the node reports the skip
// directly, saving the whole transfer.
func (r *runner) completeTransfer(ctx context.Context, spec *server.JobSpec, output string, outSize int64,
	res encoders.Result, tail *tailBuffer, sink *jobLogSink, cancelled *atomic.Bool) error {
	fail := func(format string, args ...any) error {
		msg := fmt.Sprintf(format, args...)
		r.log.Warn("job failed", "job", spec.ID, "error", msg)
		sink.errorf("%s", msg)
		sink.finish()
		if err := r.client.Fail(ctx, spec.ID, msg, tail.Tail()); err != nil && !isAPIStatus(err, 409) {
			r.log.Error("worker: fail report", "job", spec.ID, "err", err)
		}
		return nil
	}

	in := CompleteInput{
		BytesAfter:      outSize,
		Encoder:         res.EncoderUsed,
		FallbackReasons: fallbackStrings(res),
		LogTail:         tail.Tail(),
		AccessMode:      store.NodeAccessTransfer,
	}
	if spec.ResultPolicy == store.ResultPolicyReplaceIfSmaller && outSize >= spec.BytesBefore {
		in.SkippedReason = fmt.Sprintf(
			"output (%s) is not smaller than the original (%s); original kept",
			server.HumanBytes(outSize), server.HumanBytes(spec.BytesBefore))
		in.BytesAfter = spec.BytesBefore
		sink.sysf("result policy: skipped, original kept (%s)", in.SkippedReason)
		sink.sysf("bytes before: %s; after: %s", server.HumanBytes(spec.BytesBefore),
			server.HumanBytes(spec.BytesBefore))
	} else {
		sink.sysf("uploading result (%s) to the server", server.HumanBytes(outSize))
		if err := r.client.UploadResult(ctx, spec.ID, output, spec.ThroughputLimitMBs, r.log); err != nil {
			if cancelled.Load() || ctx.Err() != nil {
				return nil
			}
			return fail("upload result: %v", err)
		}
		sink.sysf("result uploaded; the server verifies and applies the policy")
		sink.sysf("bytes before: %s; after: %s", server.HumanBytes(spec.BytesBefore),
			server.HumanBytes(outSize))
	}
	sink.finish()
	if err := r.client.Complete(ctx, spec.ID, in); err != nil {
		if isAPIStatus(err, 409) {
			// The job left the active state while we finished; the server
			// already discarded or kept the upload as it saw fit.
			r.log.Warn("worker: complete rejected", "job", spec.ID, "err", err)
			return nil
		}
		r.log.Error("worker: complete report", "job", spec.ID, "err", err)
		return err
	}
	r.log.Info("job complete (transfer)", "job", spec.ID,
		"encoder", res.EncoderUsed, "bytes_after", in.BytesAfter,
		"skipped", in.SkippedReason != "")
	return nil
}

// reportLoop sends progress on a fixed ticker and watches for cancellation:
// a progress response carrying a terminal job status kills the encode
// within the tick interval. The first report goes out immediately, so a
// claimed job flips to running (and starts refreshing its heartbeat) as
// soon as the encode begins. accessMode is the mode the node resolved for
// this job; the server records it. outputPath is the scratch output being
// written: every tick stats it (one cheap stat, task 18) and reports the
// size as bytes_so_far; a stat failure reports 0 and never kills the tick.
func (r *runner) reportLoop(stop <-chan struct{}, runCtx context.Context, spec *server.JobSpec,
	state *progressState, srcDuration float64, cancelled *atomic.Bool, cancelRun context.CancelFunc,
	accessMode, outputPath string) {
	report := func() bool {
		out, speed, _ := state.snapshot()
		in := ProgressInput{Speed: speed, Encoder: r.tracker.current(), AccessMode: accessMode}
		if st, err := os.Stat(outputPath); err == nil {
			in.BytesSoFar = st.Size()
		}
		if srcDuration > 0 && out > 0 {
			in.Percent = clampPercent(out / srcDuration * 100)
			if speed > 0 {
				in.ETASeconds = int64((srcDuration - out) / speed)
				if in.ETASeconds < 0 {
					in.ETASeconds = 0
				}
			}
		}
		status, err := r.client.Progress(runCtx, spec.ID, in)
		if err != nil {
			// A blip must not kill the job; the next tick retries.
			r.log.Debug("worker: progress report", "job", spec.ID, "err", err)
			return false
		}
		if jobTerminal(status) {
			r.log.Info("worker: job left the active state, stopping encode",
				"job", spec.ID, "status", status)
			cancelled.Store(true)
			cancelRun()
			return true
		}
		return false
	}

	if report() {
		return
	}
	ticker := time.NewTicker(r.cfg.progressInterval())
	defer ticker.Stop()
	for {
		select {
		case <-stop:
			return
		case <-runCtx.Done():
			return
		case <-ticker.C:
		}
		if report() {
			return
		}
	}
}

// jobTerminal reports whether a job status means the runner must stop:
// cancelled (admin), failed (sweeper at the attempt cap) or done (another
// report path closed it).
func jobTerminal(status string) bool {
	switch status {
	case store.JobStatusCancelled, store.JobStatusFailed, store.JobStatusDone:
		return true
	}
	return false
}

// execEncode runs one ffmpeg attempt with live progress parsing. The
// stderr tee feeds both the tail and the structured log (task 27); the
// tee's trailing partial line is flushed when the attempt ends.
func (r *runner) execEncode(ctx context.Context, args []string, state *progressState,
	tail *tailBuffer, sink *jobLogSink) error {
	if r.runFn != nil {
		return r.runFn(ctx, args)
	}
	tee := newStderrTee(tail, sink)
	defer tee.close()
	return execFFmpeg(ctx, r.cfg.FFmpegPath, args, newProgressWriter(state), tee)
}

// execExtract runs one subtitle-extract command (no progress stream).
func (r *runner) execExtract(ctx context.Context, args []string, tail *tailBuffer,
	sink *jobLogSink) error {
	if r.runFn != nil {
		return r.runFn(ctx, args)
	}
	tee := newStderrTee(tail, sink)
	defer tee.close()
	return execFFmpeg(ctx, r.cfg.FFmpegPath, args, io.Discard, tee)
}

// execFFmpeg runs the ffmpeg binary; stdout is the -progress stream, stderr
// feeds the log tail. The context kill (cancel/shutdown) surfaces as
// ctx.Err so the caller can distinguish it from a real encode failure.
func execFFmpeg(ctx context.Context, ffmpegPath string, args []string, stdout, stderr io.Writer) error {
	if ffmpegPath == "" {
		ffmpegPath = "ffmpeg"
	}
	cmd := exec.CommandContext(ctx, ffmpegPath, args...)
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return fmt.Errorf("ffmpeg: %w", err)
	}
	return nil
}

// registryFor returns the encoders to try for the target, each wrapped so
// progress reports can name the backend currently being tried.
func (r *runner) registryFor(target flow.Target) []encoders.Encoder {
	list := r.encs
	if list == nil {
		list = encoders.Registry(target, r.caps)
	}
	return r.tracker.wrap(list)
}

// encoderTracker remembers which encoder BuildArgs was last called on.
type encoderTracker struct {
	mu   sync.Mutex
	last string
}

func (t *encoderTracker) wrap(list []encoders.Encoder) []encoders.Encoder {
	out := make([]encoders.Encoder, len(list))
	for i, e := range list {
		out[i] = &trackedEncoder{Encoder: e, t: t}
	}
	return out
}

func (t *encoderTracker) setCurrent(name string) {
	t.mu.Lock()
	t.last = name
	t.mu.Unlock()
}

func (t *encoderTracker) current() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.last
}

// trackedEncoder records itself as in-use when the runner builds its args.
type trackedEncoder struct {
	encoders.Encoder
	t *encoderTracker
}

func (e *trackedEncoder) BuildArgs(spec encoders.Spec) []string {
	e.t.setCurrent(e.Name())
	return e.Encoder.BuildArgs(spec)
}

// copyWithContext copies src to dst in chunks so a cancel or shutdown does
// not wait for a huge read to finish.
func copyWithContext(ctx context.Context, src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer func() { _ = in.Close() }()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	buf := make([]byte, 1<<20)
	for {
		if err := ctx.Err(); err != nil {
			_ = out.Close()
			_ = os.Remove(dst)
			return err
		}
		n, rerr := in.Read(buf)
		if n > 0 {
			if _, werr := out.Write(buf[:n]); werr != nil {
				_ = out.Close()
				_ = os.Remove(dst)
				return werr
			}
		}
		if rerr == io.EOF {
			break
		}
		if rerr != nil {
			_ = out.Close()
			_ = os.Remove(dst)
			return rerr
		}
	}
	if err := out.Sync(); err != nil {
		_ = out.Close()
		_ = os.Remove(dst)
		return err
	}
	if err := out.Close(); err != nil {
		_ = os.Remove(dst)
		return err
	}
	return nil
}

// clampPercent bounds a percentage to 0..100.
func clampPercent(p float64) float64 {
	switch {
	case p < 0:
		return 0
	case p > 100:
		return 100
	default:
		return p
	}
}

// containerExt maps a Target container to its file extension.
func containerExt(container string) string {
	if container == flow.ContainerMP4 {
		return "mp4"
	}
	return "mkv"
}
