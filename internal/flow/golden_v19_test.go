// SPDX-License-Identifier: AGPL-3.0-only

package flow

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/PeiceOfToast/shrinkyourshit/internal/probe"
	"github.com/stretchr/testify/require"
)

// The v1.9 golden: a graph saved by the release before the encode settings
// tiers wave (task 54) must still parse, validate, resolve and describe
// exactly as it did. The golden file was captured from the v1.9 code
// (main at e824b7d) and is only regenerated with FLOW_UPDATE_GOLDEN=1, which
// must never be set to make a behaviour change pass.
const v19GoldenPath = "testdata/v19_golden.json"

// v19Golden is every observable output of the flow engine for the fixture
// graphs. Targets are compared as their exact marshaled bytes.
type v19Golden struct {
	Fixture struct {
		Validate    []string `json:"validate"`
		Target      string   `json:"target"`
		Describe    []string `json:"describe"`
		Rename      string   `json:"rename"`
		HealthCheck string   `json:"health_check"`
		MoveTo      string   `json:"move_to"`
		KeepTree    bool     `json:"keep_tree"`
		SkipIfSmall bool     `json:"skip_if_smaller"`
	} `json:"v19_fixture"`
	// Resolved is ResolvePlan on the fixture for a 1080p 20 Mbps HEVC-free
	// H.264 file, the case where every guard passes.
	Resolved string `json:"resolved_h264_20mbps"`
	// ResolvedSkipped is the same flow on an HEVC file: the guarded
	// transcode is skipped and the video stays a copy.
	ResolvedSkipped string `json:"resolved_hevc_skipped"`
	// EasyGraph is TargetToFlow(Default()) compiled back to a Target.
	EasyTarget   string   `json:"easy_default_target"`
	EasyDescribe []string `json:"easy_default_describe"`
}

func v19Compute(t *testing.T) v19Golden {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", "v19_graph.json"))
	require.NoError(t, err)
	g, err := ParseGraph(raw)
	require.NoError(t, err)

	var out v19Golden
	out.Fixture.Validate = []string{}
	for _, e := range Validate(g) {
		out.Fixture.Validate = append(out.Fixture.Validate, e.Error())
	}
	plan, err := Compile(g)
	require.NoError(t, err)
	out.Fixture.Target = mustJSON(t, plan.Target())
	out.Fixture.Describe = plan.Describe()
	out.Fixture.Rename = plan.RenameTemplate()
	out.Fixture.HealthCheck = mustJSON(t, plan.HealthCheck)
	out.Fixture.MoveTo = plan.MoveTo
	out.Fixture.KeepTree = plan.MoveKeepTree
	out.Fixture.SkipIfSmall = plan.HasSkipIfSmaller()

	h264 := probe.MediaInfo{Container: "mp4", Video: []probe.VideoStream{{Codec: "h264", Width: 1920, Height: 1080, Bitrate: 20_000_000}}}
	eff, err := ResolvePlan(plan, h264, probe.KindVideo)
	require.NoError(t, err)
	out.Resolved = mustJSON(t, eff.Target)

	hevc := probe.MediaInfo{Container: "mkv", Video: []probe.VideoStream{{Codec: "hevc", Width: 1920, Height: 1080, Bitrate: 4_000_000}}}
	eff, err = ResolvePlan(plan, hevc, probe.KindVideo)
	require.NoError(t, err)
	out.ResolvedSkipped = mustJSON(t, eff.Target)

	easy, err := TargetToFlow(Default())
	require.NoError(t, err)
	easyPlan, err := Compile(easy)
	require.NoError(t, err)
	out.EasyTarget = mustJSON(t, easyPlan.Target())
	out.EasyDescribe = easyPlan.Describe()
	return out
}

func mustJSON(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	require.NoError(t, err)
	return string(b)
}

func TestV19GoldenUnchanged(t *testing.T) {
	got := v19Compute(t)
	if os.Getenv("FLOW_UPDATE_GOLDEN") == "1" {
		b, err := json.MarshalIndent(got, "", "  ")
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(v19GoldenPath, append(b, '\n'), 0o644))
		t.Skip("golden regenerated")
	}
	raw, err := os.ReadFile(v19GoldenPath)
	require.NoError(t, err, "golden missing: captured from v1.9 code")
	var want v19Golden
	require.NoError(t, json.Unmarshal(raw, &want))
	require.Equal(t, want, got, "a v1.9 graph must keep its exact Target and Describe output")
	// The stored Target bytes must not gain fields: compare the raw string.
	require.Equal(t, want.Fixture.Target, got.Fixture.Target)
	require.Equal(t, want.Resolved, got.Resolved)
}
