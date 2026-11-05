// SPDX-License-Identifier: AGPL-3.0-only

package flow

import "fmt"

// Tier is the editor a library offers and the set of steps that editor may
// write (spec section 6). It is a property of the library, not of the graph:
// changing it never rewrites the stored graph (decision 4).
type Tier string

// The three rule tiers. Values are stored in libraries.rule_tier and in the
// default_rule_tier setting; do not add values without updating
// ARCHITECTURE.md.
const (
	TierBasic    Tier = "basic"
	TierModerate Tier = "moderate"
	TierAdvanced Tier = "advanced"
)

// TierValues lists the valid tiers, lowest to highest.
func TierValues() []Tier {
	return []Tier{TierBasic, TierModerate, TierAdvanced}
}

// ParseTier returns the tier named s, or an error naming the valid values.
func ParseTier(s string) (Tier, error) {
	for _, t := range TierValues() {
		if string(t) == s {
			return t, nil
		}
	}
	return "", fmt.Errorf("invalid rule tier %q: want one of %v", s, TierValues())
}

// easyCodecs are the video codecs the easy screen produces (CompileEasyMode).
var easyCodecs = map[string]bool{CodecHEVC: true, CodecAV1: true, CodecH264: true}

// easyAudioModes are the audio modes the easy slider produces.
var easyAudioModes = map[string]bool{
	AudioKeepAll: true, AudioKeepPreferred: true, AudioKeepBest: true, AudioCompact: true,
}

// RepresentableAt lists every step of g that tier cannot express, one field
// error per step, in node order. An empty result means the tier can edit the
// whole graph. The rules (spec section 6):
//
//   - basic: only what CompileEasyMode regenerates: probe, the if_codec_not
//     guard on the same codec as the encode, one transcode_video with quality
//     only (no rate control, preset or hardware override), remux_container to
//     mkv, convert_audio with the four easy modes, and subtitles keep_all or
//     remove. Any condition, scale, rename, move, plugin, raw_args, health
//     check or skip step is not representable.
//   - moderate: everything except the two escape hatches, raw_args and plugin.
//   - advanced: everything.
//
// Each field error names the step id and the lowest tier that can edit it, so
// the UI can say which editor to open. Nothing here changes the graph.
func RepresentableAt(g Graph, tier Tier) []FieldError {
	switch tier {
	case TierAdvanced:
		return nil
	case TierModerate, TierBasic:
	default:
		return []FieldError{{Path: "tier", Message: fmt.Sprintf("invalid rule tier %q: want one of %v", tier, TierValues())}}
	}

	var errs []FieldError
	for i, n := range g.Nodes {
		if stepRepresentable(n, tier) {
			continue
		}
		errs = append(errs, FieldError{
			Path: fmt.Sprintf("nodes[%d].type", i),
			Message: fmt.Sprintf("step %q (%s) is not representable in the %s tier; the %s tier can edit it",
				n.ID, n.Type, tier, lowestTierFor(n)),
		})
	}
	if tier == TierBasic {
		errs = append(errs, basicGraphErrors(g)...)
	}
	return errs
}

// lowestTierFor is the lowest tier that can express a step: moderate for
// everything except the escape hatches, which only advanced can edit.
func lowestTierFor(n Step) Tier {
	if stepRepresentable(n, TierModerate) {
		return TierModerate
	}
	return TierAdvanced
}

// stepRepresentable reports whether tier (basic or moderate) can express the
// step. Advanced expresses every step, so it is not asked here.
func stepRepresentable(n Step, tier Tier) bool {
	switch n.Type {
	case StepRawArgs, StepPlugin:
		return false
	}
	if tier == TierModerate {
		return true
	}
	return basicStepRepresentable(n)
}

// basicStepRepresentable is the basic tier's per-step rule: the step is one
// that CompileEasyMode writes, with the values the easy screen can produce.
func basicStepRepresentable(n Step) bool {
	switch n.Type {
	case StepProbe:
		return true
	case StepIfCodecNot:
		var p ifCodecNotParams
		return decodeParams(n.Params, &p) == nil && easyCodecs[p.Codec]
	case StepTranscodeVideo:
		var p transcodeVideoParams
		if decodeParams(n.Params, &p) != nil {
			return false
		}
		return easyCodecs[p.Codec] &&
			(p.RateMode == "" || p.RateMode == RateModeQuality) &&
			p.CRF == 0 && p.BitrateKBPS == 0 && p.MaxKBPS == 0 &&
			p.Preset == "" &&
			(p.Hardware == "" || p.Hardware == HardwareAuto)
	case StepRemuxContainer:
		var p remuxContainerParams
		return decodeParams(n.Params, &p) == nil && p.Container == ContainerMKV
	case StepConvertAudio:
		var p convertAudioParams
		return decodeParams(n.Params, &p) == nil && easyAudioModes[p.Mode] &&
			(p.CompactCodec == "" || p.CompactCodec == CompactCodecAAC)
	case StepSubtitles:
		var p subtitlesParams
		return decodeParams(n.Params, &p) == nil &&
			(p.Mode == SubsKeepAll || p.Mode == SubsRemove) && !p.ExtractRemoved
	}
	return false
}

// basicGraphErrors are the basic tier's whole-graph rules: the easy screen
// always encodes video and always keeps the satisfied rule as a guard on the
// encode's codec, so a graph without that shape is not regenerable.
func basicGraphErrors(g Graph) []FieldError {
	var transcodeCodec string
	var guardCodec string
	haveTranscode, haveGuard := false, false
	for _, n := range g.Nodes {
		switch n.Type {
		case StepTranscodeVideo:
			var p transcodeVideoParams
			if decodeParams(n.Params, &p) == nil && !haveTranscode {
				transcodeCodec = p.Codec
			}
			haveTranscode = true
		case StepIfCodecNot:
			var p ifCodecNotParams
			if decodeParams(n.Params, &p) == nil && !haveGuard {
				guardCodec = p.Codec
			}
			haveGuard = true
		}
	}
	var errs []FieldError
	if !haveTranscode {
		errs = append(errs, FieldError{Path: "graph", Message: "the basic tier always encodes video: the flow needs a transcode_video step"})
	}
	if !haveGuard || guardCodec != transcodeCodec {
		errs = append(errs, FieldError{Path: "graph", Message: "the basic tier skips files already in the target codec: the flow needs an if_codec_not step on the same codec as the encode"})
	}
	return errs
}
