// SPDX-License-Identifier: AGPL-3.0-only

package encoders

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"time"
)

// selfTestInput is the synthetic clip used for self-tests: one second of
// testsrc, small enough that even slow CPU encoders finish quickly.
const selfTestInput = "testsrc=duration=1:size=160x120:rate=10"

// selfTestTimeout bounds one self-test encode so a hung driver cannot stall
// detection forever.
const selfTestTimeout = 45 * time.Second

// detectEnv is the machine environment gathered once per detection pass and
// shared by all backends.
type detectEnv struct {
	ffmpegPath string
	// encoders holds the names reported by "ffmpeg -encoders".
	encoders map[string]bool
	// hwaccels holds the names reported by "ffmpeg -hwaccels".
	hwaccels map[string]bool
	// driDir is the device directory being probed (empty when the
	// platform has none).
	driDir string
	// driEntries lists the entries of driDir.
	driEntries []string
	// nvidia is true when at least one /dev/nvidia* device exists.
	nvidia bool
}

// renderNode returns the first device node to use for VAAPI, preferring
// renderD* (the recommended choice) over any other /dev/dri entry.
func (e detectEnv) renderNode() string {
	for _, name := range e.driEntries {
		if strings.HasPrefix(name, "renderD") {
			return filepath.Join(e.driDir, name)
		}
	}
	if len(e.driEntries) > 0 {
		return filepath.Join(e.driDir, e.driEntries[0])
	}
	return ""
}

// Default device locations. Detectors created for tests override the
// fields on the detector struct instead; these constants are only the
// production defaults.
const (
	driDeviceDir    = "/dev/dri"
	nvidiaDevPrefix = "/dev/nvidia"
)

// detectResult is one cached detection outcome. Errors are cached too: a
// machine that failed detection once does not re-run hardware probes on
// every heartbeat. Context errors are not cached (see detect).
type detectResult struct {
	cap Capability
	err error
}

// detector runs backend detection and caches results for the process
// lifetime, keyed by ffmpeg path. The device locations are fields so tests
// can point them at a temp directory with fake device files.
type detector struct {
	// mu guards everything below. It is held for the whole detection run
	// so concurrent callers share one run instead of racing hardware
	// probes.
	mu         sync.Mutex
	results    map[string]detectResult
	driDir     string
	nvidiaGlob string
}

func newDetector() *detector {
	return &detector{
		results:    map[string]detectResult{},
		driDir:     driDeviceDir,
		nvidiaGlob: nvidiaDevPrefix + "*",
	}
}

// processDetector caches detection for the process lifetime (ARCHITECTURE.md:
// each worker detects on start).
var processDetector = newDetector()

// DetectAll detects every registered backend with the given ffmpeg binary
// and returns the combined capability. Results are cached for the process
// lifetime, so calling this repeatedly (for example on every heartbeat) is
// cheap. An error means ffmpeg itself could not be run or no backend at all
// was detected; the capability still lists whatever was found.
func DetectAll(ctx context.Context, ffmpegPath string) (Capability, error) {
	return processDetector.detect(ctx, ffmpegPath)
}

// detect runs or returns the cached detection for one ffmpeg path.
func (d *detector) detect(ctx context.Context, ffmpegPath string) (Capability, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	if r, ok := d.results[ffmpegPath]; ok {
		return r.cap, r.err
	}
	cap, err := d.run(ctx, ffmpegPath)
	if err != nil && isContextErr(err) {
		// A canceled or timed-out probe says nothing about the machine;
		// let a later call with a live context try again.
		return cap, err
	}
	d.results[ffmpegPath] = detectResult{cap: cap, err: err}
	return cap, err
}

// run performs the actual detection pass: parse the ffmpeg lists, check
// devices, then let every backend detect (which runs the self-tests).
func (d *detector) run(ctx context.Context, ffmpegPath string) (Capability, error) {
	env, err := d.probeEnv(ctx, ffmpegPath)
	if err != nil {
		return Capability{}, fmt.Errorf("encoders: ffmpeg %s is not usable: %w", ffmpegPath, err)
	}

	var cap Capability
	var firstErr error
	for _, enc := range allEncoders {
		entry, err := d.detectEncoder(ctx, enc, env)
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		if entry != nil {
			cap.Encoders = append(cap.Encoders, *entry)
		}
	}
	if len(cap.Encoders) == 0 {
		return cap, fmt.Errorf("encoders: no encoder detected (%v)", firstErr)
	}
	return cap, nil
}

// detectEncoder runs one backend's detection. The exported Encoder.Detect
// has to fetch its own environment, so backends split detection into a
// method that takes a prepared env; this calls it with ours.
func (d *detector) detectEncoder(ctx context.Context, enc Encoder, env detectEnv) (*EncoderInfo, error) {
	switch be := enc.(type) {
	case *nvencEncoder:
		return be.detectWithEnv(ctx, env)
	case *qsvEncoder:
		return be.detectWithEnv(ctx, env)
	case *cpuEncoder:
		return be.detectWithEnv(ctx, env)
	default:
		// A backend registered without a detectWithEnv method (see
		// README.md): fall back to the interface method, which probes
		// the environment itself.
		cap, err := enc.Detect(ctx, env.ffmpegPath)
		if err != nil {
			return nil, err
		}
		if len(cap.Encoders) == 0 {
			return nil, errors.New("encoders: backend detected nothing")
		}
		return &cap.Encoders[0], nil
	}
}

// probeEnv gathers everything the backends need: the ffmpeg encoder and
// hwaccel name lists plus device presence.
func (d *detector) probeEnv(ctx context.Context, ffmpegPath string) (detectEnv, error) {
	encoderNames, err := parseEncoders(ctx, ffmpegPath)
	if err != nil {
		return detectEnv{}, err
	}
	hwaccelNames, err := parseHwaccels(ctx, ffmpegPath)
	if err != nil {
		return detectEnv{}, err
	}

	env := detectEnv{
		ffmpegPath: ffmpegPath,
		encoders:   encoderNames,
		hwaccels:   hwaccelNames,
	}
	if d.driDir != "" {
		env.driDir = d.driDir
		if entries, err := os.ReadDir(d.driDir); err == nil {
			for _, e := range entries {
				env.driEntries = append(env.driEntries, e.Name())
			}
		}
	}
	if d.nvidiaGlob != "" {
		if matches, err := filepath.Glob(d.nvidiaGlob); err == nil && len(matches) > 0 {
			env.nvidia = true
		}
	}
	return env, nil
}

// The -encoders output has one line per encoder:
//
//	V....D libx265              libx265 H.265 / HEVC (codec hevc)
//
// The first field is the capability flags (letters and dots), the second the
// encoder name.
var encoderLine = regexp.MustCompile(`^ [A-Z][A-Za-z.]*\s+(\S+)`)

// parseEncoders runs "ffmpeg -encoders" and returns the encoder names.
func parseEncoders(ctx context.Context, ffmpegPath string) (map[string]bool, error) {
	out, err := runFFmpeg(ctx, ffmpegPath, "-hide_banner", "-encoders")
	if err != nil {
		return nil, fmt.Errorf("encoders: listing ffmpeg encoders failed: %w", err)
	}
	names := map[string]bool{}
	for _, line := range strings.Split(string(out), "\n") {
		if m := encoderLine.FindStringSubmatch(line); m != nil {
			names[m[1]] = true
		}
	}
	return names, nil
}

// parseHwaccels runs "ffmpeg -hwaccels" and returns the hwaccel names.
// The output is a header line followed by one name per line.
func parseHwaccels(ctx context.Context, ffmpegPath string) (map[string]bool, error) {
	out, err := runFFmpeg(ctx, ffmpegPath, "-hide_banner", "-hwaccels")
	if err != nil {
		return nil, fmt.Errorf("encoders: listing ffmpeg hwaccels failed: %w", err)
	}
	names := map[string]bool{}
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasSuffix(line, ":") {
			continue
		}
		names[line] = true
	}
	return names, nil
}

// selfTestArgs builds the one-second testsrc self-test command for one
// ffmpeg encoder. preArgs are global options placed before the input (for
// example -vaapi_device), streamArgs configure the encoder.
func selfTestArgs(preArgs, streamArgs []string) []string {
	args := []string{"-hide_banner", "-nostdin", "-v", "error", "-y"}
	args = append(args, preArgs...)
	args = append(args, "-f", "lavfi", "-i", selfTestInput)
	args = append(args, streamArgs...)
	return append(args, "-f", "null", "-")
}

// selfTest runs the one-second testsrc encode and reports whether it
// succeeds. It is the final word on whether a codec is usable: a listed
// ffmpeg encoder can still fail on the actual hardware or driver.
func selfTest(ctx context.Context, ffmpegPath string, preArgs, streamArgs []string) error {
	ctx, cancel := context.WithTimeout(ctx, selfTestTimeout)
	defer cancel()
	args := selfTestArgs(preArgs, streamArgs)
	cmd := exec.CommandContext(ctx, ffmpegPath, args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("encoders: self-test with %s failed: %w: %s",
			lastArg(streamArgs, "-c:v"), err, oneLine(stderr.String()))
	}
	return nil
}

// lastArg returns the value following a flag in an argument list, for
// readable error messages.
func lastArg(args []string, flag string) string {
	for i, a := range args {
		if a == flag && i+1 < len(args) {
			return args[i+1]
		}
	}
	return flag
}

// oneLine collapses an ffmpeg error output to a single short line.
func oneLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = strings.TrimSpace(s[:i])
	}
	if len(s) > 200 {
		s = s[:200]
	}
	return s
}

// runFFmpeg runs ffmpeg with arguments that only produce listing output and
// returns stdout.
func runFFmpeg(ctx context.Context, ffmpegPath string, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, selfTestTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, ffmpegPath, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("%w: %s", err, oneLine(stderr.String()))
	}
	return stdout.Bytes(), nil
}

// isContextErr reports whether the error is a canceled context or timeout.
func isContextErr(err error) bool {
	return errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)
}

// requireDevices is the shared Linux device gate. It returns an error when
// the backend's hardware is not present, so no ffmpeg process is spawned for
// a machine that cannot possibly use it. On Windows the paths do not exist
// while the hardware can still work, so the check is skipped there and the
// self-test is the gate.
func requireDevices(present bool, what string) error {
	if runtime.GOOS == "windows" {
		return nil
	}
	if !present {
		return fmt.Errorf("encoders: no %s device found", what)
	}
	return nil
}
