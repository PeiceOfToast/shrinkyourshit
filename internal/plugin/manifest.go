// SPDX-License-Identifier: AGPL-3.0-only

// Package plugin is the v1 plugin support (task 31): the plugin.yaml
// manifest, the process runner and the install sources. A plugin is a
// directory with a manifest whose command runs as a subprocess of the
// server at defined moments of a job's life. Nothing here knows about
// media: the job context is plain JSON.
package plugin

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

// Plugin types (task 31; the manifest's type field is the moment the
// plugin runs). TypeStep (task 32) is not a moment: a step plugin runs
// only where a flow places it, in the middle of the chain. Do not add
// values without updating docs/plugins.md and ARCHITECTURE.md.
const (
	TypePreflight   = "preflight"
	TypePostprocess = "postprocess"
	TypeNotify      = "notify"
	TypeStep        = "step"
)

// TypeValues lists the valid manifest types.
func TypeValues() []string {
	return []string{TypePreflight, TypePostprocess, TypeNotify, TypeStep}
}

// Timeout bounds (task 31): the manifest's timeout_seconds defaults to 30
// and is clamped to 1..300.
const (
	TimeoutDefault = 30
	TimeoutMin     = 1
	TimeoutMax     = 300
)

// nameRe is the manifest name rule (task 31): lowercase, digits and
// dashes, starting with a letter or digit, 2 to 41 characters.
var nameRe = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{1,40}$`)

// envNameRe is the shape of a name that can be exported to a process
// environment (manifest env entries and, by the same rule, the setting
// keys a plugin's env list can be fed from).
var envNameRe = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// ValidName reports whether name satisfies the manifest name rule
// (task 32: the flow's plugin step reuses this check instead of a second
// regex, so a flow can never name a plugin the installer would refuse).
func ValidName(name string) bool { return nameRe.MatchString(name) }

// ValidEnvName reports whether name is exportable to a process
// environment. Manifest env entries must satisfy it; the setting keys a
// plugin step overrides with must look the same (task 32), because every
// setting value becomes the environment variable its key names.
func ValidEnvName(name string) bool { return envNameRe.MatchString(name) }

// metachars are the characters a command element must not contain (task
// 31): the command is exec'd directly, never through a shell, so anything
// a shell would interpret is a mistake a typo would produce silently.
// The set is documented in docs/plugins.md.
const metachars = "|&;<>()$`\\\"'*?[]#~\n\r"

// Setting is one settings entry of a manifest: the value is collected in
// the UI after install and stored in the plugins row. Required keys that
// have no value stop the plugin from running (never a guess).
type Setting struct {
	Key      string `yaml:"key" json:"key"`
	Label    string `yaml:"label" json:"label"`
	Required bool   `yaml:"required" json:"required"`
}

// Manifest is plugin.yaml. Unknown top level fields are an error so a
// typo fails loudly at install time instead of at the first job.
type Manifest struct {
	Name           string    `yaml:"name"`
	Version        string    `yaml:"version"`
	Description    string    `yaml:"description"`
	Author         string    `yaml:"author"`
	Type           string    `yaml:"type"`
	Command        []string  `yaml:"command"`
	TimeoutSeconds int       `yaml:"timeout_seconds"`
	Env            []string  `yaml:"env"`
	Settings       []Setting `yaml:"settings"`
}

// FileName is the manifest file every plugin directory carries.
const FileName = "plugin.yaml"

// Parse decodes and validates manifest bytes. Every rejection names the
// field (docs/plugins.md, acceptance: "an invalid manifest is rejected
// with a message naming the field").
func Parse(raw []byte) (Manifest, error) {
	var m Manifest
	dec := yaml.NewDecoder(bytes.NewReader(raw))
	dec.KnownFields(true) // unknown top level fields are an error
	if err := dec.Decode(&m); err != nil {
		return Manifest{}, fmt.Errorf("plugin.yaml: %v", err)
	}
	if err := m.Validate(); err != nil {
		return Manifest{}, err
	}
	return m, nil
}

// Load reads and parses the manifest of the plugin directory dir.
func Load(dir string) (Manifest, error) {
	raw, err := os.ReadFile(filepath.Join(dir, FileName))
	if err != nil {
		return Manifest{}, fmt.Errorf("plugin.yaml: %v", err)
	}
	return Parse(raw)
}

// Validate checks the manifest in place (the timeout clamp must be
// visible to the caller) and rejects, naming the field: a bad name, an
// unknown or missing type, a bad command shape, a bad env name and a bad
// settings entry.
func (m *Manifest) Validate() error {
	m.Name = strings.TrimSpace(m.Name)
	if !nameRe.MatchString(m.Name) {
		return fmt.Errorf("name: %q must match %s", m.Name, nameRe.String())
	}
	known := false
	for _, t := range TypeValues() {
		if m.Type == t {
			known = true
			break
		}
	}
	if !known {
		return fmt.Errorf("type: %q must be one of %v", m.Type, TypeValues())
	}
	if len(m.Command) == 0 {
		return fmt.Errorf("command: must be a non-empty array")
	}
	for i, part := range m.Command {
		if strings.TrimSpace(part) == "" {
			return fmt.Errorf("command: element %d must not be empty", i)
		}
		if strings.ContainsAny(part, metachars) {
			return fmt.Errorf("command: element %d contains a shell metacharacter; the command is exec'd directly, never through a shell", i)
		}
	}

	// The timeout is clamped to 1..300 (task 31); 0 and negatives mean
	// "not set" and take the default.
	switch {
	case m.TimeoutSeconds == 0, m.TimeoutSeconds < 0:
		m.TimeoutSeconds = TimeoutDefault
	case m.TimeoutSeconds > TimeoutMax:
		m.TimeoutSeconds = TimeoutMax
	}

	for _, name := range m.Env {
		name = strings.TrimSpace(name)
		if !envNameRe.MatchString(name) {
			return fmt.Errorf("env: %q is not a valid environment variable name", name)
		}
	}
	m.Env = trimList(m.Env)

	seen := map[string]bool{}
	for i := range m.Settings {
		s := &m.Settings[i]
		s.Key = strings.TrimSpace(s.Key)
		if s.Key == "" {
			return fmt.Errorf("settings: entry %d has an empty key", i)
		}
		if seen[s.Key] {
			return fmt.Errorf("settings: duplicate key %q", s.Key)
		}
		seen[s.Key] = true
		if s.Label == "" {
			s.Label = s.Key
		}
	}
	return nil
}

// Timeout is the manifest's timeout as a duration-safe int (already
// clamped by Validate; a zero value that skipped validation still gets
// the default so a hand-built manifest cannot hang a job).
func (m Manifest) Timeout() int {
	if m.TimeoutSeconds <= 0 {
		return TimeoutDefault
	}
	if m.TimeoutSeconds > TimeoutMax {
		return TimeoutMax
	}
	return m.TimeoutSeconds
}

// trimList trims and drops the empty entries of a string list.
func trimList(in []string) []string {
	out := make([]string, 0, len(in))
	for _, s := range in {
		if s = strings.TrimSpace(s); s != "" {
			out = append(out, s)
		}
	}
	return out
}
