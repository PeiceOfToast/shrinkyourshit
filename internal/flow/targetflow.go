// SPDX-License-Identifier: AGPL-3.0-only

package flow

import (
	"encoding/json"
	"fmt"
)

// TargetToFlow compiles a Target into its canonical flow graph (DECISIONS.md
// 6 and B3): easy mode and target-only saves keep writing the Target, and
// this compiler generates the equivalent flow so both are always present.
//
// The generated graph is a single chain:
//
//	probe -> [if_codec_not] -> [transcode_video] -> remux_container
//	      -> convert_audio -> subtitles
//
//   - video.codec "copy" produces no guard and no transcode step: the video
//     is never re-encoded (remux and track changes only);
//   - reencode_if_same_codec false (the default, DECISIONS.md B2) adds the
//     if_codec_not guard, which is exactly the satisfied rule expressed as
//     a condition: encode only when the video is not already the target
//     codec. ReencodeIfSameCodec true omits the guard, so the encode always
//     runs;
//   - rate control, speed preset and scale (task 54) are written only when
//     set, so a Target without them generates the same graph as before. A
//     scale step follows the encode and repeats its guard, so a file whose
//     encode is skipped is not scaled either;
//   - remux_container is always present: the container decides the output
//     file type, and every valid flow states it;
//   - audio and subtitle policies always present.
//
// Compile(TargetToFlow(t)) resolves back to a Target that produces byte
// identical ffmpeg arguments to applying t directly (internal/encoders
// parity test). It is a test gate, not an aspiration.
func TargetToFlow(t Target) (Graph, error) {
	if err := t.Validate(); err != nil {
		return Graph{}, fmt.Errorf("flow: %w", err)
	}

	steps := []Step{{ID: "probe", Type: StepProbe}}
	edges := []Edge{}

	guarded := t.Video.Codec != CodecCopy && !t.Video.ReencodeIfSameCodec
	if guarded {
		guard, err := marshalStep(StepIfCodecNot, ifCodecNotParams{Codec: t.Video.Codec})
		if err != nil {
			return Graph{}, err
		}
		steps = append(steps, guard)
	}
	if t.Video.Codec != CodecCopy {
		params := transcodeVideoParams{
			Codec:   t.Video.Codec,
			Quality: t.Video.Quality,
			Preset:  t.Video.Preset,
		}
		if r := t.Video.Rate; r != nil && r.Mode != RateModeQuality {
			params.RateMode = r.Mode
			params.CRF = r.CRF
			params.BitrateKBPS = r.BitrateKBPS
			params.MaxKBPS = r.MaxKBPS
		}
		transcode, err := marshalStep(StepTranscodeVideo, params)
		if err != nil {
			return Graph{}, err
		}
		steps = append(steps, transcode)
	}
	if sc := t.Video.Scale; sc != nil {
		if guarded {
			// The scale must not outlive a skipped encode: repeat the guard.
			guard, err := marshalStepID("if_codec_not_scale", StepIfCodecNot, ifCodecNotParams{Codec: t.Video.Codec})
			if err != nil {
				return Graph{}, err
			}
			steps = append(steps, guard)
		}
		onlyIfLarger := sc.OnlyIfLarger
		scale, err := marshalStep(StepScale, scaleParams{
			Height:       scaleHeight(sc.Height),
			OnlyIfLarger: &onlyIfLarger,
			AllowUpscale: sc.AllowUpscale,
		})
		if err != nil {
			return Graph{}, err
		}
		steps = append(steps, scale)
	}
	container, err := marshalStep(StepRemuxContainer, remuxContainerParams{Container: t.Container})
	if err != nil {
		return Graph{}, err
	}
	steps = append(steps, container)

	audio, err := marshalStep(StepConvertAudio, convertAudioParams{
		Mode:         t.Audio.Mode,
		Languages:    t.Audio.PreferredLanguages,
		CompactCodec: t.Audio.CompactCodec,
	})
	if err != nil {
		return Graph{}, err
	}
	steps = append(steps, audio)

	subs, err := marshalStep(StepSubtitles, subtitlesParams{
		Mode:           t.Subtitles.Mode,
		Languages:      t.Subtitles.PreferredLanguages,
		ExtractRemoved: t.Subtitles.ExtractRemoved,
	})
	if err != nil {
		return Graph{}, err
	}
	steps = append(steps, subs)

	for i := 1; i < len(steps); i++ {
		edges = append(edges, Edge{From: steps[i-1].ID, To: steps[i].ID})
	}
	return Graph{Schema: FlowSchema, Nodes: steps, Edges: edges}, nil
}

// marshalStep builds one step with canonical JSON params.
func marshalStep(typ string, params any) (Step, error) {
	return marshalStepID(stepIDFor(typ), typ, params)
}

// marshalStepID is marshalStep with an explicit id, for a generated step
// whose type repeats (the scale guard) and needs its own id.
func marshalStepID(id, typ string, params any) (Step, error) {
	raw, err := json.Marshal(params)
	if err != nil {
		return Step{}, fmt.Errorf("flow: marshal %s params: %w", typ, err)
	}
	return Step{ID: id, Type: typ, Params: raw}, nil
}

// stepIDFor gives generated steps stable, readable ids: "transcode_video"
// becomes "transcode_video", and repeated types would collide — the
// generated shapes never repeat a type, so a plain numbering suffix keeps
// it simple without extra state.
func stepIDFor(typ string) string {
	return typ
}
