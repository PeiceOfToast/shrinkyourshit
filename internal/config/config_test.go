// SPDX-License-Identifier: AGPL-3.0-only
package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// testHome points the test's home dir at a temp dir so the default data dir
// is deterministic.
func testHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home) // os.UserHomeDir on Windows
	return home
}

func TestDefaults(t *testing.T) {
	home := testHome(t)
	env := []string{
		"HOSTNAME=test-host",
		"SHRINK_PORT=",
	}
	// Strip variables the environment may set; only our controlled env counts.
	for _, k := range []string{"SHRINK_ROLE", "SHRINK_PORT", "SHRINK_DATA_DIR", "SHRINK_SCRATCH_DIR",
		"SHRINK_SERVER", "SHRINK_TOKEN", "SHRINK_NODE_NAME", "SHRINK_FFMPEG_PATH", "SHRINK_LOG_LEVEL"} {
		env = append(env, k+"=")
	}

	cfg, err := Load(Options{Environ: env})
	require.NoError(t, err)

	// os.Hostname does not read the HOSTNAME env var; compare with reality.
	wantHost, herr := os.Hostname()
	if herr != nil || wantHost == "" {
		wantHost = "shrinkyourshit"
	}

	assert.Equal(t, RoleBoth, cfg.Role)
	assert.Equal(t, 8765, cfg.Port)
	assert.Equal(t, filepath.Join(home, ".shrinkyourshit"), cfg.DataDir)
	assert.Equal(t, filepath.Join(home, ".shrinkyourshit", "scratch"), cfg.ScratchDir)
	assert.Equal(t, "", cfg.Server)
	assert.Equal(t, "", cfg.Token)
	assert.Equal(t, wantHost, cfg.NodeName)
	assert.Equal(t, "", cfg.FFmpegPath)
	assert.Equal(t, "info", cfg.LogLevel)
}

func TestEnvOverridesDefault(t *testing.T) {
	testHome(t)
	env := []string{
		"SHRINK_ROLE=worker",
		"SHRINK_PORT=9000",
		"SHRINK_DATA_DIR=/data",
		"SHRINK_SCRATCH_DIR=/scratchy",
		"SHRINK_SERVER=http://192.168.1.10:8765",
		"SHRINK_TOKEN=abc123",
		"SHRINK_NODE_NAME=nas",
		"SHRINK_FFMPEG_PATH=/usr/bin/ffmpeg",
		"SHRINK_LOG_LEVEL=debug",
	}

	cfg, err := Load(Options{Environ: env})
	require.NoError(t, err)

	assert.Equal(t, "worker", cfg.Role)
	assert.Equal(t, 9000, cfg.Port)
	assert.Equal(t, "/data", cfg.DataDir)
	assert.Equal(t, "/scratchy", cfg.ScratchDir)
	assert.Equal(t, "http://192.168.1.10:8765", cfg.Server)
	assert.Equal(t, "abc123", cfg.Token)
	assert.Equal(t, "nas", cfg.NodeName)
	assert.Equal(t, "/usr/bin/ffmpeg", cfg.FFmpegPath)
	assert.Equal(t, "debug", cfg.LogLevel)
	assert.Equal(t, "/usr/bin/ffprobe", cfg.FFprobePath())
}

func TestFlagOverridesEnv(t *testing.T) {
	testHome(t)
	env := []string{"SHRINK_PORT=9000", "SHRINK_ROLE=worker"}

	cfg, err := Load(Options{
		Args:    []string{"--port", "9999", "--role", "server"},
		Environ: env,
	})
	require.NoError(t, err)

	assert.Equal(t, 9999, cfg.Port)
	assert.Equal(t, "server", cfg.Role)
}

func TestFileOverridesDefault(t *testing.T) {
	home := testHome(t)
	dir := t.TempDir()
	cfgFile := filepath.Join(dir, "config.yaml")
	require.NoError(t, os.WriteFile(cfgFile, []byte(
		"role: server\nport: 7000\nnode_name: file-node\nlog_level: warn\n"), 0o644))

	cfg, err := Load(Options{Environ: nil, FileOverride: cfgFile})
	require.NoError(t, err)

	assert.Equal(t, "server", cfg.Role)
	assert.Equal(t, 7000, cfg.Port)
	assert.Equal(t, "file-node", cfg.NodeName)
	assert.Equal(t, "warn", cfg.LogLevel)
	// The file does not set data_dir, so the default still applies.
	assert.Equal(t, filepath.Join(home, ".shrinkyourshit"), cfg.DataDir)
	// Scratch defaults under the resolved data dir.
	assert.Equal(t, filepath.Join(home, ".shrinkyourshit", "scratch"), cfg.ScratchDir)
}

func TestFileOverridesDefault_envOverridesFile(t *testing.T) {
	testHome(t)
	dir := t.TempDir()
	cfgFile := filepath.Join(dir, "config.yaml")
	require.NoError(t, os.WriteFile(cfgFile, []byte("port: 7000\n"), 0o644))

	cfg, err := Load(Options{
		Environ:      []string{"SHRINK_PORT=8000"},
		FileOverride: cfgFile,
	})
	require.NoError(t, err)
	assert.Equal(t, 8000, cfg.Port)
}

func TestFlagOverridesEnvAndFile(t *testing.T) {
	testHome(t)
	dir := t.TempDir()
	cfgFile := filepath.Join(dir, "config.yaml")
	require.NoError(t, os.WriteFile(cfgFile, []byte("port: 7000\nnode_name: file-node\n"), 0o644))

	cfg, err := Load(Options{
		Args:         []string{"--port", "8123"},
		Environ:      []string{"SHRINK_PORT=8000"},
		FileOverride: cfgFile,
	})
	require.NoError(t, err)
	assert.Equal(t, 8123, cfg.Port)
	// File still wins for keys the flag and env do not set.
	assert.Equal(t, "file-node", cfg.NodeName)
}

func TestHomeRelativeDataDirFindsConfigFile(t *testing.T) {
	home := testHome(t)
	require.NoError(t, os.MkdirAll(filepath.Join(home, "shrink-config"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(home, "shrink-config", "config.yaml"),
		[]byte("port: 7000\n"), 0o644))

	env := []string{"SHRINK_DATA_DIR=~/shrink-config"}

	cfg, err := Load(Options{Environ: env})
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(home, "shrink-config"), cfg.DataDir)
	// The config file inside the home-relative data dir must be found.
	assert.Equal(t, 7000, cfg.Port)
}

func TestFileDataDirWinsOverDefault(t *testing.T) {
	testHome(t)
	// The file lives in the default data dir and moves the data dir.
	home, err := os.UserHomeDir()
	require.NoError(t, err)
	defaultDir := filepath.Join(home, ".shrinkyourshit")
	require.NoError(t, os.MkdirAll(defaultDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(defaultDir, "config.yaml"),
		[]byte("data_dir: /elsewhere\n"), 0o644))

	cfg, err := Load(Options{Environ: []string{"HOSTNAME=x"}})
	require.NoError(t, err)
	assert.Equal(t, "/elsewhere", cfg.DataDir)
	assert.Equal(t, "/elsewhere/scratch", cfg.ScratchDir)
}

func TestScratchDefaultsUnderDataDir(t *testing.T) {
	testHome(t)
	env := []string{"SHRINK_DATA_DIR=/data"}

	cfg, err := Load(Options{Environ: env})
	require.NoError(t, err)
	assert.Equal(t, "/data/scratch", cfg.ScratchDir)
}

func TestInvalidValues(t *testing.T) {
	testHome(t)
	cases := []struct {
		name string
		opts Options
		want string
	}{
		{"bad role", Options{Environ: []string{"SHRINK_ROLE=cluster"}}, "invalid role"},
		{"bad port env", Options{Environ: []string{"SHRINK_PORT=notanumber"}}, "invalid SHRINK_PORT"},
		{"port too high", Options{Environ: []string{"SHRINK_PORT=70000"}}, "invalid port"},
		{"bad log level", Options{Environ: []string{"SHRINK_LOG_LEVEL=loud"}}, "invalid log level"},
		{"bad flag port", Options{Args: []string{"--port", "zero"}}, "invalid argument"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Load(tc.opts)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.want)
		})
	}
}

func TestBrokenConfigFile(t *testing.T) {
	testHome(t)
	dir := t.TempDir()
	cfgFile := filepath.Join(dir, "config.yaml")
	require.NoError(t, os.WriteFile(cfgFile, []byte("port: [not, a, number]\n"), 0o644))

	_, err := Load(Options{FileOverride: cfgFile})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "parse config file")
}

func TestMissingConfigFileOK(t *testing.T) {
	testHome(t)
	_, err := Load(Options{FileOverride: filepath.Join(t.TempDir(), "nope.yaml")})
	require.NoError(t, err)
}

func TestSlogLevel(t *testing.T) {
	cfg := Config{LogLevel: "debug"}
	level, err := cfg.SlogLevel()
	require.NoError(t, err)
	assert.Equal(t, "DEBUG", level.String())

	cfg.LogLevel = "bogus"
	_, err = cfg.SlogLevel()
	assert.Error(t, err)
}

func TestFFprobePathEmptyWhenAutoDetect(t *testing.T) {
	cfg := Config{}
	assert.Equal(t, "", cfg.FFprobePath())
}
