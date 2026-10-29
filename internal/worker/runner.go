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
// the output verified.
type runner struct {
	cfg     Config
	client  *Client
	caps    encoders.Capability
	encs    []encoders.Encoder // test override; nil → Registry(target, caps)
	prober  Prober
	runFn   encoders.RunFn // test override; nil → ffmpeg subprocess
	sc      *scratch
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
	fail := func(format string, args ...any) error {
		msg := fmt.Sprintf(format, args...)
		r.log.Warn("job failed", "job", spec.ID, "error", msg)
		if err := r.client.Fail(ctx, spec.ID, msg, tail.Tail()); err != nil && !isAPIStatus(err, 409) {
			r.log.Error("worker: fail report", "job", spec.ID, "err", err)
		}
		return nil
	}

	dir, err := r.sc.ensure(spec.ID)
	if err != nil {
		return fail("prepare scratch: %v", err)
	}

	if spec.AccessMode != store.NodeAccessShared {
		return fail("access mode %q is not supported yet: transfer mode arrives with task 09", spec.AccessMode)
	}

	// Copy the source into scratch (never encode on the library), keeping
	// its mtime so the fingerprint matches the original row.
	srcExt := filepath.Ext(spec.SourcePath)
	local := filepath.Join(dir, "source"+srcExt)
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

	srcInfo, err := r.prober(ctx, r.cfg.FFprobePath, local)
	if err != nil {
		return fail("probe source: %v", err)
	}

	// Encode with fallback while a ticker reports -progress every interval.
	output := filepath.Join(dir, "output."+containerExt(spec.Target.Container))
	sp := encoders.NewSpec(local, output, srcInfo, spec.Target)
	state := newProgressState()

	runCtx, cancelRun := context.WithCancel(ctx)
	defer cancelRun()
	reportDone := make(chan struct{})
	stopReport := make(chan struct{})
	go func() {
		defer close(reportDone)
		r.reportLoop(stopReport, runCtx, spec, state, srcInfo.DurationSeconds, cancelled, cancelRun)
	}()

	res, encErr := encoders.RunWithFallback(runCtx, r.registryFor(spec.Target), sp,
		func(ctx context.Context, args []string) error {
			return r.execEncode(ctx, args, state, tail)
		})
	close(stopReport)
	<-reportDone

	if cancelled.Load() || ctx.Err() != nil {
		return nil
	}
	if encErr != nil {
		return fail("encode failed: %v", encErr)
	}

	// Removed text subtitles become .srt files beside the (scratch) output;
	// they move into the library only when the result is applied.
	for _, ec := range encoders.ExtractCommands(sp) {
		if err := r.execExtract(runCtx, ec.Args, tail); err != nil {
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
	if err := encoders.VerifyOutput(sp, srcInfo.DurationSeconds, outInfo, st.Size()); err != nil {
		return fail("output verification failed: %v", err)
	}

	applied, err := applyResultPolicy(ctx, spec.ResultPolicy, output, spec.SourcePath,
		st.Size(), spec.BytesBefore, r.sc.extractFiles(dir, "output"))
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
	}
	if applied.SkipReason != "" {
		in.SkippedReason = applied.SkipReason
		in.BytesAfter = spec.BytesBefore
	} else {
		in.OutputPath = applied.FinalPath
	}
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

// reportLoop sends progress on a fixed ticker and watches for cancellation:
// a progress response carrying a cancelled job kills the encode within the
// tick interval.
func (r *runner) reportLoop(stop <-chan struct{}, runCtx context.Context, spec *server.JobSpec,
	state *progressState, srcDuration float64, cancelled *atomic.Bool, cancelRun context.CancelFunc) {
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
		out, speed, _ := state.snapshot()
		in := ProgressInput{Speed: speed, Encoder: r.tracker.current()}
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
			continue
		}
		if status == store.JobStatusCancelled {
			r.log.Info("worker: job cancelled, stopping encode", "job", spec.ID)
			cancelled.Store(true)
			cancelRun()
			return
		}
	}
}

// execEncode runs one ffmpeg attempt with live progress parsing.
func (r *runner) execEncode(ctx context.Context, args []string, state *progressState, tail *tailBuffer) error {
	if r.runFn != nil {
		return r.runFn(ctx, args)
	}
	return execFFmpeg(ctx, r.cfg.FFmpegPath, args, newProgressWriter(state), tail)
}

// execExtract runs one subtitle-extract command (no progress stream).
func (r *runner) execExtract(ctx context.Context, args []string, tail *tailBuffer) error {
	if r.runFn != nil {
		return r.runFn(ctx, args)
	}
	return execFFmpeg(ctx, r.cfg.FFmpegPath, args, io.Discard, tail)
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
	e.t.setCurrent(e.Encoder.Name())
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
