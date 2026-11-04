// SPDX-License-Identifier: AGPL-3.0-only

package flow

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/PeiceOfToast/shrinkyourshit/internal/probe"
)

// Condition is one resolved guard on a plan step. Guards collected from
// consecutive `if_*` steps AND together: every condition must pass for the
// guarded step to run.
type Condition struct {
	// Kind is the guard step type that produced it (if_codec_not,
	// if_bitrate_above, if_resolution_above, if_audio_channels_above or if_hdr).
	Kind string `json:"kind"`
	// Codec applies to kind "codec_not": the codec the video must NOT be.
	Codec string `json:"codec,omitempty"`
	// BitrateMBPS applies to kind "bitrate_above": the threshold.
	BitrateMBPS float64 `json:"bitrate_mbps,omitempty"`
	// Height applies to kind "resolution_above": the video must be taller
	// than this many pixels.
	Height int `json:"height,omitempty"`
	// Channels applies to kind "audio_channels_above": some audio track must
	// have more channels than this.
	Channels int `json:"channels,omitempty"`
	// IsHDR applies to kind "hdr": true requires HDR video, false requires
	// video that is not HDR.
	IsHDR bool `json:"is_hdr,omitempty"`
}

// pass evaluates the condition against a probed file. A file without a
// real video stream never passes a video condition: there is nothing to
// guard (the encode would copy nothing anyway). The audio condition reads
// the audio streams only, so an audio-only file can pass it.
func (c Condition) pass(info probe.MediaInfo) bool {
	switch c.Kind {
	case StepIfCodecNot, StepIfBitrateAbove, StepIfResolutionAbove, StepIfHDR:
		if len(info.Video) == 0 {
			return false
		}
	}
	switch c.Kind {
	case StepIfCodecNot:
		return info.Video[0].Codec != c.Codec
	case StepIfBitrateAbove:
		return info.Video[0].Bitrate > int64(c.BitrateMBPS*1_000_000)
	case StepIfResolutionAbove:
		return info.Video[0].Height > c.Height
	case StepIfHDR:
		return info.Video[0].HDR == c.IsHDR
	case StepIfAudioChannelsAbove:
		// Any track counts: a file with a surround track passes even when
		// its first track is stereo.
		for _, a := range info.Audio {
			if a.Channels > c.Channels {
				return true
			}
		}
	}
	return false
}

// PlanStep is one executable step of a compiled plan: the step with the
// conditions gathered from the guards before it.
type PlanStep struct {
	ID     string          `json:"id"`
	Type   string          `json:"type"`
	Params json.RawMessage `json:"params,omitempty"`
	// When lists the guard conditions; nil when the step always runs.
	When []Condition `json:"when,omitempty"`
}

// Plan is the ordered action plan the worker runs (ARCHITECTURE.md:
// flow.Compile produces the plan; parity with the pre-flow builder is a
// test gate). Steps run in order; the worker probes first, resolves each
// step's conditions against the probe and applies what passes. HealthCheck
// and the move destination (task 24) ride along: they change nothing about
// the encode, they act after the output is produced.
type Plan struct {
	Steps []PlanStep
	// HealthCheck is set when the graph holds a health_check step. It is
	// carried as compiled (defaults filled), because verification happens
	// after the encode and never depends on the probe.
	HealthCheck *HealthCheckSpec
	// MoveTo is the trimmed destination of the graph's move step, empty
	// when there is none. MoveKeepTree rebuilds the file's folder layout
	// under it.
	MoveTo       string
	MoveKeepTree bool
}

// HealthCheckSpec is the compiled health_check step (task 24): how the
// worker verifies the produced file before anything is reported complete.
type HealthCheckSpec struct {
	// Mode is "sample" (decode Minutes from the middle) or "full".
	Mode string `json:"mode"`
	// Minutes is the sample decode length, 1 to 120.
	Minutes int `json:"minutes"`
	// DurationTolerance is the fraction of the source duration the output
	// must still have, 0 to 0.5.
	DurationTolerance float64 `json:"duration_tolerance"`
}

// Compile validates a graph and flattens it into the ordered plan. Guards
// attach to the next action step, so `probe -> if_codec_not -> transcode`
// compiles to a plan whose transcode step carries the codec condition.
func Compile(g Graph) (Plan, error) {
	if errs := Validate(g); len(errs) > 0 {
		return Plan{}, fmt.Errorf("flow: invalid graph: %s", joinErrors(errs))
	}
	byID := make(map[string]int, len(g.Nodes))
	for i, n := range g.Nodes {
		byID[n.ID] = i
	}
	c := walkChain(g, chainStart(g, byID), byID)

	var pending []Condition
	plan := Plan{}
	for _, id := range c.order {
		n := g.Nodes[byID[id]]
		switch n.Type {
		case StepIfCodecNot:
			var p ifCodecNotParams
			if err := decodeParams(n.Params, &p); err != nil {
				return Plan{}, fmt.Errorf("flow: step %s: %w", n.ID, err)
			}
			pending = append(pending, Condition{Kind: StepIfCodecNot, Codec: p.Codec})
		case StepIfBitrateAbove:
			var p ifBitrateAboveParams
			if err := decodeParams(n.Params, &p); err != nil {
				return Plan{}, fmt.Errorf("flow: step %s: %w", n.ID, err)
			}
			pending = append(pending, Condition{Kind: StepIfBitrateAbove, BitrateMBPS: p.MBPS})
		case StepIfResolutionAbove:
			var p ifResolutionAboveParams
			if err := decodeParams(n.Params, &p); err != nil {
				return Plan{}, fmt.Errorf("flow: step %s: %w", n.ID, err)
			}
			pending = append(pending, Condition{Kind: StepIfResolutionAbove, Height: p.Height})
		case StepIfAudioChannelsAbove:
			var p ifAudioChannelsAboveParams
			if err := decodeParams(n.Params, &p); err != nil {
				return Plan{}, fmt.Errorf("flow: step %s: %w", n.ID, err)
			}
			pending = append(pending, Condition{Kind: StepIfAudioChannelsAbove, Channels: p.Channels})
		case StepIfHDR:
			var p ifHDRParams
			if err := decodeParams(n.Params, &p); err != nil {
				return Plan{}, fmt.Errorf("flow: step %s: %w", n.ID, err)
			}
			if p.IsHDR == nil {
				return Plan{}, fmt.Errorf("flow: step %s: is_hdr is required", n.ID)
			}
			pending = append(pending, Condition{Kind: StepIfHDR, IsHDR: *p.IsHDR})
		default:
			plan.Steps = append(plan.Steps, PlanStep{
				ID:     n.ID,
				Type:   n.Type,
				Params: n.Params,
				When:   pending,
			})
			pending = nil
			// health_check and move ride along outside the steps list:
			// they act after the output is produced (task 24), so the plan
			// carries them as plain fields. Guards are ignored here — the
			// step runs last no matter where it sits in the chain.
			switch n.Type {
			case StepHealthCheck:
				var p healthCheckParams
				if err := decodeParams(n.Params, &p); err != nil {
					return Plan{}, fmt.Errorf("flow: step %s: %w", n.ID, err)
				}
				spec := normalizeHealthCheck(p)
				plan.HealthCheck = &spec
			case StepMove:
				var p moveParams
				if err := decodeParams(n.Params, &p); err != nil {
					return Plan{}, fmt.Errorf("flow: step %s: %w", n.ID, err)
				}
				plan.MoveTo = TrimMoveDestination(p.Destination)
				plan.MoveKeepTree = p.KeepTree
			}
		}
	}
	return plan, nil
}

// DecodePluginStep decodes a plugin step's parameters (task 32). Unknown
// fields are rejected, the same as validation.
func DecodePluginStep(s PlanStep) (PluginStepParams, error) {
	var p PluginStepParams
	err := decodeParams(s.Params, &p)
	return p, err
}

// chainStart finds the entry step's index (the node without an incoming
// edge). Validate guarantees exactly one for a valid graph; this returns -1
// defensively for graphs that skipped validation.
func chainStart(g Graph, byID map[string]int) int {
	in := map[string]bool{}
	for _, e := range g.Edges {
		in[e.To] = true
	}
	for i, n := range g.Nodes {
		if !in[n.ID] {
			return i
		}
	}
	return -1
}

func joinErrors(errs []FieldError) string {
	msgs := make([]string, 0, len(errs))
	for _, e := range errs {
		msgs = append(msgs, e.Error())
	}
	return strings.Join(msgs, "; ")
}

// NotMediaReason is the plan reason for a file that is not media (task
// 34): nothing encodes, only the kind-agnostic steps (rename, move,
// skip_if_smaller, plugin) run. The UI shows it as the skip reason.
const NotMediaReason = "not media: nothing to encode"

// Effective is what a plan resolves to for one probed file: the Target the
// encode runs with (the satisfied rule and command builder speak this
// shape), plus the two flow-only outcomes.
type Effective struct {
	// Target is the effective end state after the conditions passed or
	// failed for this file. A skipped transcode leaves the video at copy
	// (quality 3: meaningless for a copy, any valid value keeps the
	// resolved target valid). For a file that is not media the Target
	// stays empty: no codec, no quality, no audio or subtitle policy.
	Target Target
	// SkipIfSmaller is set when the plan holds a skip_if_smaller step that
	// ran (it always does — it has no conditions that matter here): the
	// job's result policy becomes replace_if_smaller, winning over the
	// library column.
	SkipIfSmaller bool
	// Rename is the rename template, empty when the plan has none.
	Rename string
	// HealthCheck is the plan's verification step (task 24), nil when the
	// graph has none. It changes nothing about the encode; the worker runs
	// it after the output is produced.
	HealthCheck *HealthCheckSpec
	// MoveTo is the move destination, empty when the plan has none.
	// MoveKeepTree rebuilds the file's folder layout under it.
	MoveTo       string
	MoveKeepTree bool
	// Reason is the plan's skip reason for a file that is not media (task
	// 34, NotMediaReason); empty for media files, which report through the
	// satisfied rule instead.
	Reason string
	// Hardware is the encoder choice of the transcode step that ran
	// (task 57): "" or HardwareAuto means the global policy, any other
	// value names one encoder the step requires. HardwareStep is that
	// step's ID, so a failure can name the step. Both are empty when no
	// transcode ran.
	Hardware     string
	HardwareStep string
	// RawArgs are the raw_args steps that ran (task 58), in chain order.
	// Each names its stage (input or output). The worker splits them by
	// stage into the encoder spec (InputArgs, OutputArgs) and logs them.
	RawArgs []RawArg
}

// FreshEffective is the Effective a resolution starts from (task 32):
// video at copy, audio and subtitles at keep-all. ResolvePlan and the
// worker's step walk both start here, so the defaults live in one place.
func FreshEffective() Effective {
	return Effective{Target: Target{
		Video:     VideoTarget{Codec: CodecCopy, Quality: 3},
		Audio:     AudioTarget{Mode: AudioKeepAll},
		Subtitles: SubtitleTarget{Mode: SubsKeepAll},
	}}
}

// ApplyStep applies one plan step to eff when its conditions pass for the
// probed file info, and reports whether it ran (task 32). The worker's
// step walk calls this per plan step — with the *current* probe, which a
// step plugin's re-probe may have replaced — and runs the plugin itself
// when the step is a plugin step (a plugin step changes nothing here).
// ResolvePlan is this loop with one probe for the whole plan.
//
// kind is what the file is (task 34): a step that needs media never runs
// on a file that is not media — the caller logs that skip — and the
// kind-agnostic steps (rename, move, skip_if_smaller, plugin) apply
// regardless.
func ApplyStep(eff *Effective, s PlanStep, info probe.MediaInfo, kind probe.Kind) (bool, error) {
	if !probe.IsMedia(kind) && StepNeedsMedia(s.Type) {
		return false, nil
	}
	if len(s.When) > 0 {
		for _, c := range s.When {
			if !c.pass(info) {
				return false, nil
			}
		}
	}
	switch s.Type {
	case StepTranscodeVideo:
		var par transcodeVideoParams
		if err := decodeParams(s.Params, &par); err != nil {
			return true, fmt.Errorf("flow: step %s: %w", s.ID, err)
		}
		// Guarded only by if_codec_not means "encode unless already this
		// codec" — exactly the default satisfied rule (DECISIONS.md B2),
		// so the flag stays off.
		applyTranscode(&eff.Target.Video, par, !allCodecNot(s.When))
		eff.Hardware = par.Hardware
		eff.HardwareStep = s.ID
	case StepScale:
		var par scaleParams
		if err := decodeParams(s.Params, &par); err != nil {
			return true, fmt.Errorf("flow: step %s: %w", s.ID, err)
		}
		eff.Target.Video.Scale = scaleTarget(par)
	case StepRemuxContainer:
		var par remuxContainerParams
		if err := decodeParams(s.Params, &par); err != nil {
			return true, fmt.Errorf("flow: step %s: %w", s.ID, err)
		}
		eff.Target.Container = par.Container
	case StepConvertAudio:
		var par convertAudioParams
		if err := decodeParams(s.Params, &par); err != nil {
			return true, fmt.Errorf("flow: step %s: %w", s.ID, err)
		}
		eff.Target.Audio = AudioTarget{
			Mode:               par.Mode,
			PreferredLanguages: par.Languages,
			CompactCodec:       par.CompactCodec,
		}
		if eff.Target.Audio.CompactCodec == "" {
			eff.Target.Audio.CompactCodec = CompactCodecAAC
		}
	case StepSubtitles:
		var par subtitlesParams
		if err := decodeParams(s.Params, &par); err != nil {
			return true, fmt.Errorf("flow: step %s: %w", s.ID, err)
		}
		eff.Target.Subtitles = SubtitleTarget{
			Mode:               par.Mode,
			PreferredLanguages: par.Languages,
			ExtractRemoved:     par.ExtractRemoved,
		}
	case StepSkipIfSmaller:
		eff.SkipIfSmaller = true
	case StepRawArgs:
		var par rawArgsParams
		if err := decodeParams(s.Params, &par); err != nil {
			return true, fmt.Errorf("flow: step %s: %w", s.ID, err)
		}
		words, err := ParseRawArgs(par.Args)
		if err != nil {
			return true, fmt.Errorf("flow: step %s: %w", s.ID, err)
		}
		eff.RawArgs = append(eff.RawArgs, RawArg{
			StepID: s.ID, Stage: par.Stage, Args: words, Raw: par.Args, Note: par.Note,
		})
	case StepRename:
		var par renameParams
		if err := decodeParams(s.Params, &par); err != nil {
			return true, fmt.Errorf("flow: step %s: %w", s.ID, err)
		}
		eff.Rename = par.Template
	case StepHealthCheck:
		var par healthCheckParams
		if err := decodeParams(s.Params, &par); err != nil {
			return true, fmt.Errorf("flow: step %s: %w", s.ID, err)
		}
		spec := normalizeHealthCheck(par)
		eff.HealthCheck = &spec
	case StepMove:
		var par moveParams
		if err := decodeParams(s.Params, &par); err != nil {
			return true, fmt.Errorf("flow: step %s: %w", s.ID, err)
		}
		eff.MoveTo = TrimMoveDestination(par.Destination)
		eff.MoveKeepTree = par.KeepTree
	case StepProbe, StepPlugin:
		// probe: the worker probes before resolving the plan.
		// plugin: the worker runs it (task 32); resolution is unaffected.
	}
	return true, nil
}

// ResolvePlan evaluates the plan for one probed file and returns the
// effective Target. This is the single place flow semantics meet the
// Target shape:
//
//   - a transcode step runs only when its guards pass; when it runs it
//     becomes the effective video (reencode_if_same_codec false when it
//     was guarded by if_codec_not — the guard already excludes the same
//     codec — true otherwise, so a bitrate-gated transcode really runs);
//   - a skipped transcode leaves the video at copy (never re-encode);
//   - remux_container, convert_audio and subtitles always define their
//     Target section (they are guarded like any step, but present in
//     every valid graph for the container);
//   - skip_if_smaller and rename surface outside the Target;
//   - a plugin step (task 32) is carried in the plan and run by the
//     worker at its position; it changes nothing here.
//
// kind is what the file is, decided by extension (task 34). For a file
// that is not media the plan resolves without error even though the probe
// holds no streams: the media steps do not apply, the Target stays empty
// (no codec, no quality, no audio or subtitle policy), and the plan
// carries the skip reason NotMediaReason so the UI shows a skip with a
// reason rather than a failure.
func ResolvePlan(p Plan, info probe.MediaInfo, kind probe.Kind) (Effective, error) {
	eff := Effective{}
	if probe.IsMedia(kind) {
		eff = FreshEffective()
	} else {
		eff.Reason = NotMediaReason
	}
	for _, s := range p.Steps {
		if _, err := ApplyStep(&eff, s, info, kind); err != nil {
			return eff, err
		}
	}
	if probe.IsMedia(kind) {
		if err := eff.Target.Validate(); err != nil {
			return eff, fmt.Errorf("flow: resolved target: %w", err)
		}
	}
	return eff, nil
}

// allCodecNot reports whether every condition is a codec_not guard.
func allCodecNot(when []Condition) bool {
	for _, c := range when {
		if c.Kind != StepIfCodecNot {
			return false
		}
	}
	return len(when) > 0
}

// applyTranscode writes a transcode_video step onto the video target. It
// keeps Scale (a scale step may come before the encode in the chain), and
// replaces everything the encode owns. An empty rate_mode is quality, so the
// rate stays nil and an existing two field graph yields today's Target.
func applyTranscode(v *VideoTarget, par transcodeVideoParams, reencodeIfSameCodec bool) {
	v.Codec = par.Codec
	v.Quality = par.Quality
	v.ReencodeIfSameCodec = reencodeIfSameCodec
	v.Rate = nil
	switch par.RateMode {
	case RateModeCRF:
		v.Rate = &RateTarget{Mode: RateModeCRF, CRF: par.CRF, MaxKBPS: par.MaxKBPS}
	case RateModeBitrate:
		v.Rate = &RateTarget{Mode: RateModeBitrate, BitrateKBPS: par.BitrateKBPS, MaxKBPS: par.MaxKBPS}
	}
	v.Preset = par.Preset
}

// scaleTarget turns a scale step into its Target form, filling the defaults:
// only_if_larger true, allow_upscale false.
func scaleTarget(par scaleParams) *ScaleTarget {
	onlyIfLarger := true
	if par.OnlyIfLarger != nil {
		onlyIfLarger = *par.OnlyIfLarger
	}
	return &ScaleTarget{Height: int(par.Height), OnlyIfLarger: onlyIfLarger, AllowUpscale: par.AllowUpscale}
}

// normalizeHealthCheck fills a health_check step's zero fields with the
// defaults (task 24): mode sample, minutes 2, tolerance 0.02.
func normalizeHealthCheck(p healthCheckParams) HealthCheckSpec {
	hc := HealthCheckSpec(p)
	if hc.Mode == "" {
		hc.Mode = HealthCheckSample
	}
	if hc.Minutes == 0 {
		hc.Minutes = HealthCheckMinutesDefault
	}
	if hc.DurationTolerance == 0 {
		hc.DurationTolerance = HealthCheckToleranceDefault
	}
	return hc
}

// Target collapses the plan to the Target it always produces, ignoring the
// conditions (the static shape of the flow). It compares a stored flow
// against the stored Target for the mismatch warning, and gives transfer
// uploads their extension without a probe. A graph generated from a Target
// collapses back to exactly that Target.
func (p Plan) Target() Target {
	t := Target{
		Video:     VideoTarget{Codec: CodecCopy, Quality: 3},
		Audio:     AudioTarget{Mode: AudioKeepAll},
		Subtitles: SubtitleTarget{Mode: SubsKeepAll},
	}
	for _, s := range p.Steps {
		switch s.Type {
		case StepTranscodeVideo:
			var par transcodeVideoParams
			if err := decodeParams(s.Params, &par); err == nil {
				applyTranscode(&t.Video, par, !allCodecNot(s.When))
			}
		case StepScale:
			var par scaleParams
			if err := decodeParams(s.Params, &par); err == nil {
				t.Video.Scale = scaleTarget(par)
			}
		case StepRemuxContainer:
			var par remuxContainerParams
			if err := decodeParams(s.Params, &par); err == nil {
				t.Container = par.Container
			}
		case StepConvertAudio:
			var par convertAudioParams
			if err := decodeParams(s.Params, &par); err == nil {
				t.Audio = AudioTarget{Mode: par.Mode, PreferredLanguages: par.Languages, CompactCodec: par.CompactCodec}
				if t.Audio.CompactCodec == "" {
					t.Audio.CompactCodec = CompactCodecAAC
				}
			}
		case StepSubtitles:
			var par subtitlesParams
			if err := decodeParams(s.Params, &par); err == nil {
				t.Subtitles = SubtitleTarget{Mode: par.Mode, PreferredLanguages: par.Languages, ExtractRemoved: par.ExtractRemoved}
			}
		}
	}
	return t
}

// HasSkipIfSmaller reports whether the plan holds a skip_if_smaller step.
func (p Plan) HasSkipIfSmaller() bool {
	for _, s := range p.Steps {
		if s.Type == StepSkipIfSmaller {
			return true
		}
	}
	return false
}

// HasKindAgnosticWork reports whether the plan holds a step that acts on a
// file of any kind (task 34): rename, move or plugin. A non-media file
// only needs a job when the flow carries one of these — skip_if_smaller
// alone changes no file, it only modulates the result policy of an encode.
func (p Plan) HasKindAgnosticWork() bool {
	for _, s := range p.Steps {
		switch s.Type {
		case StepRename, StepMove, StepPlugin:
			return true
		}
	}
	return false
}

// KindAgnosticReasons renders one plain-language reason per kind-agnostic
// step of the plan (task 34): why a non-media file is queued even though
// nothing encodes.
func (p Plan) KindAgnosticReasons() []string {
	var out []string
	for _, s := range p.Steps {
		switch s.Type {
		case StepRename:
			out = append(out, "the flow renames the result")
		case StepMove:
			out = append(out, "the flow moves the file")
		case StepPlugin:
			out = append(out, "the flow runs a plugin on this file")
		}
	}
	return out
}

// RenameTemplate returns the plan's rename template, empty when none.
func (p Plan) RenameTemplate() string {
	for _, s := range p.Steps {
		if s.Type == StepRename {
			var par renameParams
			if err := decodeParams(s.Params, &par); err == nil {
				return par.Template
			}
		}
	}
	return ""
}

// ParseGraph unmarshals a Graph from JSON. It is the single entry point for
// graphs read from the database, API bodies or job specs. It does not
// validate: callers that need a valid graph call Validate (and get
// field-scoped errors back) or Compile.
func ParseGraph(data []byte) (Graph, error) {
	var g Graph
	if err := json.Unmarshal(data, &g); err != nil {
		return Graph{}, fmt.Errorf("flow: invalid graph json: %w", err)
	}
	return g, nil
}

// ParseParams decodes one step's parameters into the typed shape for its
// type. Exported for the worker and tests; unknown fields are rejected.
func ParseParams(s Step) (any, error) {
	switch s.Type {
	case StepProbe, StepSkipIfSmaller:
		var p emptyParams
		return p, decodeParams(s.Params, &p)
	case StepIfCodecNot:
		var p ifCodecNotParams
		return p, decodeParams(s.Params, &p)
	case StepIfBitrateAbove:
		var p ifBitrateAboveParams
		return p, decodeParams(s.Params, &p)
	case StepIfResolutionAbove:
		var p ifResolutionAboveParams
		return p, decodeParams(s.Params, &p)
	case StepIfAudioChannelsAbove:
		var p ifAudioChannelsAboveParams
		return p, decodeParams(s.Params, &p)
	case StepIfHDR:
		var p ifHDRParams
		return p, decodeParams(s.Params, &p)
	case StepTranscodeVideo:
		var p transcodeVideoParams
		return p, decodeParams(s.Params, &p)
	case StepScale:
		var p scaleParams
		return p, decodeParams(s.Params, &p)
	case StepRemuxContainer:
		var p remuxContainerParams
		return p, decodeParams(s.Params, &p)
	case StepConvertAudio:
		var p convertAudioParams
		return p, decodeParams(s.Params, &p)
	case StepSubtitles:
		var p subtitlesParams
		return p, decodeParams(s.Params, &p)
	case StepRename:
		var p renameParams
		return p, decodeParams(s.Params, &p)
	case StepHealthCheck:
		var p healthCheckParams
		return p, decodeParams(s.Params, &p)
	case StepMove:
		var p moveParams
		return p, decodeParams(s.Params, &p)
	case StepPlugin:
		var p pluginParams
		return p, decodeParams(s.Params, &p)
	case StepRawArgs:
		var p rawArgsParams
		return p, decodeParams(s.Params, &p)
	}
	return nil, fmt.Errorf("flow: unknown step type %q", s.Type)
}

// Describe renders one plain-language line per step of a compiled plan,
// in run order. This is the "what this does" surface: the easy screen and
// the step stack show it verbatim, so it carries no codec jargon beyond
// the friendly codec names and stays free of encoder terms.
func (p Plan) Describe() []string {
	lines := make([]string, 0, len(p.Steps))
	for _, s := range p.Steps {
		lines = append(lines, describeStep(s))
	}
	return lines
}

func describeStep(s PlanStep) string {
	guard := ""
	if len(s.When) > 0 {
		guard = describeConditions(s.When) + " "
	}
	switch s.Type {
	case StepProbe:
		return "Look inside the file to see what it is."
	case StepIfCodecNot:
		return guard + "(nothing follows)"
	case StepIfBitrateAbove, StepIfResolutionAbove, StepIfAudioChannelsAbove, StepIfHDR:
		return guard + "(nothing follows)"
	case StepTranscodeVideo:
		var p transcodeVideoParams
		_ = decodeParams(s.Params, &p)
		return guard + describeTranscode(p)
	case StepScale:
		var p scaleParams
		_ = decodeParams(s.Params, &p)
		return guard + describeScale(scaleTarget(p))
	case StepRemuxContainer:
		var p remuxContainerParams
		_ = decodeParams(s.Params, &p)
		return guard + fmt.Sprintf("Put everything into a %s file.", strings.ToUpper(p.Container))
	case StepConvertAudio:
		var p convertAudioParams
		_ = decodeParams(s.Params, &p)
		langs := languageList(p.Languages)
		switch p.Mode {
		case AudioKeepAll:
			return guard + "Keep every audio track exactly as it is."
		case AudioKeepPreferred:
			return guard + fmt.Sprintf("Keep only the audio tracks in %s; if none match, keep all.", langs)
		case AudioKeepBest:
			return guard + fmt.Sprintf("Keep one audio track: the best one in %s (most channels), or the first track.", langs)
		default: // compact
			return guard + fmt.Sprintf("Keep one audio track and convert it to %s stereo to save space.", strings.ToUpper(p.CompactCodec))
		}
	case StepSubtitles:
		var p subtitlesParams
		_ = decodeParams(s.Params, &p)
		langs := languageList(p.Languages)
		line := ""
		switch p.Mode {
		case SubsKeepAll:
			line = "Keep all subtitles."
		case SubsKeepPreferred:
			line = fmt.Sprintf("Keep only the subtitles in %s; if none match, keep all.", langs)
		default: // remove
			line = "Remove all subtitles."
			if p.ExtractRemoved {
				line += " Text subtitles are first saved as separate .srt files."
			}
		}
		return guard + line
	case StepRename:
		var p renameParams
		_ = decodeParams(s.Params, &p)
		return guard + fmt.Sprintf("Name the result %q (where {name} is the original name).", p.Template)
	case StepSkipIfSmaller:
		return guard + "Keep the original when the new file is not smaller."
	case StepHealthCheck:
		var raw healthCheckParams
		_ = decodeParams(s.Params, &raw)
		p := normalizeHealthCheck(raw)
		if p.Mode == HealthCheckFull {
			return guard + "Play the whole finished file before anything is replaced; if it does not play cleanly, the job fails."
		}
		return guard + fmt.Sprintf("Play %d minute(s) from the middle of the finished file before anything is replaced; if it does not play cleanly, the job fails.", p.Minutes)
	case StepMove:
		var p moveParams
		_ = decodeParams(s.Params, &p)
		if p.KeepTree {
			return guard + fmt.Sprintf("Move the finished file into %s, keeping its folder layout.", p.Destination)
		}
		return guard + fmt.Sprintf("Move the finished file into %s.", p.Destination)
	case StepRawArgs:
		var p rawArgsParams
		_ = decodeParams(s.Params, &p)
		return describeRawArgs(p, guard)
	case StepPlugin:
		var p pluginParams
		_ = decodeParams(s.Params, &p)
		line := fmt.Sprintf("Run plugin %s.", p.Plugin)
		if p.ContinueOnError {
			line += " If it fails, the job carries on anyway."
		}
		return guard + line
	}
	return guard + "(unknown step)"
}

// describeTranscode renders a transcode_video step. A step with only the
// original fields reads exactly as it did before task 54 (the golden test
// holds it to that).
func describeTranscode(p transcodeVideoParams) string {
	var line string
	switch p.RateMode {
	case RateModeCRF:
		line = fmt.Sprintf("Encode the video as %s at a constant quality of CRF %s (lower is better quality and a bigger file).", codecLabel(p.Codec), trimFloat(p.CRF))
	case RateModeBitrate:
		line = fmt.Sprintf("Encode the video as %s at a target of %d kbps.", codecLabel(p.Codec), p.BitrateKBPS)
	default:
		line = fmt.Sprintf("Encode the video as %s at quality %d of 5 (lower saves more space).", codecLabel(p.Codec), p.Quality)
	}
	if p.MaxKBPS > 0 {
		line += fmt.Sprintf(" Never more than %d kbps.", p.MaxKBPS)
	}
	if p.Preset != "" {
		line += fmt.Sprintf(" Speed preset %s (slower presets save more space and take longer).", p.Preset)
	}
	if p.Hardware != "" && p.Hardware != HardwareAuto {
		line += fmt.Sprintf(" Hardware: %s.", p.Hardware)
	}
	return line
}

// describeScale renders a scale step in plain language.
func describeScale(sc *ScaleTarget) string {
	size := fmt.Sprintf("%dp", sc.Height)
	switch {
	case sc.AllowUpscale:
		return fmt.Sprintf("Scale the video to %s, enlarging it when it is smaller.", size)
	case sc.OnlyIfLarger:
		return fmt.Sprintf("Scale the video down to %s when it is taller; a smaller video is left as it is.", size)
	default:
		return fmt.Sprintf("Scale the video to %s; a smaller video is not enlarged.", size)
	}
}

func describeConditions(when []Condition) string {
	parts := make([]string, 0, len(when))
	for _, c := range when {
		switch c.Kind {
		case StepIfCodecNot:
			parts = append(parts, fmt.Sprintf("the video is not %s", codecLabel(c.Codec)))
		case StepIfBitrateAbove:
			parts = append(parts, fmt.Sprintf("the video bitrate is above %s megabits per second", trimFloat(c.BitrateMBPS)))
		case StepIfResolutionAbove:
			parts = append(parts, fmt.Sprintf("the video is taller than %dp", c.Height))
		case StepIfAudioChannelsAbove:
			parts = append(parts, fmt.Sprintf("the audio has more than %s", channelCount(c.Channels)))
		case StepIfHDR:
			if c.IsHDR {
				parts = append(parts, "the video is HDR")
			} else {
				parts = append(parts, "the video is not HDR")
			}
		}
	}
	return "Only when " + strings.Join(parts, " and ") + ":"
}

// channelCount renders an audio channel count for plain language.
func channelCount(n int) string {
	if n == 1 {
		return "1 channel"
	}
	return fmt.Sprintf("%d channels", n)
}

// codecLabel maps a target codec to its friendly name. These names appear
// on the step stack and the preview panel (the advanced surfaces); the
// easy screen itself never shows them.
func codecLabel(codec string) string {
	switch codec {
	case CodecHEVC:
		return "HEVC"
	case CodecAV1:
		return "AV1"
	case CodecH264:
		return "H.264"
	case CodecCopy:
		return "a copy of the original video"
	}
	return codec
}

// languageList renders language tags for plain language. Common tags get
// their English names; anything else is shown as-is.
var languageNames = map[string]string{
	"eng": "English", "ger": "German", "deu": "German", "fre": "French",
	"fra": "French", "spa": "Spanish", "ita": "Italian", "rus": "Russian",
	"jpn": "Japanese", "chi": "Chinese", "zho": "Chinese", "kor": "Korean",
	"por": "Portuguese", "dut": "Dutch", "nld": "Dutch", "pol": "Polish",
	"tur": "Turkish", "ara": "Arabic", "hin": "Hindi", "swe": "Swedish",
	"nor": "Norwegian", "dan": "Danish", "fin": "Finnish", "und": "undefined",
}

func languageList(langs []string) string {
	if len(langs) == 0 {
		return "your preferred languages"
	}
	out := make([]string, 0, len(langs))
	for _, l := range langs {
		if name, ok := languageNames[strings.ToLower(l)]; ok {
			out = append(out, name)
		} else {
			out = append(out, l)
		}
	}
	if len(out) == 1 {
		return out[0]
	}
	return strings.Join(out[:len(out)-1], ", ") + " or " + out[len(out)-1]
}

func trimFloat(f float64) string {
	s := strconv.FormatFloat(f, 'f', -1, 64)
	return s
}

// ParamField describes one parameter of a step type: the schema the UI
// renders the palette and the step forms from, so the step list is never
// hardcoded twice (API.md task 14). Bounds are floats so fractional ranges
// (the health check's duration tolerance, task 24) reach the editor; whole
// numbers marshal unchanged. Kind "plugin" (task 32) is a select whose
// options the API fills with the installed, enabled step plugins.
type ParamField struct {
	Name     string   `json:"name"`
	Label    string   `json:"label"`
	Kind     string   `json:"kind"` // select | number | text | languages | toggle | plugin
	Options  []string `json:"options,omitempty"`
	Min      *float64 `json:"min,omitempty"`
	Max      *float64 `json:"max,omitempty"`
	Required bool     `json:"required,omitempty"`
	Default  any      `json:"default,omitempty"`
	Help     string   `json:"help,omitempty"`
}

// StepTypeInfo is one palette entry of GET /api/v1/flows/step-types.
type StepTypeInfo struct {
	Type        string `json:"type"`
	Label       string `json:"label"`
	Description string `json:"description"`
	// Condition marks guard steps: they gate the step that follows them.
	Condition bool `json:"condition"`
	// Unsafe marks a step that hands the engine words it does not model
	// (task 58): the UI styles it as a warning. Only raw_args sets it.
	Unsafe bool         `json:"unsafe,omitempty"`
	Params []ParamField `json:"params"`
}

func floatPtr(v float64) *float64 { return &v }

// intStrings renders integer options as the strings the select fields carry.
func intStrings(vs []int) []string {
	out := make([]string, 0, len(vs))
	for _, v := range vs {
		out = append(out, strconv.Itoa(v))
	}
	return out
}

// StepTypes is the v1 palette, in stack order (probe first, guards before
// the actions they gate).
func StepTypes() []StepTypeInfo {
	return []StepTypeInfo{
		{
			Type:        StepProbe,
			Label:       "Check the file",
			Description: "Look inside the file to see what it is. Always the first step.",
			Params:      nil,
		},
		{
			Type:        StepIfCodecNot,
			Label:       "If the video is not…",
			Description: "Runs the step after it only when the video is not this codec.",
			Condition:   true,
			Params: []ParamField{{
				Name: "codec", Label: "codec", Kind: "select",
				Options: []string{CodecHEVC, CodecAV1, CodecH264}, Required: true,
				Default: CodecHEVC, Help: "The codec to compare the video against.",
			}},
		},
		{
			Type:        StepIfBitrateAbove,
			Label:       "If the video bitrate is above…",
			Description: "Runs the step after it only when the video bitrate is above this many megabits per second.",
			Condition:   true,
			Params: []ParamField{{
				Name: "mbps", Label: "megabits per second", Kind: "number",
				Min: floatPtr(1), Required: true, Default: 10,
				Help: "Bitrate threshold in megabits per second.",
			}},
		},
		{
			Type:        StepIfResolutionAbove,
			Label:       "If the video is taller than…",
			Description: "Runs the step after it only when the video is taller than this many pixels. 1080 is full HD, 2160 is 4K.",
			Condition:   true,
			Params: []ParamField{{
				Name: "height", Label: "height in pixels", Kind: "number",
				Min: floatPtr(ResolutionAboveMin), Max: floatPtr(ResolutionAboveMax), Required: true, Default: 1080,
				Help: "A video counts only when it is taller than this height. 1080 is full HD, 2160 is 4K.",
			}},
		},
		{
			Type:        StepIfAudioChannelsAbove,
			Label:       "If the audio has more channels than…",
			Description: "Runs the step after it only when an audio track has more channels than this. 2 is stereo, 6 is 5.1 surround.",
			Condition:   true,
			Params: []ParamField{{
				Name: "channels", Label: "channels", Kind: "number",
				Min: floatPtr(ChannelsAboveMin), Max: floatPtr(ChannelsAboveMax), Required: true, Default: 2,
				Help: "An audio track counts only when it has more channels than this. 2 is stereo, 6 is 5.1 surround.",
			}},
		},
		{
			Type:        StepIfHDR,
			Label:       "If the video is HDR",
			Description: "Runs the step after it only when the video is HDR (or only when it is not, if turned off).",
			Condition:   true,
			Params: []ParamField{{
				Name: "is_hdr", Label: "video is HDR", Kind: "toggle", Required: true, Default: true,
				Help: "On: runs only for HDR video. Off: runs only for video that is not HDR.",
			}},
		},
		{
			Type:        StepScale,
			Label:       "Scale the video",
			Description: "Resize the video to a height, keeping its shape. Never enlarges a smaller video unless allowed.",
			Params: []ParamField{
				{Name: "height", Label: "height", Kind: "select", Options: intStrings(ScaleHeights()), Required: true, Default: 1080, Help: "The height to scale to: 2160 is 4K, 1080 is full HD."},
				{Name: "only_if_larger", Label: "only when taller than this", Kind: "toggle", Default: true, Help: "Scale only videos taller than the height. On by default."},
				{Name: "allow_upscale", Label: "allow enlarging", Kind: "toggle", Default: false, Help: "Also enlarge a video that is smaller than the height. Off by default."},
			},
		},
		{
			Type:        StepTranscodeVideo,
			Label:       "Encode the video",
			Description: "Re-encode the video with this codec and quality (1 saves the most space, 5 the least). Rate, speed and hardware are optional.",
			Params: []ParamField{
				{Name: "codec", Label: "codec", Kind: "select", Options: []string{CodecHEVC, CodecAV1, CodecH264}, Required: true, Default: CodecHEVC},
				{Name: "quality", Label: "quality", Kind: "number", Min: floatPtr(QualityMin), Max: floatPtr(QualityMax), Required: true, Default: 3},
				{Name: "rate_mode", Label: "rate control", Kind: "select", Options: []string{RateModeQuality, RateModeCRF, RateModeBitrate}, Default: RateModeQuality, Help: "quality uses the quality number above; crf holds a constant quality; bitrate aims at a size."},
				{Name: "crf", Label: "crf", Kind: "number", Min: floatPtr(CRFMin), Max: floatPtr(CRFMax), Help: "Only for crf rate control. Above 0; lower is better quality."},
				{Name: "bitrate_kbps", Label: "target bitrate (kbps)", Kind: "number", Min: floatPtr(1), Help: "Only for bitrate rate control."},
				{Name: "max_kbps", Label: "maximum bitrate (kbps)", Kind: "number", Min: floatPtr(1), Help: "Optional cap for crf or bitrate control. Leave empty for no cap."},
				{Name: "preset", Label: "speed preset", Kind: "select", Options: Presets(), Help: "Slower presets save more space and take longer. Leave empty for balanced."},
				{Name: "hardware", Label: "hardware", Kind: "select", Options: []string{HardwareAuto, HardwareQSV, HardwareVAAPI, HardwareVideoToolbox, HardwareSoftware}, Default: HardwareAuto, Help: "auto follows the global encoder policy for this step."},
			},
		},
		{
			Type:        StepRemuxContainer,
			Label:       "Choose the container",
			Description: "The file type of the result: MKV or MP4. Every flow has exactly one of these.",
			Params: []ParamField{{
				Name: "container", Label: "container", Kind: "select",
				Options: []string{ContainerMKV, ContainerMP4}, Required: true, Default: ContainerMKV,
			}},
		},
		{
			Type:        StepConvertAudio,
			Label:       "Set the audio policy",
			Description: "Keep or shrink audio tracks.",
			Params: []ParamField{
				{Name: "mode", Label: "mode", Kind: "select", Options: []string{AudioKeepAll, AudioKeepPreferred, AudioKeepBest, AudioCompact}, Required: true, Default: AudioKeepAll},
				{Name: "languages", Label: "languages", Kind: "languages", Help: "Preferred language tags, matched case-insensitively."},
				{Name: "compact_codec", Label: "compact codec", Kind: "select", Options: []string{CompactCodecAAC, CompactCodecOpus}, Default: CompactCodecAAC, Help: "Only used by the compact mode."},
			},
		},
		{
			Type:        StepSubtitles,
			Label:       "Set the subtitle policy",
			Description: "Keep or remove subtitle tracks.",
			Params: []ParamField{
				{Name: "mode", Label: "mode", Kind: "select", Options: []string{SubsKeepAll, SubsKeepPreferred, SubsRemove}, Required: true, Default: SubsKeepAll},
				{Name: "languages", Label: "languages", Kind: "languages", Help: "Preferred language tags, matched case-insensitively."},
				{Name: "extract_removed", Label: "save removed text subtitles as .srt", Kind: "toggle"},
			},
		},
		{
			Type:        StepRename,
			Label:       "Rename the result",
			Description: "Name the produced file from a template with {name} (original name) and {ext}. Applies when the result is written beside the original.",
			Params: []ParamField{{
				Name: "template", Label: "name template", Kind: "text", Required: true,
				Default: "{name}.shrunk", Help: "Example: {name}.shrunk",
			}},
		},
		{
			Type:        StepSkipIfSmaller,
			Label:       "Skip when not smaller",
			Description: "Keep the original when the new file is not smaller. Overrides the library's result policy for these jobs.",
			Params:      nil,
		},
		{
			Type:        StepHealthCheck,
			Label:       "Verify the result",
			Description: "Play the finished file before anything is replaced: if it does not play cleanly, the job fails and the original stays.",
			Params: []ParamField{
				{Name: "mode", Label: "mode", Kind: "select", Options: []string{HealthCheckSample, HealthCheckFull}, Required: true, Default: HealthCheckSample, Help: "sample plays a few minutes from the middle; full plays everything."},
				{Name: "minutes", Label: "minutes to play", Kind: "number", Min: floatPtr(HealthCheckMinutesMin), Max: floatPtr(HealthCheckMinutesMax), Default: HealthCheckMinutesDefault, Help: "Only used by the sample mode."},
				{Name: "duration_tolerance", Label: "duration tolerance", Kind: "number", Min: floatPtr(0), Max: floatPtr(HealthCheckToleranceMax), Default: HealthCheckToleranceDefault, Help: "How much shorter than the source the result may be, as a fraction (0 to 0.5)."},
			},
		},
		{
			Type:        StepMove,
			Label:       "Move the result",
			Description: "Put the finished file into a folder outside the library, keeping the folder layout when asked.",
			Params: []ParamField{
				{Name: "destination", Label: "destination folder", Kind: "text", Required: true, Help: "Absolute path outside the library, e.g. /media/shrunk."},
				{Name: "keep_tree", Label: "keep the folder layout", Kind: "toggle", Help: "Rebuild the file's folder structure under the destination."},
			},
		},
		{
			Type:        StepPlugin,
			Label:       "Run a plugin",
			Description: "Run an installed step plugin on the working copy, at this position in the chain. A later condition sees what it did.",
			Params: []ParamField{
				{Name: "plugin", Label: "plugin", Kind: "plugin", Required: true,
					Help: "An installed, enabled step plugin."},
				{Name: "continue_on_error", Label: "continue on error", Kind: "toggle",
					Help: "For logging and sidecar style plugins only: a failing step is a warning and the chain continues."},
			},
		},
		{
			Type:        StepRawArgs,
			Label:       "Raw ffmpeg arguments",
			Description: "Advanced and unsafe: add ffmpeg options that no other step offers. The words are passed to ffmpeg, never to a shell. Input and output paths, overwrite flags, devices and shell characters are refused.",
			Unsafe:      true,
			Params: []ParamField{
				{Name: "stage", Label: "where", Kind: "select", Options: []string{RawArgsStageInput, RawArgsStageOutput}, Required: true, Default: RawArgsStageOutput,
					Help: "input: options placed before the input file. output: options placed after it, before the output file."},
				{Name: "args", Label: "arguments", Kind: "text", Required: true,
					Help: "ffmpeg options only, space separated; quotes group words. No input or output path, no -y or -n, no devices, no ; | & ` or $( characters."},
				{Name: "note", Label: "note", Kind: "text",
					Help: "Why this step exists. Shown next to the arguments in the flow preview."},
			},
		},
	}
}
