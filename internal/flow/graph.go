// SPDX-License-Identifier: AGPL-3.0-only

package flow

import (
	"bytes"
	"encoding/json"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/PeiceOfToast/shrinkyourshit/internal/plugin"
)

// FlowSchema is the graph schema version. A graph carries it so older
// databases and newer builds can disagree loudly instead of silently.
const FlowSchema = 1

// The v1 step types (ARCHITECTURE.md flow schema). A graph is a single
// linear chain of these: an optional guard (`if_*`) before an action means
// the action runs only when the guard passes for the probed file.
const (
	StepProbe          = "probe"
	StepIfCodecNot     = "if_codec_not"
	StepIfBitrateAbove = "if_bitrate_above"
	StepTranscodeVideo = "transcode_video"
	StepRemuxContainer = "remux_container"
	StepConvertAudio   = "convert_audio"
	StepSubtitles      = "subtitles"
	StepRename         = "rename"
	StepSkipIfSmaller  = "skip_if_smaller"
	StepHealthCheck    = "health_check"
	StepMove           = "move"
	StepPlugin         = "plugin"
)

// Flow sources: who owns the graph on a library.
const (
	FlowSourceEasy     = "easy"     // generated from the easy screen
	FlowSourceSteps    = "steps"    // built in the step stack editor
	FlowSourceAdvanced = "advanced" // manual (target panel, later the canvas)
)

// health_check modes and bounds (task 24).
const (
	HealthCheckSample = "sample"
	HealthCheckFull   = "full"

	HealthCheckMinutesMin     = 1
	HealthCheckMinutesMax     = 120
	HealthCheckMinutesDefault = 2
	// HealthCheckToleranceMax bounds duration_tolerance; the default lets the
	// output sit 2% under the source duration.
	HealthCheckToleranceMax     = 0.5
	HealthCheckToleranceDefault = 0.02
)

// FlowSources lists the valid flow sources.
func FlowSources() []string {
	return []string{FlowSourceEasy, FlowSourceSteps, FlowSourceAdvanced}
}

// Graph is the canonical rule object (DECISIONS.md 6): easy mode, the step
// stack and the canvas all generate and edit this shape.
type Graph struct {
	Schema int    `json:"schema"`
	Nodes  []Step `json:"nodes"`
	Edges  []Edge `json:"edges"`
}

// Step is one node of the graph. Params is the raw JSON for the step type;
// Validate decodes it against the typed param structs.
type Step struct {
	ID     string          `json:"id"`
	Type   string          `json:"type"`
	Params json.RawMessage `json:"params,omitempty"`
}

// Edge orders the chain. When is reserved for the visual editor (task 15);
// v1 guards are `if_*` steps, so compile ignores a non-empty When.
type Edge struct {
	From string `json:"from"`
	To   string `json:"to"`
	When string `json:"when,omitempty"`
}

// FieldError names where a graph is wrong: a field path plus a human
// message, so the UI can attach the error to the row it belongs to.
type FieldError struct {
	Path    string `json:"path"`
	Message string `json:"message"`
}

func (e FieldError) Error() string { return e.Path + ": " + e.Message }

// The typed parameter shapes. They double as the validation target and the
// documentation of what each step takes.
type (
	// probe and skip_if_smaller take no parameters.
	emptyParams struct{}

	// if_codec_not: run the guarded step only when the video is not this
	// codec (hevc, av1 or h264).
	ifCodecNotParams struct {
		Codec string `json:"codec"`
	}

	// if_bitrate_above: run the guarded step only when the video bitrate
	// is above this many megabits per second.
	ifBitrateAboveParams struct {
		MBPS float64 `json:"mbps"`
	}

	// transcode_video: encode the video with this codec (never copy — a
	// copy video is expressed by leaving the step out) at quality 1..5.
	transcodeVideoParams struct {
		Codec   string `json:"codec"`
		Quality int    `json:"quality"`
	}

	// remux_container: the output container (mkv or mp4). Exactly one of
	// these steps is required in every graph.
	remuxContainerParams struct {
		Container string `json:"container"`
	}

	// convert_audio: the audio policy (same modes as Target.Audio).
	convertAudioParams struct {
		Mode         string   `json:"mode"`
		Languages    []string `json:"languages"`
		CompactCodec string   `json:"compact_codec"`
	}

	// subtitles: the subtitle policy (same modes as Target.Subtitles).
	subtitlesParams struct {
		Mode           string   `json:"mode"`
		Languages      []string `json:"languages"`
		ExtractRemoved bool     `json:"extract_removed"`
	}

	// rename: the file name of the produced output, built from {name}
	// (the original base name) and {ext} (the output extension). Same
	// directory as the source, no separators.
	renameParams struct {
		Template string `json:"template"`
	}

	// health_check: verify the produced file. "sample" decodes minutes from
	// the middle of the file, "full" decodes all of it. duration_tolerance is
	// the fraction of the source duration the output must still have. Zero
	// fields are legal: Validate fills them with the defaults below.
	healthCheckParams struct {
		Mode              string  `json:"mode"`
		Minutes           int     `json:"minutes"`
		DurationTolerance float64 `json:"duration_tolerance"`
	}

	// move: after a successful result, put the file under destination,
	// keeping its path relative to the library root when keep_tree is set.
	// Exactly one move step per graph.
	moveParams struct {
		Destination string `json:"destination"`
		KeepTree    bool   `json:"keep_tree"`
	}

	// plugin: run an installed step plugin against the working file
	// (task 32). The plugin is named by its manifest name; settings
	// overrides the plugin's stored settings for this step only;
	// continue_on_error (default false) keeps the chain going when the
	// step fails — for logging and sidecar style steps only, never for
	// anything that changes the file.
	pluginParams struct {
		Plugin          string            `json:"plugin"`
		Settings        map[string]string `json:"settings"`
		ContinueOnError bool              `json:"continue_on_error"`
	}

	// PluginStepParams is the exported view of a plugin step's parameters
	// (task 32): the server decodes them to check and resolve the step.
	PluginStepParams = pluginParams
)

// decodeParams strictly decodes a step's params into out, rejecting unknown
// fields so a typo in a hand-written graph is a validation error, not a
// silently ignored value.
func decodeParams(raw json.RawMessage, out any) error {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return nil // absent params are the zero struct
	}
	dec := json.NewDecoder(bytes.NewReader(trimmed))
	dec.DisallowUnknownFields()
	if err := dec.Decode(out); err != nil {
		return err
	}
	// Trailing content (two objects, garbage after the object) is also a
	// mistake.
	if dec.More() {
		return fmt.Errorf("unexpected content after the params object")
	}
	return nil
}

// Validate checks a graph completely: schema, unique ids, edge endpoints,
// one entry, a single connected chain without loops, per-type parameters
// and the one-step-per-shaping-type rule. An empty result means valid.
func Validate(g Graph) []FieldError {
	var errs []FieldError
	add := func(path, format string, args ...any) {
		errs = append(errs, FieldError{Path: path, Message: fmt.Sprintf(format, args...)})
	}
	if g.Schema != FlowSchema {
		add("schema", "want schema %d, got %d", FlowSchema, g.Schema)
	}

	byID := make(map[string]int, len(g.Nodes))
	for i, n := range g.Nodes {
		path := fmt.Sprintf("nodes[%d]", i)
		if n.ID == "" {
			add(path+".id", "every step needs an id")
			continue
		}
		if prev, dup := byID[n.ID]; dup {
			add(path+".id", "duplicate id %q (also on nodes[%d])", n.ID, prev)
			continue
		}
		byID[n.ID] = i
		if err := validateStepParams(n); err != nil {
			add(path+"."+err.path, "%s", err.message)
		}
	}

	// Edges: endpoints must exist, self loops are meaningless, and v1 is
	// a single chain: at most one edge in and one out per node.
	out := map[string][]int{}
	in := map[string][]int{}
	for j, e := range g.Edges {
		path := fmt.Sprintf("edges[%d]", j)
		if _, ok := byID[e.From]; !ok {
			add(path+".from", "unknown step %q", e.From)
		}
		if _, ok := byID[e.To]; !ok {
			add(path+".to", "unknown step %q", e.To)
		}
		if e.From == e.To && byIDOrMinusOne(byID, e.From) >= 0 {
			add(path, "a step cannot point to itself")
		}
		out[e.From] = append(out[e.From], j)
		in[e.To] = append(in[e.To], j)
	}
	for id, js := range out {
		if len(js) > 1 && byIDOrMinusOne(byID, id) >= 0 {
			add(fmt.Sprintf("nodes[%d].id", byID[id]), "step %q has more than one outgoing edge; v1 flows are a single chain", id)
		}
	}
	for id, js := range in {
		if len(js) > 1 && byIDOrMinusOne(byID, id) >= 0 {
			add(fmt.Sprintf("nodes[%d].id", byID[id]), "step %q has more than one incoming edge; v1 flows are a single chain", id)
		}
	}

	// Walk the chain from the entry to find loops and unreachable steps.
	entry := -1
	for i, n := range g.Nodes {
		if len(in[n.ID]) == 0 {
			if entry != -1 {
				add(fmt.Sprintf("nodes[%d].id", i), "step %q has no input: the flow must have exactly one starting step", n.ID)
				continue
			}
			entry = i
		}
	}
	if entry == -1 && len(g.Nodes) > 0 {
		add("graph", "the flow has no starting step (every step has an input)")
	}
	chain := walkChain(g, entry, byID)
	if chain.cycleAt != "" {
		add(fmt.Sprintf("nodes[%d].id", byID[chain.cycleAt]), "step %q is part of a loop", chain.cycleAt)
	} else {
		if entry >= 0 && len(g.Nodes) > 0 {
			if g.Nodes[entry].Type != StepProbe {
				add(fmt.Sprintf("nodes[%d].type", entry), "the flow must start with a %s step", StepProbe)
			}
			// A plugin step runs on the probed working file (task 32): a
			// probe step must come before it in the chain.
			sawProbe := false
			for _, id := range chain.order {
				n := g.Nodes[byID[id]]
				switch {
				case n.Type == StepProbe:
					sawProbe = true
				case n.Type == StepPlugin && !sawProbe:
					add(fmt.Sprintf("nodes[%d].type", byID[id]),
						"a %s step needs a %s step before it in the chain: it runs on the probed working file", StepPlugin, StepProbe)
				}
			}
			// A trailing guard guards nothing.
			if order := chain.order; len(order) > 0 {
				if last := order[len(order)-1]; isGuard(g.Nodes[byID[last]].Type) {
					add(fmt.Sprintf("nodes[%d].id", byID[last]), "a condition step needs a step after it")
				}
			}
		}
		for i, n := range g.Nodes {
			if !chain.seen[n.ID] {
				add(fmt.Sprintf("nodes[%d].id", i), "step %q is not connected to the flow", n.ID)
			}
		}
	}

	// Shaping steps are unique so a graph always describes exactly one
	// video, container, audio, subtitle, rename and skip policy.
	counts := map[string]int{}
	for _, n := range g.Nodes {
		if isShaping(n.Type) {
			counts[n.Type]++
		}
	}
	for _, n := range g.Nodes {
		if counts[n.Type] > 1 {
			add(fmt.Sprintf("nodes[%d].type", byID[n.ID]), "only one %s step is allowed", n.Type)
		}
	}
	// The container decides the output file type; every flow states it,
	// which also rejects an empty graph (no steps can run anything).
	if counts[StepRemuxContainer] == 0 {
		add("graph", "the flow needs exactly one %s step", StepRemuxContainer)
	}
	return errs
}

func byIDOrMinusOne(byID map[string]int, id string) int {
	if i, ok := byID[id]; ok {
		return i
	}
	return -1
}

func isGuard(t string) bool { return t == StepIfCodecNot || t == StepIfBitrateAbove }

// isShaping reports whether the step type must appear at most once (and,
// for remux_container, exactly once) in a graph.
func isShaping(t string) bool {
	switch t {
	case StepTranscodeVideo, StepRemuxContainer, StepConvertAudio, StepSubtitles,
		StepRename, StepSkipIfSmaller, StepHealthCheck, StepMove:
		return true
	}
	return false
}

// paramError is a field-scoped validation failure inside one step.
type paramError struct {
	path    string // relative to the step node, e.g. "params.codec"
	message string
}

func (e *paramError) Error() string { return e.path + ": " + e.message }

func perr(path, format string, args ...any) *paramError {
	return &paramError{path: path, message: fmt.Sprintf(format, args...)}
}

// validateStepParams decodes and checks one step's parameters. The returned
// error path is relative to the node ("params.codec").
func validateStepParams(n Step) *paramError {
	switch n.Type {
	case StepProbe:
		var p emptyParams
		if err := decodeParams(n.Params, &p); err != nil {
			return perr("params", "%s takes no parameters (%s)", StepProbe, err)
		}
		if hasContent(n.Params) {
			return perr("params", "%s takes no parameters", StepProbe)
		}
		return nil
	case StepSkipIfSmaller:
		var p emptyParams
		if err := decodeParams(n.Params, &p); err != nil {
			return perr("params", "%s takes no parameters (%s)", StepSkipIfSmaller, err)
		}
		if hasContent(n.Params) {
			return perr("params", "%s takes no parameters", StepSkipIfSmaller)
		}
		return nil
	case StepIfCodecNot:
		var p ifCodecNotParams
		if err := decodeParams(n.Params, &p); err != nil {
			return perr("params", "%s", err)
		}
		switch p.Codec {
		case CodecHEVC, CodecAV1, CodecH264:
		default:
			return perr("params.codec", "want hevc, av1 or h264, got %q", p.Codec)
		}
		return nil
	case StepIfBitrateAbove:
		var p ifBitrateAboveParams
		if err := decodeParams(n.Params, &p); err != nil {
			return perr("params", "%s", err)
		}
		if p.MBPS <= 0 {
			return perr("params.mbps", "want a number above 0, got %v", p.MBPS)
		}
		return nil
	case StepTranscodeVideo:
		var p transcodeVideoParams
		if err := decodeParams(n.Params, &p); err != nil {
			return perr("params", "%s", err)
		}
		switch p.Codec {
		case CodecHEVC, CodecAV1, CodecH264:
		default:
			return perr("params.codec", "want hevc, av1 or h264, got %q (copy is expressed by leaving this step out)", p.Codec)
		}
		if p.Quality < QualityMin || p.Quality > QualityMax {
			return perr("params.quality", "want %d to %d, got %d", QualityMin, QualityMax, p.Quality)
		}
		return nil
	case StepRemuxContainer:
		var p remuxContainerParams
		if err := decodeParams(n.Params, &p); err != nil {
			return perr("params", "%s", err)
		}
		switch p.Container {
		case ContainerMKV, ContainerMP4:
		default:
			return perr("params.container", "want mkv or mp4, got %q", p.Container)
		}
		return nil
	case StepConvertAudio:
		var p convertAudioParams
		if err := decodeParams(n.Params, &p); err != nil {
			return perr("params", "%s", err)
		}
		switch p.Mode {
		case AudioKeepAll, AudioKeepPreferred, AudioKeepBest, AudioCompact:
		default:
			return perr("params.mode", "want keep_all, keep_preferred, keep_best or compact, got %q", p.Mode)
		}
		if err := validateLanguages(p.Languages); err != nil {
			return perr("params.languages", "%s", err)
		}
		if p.Mode == AudioCompact {
			switch p.CompactCodec {
			case CompactCodecAAC, CompactCodecOpus:
			default:
				return perr("params.compact_codec", "want aac or opus, got %q", p.CompactCodec)
			}
		}
		return nil
	case StepSubtitles:
		var p subtitlesParams
		if err := decodeParams(n.Params, &p); err != nil {
			return perr("params", "%s", err)
		}
		switch p.Mode {
		case SubsKeepAll, SubsKeepPreferred, SubsRemove:
		default:
			return perr("params.mode", "want keep_all, keep_preferred or remove, got %q", p.Mode)
		}
		if err := validateLanguages(p.Languages); err != nil {
			return perr("params.languages", "%s", err)
		}
		return nil
	case StepRename:
		var p renameParams
		if err := decodeParams(n.Params, &p); err != nil {
			return perr("params", "%s", err)
		}
		t := p.Template
		switch {
		case strings.TrimSpace(t) == "":
			return perr("params.template", "a file name template is required")
		case strings.ContainsAny(t, "/\\"):
			return perr("params.template", "the template names a file inside the same folder; separators are not allowed")
		case strings.Contains(t, ".."):
			return perr("params.template", "the template must not contain %q", "..")
		case !strings.Contains(t, "{name}"):
			return perr("params.template", `the template must contain {name} so every file keeps its own name`)
		}
		return nil
	case StepHealthCheck:
		var p healthCheckParams
		if err := decodeParams(n.Params, &p); err != nil {
			return perr("params", "%s", err)
		}
		// Zero fields are the defaults; only real values are checked.
		mode := p.Mode
		if mode == "" {
			mode = HealthCheckSample
		}
		switch mode {
		case HealthCheckSample, HealthCheckFull:
		default:
			return perr("params.mode", "want sample or full, got %q", p.Mode)
		}
		minutes := p.Minutes
		if minutes == 0 {
			minutes = HealthCheckMinutesDefault
		}
		if minutes < HealthCheckMinutesMin || minutes > HealthCheckMinutesMax {
			return perr("params.minutes", "want %d to %d, got %d", HealthCheckMinutesMin, HealthCheckMinutesMax, p.Minutes)
		}
		tolerance := p.DurationTolerance
		if tolerance == 0 {
			tolerance = HealthCheckToleranceDefault
		}
		if tolerance < 0 || tolerance > HealthCheckToleranceMax {
			return perr("params.duration_tolerance", "want 0 to %v, got %v", HealthCheckToleranceMax, p.DurationTolerance)
		}
		return nil
	case StepMove:
		var p moveParams
		if err := decodeParams(n.Params, &p); err != nil {
			return perr("params", "%s", err)
		}
		d := TrimMoveDestination(p.Destination)
		switch {
		case d == "":
			return perr("params.destination", "a destination folder is required")
		case !filepath.IsAbs(d):
			return perr("params.destination", "the destination must be an absolute path, got %q", p.Destination)
		}
		return nil
	case StepPlugin:
		var p pluginParams
		if err := decodeParams(n.Params, &p); err != nil {
			return perr("params", "%s", err)
		}
		// The name is required and must satisfy the manifest's name rule
		// (task 32: plugin's own check is reused, so a flow can never
		// name a plugin the installer would refuse).
		if !plugin.ValidName(p.Plugin) {
			return perr("params.plugin", "the name of an installed step plugin is required (lowercase letters, digits and dashes)")
		}
		// Settings keys must be non-empty and look like manifest setting
		// keys: every value becomes the environment variable its key
		// names, so the key must be exportable. Values may be empty.
		for _, key := range sortedKeys(p.Settings) {
			switch {
			case strings.TrimSpace(key) == "":
				return perr("params.settings", "a settings entry needs a key")
			case !plugin.ValidEnvName(key):
				return perr("params.settings."+key, "%q is not a valid setting key: settings keys name the environment variable the value becomes", key)
			}
		}
		return nil
	default:
		return perr("type", "unknown step type %q", n.Type)
	}
}

// sortedKeys returns a map's keys in a stable order so validation errors
// are reproducible.
func sortedKeys(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// hasContent reports whether raw holds anything other than an empty object.
func hasContent(raw json.RawMessage) bool {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return false
	}
	var m map[string]any
	if err := json.Unmarshal(trimmed, &m); err != nil {
		return true // not even an object; decodeParams already reported it
	}
	return len(m) > 0
}

func validateLanguages(langs []string) error {
	for i, l := range langs {
		if strings.TrimSpace(l) == "" {
			return fmt.Errorf("language %d is empty", i)
		}
	}
	return nil
}

// TrimMoveDestination trims spaces and trailing slashes from a move
// destination: the canonical form the plan, the spec and the result policy
// all use (task 24).
func TrimMoveDestination(d string) string {
	return strings.TrimRight(strings.TrimSpace(d), "/")
}

// MoveDestination returns a move step's trimmed destination, empty when the
// step is not a move or its params do not decode. The server checks it
// against the library root at save time.
func MoveDestination(s Step) string {
	if s.Type != StepMove {
		return ""
	}
	var p moveParams
	if err := decodeParams(s.Params, &p); err != nil {
		return ""
	}
	return TrimMoveDestination(p.Destination)
}

// KeepMoveTree reports whether a move step rebuilds the file's folder
// layout under the destination.
func KeepMoveTree(s Step) bool {
	if s.Type != StepMove {
		return false
	}
	var p moveParams
	if err := decodeParams(s.Params, &p); err != nil {
		return false
	}
	return p.KeepTree
}

// chain is the result of walking the single chain from the entry step.
type chain struct {
	order   []string // step ids in chain order
	seen    map[string]bool
	cycleAt string // non-empty when the walk revisited a step
}

// walkChain follows single in/out edges from the entry. A revisited step
// ends the walk with cycleAt set.
func walkChain(g Graph, entry int, byID map[string]int) chain {
	c := chain{seen: map[string]bool{}}
	if entry < 0 || entry >= len(g.Nodes) {
		return c
	}
	next := map[string]string{}
	for _, e := range g.Edges {
		next[e.From] = e.To // single-out validated elsewhere
	}
	cur := g.Nodes[entry].ID
	for {
		if c.seen[cur] {
			c.cycleAt = cur
			return c
		}
		c.seen[cur] = true
		c.order = append(c.order, cur)
		nxt, ok := next[cur]
		if !ok {
			return c
		}
		cur = nxt
	}
}
