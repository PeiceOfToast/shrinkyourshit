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
