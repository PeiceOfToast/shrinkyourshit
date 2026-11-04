// SPDX-License-Identifier: AGPL-3.0-only

package flow

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/PeiceOfToast/shrinkyourshit/internal/probe"
	"github.com/stretchr/testify/require"
)

// rawGraph is probe → raw_args(stage, args) → remux (mkv): the smallest
// valid graph with one raw_args step at index 1 of Nodes.
func rawGraph(t *testing.T, stage, args string) Graph {
	t.Helper()
	params, err := json.Marshal(map[string]any{"stage": stage, "args": args, "note": "test"})
	require.NoError(t, err)
	return Graph{
		Schema: FlowSchema,
		Nodes: []Step{
			{ID: "probe", Type: StepProbe},
			{ID: "raw", Type: StepRawArgs, Params: params},
			{ID: "remux", Type: StepRemuxContainer, Params: json.RawMessage(`{"container":"mkv"}`)},
		},
		Edges: []Edge{{From: "probe", To: "raw"}, {From: "raw", To: "remux"}},
	}
}

// errorAt returns the message of the field error at path, or fails the test.
func errorAt(t *testing.T, errs []FieldError, path string) string {
	t.Helper()
	for _, e := range errs {
		if e.Path == path {
			return e.Message
		}
	}
	require.Failf(t, "no field error", "want an error at %q, got %v", path, errs)
	return ""
}

/* ------------------------------------------------------------------ */
/* Tokeniser                                                           */
/* ------------------------------------------------------------------ */

func TestSplitRawArgsWords(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want []string
	}{
		{"plain words", "-preset  fast\t-crf 20", []string{"-preset", "fast", "-crf", "20"}},
		{"single quotes keep spaces and backslashes", `-metadata 'title=a b\c'`, []string{"-metadata", `title=a b\c`}},
		{"double quotes keep spaces", `-metadata "title=My Track"`, []string{"-metadata", "title=My Track"}},
		{"double quote escapes", `-metadata "title=say \"hi\""`, []string{"-metadata", `title=say "hi"`}},
		{"double quote keeps other backslashes", `-x "a\nb"`, []string{"-x", `a\nb`}},
		{"backslash escapes a space outside quotes", `-metadata title=a\ b`, []string{"-metadata", "title=a b"}},
		{"empty quoted word is a word", `-metadata ""`, []string{"-metadata", ""}},
		{"adjacent quoting joins one word", `-metadata:s:a:0 title="My "'Track'`, []string{"-metadata:s:a:0", "title=My Track"}},
		{"empty input", "   ", nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := splitRawArgs(c.in)
			require.NoError(t, err)
			require.Equal(t, c.want, got)
		})
	}
}

func TestSplitRawArgsUnbalancedQuote(t *testing.T) {
	for _, in := range []string{`-metadata "title=open`, `-metadata 'title=open`} {
		_, err := splitRawArgs(in)
		require.Error(t, err, in)
		require.Contains(t, err.Error(), "unbalanced")
	}
	_, err := splitRawArgs(`-x a\`)
	require.Error(t, err)
	require.Contains(t, err.Error(), "backslash")
}

func TestParseRawArgsWellFormed(t *testing.T) {
	words, err := ParseRawArgs(`-vf scale=-2:1080 -preset fast -metadata:s:a:0 title="Director's cut"`)
	require.NoError(t, err)
	require.Equal(t, []string{"-vf", "scale=-2:1080", "-preset", "fast", "-metadata:s:a:0", "title=Director's cut"}, words)

	_, err = ParseRawArgs("-thread_queue_size 512 -an")
	require.NoError(t, err)
}

/* ------------------------------------------------------------------ */
/* Denylist: every rejected pattern produces a FieldError at its path  */
/* ------------------------------------------------------------------ */

func TestRawArgsRejectedPatternsFieldErrors(t *testing.T) {
	cases := []struct {
		name  string
		stage string
		args  string
		want  string // substring of the message
	}{
		{"input path with -i", RawArgsStageOutput, "-i /etc/other.mkv", "-i sets the input file"},
		{"input path with -i on the input stage", RawArgsStageInput, "-i other.mkv", "-i sets the input file"},
		{"bare output file name", RawArgsStageOutput, "-crf 20 /media/out.mkv", `"/media/out.mkv" is a bare word`},
		{"bare word first", RawArgsStageOutput, "out.mkv", `"out.mkv" is a bare word`},
		{"bare word after a boolean option", RawArgsStageOutput, "-an out.mkv", `"out.mkv" is a bare word`},
		{"overwrite with -y", RawArgsStageOutput, "-y -preset fast", "-y changes how the output file is overwritten"},
		{"overwrite refusal with -n", RawArgsStageOutput, "-n", "-n changes how the output file is overwritten"},
		{"device format lavfi", RawArgsStageInput, "-f lavfi", "-f lavfi reads from a device"},
		{"device format v4l2", RawArgsStageInput, "-f v4l2", "-f v4l2 reads from a device"},
		{"device path", RawArgsStageInput, "-f /dev/video0", "-f /dev/video0 reads from a device"},
		{"semicolon", RawArgsStageOutput, "-vf scale=1:1;null", `shell character ";"`},
		{"pipe", RawArgsStageOutput, "-metadata title=a|b", `shell character "|"`},
		{"ampersand", RawArgsStageOutput, "-metadata title=a&b", `shell character "&"`},
		{"backtick", RawArgsStageOutput, "-metadata title=`id`", "shell character \"`\""},
		{"command substitution", RawArgsStageOutput, "-metadata title=$(id)", `shell character "$("`},
		{"newline", RawArgsStageOutput, "-preset fast\n-crf 20", "newlines are not allowed"},
		{"carriage return", RawArgsStageOutput, "-preset fast\r-crf 20", "newlines are not allowed"},
		{"unbalanced quote", RawArgsStageOutput, `-metadata "title=open`, "unbalanced double quote"},
		{"empty arguments", RawArgsStageOutput, "   ", "the arguments are empty"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			g := rawGraph(t, c.stage, c.args)
			errs := Validate(g)
			msg := errorAt(t, errs, "nodes[1].params.args")
			require.Contains(t, msg, c.want)
			// The error reaches the caller through Compile too.
			_, err := Compile(g)
			require.Error(t, err)
		})
	}
}

func TestRawArgsLengthLimit(t *testing.T) {
	long := "-preset " + strings.Repeat("a", RawArgsMaxBytes)
	errs := Validate(rawGraph(t, RawArgsStageOutput, long))
	require.Contains(t, errorAt(t, errs, "nodes[1].params.args"), "longer than")
}

func TestRawArgsInvalidStageAndNote(t *testing.T) {
	g := rawGraph(t, "middle", "-preset fast")
	require.Contains(t, errorAt(t, Validate(g), "nodes[1].params.stage"), `want input or output, got "middle"`)

	params, err := json.Marshal(map[string]any{
		"stage": RawArgsStageOutput, "args": "-preset fast", "note": strings.Repeat("n", RawArgsNoteMax+1),
	})
	require.NoError(t, err)
	g = rawGraph(t, RawArgsStageOutput, "-preset fast")
	g.Nodes[1].Params = params
	require.Contains(t, errorAt(t, Validate(g), "nodes[1].params.note"), "at most")
}

func TestRawArgsUnknownFieldRejected(t *testing.T) {
	g := rawGraph(t, RawArgsStageOutput, "-preset fast")
	g.Nodes[1].Params = json.RawMessage(`{"stage":"output","args":"-preset fast","shell":true}`)
	errs := Validate(g)
	require.Equal(t, "nodes[1].params", errs[0].Path)
}

/* ------------------------------------------------------------------ */
/* Plan: stage, Describe verbatim, and the media gate                  */
/* ------------------------------------------------------------------ */

func TestRawArgsWellFormedStepInPlanAndDescribe(t *testing.T) {
	const raw = `-vf "scale=-2:1080" -preset fast`
	g := rawGraph(t, RawArgsStageOutput, raw)
	require.Empty(t, Validate(g))

	plan, err := Compile(g)
	require.NoError(t, err)
	require.Len(t, plan.Steps, 3)
	require.Equal(t, StepRawArgs, plan.Steps[1].Type)
	require.Equal(t, "raw", plan.Steps[1].ID)

	lines := plan.Describe()
	require.Len(t, lines, 3)
	require.Contains(t, lines[1], "UNSAFE raw ffmpeg arguments")
	require.Contains(t, lines[1], "as output options")
	require.Contains(t, lines[1], raw, "Describe prints the raw string verbatim")
	require.Contains(t, lines[1], "Note: test")

	eff, err := ResolvePlan(plan, probe.MediaInfo{Video: []probe.VideoStream{{Codec: "h264", Height: 1080}}}, probe.KindVideo)
	require.NoError(t, err)
	require.Equal(t, []RawArg{{
		StepID: "raw", Stage: RawArgsStageOutput,
		Args: []string{"-vf", "scale=-2:1080", "-preset", "fast"},
		Raw:  raw, Note: "test",
	}}, eff.RawArgs)
	// The Target never changes because of raw arguments.
	require.Equal(t, plan.Target(), eff.Target)
}

func TestRawArgsInputStageAtItsPosition(t *testing.T) {
	g := rawGraph(t, RawArgsStageInput, "-thread_queue_size 512")
	plan, err := Compile(g)
	require.NoError(t, err)
	require.Contains(t, plan.Describe()[1], "as input options, before the input file")

	eff, err := ResolvePlan(plan, probe.MediaInfo{Video: []probe.VideoStream{{Codec: "h264"}}}, probe.KindVideo)
	require.NoError(t, err)
	require.Len(t, eff.RawArgs, 1)
	require.Equal(t, RawArgsStageInput, eff.RawArgs[0].Stage)
	require.Equal(t, []string{"-thread_queue_size", "512"}, eff.RawArgs[0].Args)
}

func TestRawArgsSkippedOnNonMedia(t *testing.T) {
	require.True(t, StepNeedsMedia(StepRawArgs), "raw arguments are ffmpeg options: media only")
	plan, err := Compile(rawGraph(t, RawArgsStageOutput, "-preset fast"))
	require.NoError(t, err)
	var eff Effective
	ran, err := ApplyStep(&eff, plan.Steps[1], probe.MediaInfo{}, probe.KindOther)
	require.NoError(t, err)
	require.False(t, ran)
	require.Empty(t, eff.RawArgs)
}

func TestRawArgsRefusalsListEveryStep(t *testing.T) {
	require.Empty(t, RawArgsRefusals(Graph{Schema: FlowSchema}), "no raw_args step, no refusal")
	g := rawGraph(t, RawArgsStageOutput, "-preset fast")
	require.Len(t, RawArgsRefusals(g), 1)
	require.Equal(t, "nodes[1].type", RawArgsRefusals(g)[0].Path)

	plain := Graph{Schema: FlowSchema, Nodes: []Step{{ID: "probe", Type: StepProbe}}}
	require.Empty(t, RawArgsRefusals(plain))
}

// TestStepTypesIncludeRawArgsUnsafe: the palette carries the step with the
// unsafe marker, and only that step carries it.
func TestStepTypesIncludeRawArgsUnsafe(t *testing.T) {
	var found bool
	for _, st := range StepTypes() {
		if st.Type == StepRawArgs {
			found = true
			require.True(t, st.Unsafe)
			require.False(t, st.Condition)
			require.Len(t, st.Params, 3)
			require.Equal(t, "stage", st.Params[0].Name)
			require.Equal(t, []string{RawArgsStageInput, RawArgsStageOutput}, st.Params[0].Options)
			require.Equal(t, "args", st.Params[1].Name)
			continue
		}
		require.False(t, st.Unsafe, "only raw_args is unsafe, got %s", st.Type)
	}
	require.True(t, found, "raw_args must be in the palette")

	raw, err := json.Marshal(StepTypes()[0])
	require.NoError(t, err)
	require.NotContains(t, string(raw), `"unsafe"`, "the marker is omitted on safe steps")
}
