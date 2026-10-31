// SPDX-License-Identifier: AGPL-3.0-only

// Package deploy holds the NAS deploy templates (task 29): one compose
// file per platform and the Unraid Community Applications template. The
// test in this package is the contract between the templates and the docs
// — an edit that breaks a template fails `make test` naming the file and
// the missing key, instead of shipping a broken install.
package deploy

import (
	"encoding/xml"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

const imageRepo = "ghcr.io/peiceoftoast/shrinkyourshit"

// composeFile is the small slice of the compose schema the contract needs.
type composeFile struct {
	Services map[string]composeService `yaml:"services"`
}

type composeService struct {
	Image       string   `yaml:"image"`
	Restart     string   `yaml:"restart"`
	Ports       []string `yaml:"ports"`
	Volumes     []string `yaml:"volumes"`
	Privileged  *bool    `yaml:"privileged"`
	NetworkMode string   `yaml:"network_mode"`
	Environment envList  `yaml:"environment"`
}

// envList accepts both compose environment shapes: a map (PUID: 1000) and
// a list (- PUID=1000).
type envList struct {
	values map[string]string
}

func (e *envList) UnmarshalYAML(value *yaml.Node) error {
	e.values = map[string]string{}
	switch value.Kind {
	case yaml.MappingNode:
		for i := 0; i+1 < len(value.Content); i += 2 {
			var v string
			if err := value.Content[i+1].Decode(&v); err != nil {
				return err
			}
			e.values[value.Content[i].Value] = v
		}
	case yaml.SequenceNode:
		for _, item := range value.Content {
			if k, v, ok := strings.Cut(item.Value, "="); ok {
				e.values[k] = v
			}
		}
	default:
		return fmt.Errorf("environment must be a map or a list of KEY=value lines")
	}
	return nil
}

// pinnedImageTag returns the release tag of a ghcr image reference, or ""
// when the reference is not the pinned repo with an explicit tag.
func pinnedImageTag(image string) string {
	if !strings.HasPrefix(image, imageRepo+":") {
		return ""
	}
	return strings.TrimPrefix(image, imageRepo+":")
}

// publishesPort reports whether ports publishes containerPort as hostPort
// in short syntax ("8765:8765", optionally with a /tcp or /udp suffix).
func publishesPort(ports []string, hostPort, containerPort string) bool {
	for _, p := range ports {
		parts := strings.SplitN(p, ":", 2)
		if len(parts) != 2 || parts[0] != hostPort {
			continue
		}
		if container, _, _ := strings.Cut(parts[1], "/"); container == containerPort {
			return true
		}
	}
	return false
}

// volumeTargets returns the container-side path of each short-syntax
// volume ("host:container[:mode]").
func volumeTargets(volumes []string) []string {
	targets := make([]string, 0, len(volumes))
	for _, v := range volumes {
		parts := strings.SplitN(v, ":", 3)
		if len(parts) >= 2 {
			targets = append(targets, parts[1])
		}
	}
	return targets
}

func composeTemplateFiles(t *testing.T) []string {
	t.Helper()
	files, err := filepath.Glob("*/docker-compose.yml")
	require.NoError(t, err)
	require.NotEmpty(t, files,
		"no deploy/*/docker-compose.yml files found — the deploy templates are missing")
	return files
}

func TestComposeTemplatesFollowTheContract(t *testing.T) {
	for _, file := range composeTemplateFiles(t) {
		t.Run(file, func(t *testing.T) {
			data, err := os.ReadFile(file)
			require.NoError(t, err)

			var cf composeFile
			require.NoError(t, yaml.Unmarshal(data, &cf), "%s is not valid YAML", file)

			require.Len(t, cf.Services, 1,
				"%s: services must contain exactly the shrinkyourshit service", file)
			svc, ok := cf.Services["shrinkyourshit"]
			require.True(t, ok,
				"%s: services.shrinkyourshit is missing — the service must be named shrinkyourshit", file)

			// Pinned image: the ghcr repo with an explicit release tag,
			// never latest.
			tag := pinnedImageTag(svc.Image)
			require.NotEmpty(t, tag,
				"%s: services.shrinkyourshit.image must be %s:<release-tag>, got %q", file, imageRepo, svc.Image)
			require.NotEqual(t, "latest", strings.ToLower(tag),
				"%s: services.shrinkyourshit.image must pin a release, not :latest", file)

			// The web UI and API port, published as 8765:8765.
			require.True(t, publishesPort(svc.Ports, "8765", "8765"),
				"%s: services.ports must publish the container port 8765 as 8765 (\"8765:8765\"), got %q",
				file, svc.Ports)

			// The three volumes of the volume contract.
			targets := volumeTargets(svc.Volumes)
			for _, want := range []string{"/config", "/media", "/scratch"} {
				require.Contains(t, targets, want,
					"%s: services.volumes must map %s (missing from %q)", file, want, svc.Volumes)
			}

			// PUID/PGID so the files written are owned by the user.
			require.NotEmpty(t, svc.Environment.values["PUID"],
				"%s: services.environment must set PUID", file)
			require.NotEmpty(t, svc.Environment.values["PGID"],
				"%s: services.environment must set PGID", file)

			// Restart policy.
			require.Equal(t, "unless-stopped", svc.Restart,
				"%s: services.restart must be unless-stopped", file)

			// Nothing privileged, no host networking, no docker socket.
			if svc.Privileged != nil {
				require.False(t, *svc.Privileged, "%s: services.privileged must not be true", file)
			}
			require.NotEqual(t, "host", svc.NetworkMode,
				"%s: services.network_mode must not be host", file)
			for _, vol := range svc.Volumes {
				require.NotContains(t, vol, "docker.sock",
					"%s: services.volumes must not mount the docker socket (found in %q)", file, vol)
			}
		})
	}
}

type unraidTemplate struct {
	XMLName    xml.Name       `xml:"Container"`
	Repository string         `xml:"Repository"`
	Network    string         `xml:"Network"`
	Privileged bool           `xml:"Privileged"`
	Icon       string         `xml:"Icon"`
	Overview   string         `xml:"Overview"`
	Configs    []unraidConfig `xml:"Config"`
}

type unraidConfig struct {
	Name   string `xml:"Name,attr"`
	Target string `xml:"Target,attr"`
	Mode   string `xml:"Mode,attr"`
	Type   string `xml:"Type,attr"`
}

func TestUnraidTemplateFollowsTheContract(t *testing.T) {
	const file = "unraid/shrinkyourshit.xml"
	data, err := os.ReadFile(file)
	require.NoError(t, err, "%s is missing — the Unraid template must exist", file)

	var tpl unraidTemplate
	require.NoError(t, xml.Unmarshal(data, &tpl), "%s is not valid XML", file)

	// Repository: the ghcr repo with an explicit release tag, never latest.
	tag := pinnedImageTag(tpl.Repository)
	require.NotEmpty(t, tag,
		"%s: Repository must be %s:<release-tag>, got %q", file, imageRepo, tpl.Repository)
	require.NotEqual(t, "latest", strings.ToLower(tag),
		"%s: Repository must pin a release, not :latest", file)

	require.Equal(t, "bridge", tpl.Network, "%s: Network must be bridge", file)
	require.False(t, tpl.Privileged, "%s: Privileged must be false", file)
	require.NotEmpty(t, tpl.Icon, "%s: Icon must point at the template icon", file)
	require.NotEmpty(t, tpl.Overview, "%s: Overview must describe the app in plain language", file)

	byTarget := map[string]unraidConfig{}
	for _, c := range tpl.Configs {
		byTarget[c.Target] = c
	}

	port, ok := byTarget["8765"]
	require.True(t, ok, "%s: a Config with Target=\"8765\" (the web UI port) is missing", file)
	require.Equal(t, "Port", port.Type, "%s: the 8765 Config must be Type=Port", file)

	for _, path := range []string{"/config", "/media", "/scratch"} {
		_, ok := byTarget[path]
		require.True(t, ok, "%s: a Config with Target=%s is missing", file, path)
	}
	require.Equal(t, "rw", byTarget["/config"].Mode,
		"%s: the /config Config must exist with Mode=rw", file)
	require.Equal(t, "rw", byTarget["/media"].Mode,
		"%s: the /media Config must exist with Mode=rw (results replace files in place)", file)

	require.NotEmpty(t, byTarget["PUID"].Target, "%s: a Config with Target=PUID is missing", file)
	require.NotEmpty(t, byTarget["PGID"].Target, "%s: a Config with Target=PGID is missing", file)
}
