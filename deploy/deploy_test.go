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
	"os/exec"
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
	CapAdd      []string `yaml:"cap_add"`
	SecurityOpt []string `yaml:"security_opt"`
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
	XMLName     xml.Name       `xml:"Container"`
	Repository  string         `xml:"Repository"`
	Network     string         `xml:"Network"`
	Privileged  bool           `xml:"Privileged"`
	Icon        string         `xml:"Icon"`
	Overview    string         `xml:"Overview"`
	ExtraParams string         `xml:"ExtraParams"`
	Configs     []unraidConfig `xml:"Config"`
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

// mountCapabilityArgs are the two settings that let the server mount network
// shares (task 67). Anything narrower than privileged; both are required.
const (
	mountCapability = "SYS_ADMIN"
	mountSecurity   = "apparmor=unconfined"
)

// TestComposeTemplatesGrantTheMountCapability keeps the NAS templates in step
// with docker/compose.example.yml: an old copy without these two lines gets
// "permission denied" on every storage mount, so the templates must carry them.
func TestComposeTemplatesGrantTheMountCapability(t *testing.T) {
	for _, file := range composeTemplateFiles(t) {
		t.Run(file, func(t *testing.T) {
			data, err := os.ReadFile(file)
			require.NoError(t, err)
			var cf composeFile
			require.NoError(t, yaml.Unmarshal(data, &cf))
			svc := cf.Services["shrinkyourshit"]
			require.Contains(t, svc.CapAdd, mountCapability,
				"%s: services.shrinkyourshit.cap_add must include %s so the container can mount network storage", file, mountCapability)
			require.Contains(t, svc.SecurityOpt, mountSecurity,
				"%s: services.shrinkyourshit.security_opt must include %s (Docker's default AppArmor profile blocks mount(2))", file, mountSecurity)
		})
	}
}

func TestUnraidTemplateGrantsTheMountCapability(t *testing.T) {
	const file = "unraid/shrinkyourshit.xml"
	data, err := os.ReadFile(file)
	require.NoError(t, err)
	var tpl unraidTemplate
	require.NoError(t, xml.Unmarshal(data, &tpl))
	require.Contains(t, tpl.ExtraParams, "--cap-add="+mountCapability,
		"%s: ExtraParams must carry --cap-add=%s", file, mountCapability)
	require.Contains(t, tpl.ExtraParams, "--security-opt "+mountSecurity,
		"%s: ExtraParams must carry --security-opt %s", file, mountSecurity)
}

// TestInstallDocsNameTheCapabilityAndLoginGate checks the README and the
// container install page (docs/installing.md) state the requirement, the
// login gate from spec 4.6, and the error a user sees after copying an old
// compose file.
func TestInstallDocsNameTheCapabilityAndLoginGate(t *testing.T) {
	for _, file := range []string{"../README.md", "../docs/installing.md"} {
		t.Run(file, func(t *testing.T) {
			data, err := os.ReadFile(file)
			require.NoError(t, err)
			doc := string(data)
			require.Contains(t, doc, mountCapability, "%s must name the %s capability", file, mountCapability)
			require.Contains(t, doc, mountSecurity, "%s must show the %s security option", file, mountSecurity)
			require.Contains(t, doc, "cap_add", "%s must show the cap_add compose lines", file)
			require.Contains(t, doc, "permission denied",
				"%s must show the error an old compose file produces", file)
			require.Contains(t, strings.ToLower(doc), "login",
				"%s must state that storage needs login enabled (spec 4.6)", file)
			require.Contains(t, doc, "App settings",
				"%s must say where login is turned on (App settings)", file)
		})
	}
}

// storageScript runs docker/entrypoint.sh with the functions loaded and the
// given shell snippet. SHRINK_ENTRYPOINT_LIB=1 stops the script from starting
// the server.
func storageScript(t *testing.T, env []string, snippet string) (string, error) {
	t.Helper()
	sh, err := exec.LookPath("sh")
	if err != nil {
		t.Skip("no sh on PATH")
	}
	script, err := filepath.Abs("../docker/entrypoint.sh")
	require.NoError(t, err)
	cmd := exec.Command(sh, "-c", ". \"$0\"; "+snippet, script)
	cmd.Env = append([]string{"SHRINK_ENTRYPOINT_LIB=1"}, env...)
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func TestEntrypointCapBitSet(t *testing.T) {
	cases := []struct {
		mask string
		bit  int
		want bool
	}{
		{"0000000000200000", 21, true},  // only SYS_ADMIN
		{"0000000000100000", 21, false}, // bit 20 set, SYS_ADMIN not
		{"000001ffffffffff", 21, true},  // full set, as in privileged
		{"0000000000000000", 21, false}, // nothing granted
	}
	for _, c := range cases {
		out, err := storageScript(t, nil, fmt.Sprintf("if cap_bit_set %s %d; then echo yes; else echo no; fi", c.mask, c.bit))
		require.NoError(t, err, out)
		want := "no"
		if c.want {
			want = "yes"
		}
		require.Equal(t, want, strings.TrimSpace(out), "cap_bit_set %s %d", c.mask, c.bit)
	}
}

func TestEntrypointStorageReportIsOffWhenDisabled(t *testing.T) {
	for _, value := range []string{"false", "0", "off", "OFF"} {
		out, err := storageScript(t, []string{"SHRINK_MANAGE_MOUNTS=" + value}, "storage_report")
		require.NoError(t, err, "the report must never fail the entrypoint: %s", out)
		require.Contains(t, out, "network storage is off (SHRINK_MANAGE_MOUNTS=false)", "SHRINK_MANAGE_MOUNTS=%s", value)
	}
}

// With no PATH the helpers and sed cannot be found. The report must name the
// problems in one line and still exit 0: the entrypoint never stops the app.
func TestEntrypointStorageReportNamesMissingPiecesAndNeverFails(t *testing.T) {
	out, err := storageScript(t, []string{"PATH=" + t.TempDir()}, "storage_report")
	require.NoError(t, err, "the report must exit 0: %s", out)
	line := strings.TrimSpace(out)
	require.Equal(t, 1, strings.Count(line, "\n")+1, "the report is one line: %q", out)
	require.Contains(t, line, "network storage is unavailable:")
	require.Contains(t, line, "mount.nfs not installed")
	require.Contains(t, line, "mount.cifs not installed")
	require.Contains(t, line, "SYS_ADMIN not granted")
	require.Contains(t, line, "cap_add SYS_ADMIN and security_opt apparmor=unconfined")
	require.Contains(t, line, "The app still starts.")
}
