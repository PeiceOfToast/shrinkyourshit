// SPDX-License-Identifier: AGPL-3.0-only

package plugin

import (
	"strconv"
	"strings"
	"testing"

	"github.com/PeiceOfToast/shrinkyourshit/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// validYAML is a manifest that must parse with defaults applied.
const validYAML = `
name: notify-telegram
version: 1.0.0
description: Send a Telegram message when a file finishes.
author: someone
type: postprocess
command: ["python3", "main.py"]
timeout_seconds: 30
env:
  - TELEGRAM_BOT_TOKEN
settings:
  - key: chat_id
    label: Chat id
    required: true
`

func TestParseValid(t *testing.T) {
	m, err := Parse([]byte(validYAML))
	require.NoError(t, err)
	assert.Equal(t, "notify-telegram", m.Name)
	assert.Equal(t, "1.0.0", m.Version)
	assert.Equal(t, TypePostprocess, m.Type)
	assert.Equal(t, []string{"python3", "main.py"}, m.Command)
	assert.Equal(t, 30, m.TimeoutSeconds)
	assert.Equal(t, []string{"TELEGRAM_BOT_TOKEN"}, m.Env)
	require.Len(t, m.Settings, 1)
	assert.Equal(t, "chat_id", m.Settings[0].Key)
	assert.True(t, m.Settings[0].Required)
	assert.Equal(t, "Chat id", m.Settings[0].Label)
}

func TestParseDefaultsTimeout(t *testing.T) {
	m, err := Parse([]byte(strings.ReplaceAll(validYAML, "timeout_seconds: 30\n", "")))
	require.NoError(t, err)
	assert.Equal(t, TimeoutDefault, m.TimeoutSeconds, "absent timeout takes the default")
}

func TestClampTimeout(t *testing.T) {
	for _, tc := range []struct {
		in, want int
	}{
		{0, TimeoutDefault},
		{-5, TimeoutDefault},
		{1, 1},
		{299, 299},
		{300, 300},
		{301, TimeoutMax},
		{10_000, TimeoutMax},
	} {
		raw := strings.Replace(validYAML, "timeout_seconds: 30", "timeout_seconds: "+strconv.Itoa(tc.in), 1)
		m, err := Parse([]byte(raw))
		require.NoError(t, err)
		assert.Equal(t, tc.want, m.TimeoutSeconds, "timeout %d", tc.in)
		assert.Equal(t, tc.want, m.Timeout(), "Timeout() for %d", tc.in)
	}
}

func TestRejectUnknownField(t *testing.T) {
	_, err := Parse([]byte(strings.ReplaceAll(validYAML, "author: someone", "authors: someone")))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "authors", "the message names the unknown field")
}

func TestRejectBadName(t *testing.T) {
	for _, name := range []string{"", "A-upper", "has space", "-leading", "a", "with_underscore"} {
		raw := strings.Replace(validYAML, "name: notify-telegram", "name: "+name, 1)
		_, err := Parse([]byte(raw))
		require.Error(t, err, "name %q must be rejected", name)
		assert.Contains(t, err.Error(), "name:", "the message names the field")
	}
}

func TestRejectBadType(t *testing.T) {
	for _, typ := range []string{"", "hook", "Preflight", "flow_step"} {
		raw := strings.Replace(validYAML, "type: postprocess", "type: "+typ, 1)
		_, err := Parse([]byte(raw))
		require.Error(t, err, "type %q must be rejected", typ)
		assert.Contains(t, err.Error(), "type:", "the message names the field")
	}
}

func TestRejectBadCommand(t *testing.T) {
	// Empty array.
	_, err := Parse([]byte(strings.ReplaceAll(validYAML, `command: ["python3", "main.py"]`, "command: []")))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "command:")

	// Shell metacharacters are rejected element by element, with the
	// element index in the message. (A backslash, a newline and raw
	// quotes are checked directly on Validate below — YAML double-quoted
	// strings escape them before the parser ever sees them.)
	for _, bad := range []string{"python3|tee", "main.py;rm", "a&b", "$(whoami)", "`id`", "a<b", "a>b", "a*b", "a?b", "a[b]", "a~b", "a#b"} {
		raw := strings.Replace(validYAML, `"main.py"`, `"`+bad+`"`, 1)
		_, err := Parse([]byte(raw))
		require.Error(t, err, "command element %q must be rejected", bad)
		assert.Contains(t, err.Error(), "command:", "the message names the field for %q", bad)
	}

	m := Manifest{Name: "notify-test", Type: TypeNotify, Command: []string{"sh", `a\b`, "line\nbreak", `a"b`, "a'b"}}
	err = m.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "command:")

	// An empty element.
	raw := strings.Replace(validYAML, `command: ["python3", "main.py"]`, `command: ["python3", ""]`, 1)
	_, err = Parse([]byte(raw))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "command:")
}

func TestRejectBadEnvName(t *testing.T) {
	raw := strings.Replace(validYAML, "  - TELEGRAM_BOT_TOKEN", "  - not a name", 1)
	_, err := Parse([]byte(raw))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "env:", "the message names the field")
}

func TestRejectBadSettings(t *testing.T) {
	// An empty key.
	raw := strings.ReplaceAll(validYAML, "key: chat_id", "key: \"\"")
	_, err := Parse([]byte(raw))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "settings:")

	// A duplicate key.
	raw = strings.Replace(validYAML, "  - key: chat_id", "  - key: chat_id\n  - key: chat_id", 1)
	_, err = Parse([]byte(raw))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "duplicate")

	// A label defaults to the key.
	raw = strings.Replace(validYAML, "    label: Chat id\n", "", 1)
	m, err := Parse([]byte(raw))
	require.NoError(t, err)
	require.Len(t, m.Settings, 1)
	assert.Equal(t, "chat_id", m.Settings[0].Label)
}

func TestValidateNormalizationOnHandBuiltManifest(t *testing.T) {
	m := Manifest{
		Name:    "notify-test",
		Type:    TypeNotify,
		Command: []string{"echo", "hi"},
		Env:     []string{" MY_VAR "},
	}
	require.NoError(t, m.Validate())
	assert.Equal(t, TimeoutDefault, m.TimeoutSeconds, "hand-built manifests get the default timeout too")
	assert.Equal(t, []string{"MY_VAR"}, m.Env, "env names are trimmed")
}

func TestLoadMissingManifest(t *testing.T) {
	_, err := Load(t.TempDir())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "plugin.yaml")
}

// TestStepTypeIsKnown (task 32): the fourth manifest type installs, and
// the store's own value list (which the store package validates columns
// against, to avoid an import cycle) stays in sync with this one.
func TestStepTypeIsKnown(t *testing.T) {
	m, err := Parse([]byte(strings.ReplaceAll(validYAML, "type: postprocess", "type: step")))
	require.NoError(t, err)
	assert.Equal(t, TypeStep, m.Type)
	assert.Equal(t, store.PluginTypeValues(), TypeValues(),
		"the store mirrors these strings; the two lists must not drift")
	assert.Contains(t, TypeValues(), TypeStep)
}

// TestValidNameAndEnvName (task 32): the checks the flow layer reuses for
// a plugin step's parameters. A flow can never name a plugin the
// installer would refuse, and a settings key must be exportable to the
// environment its value becomes.
func TestValidNameAndEnvName(t *testing.T) {
	assert.True(t, ValidName("remove-chapters"))
	assert.True(t, ValidName("a2"))
	assert.False(t, ValidName("Remove Chapters"))
	assert.False(t, ValidName("remove_chapters"))
	assert.False(t, ValidName("x"))
	assert.False(t, ValidName(""))
	assert.True(t, ValidEnvName("TINY_MKV"))
	assert.True(t, ValidEnvName("chat_id"))
	assert.False(t, ValidEnvName("not a name"))
	assert.False(t, ValidEnvName("1bad"))
}

/* ------------------------------------------------------------------ */
/* The wasm runtime (task 33)                                          */
/* ------------------------------------------------------------------ */

// TestWasmManifestDefaults (task 33): runtime: wasm with entry only. The
// memory budget defaults to 1024 pages (64 MiB) and the timeout to 30s,
// exactly like a native manifest's defaults.
func TestWasmManifestDefaults(t *testing.T) {
	m, err := Parse([]byte(
		"name: wasm-plugin\ntype: step\nruntime: wasm\nentry: main.wasm\n"))
	require.NoError(t, err)
	assert.Equal(t, RuntimeWASM, m.Runtime)
	assert.Equal(t, "main.wasm", m.Entry)
	assert.Equal(t, MemoryPagesDefault, m.MemoryPages)
	assert.Equal(t, TimeoutDefault, m.TimeoutSeconds)
}

// TestWasmMemoryPagesClamps (task 33): memory_pages clamps to 16..4096;
// 0 and negatives mean "not set" and take the default.
func TestWasmMemoryPagesClamps(t *testing.T) {
	cases := map[int]int{
		0: MemoryPagesDefault, -3: MemoryPagesDefault,
		1: MemoryPagesMin, 15: MemoryPagesMin,
		1024: 1024, 4096: MemoryPagesMax, 99999: MemoryPagesMax,
	}
	for in, want := range cases {
		m := Manifest{Name: "wasm-probe", Type: TypeStep, Runtime: RuntimeWASM, Entry: "main.wasm", MemoryPages: in}
		require.NoError(t, m.Validate())
		assert.Equal(t, want, m.MemoryPages, "memory_pages %d", in)
		assert.Equal(t, want, m.MemoryLimit())
	}
}

// TestWasmManifestWithCommandRejected (task 33): a manifest that sets both
// runtime: wasm and command is rejected naming both fields.
func TestWasmManifestWithCommandRejected(t *testing.T) {
	m := Manifest{Name: "wasm-probe", Type: TypeStep, Runtime: RuntimeWASM,
		Entry: "main.wasm", Command: []string{"echo", "hi"}}
	err := m.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "runtime")
	assert.Contains(t, err.Error(), "command")
	assert.Contains(t, err.Error(), "entry")
}

// TestWasmEntryShapeRejected (task 33): a wasm entry must be a plain file
// name inside the plugin directory.
func TestWasmEntryShapeRejected(t *testing.T) {
	for _, entry := range []string{"", "../evil.wasm", "sub/main.wasm", "/abs.wasm", "."} {
		m := Manifest{Name: "wasm-probe", Type: TypeStep, Runtime: RuntimeWASM, Entry: entry}
		err := m.Validate()
		require.Error(t, err, "entry %q", entry)
		assert.Contains(t, err.Error(), "entry:", "entry %q: the message names the field", entry)
	}
}

// TestNativeManifestWithWasmFieldsRejected (task 33): entry and
// memory_pages are wasm-only; setting them on a native manifest is a typo
// that must fail at install time naming the field.
func TestNativeManifestWithWasmFieldsRejected(t *testing.T) {
	m := Manifest{Name: "native-probe", Type: TypeStep, Runtime: RuntimeNative,
		Command: []string{"echo", "hi"}, Entry: "main.wasm"}
	err := m.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "entry:")

	m = Manifest{Name: "native-probe", Type: TypeStep, Command: []string{"echo", "hi"}, MemoryPages: 64}
	err = m.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "memory_pages:")
}

// TestUnknownRuntimeRejected (task 33): an unknown runtime value is an
// error naming the field.
func TestUnknownRuntimeRejected(t *testing.T) {
	m := Manifest{Name: "wasm-probe", Type: TypeStep, Runtime: "go", Entry: "main.wasm"}
	err := m.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "runtime:")
}
