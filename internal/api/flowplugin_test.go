// SPDX-License-Identifier: AGPL-3.0-only

package api

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/PeiceOfToast/shrinkyourshit/internal/flow"
	"github.com/PeiceOfToast/shrinkyourshit/internal/plugin"
	"github.com/PeiceOfToast/shrinkyourshit/internal/server"
	"github.com/PeiceOfToast/shrinkyourshit/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// pluginFlowHarness is an api harness whose server has a real data dir, so
// step plugins install into it and the flow endpoints resolve them. The
// test plugins exec shell; the helper skips when sh is missing.
func pluginFlowHarness(t *testing.T) (*apiHarness, string) {
	t.Helper()
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("sh is not available")
	}
	var dataDir string
	h := newAPIHarness(t, func(cfg *server.Config) {
		dataDir = filepath.Join(t.TempDir(), "data")
		cfg.DataDir = dataDir
	})
	return h, dataDir
}

// installStepPlugin writes and installs a plugin of the given type,
// returning the installed service shape.
func installStepPlugin(t *testing.T, h *apiHarness, name, typ, script string) server.Installed {
	t.Helper()
	src := filepath.Join(t.TempDir(), name)
	require.NoError(t, os.MkdirAll(src, 0o755))
	scriptPath := filepath.Join(src, "run.sh")
	require.NoError(t, os.WriteFile(scriptPath, []byte(script+"\n"), 0o755))
	raw := `name: ` + name + `
version: 1.0.0
type: ` + typ + `
command: ["sh", "` + scriptPath + `"]
`
	require.NoError(t, os.WriteFile(filepath.Join(src, plugin.FileName), []byte(raw), 0o644))
	inst, err := h.svc.Plugins().Install(context.Background(), server.InstallRequest{Path: src})
	require.NoError(t, err)
	return inst
}

// enablePluginFlow flips a plugin on through the service.
func enablePluginFlow(t *testing.T, h *apiHarness, id string, on bool) {
	t.Helper()
	_, err := h.svc.Plugins().Update(context.Background(), id, server.UpdateRequest{Enabled: &on})
	require.NoError(t, err)
}

// pluginFlow is probe → plugin → remux: the smallest valid flow with a
// plugin step.
func pluginFlow() flow.Graph {
	return flow.Graph{
		Schema: flow.FlowSchema,
		Nodes: []flow.Step{
			{ID: "probe", Type: flow.StepProbe},
			{ID: "plug", Type: flow.StepPlugin, Params: json.RawMessage(`{"plugin":"remove-chapters"}`)},
			{ID: "remux", Type: flow.StepRemuxContainer, Params: json.RawMessage(`{"container":"mkv"}`)},
		},
		Edges: []flow.Edge{
			{From: "probe", To: "plug"}, {From: "plug", To: "remux"},
		},
	}
}

// TestFlowStepTypesListStepPlugins (task 32 acceptance): a manifest with
// `type: step` installs and is listed as a step plugin; a `preflight`
// plugin is not offered in the flow editor — the plugin select's options
// carry only the enabled step plugins.
func TestFlowStepTypesListStepPlugins(t *testing.T) {
	h, _ := pluginFlowHarness(t)

	stepInst := installStepPlugin(t, h, "remove-chapters", store.PluginTypeStep, "echo step")
	preflightInst := installStepPlugin(t, h, "gate-guard", store.PluginTypePreflight, "echo gate")
	enablePluginFlow(t, h, preflightInst.ID, true)

	// Nothing enabled yet: the option list is empty.
	rec := h.do(http.MethodGet, "/api/v1/flows/step-types", nil)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	var list struct {
		Items []flow.StepTypeInfo `json:"items"`
		Total int                 `json:"total"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &list))
	var plug flow.StepTypeInfo
	for _, st := range list.Items {
		if st.Type == flow.StepPlugin {
			plug = st
		}
	}
	require.Equal(t, "Run a plugin", plug.Label)
	require.Empty(t, plug.Params[0].Options)

	// Enabled step plugin: listed. The enabled preflight plugin is not.
	enablePluginFlow(t, h, stepInst.ID, true)
	rec = h.do(http.MethodGet, "/api/v1/flows/step-types", nil)
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &list))
	for _, st := range list.Items {
		if st.Type != flow.StepPlugin {
			continue
		}
		require.Equal(t, []string{"remove-chapters"}, st.Params[0].Options)
	}
}

// TestFlowPluginStepRoundTrip (task 32 acceptance): a valid plugin step
// round trips through PUT /api/v1/libraries/{id}/flow.
func TestFlowPluginStepRoundTrip(t *testing.T) {
	h, _ := pluginFlowHarness(t)
	inst := installStepPlugin(t, h, "remove-chapters", store.PluginTypeStep, "echo ran")
	enablePluginFlow(t, h, inst.ID, true)
	lib := h.createLibrary(false)

	rec := h.do(http.MethodPut, "/api/v1/libraries/"+lib.ID+"/flow", map[string]any{
		"source": "steps", "graph": pluginFlow(),
	})
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	rec = h.do(http.MethodGet, "/api/v1/libraries/"+lib.ID+"/flow", nil)
	require.Equal(t, http.StatusOK, rec.Code)
	var got flowResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
	require.Len(t, got.Graph.Nodes, 3)
	require.Equal(t, flow.StepPlugin, got.Graph.Nodes[1].Type)

	// The preview renders the plugin line.
	rec = h.do(http.MethodPost, "/api/v1/flows/preview", map[string]any{"graph": pluginFlow()})
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	var prev struct {
		Steps []struct {
			Plain string `json:"plain"`
		} `json:"steps"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &prev))
	require.Contains(t, prev.Steps[1].Plain, "Run plugin remove-chapters")
}

// TestFlowPluginStepSaveRefusals (task 32 acceptance): saving a flow that
// names a missing, disabled or non-step plugin returns 422 naming the
// plugin and the reason.
func TestFlowPluginStepSaveRefusals(t *testing.T) {
	h, _ := pluginFlowHarness(t)
	inst := installStepPlugin(t, h, "remove-chapters", store.PluginTypeStep, "echo ran")
	lib := h.createLibrary(false)

	body := func(name string) map[string]any {
		g := pluginFlow()
		g.Nodes[1].Params = json.RawMessage(`{"plugin":"` + name + `"}`)
		return map[string]any{"source": "steps", "graph": g}
	}

	// Not installed (the graph names a plugin that has no row).
	rec := h.do(http.MethodPut, "/api/v1/libraries/"+lib.ID+"/flow", body("never-installed"))
	require.Equal(t, http.StatusUnprocessableEntity, rec.Code, rec.Body.String())
	require.Contains(t, rec.Body.String(), "never-installed")
	require.Contains(t, rec.Body.String(), "plugin_unavailable")
	require.Contains(t, rec.Body.String(), "not installed")

	// Installed but disabled.
	enablePluginFlow(t, h, inst.ID, true)
	rec = h.do(http.MethodPut, "/api/v1/libraries/"+lib.ID+"/flow", body("remove-chapters"))
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	enablePluginFlow(t, h, inst.ID, false)
	rec = h.do(http.MethodPut, "/api/v1/libraries/"+lib.ID+"/flow", body("remove-chapters"))
	require.Equal(t, http.StatusUnprocessableEntity, rec.Code, rec.Body.String())
	require.Contains(t, rec.Body.String(), "remove-chapters")
	require.Contains(t, rec.Body.String(), "disabled")

	// Installed and enabled but the wrong type (a preflight plugin).
	hook := installStepPlugin(t, h, "gate-guard", store.PluginTypePreflight, "echo gate")
	enablePluginFlow(t, h, hook.ID, true)
	rec = h.do(http.MethodPut, "/api/v1/libraries/"+lib.ID+"/flow", body("gate-guard"))
	require.Equal(t, http.StatusUnprocessableEntity, rec.Code, rec.Body.String())
	require.Contains(t, rec.Body.String(), "gate-guard")
	require.Contains(t, rec.Body.String(), "preflight")
}

// TestFlowPluginStepDispatch (task 32 acceptance): a flow that names a
// plugin somebody later disabled must not silently skip the step —
// dispatching that flow fails the job with a message naming the plugin,
// and the spec endpoint answers 422. A valid dispatch carries the
// resolved plugin steps in the spec.
func TestFlowPluginStepDispatch(t *testing.T) {
	h, dataDir := pluginFlowHarness(t)
	inst := installStepPlugin(t, h, "remove-chapters", store.PluginTypeStep, "echo ran")
	enablePluginFlow(t, h, inst.ID, true)

	lib, _, _ := h.queueOne(t)
	_, err := h.svc.Libraries().UpdateFlow(context.Background(), lib.ID, "steps", pluginFlow())
	require.NoError(t, err)

	// Disable the plugin after the flow saved.
	enablePluginFlow(t, h, inst.ID, false)

	key, _ := h.addNode(t, "w1", store.NodeStatusApproved, true)
	rec := h.doNode(http.MethodPost, "/api/v1/worker/claim", nil, key)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	var job server.JobDTO
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &job))

	rec = h.doNode(http.MethodGet, "/api/v1/worker/jobs/"+job.ID, nil, key)
	require.Equal(t, http.StatusUnprocessableEntity, rec.Code, rec.Body.String())
	require.Contains(t, rec.Body.String(), "plugin_unavailable")
	require.Contains(t, rec.Body.String(), "remove-chapters")

	// The job failed instead of running without the step.
	failed, err := h.st.GetJob(context.Background(), job.ID)
	require.NoError(t, err)
	require.Equal(t, store.JobStatusFailed, failed.Status)
	require.Contains(t, failed.Error, "remove-chapters")
	require.Contains(t, failed.Error, "disabled")

	// A valid dispatch carries the resolved step. Re-enable, retry.
	enablePluginFlow(t, h, inst.ID, true)
	retried, err := h.svc.Queue().Retry(context.Background(), failed.ID)
	require.NoError(t, err)
	rec = h.doNode(http.MethodPost, "/api/v1/worker/claim", nil, key)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &job))
	require.Equal(t, retried.ID, job.ID)
	rec = h.doNode(http.MethodGet, "/api/v1/worker/jobs/"+job.ID, nil, key)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	var spec server.JobSpec
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &spec))
	require.Len(t, spec.Plugins, 1)
	assert.Equal(t, "remove-chapters", spec.Plugins[0].Name)
	assert.Equal(t, 1, spec.Plugins[0].StepIndex, "probe is index 0, the plugin follows")
	assert.Equal(t, plugin.TimeoutDefault, spec.Plugins[0].TimeoutSec)
	installed, err := plugin.Load(filepath.Join(dataDir, "plugins", "remove-chapters"))
	require.NoError(t, err)
	assert.Equal(t, installed.Command, spec.Plugins[0].Command,
		"the manifest facts travel in the spec, read from the installed directory")
	assert.Equal(t, installed.Env, spec.Plugins[0].EnvNames)
}
