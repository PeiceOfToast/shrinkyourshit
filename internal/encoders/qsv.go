// SPDX-License-Identifier: AGPL-3.0-only

package encoders

import (
	"context"
	"fmt"
	"log/slog"
	"runtime"
	"sync"

	"github.com/PeiceOfToast/shrinkyourshit/internal/flow"
)

// qsvEncoder drives Intel Quick Sync Video (DECISIONS.md 14). The primary
// path uses the qsv ffmpeg encoders (h264_qsv, hevc_qsv, av1_qsv). On Linux,
// when the QSV session cannot be initialized (for example on older drivers
// or when only the VAAPI stack is present), the VAAPI equivalent encoders
// are tried instead; the working variant per codec is chosen during
// detection and used by BuildArgs.
//
// Beyond the fixed argument set, the backend probes the optional encoder
// features the driver may support (task 35): look-ahead with extbrc, the
// async pipeline depth and low-power mode. Each is probed per codec with
// the standard one-second testsrc self-test and the candidate flag set;
// the result is cached on the backend behind mu. None of the flags is ever
// added without the user's opt-in (the qsv settings, task 35), and a
// requested feature the probe rejected is carried on without — a setting
// must never turn into a failed job.
type qsvEncoder struct {
	// variants maps a target codec to the ffmpeg encoder chosen during
	// detection ("hevc_qsv" or "hevc_vaapi" style) plus the VAAPI device
	// node for the vaapi variant. Guarded by mu; written by detection,
	// read by BuildArgs. A codec without an entry defaults to the qsv
	// encoder.
	mu       sync.Mutex
	variants map[string]qsvVariant
	// feature probe state (task 35), same locking pattern as variants:
	// written by detection, read by BuildArgs and the accessors.
	probed   bool                   // a probe pass ran (results stable)
	features map[string]qsvFeatures // per target codec; qsv variants only
	prefs    QSVPrefs               // the user opt-ins (qsv settings)
	warned   map[string]bool        // requested-but-unsupported warns: once each
}

// qsvFeatures is the optional feature set probed for one codec's qsv
// encoder. All false when the probe could not run or the flag was
// rejected.
type qsvFeatures struct {
	LookAhead  bool // -look_ahead 1 (with extbrc)
	ExtBRC     bool // -extbrc 1
	AsyncDepth bool // -async_depth N
	LowPower   bool // -low_power 1
}

// QSVPrefs are the user's QSV opt-ins (task 35 settings), carried to the
// worker on the heartbeat. Both default off: with both off, the argument
// vector is byte-identical to the behaviour before task 35.
type QSVPrefs struct {
	// ExtraQuality turns on look-ahead and extbrc where the machine
	// supports both (setting qsv_extra_quality).
	ExtraQuality bool
	// LowPower passes -low_power 1 where supported (setting
	// qsv_low_power): faster, slightly worse.
	LowPower bool
}

// SetQSVPrefs applies the server's QSV opt-ins to the qsv backend (the
// worker calls it on every heartbeat, task 35). When a requested feature
// probed unsupported on this machine, one warn line names the feature and
// the machine; the warn fires once per feature per process so repeated
// heartbeats stay quiet. log may be nil. This never returns an error: a
// setting must not turn into a failed job.
func SetQSVPrefs(p QSVPrefs, machine string, log *slog.Logger) {
	QSV.(*qsvEncoder).setPrefs(p, machine, log)
}

// QSVRequested reports the QSV opt-ins currently applied to the qsv
// backend. Observability for logs and tests; the truth about what a
// machine supports is the probed QSVFeatures on the capability.
func QSVRequested() QSVPrefs {
	qsv := QSV.(*qsvEncoder)
	qsv.mu.Lock()
	defer qsv.mu.Unlock()
	return qsv.prefs
}

// setPrefs stores the opt-ins and warns once per requested feature the
// probe rejected. Warns wait for a completed probe pass: when the probe
// could not run, nothing is known and nothing is guessed.
func (e *qsvEncoder) setPrefs(p QSVPrefs, machine string, log *slog.Logger) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.prefs = p
	if !e.probed || log == nil {
		return
	}
	u := e.unionLocked()
	requests := map[string]bool{"look-ahead": p.ExtraQuality, "extbrc": p.ExtraQuality, "low power": p.LowPower}
	supported := map[string]bool{
		"look-ahead": u.LookAhead,
		"extbrc":     u.ExtBRC,
		"low power":  u.LowPower,
	}
	for feature, want := range requests {
		if !want || supported[feature] || e.warned[feature] {
			continue
		}
		if e.warned == nil {
			e.warned = map[string]bool{}
		}
		e.warned[feature] = true
		log.Warn("qsv: requested feature is not supported on this machine; continuing without it",
			"feature", feature, "node", machine)
	}
}

// qsvVariant is the ffmpeg encoder setup chosen for one codec.
type qsvVariant struct {
	// encoderName is the ffmpeg -c:v value.
	encoderName string
	// device is the VAAPI device node; empty for the qsv variant.
	device string
}

// qsvEncoderFor and vaapiEncoderFor map a target codec to the ffmpeg
// encoder names of each variant.
var (
	qsvEncoderFor = map[string]string{
		flow.CodecHEVC: "hevc_qsv",
		flow.CodecAV1:  "av1_qsv",
		flow.CodecH264: "h264_qsv",
	}
	vaapiEncoderFor = map[string]string{
		flow.CodecHEVC: "hevc_vaapi",
		flow.CodecAV1:  "av1_vaapi",
		flow.CodecH264: "h264_vaapi",
	}
)

func (e *qsvEncoder) Name() string { return "qsv" }

func (e *qsvEncoder) Codecs() []string {
	return []string{flow.CodecHEVC, flow.CodecAV1, flow.CodecH264}
}

// Detect checks Intel device presence and self-tests each codec: first the
// qsv encoder, then (on Linux) the VAAPI equivalent when QSV init fails.
func (e *qsvEncoder) Detect(ctx context.Context, ffmpegPath string) (Capability, error) {
	env, err := processDetector.probeEnv(ctx, ffmpegPath)
	if err != nil {
		return Capability{}, err
	}
	entry, err := e.detectWithEnv(ctx, env)
	if err != nil {
		return Capability{}, err
	}
	return Capability{Encoders: []EncoderInfo{*entry}}, nil
}

// detectWithEnv runs detection against a prepared environment (see detect.go).
func (e *qsvEncoder) detectWithEnv(ctx context.Context, env detectEnv) (*EncoderInfo, error) {
	hasDRI := len(env.driEntries) > 0
	if err := requireDevices(hasDRI, "Intel (VAAPI/QSV)"); err != nil {
		return nil, err
	}

	var codecs []string
	var features map[string]qsvFeatures
	for _, codec := range e.Codecs() {
		// Primary path: the qsv encoder. QSV accepts software frames and
		// creates its own device, so no pre-input options are needed.
		name := qsvEncoderFor[codec]
		if env.encoders[name] {
			if err := selfTest(ctx, env.ffmpegPath, nil,
				[]string{"-c:v", name}); err == nil {
				e.setVariant(codec, qsvVariant{encoderName: name})
				codecs = append(codecs, codec)
				continue
			}
		}
		// Fallback path: the VAAPI equivalent. It needs a device node
		// (prefer the render node) and hardware frames, uploaded with an
		// explicit filter chain. Only tried on Linux, where the VAAPI
		// stack lives.
		if runtime.GOOS != "linux" || !env.hwaccels["vaapi"] {
			continue
		}
		vname := vaapiEncoderFor[codec]
		device := env.renderNode()
		if !env.encoders[vname] || device == "" {
			continue
		}
		if err := selfTest(ctx, env.ffmpegPath,
			[]string{"-vaapi_device", device},
			[]string{"-vf", "format=nv12,hwupload", "-c:v", vname}); err == nil {
			e.setVariant(codec, qsvVariant{encoderName: vname, device: device})
			codecs = append(codecs, codec)
		}
	}
	if len(codecs) == 0 {
		return nil, fmt.Errorf("encoders: qsv self-test failed on all codecs")
	}

	// Optional features (task 35): probe each one on the qsv variant of
	// every codec that uses it. The *_vaapi variants have none of these
	// options, so their codecs are probed as all-off.
	features = e.probeFeatures(ctx, env)
	e.setFeatures(features)
	union := e.unionFeatures()
	return &EncoderInfo{Name: e.Name(), Codecs: codecs, QSVFeatures: &union}, nil
}

// probeFeatures tests each optional QSV feature per codec by running the
// standard one-second testsrc self-test with the candidate flag set and
// reading the exit code — the same way the codec self-tests and the vaapi
// backend's device probe work. A probe that cannot run leaves the feature
// off rather than guessing.
//
// Every probe mirrors the runtime argument shape as closely as the flag
// allows, so "probed true" means the flags BuildArgs emits are accepted:
//   - look-ahead: -look_ahead 1 -extbrc 1 with -global_quality (ffmpeg
//     errors on look-ahead without a bitrate or quality, and with a
//     quality the pair selects the look-ahead + ICQ mode);
//   - low power: -low_power 1 with -global_quality (the runtime always
//     pairs the low-power mode with the ICQ quality flag);
//   - extbrc and async depth stand alone.
//
// look-ahead requires extbrc, so a codec whose extbrc probe failed skips
// its look-ahead probe. Note that ffmpeg's -look_ahead option exists only
// on h264_qsv (hevc_qsv and av1_qsv have no such option), so look-ahead
// can only ever probe true for the h264 target — per-codec results keep
// hevc and av1 honest instead of inheriting h264's answer.
func (e *qsvEncoder) probeFeatures(ctx context.Context, env detectEnv) map[string]qsvFeatures {
	out := map[string]qsvFeatures{}
	// The probe runs against the same ICQ value the builder emits at
	// quality step 3; the specific number does not matter, only that the
	// flag set is accepted.
	quality := []string{"-global_quality", itoa(qualityFor(3).QSVGQ)}
	for _, codec := range e.Codecs() {
		var f qsvFeatures
		if v := e.variantFor(codec); v.device == "" && env.encoders[v.encoderName] {
			base := append([]string{"-c:v"}, v.encoderName)
			extbrc := append(append([]string{}, base...), "-extbrc", "1")
			lookAhead := append(append(append([]string{}, base...), quality...),
				"-look_ahead", "1", "-extbrc", "1")
			async := append(append([]string{}, base...), "-async_depth", "4")
			lowPower := append(append(append([]string{}, base...), quality...), "-low_power", "1")
			f.ExtBRC = selfTest(ctx, env.ffmpegPath, nil, extbrc) == nil
			f.LookAhead = f.ExtBRC && selfTest(ctx, env.ffmpegPath, nil, lookAhead) == nil
			f.AsyncDepth = selfTest(ctx, env.ffmpegPath, nil, async) == nil
			f.LowPower = selfTest(ctx, env.ffmpegPath, nil, lowPower) == nil
		}
		out[codec] = f
	}
	return out
}

// setFeatures records a completed feature probe pass (even when nothing
// was probed — that result is "all off", and it is stable).
func (e *qsvEncoder) setFeatures(features map[string]qsvFeatures) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.probed = true
	e.features = features
}

// featuresFor returns the probed features for a codec; all off when the
// probe has not run for it.
func (e *qsvEncoder) featuresFor(codec string) qsvFeatures {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.features[codec]
}

// unionLocked folds the per-codec probe results into the machine-level
// feature set: a feature is reported when any probed codec's encoder
// accepted it. Callers hold e.mu.
func (e *qsvEncoder) unionLocked() QSVFeatures {
	var u QSVFeatures
	for _, f := range e.features {
		u.LookAhead = u.LookAhead || f.LookAhead
		u.ExtBRC = u.ExtBRC || f.ExtBRC
		u.AsyncDepth = u.AsyncDepth || f.AsyncDepth
		u.LowPower = u.LowPower || f.LowPower
	}
	return u
}

// unionFeatures is unionLocked with the lock taken (for callers outside
// detection).
func (e *qsvEncoder) unionFeatures() QSVFeatures {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.unionLocked()
}

// setVariant records the working ffmpeg encoder for a codec.
func (e *qsvEncoder) setVariant(codec string, v qsvVariant) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.variants == nil {
		e.variants = map[string]qsvVariant{}
	}
	e.variants[codec] = v
}

// variantFor returns the ffmpeg encoder for a codec, defaulting to the qsv
// variant when detection has not run (or has not chosen a variant).
func (e *qsvEncoder) variantFor(codec string) qsvVariant {
	e.mu.Lock()
	defer e.mu.Unlock()
	if v, ok := e.variants[codec]; ok {
		return v
	}
	return qsvVariant{encoderName: qsvEncoderFor[codec]}
}

// claimedDevices returns the VAAPI device nodes the qsv backend took for its
// own vaapi variants (task 30). The vaapi backend skips these so one GPU is
// never reported by both backends; devices qsv does not hold remain free for
// vaapi — a machine can legitimately report both qsv and vaapi.
func (e *qsvEncoder) claimedDevices() map[string]bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	out := map[string]bool{}
	for _, v := range e.variants {
		if v.device != "" {
			out[v.device] = true
		}
	}
	return out
}

// SelfTest runs the one-second testsrc encode for the backend's primary
// codec with "ffmpeg" from PATH, using the variant detection chose.
func (e *qsvEncoder) SelfTest(ctx context.Context) error {
	v := e.variantFor(flow.CodecHEVC)
	if v.device == "" {
		return selfTest(ctx, "ffmpeg", nil, []string{"-c:v", v.encoderName})
	}
	return selfTest(ctx, "ffmpeg", []string{"-vaapi_device", v.device},
		[]string{"-vf", "format=nv12,hwupload", "-c:v", v.encoderName})
}

// BuildArgs maps the quality step onto the variant chosen during detection.
// The qsv variant takes -global_quality (Intel's ICQ mode, see quality.go)
// plus, when the user opted in and the probe accepted them, the optional
// encoder flags; the vaapi variant additionally needs the device node and
// a hwupload filter chain and never carries the QSV extras (the *_vaapi
// encoders have none of those options). A copied video (explicit copy, or
// the satisfied rule) needs no device, so the args are the plain remux
// shape from every backend.
func (e *qsvEncoder) BuildArgs(spec Spec) []string {
	if spec.Target.Video.Codec == flow.CodecCopy || videoShouldCopy(spec) {
		return buildCommand(spec, nil, nil)
	}
	codec := spec.Target.Video.Codec
	v := e.variantFor(codec)
	q := qualityFor(spec.Target.Video.Quality)
	if v.device == "" {
		video := []string{"-c:v", v.encoderName, "-global_quality", itoa(q.QSVGQ)}
		video = e.appendExtras(video, codec, spec)
		return buildCommand(spec, nil, video)
	}
	video := []string{"-c:v", v.encoderName, "-global_quality", itoa(q.QSVGQ), "-vf", "format=nv12,hwupload"}
	// h264_vaapi has no global_quality mode; -qp sets the fixed quantizer.
	if codec == flow.CodecH264 {
		video = []string{"-c:v", v.encoderName, "-qp", itoa(q.QSVGQ), "-vf", "format=nv12,hwupload"}
	}
	return buildCommand(spec, []string{"-vaapi_device", v.device}, video)
}

// appendExtras adds the opt-in QSV encoder flags after the quality flag,
// in a fixed order: -look_ahead, -extbrc (look-ahead requires it),
// -low_power, -async_depth. Every flag needs both the user's opt-in (the
// qsv settings, task 35) and a positive feature probe on this codec's
// encoder: with the settings off — or on a machine where the probe found
// nothing — the argument vector is byte-identical to the pre-task-35
// behaviour, and a requested feature the probe rejected is simply absent
// (SetQSVPrefs warned about it instead).
func (e *qsvEncoder) appendExtras(video []string, codec string, spec Spec) []string {
	prefs := e.requestedPrefs()
	f := e.featuresFor(codec)
	if prefs.ExtraQuality && f.LookAhead && f.ExtBRC {
		video = append(video, "-look_ahead", "1", "-extbrc", "1")
	}
	if prefs.LowPower && f.LowPower {
		video = append(video, "-low_power", "1")
	}
	// -async_depth has no setting of its own: it is a throughput knob, not
	// a quality one. It joins the argument vector only once the user has
	// opted into a QSV extra on either switch, and its value follows the
	// quality step (higher quality, shallower pipeline — qsvAsyncDepth).
	if (prefs.ExtraQuality || prefs.LowPower) && f.AsyncDepth {
		video = append(video, "-async_depth", itoa(qsvAsyncDepth(spec.Target.Video.Quality)))
	}
	return video
}

// requestedPrefs snapshots the applied opt-ins.
func (e *qsvEncoder) requestedPrefs() QSVPrefs {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.prefs
}

// qsvAsyncDepth picks the async pipeline depth from the quality step (task
// 35; deliberately not a setting — see appendExtras). A deeper pipeline
// keeps more frames in flight and buys throughput at the cost of latency;
// the lowest quality steps take the deepest pipeline (4, the value the
// capability probe tests), the highest quality steps keep it shallow so
// the encoder stays responsive. quality is clamped into 1..5 first so a
// bad Target degrades instead of panicking.
func qsvAsyncDepth(quality int) int {
	switch {
	case quality < 1:
		quality = 1
	case quality > flow.QualityMax:
		quality = flow.QualityMax
	}
	switch {
	case quality <= 2:
		return 4
	case quality == 3:
		return 3
	default:
		return 2
	}
}
