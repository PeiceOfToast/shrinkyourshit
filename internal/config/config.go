// SPDX-License-Identifier: AGPL-3.0-only

// Package config loads ShrinkYourShit configuration.
//
// Sources, in precedence order (highest first): command-line flags,
// environment variables with the SHRINK_ prefix, an optional config.yaml in
// the data dir, built-in defaults. See STACK.md for the full table.
package config

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/spf13/pflag"
	"gopkg.in/yaml.v3"
)

// Valid role values.
const (
	RoleServer = "server"
	RoleWorker = "worker"
	RoleBoth   = "both"
)

// Config is the resolved runtime configuration.
type Config struct {
	// Role is one of server, worker, both.
	Role string
	// Port is the HTTP port the server listens on.
	Port int
	// DataDir holds the database, settings and the node key.
	DataDir string
	// ScratchDir is per-job temp space on fast local disk.
	ScratchDir string
	// Server is the server URL for a worker, for example http://192.168.1.10:8765.
	Server string
	// Token is the one-time pairing token for a new worker.
	Token string
	// NodeName is the display name of this node.
	NodeName string
	// FFmpegPath is the path to ffmpeg; empty means auto-detect on PATH.
	// ffprobe is assumed to sit beside it.
	FFmpegPath string
	// LogLevel is one of debug, info, warn, error.
	LogLevel string
}

// Options controls where Load reads its inputs from. The zero value uses the
// process environment and parses no command-line flags.
type Options struct {
	// Args are the command-line arguments to parse as flags. May be nil.
	Args []string
	// Environ is a list of KEY=VALUE pairs. If nil, os.Environ is used.
	Environ []string
	// FileOverride reads this file instead of <data-dir>/config.yaml.
	// The override exists for tests; production code leaves it empty.
	FileOverride string
}

// fileConfig mirrors the config.yaml keys. Pointers distinguish "not set"
// from a zero value.
type fileConfig struct {
	Role       *string `yaml:"role"`
	Port       *int    `yaml:"port"`
	DataDir    *string `yaml:"data_dir"`
	ScratchDir *string `yaml:"scratch_dir"`
	Server     *string `yaml:"server"`
	Token      *string `yaml:"token"`
	NodeName   *string `yaml:"node_name"`
	FFmpegPath *string `yaml:"ffmpeg_path"`
	LogLevel   *string `yaml:"log_level"`
}

// Flag and environment names for every setting.
const (
	flagRole       = "role"
	envRole        = "SHRINK_ROLE"
	flagPort       = "port"
	envPort        = "SHRINK_PORT"
	flagDataDir    = "data-dir"
	envDataDir     = "SHRINK_DATA_DIR"
	flagScratchDir = "scratch-dir"
	envScratchDir  = "SHRINK_SCRATCH_DIR"
	flagServer     = "server"
	envServer      = "SHRINK_SERVER"
	flagToken      = "token"
	envToken       = "SHRINK_TOKEN"
	flagNodeName   = "node-name"
	envNodeName    = "SHRINK_NODE_NAME"
	flagFFmpeg     = "ffmpeg"
	envFFmpeg      = "SHRINK_FFMPEG_PATH"
	flagLogLevel   = "log-level"
	envLogLevel    = "SHRINK_LOG_LEVEL"
)

// NewFlags builds the pflag set shared by the serve and worker subcommands.
// Load parses the same set, so callers can pass their args on.
func NewFlags(name string) *pflag.FlagSet {
	flags := pflag.NewFlagSet(name, pflag.ContinueOnError)
	flags.String(flagRole, "", "server, worker, or both")
	flags.Int(flagPort, 0, "HTTP port (server)")
	flags.String(flagDataDir, "", "data dir for DB, settings and node key")
	flags.String(flagScratchDir, "", "per-job temp space, fast local disk")
	flags.String(flagServer, "", "server URL for a worker")
	flags.String(flagToken, "", "one-time pairing token for a new worker")
	flags.String(flagNodeName, "", "display name of this node")
	flags.String(flagFFmpeg, "", "path to ffmpeg; ffprobe assumed beside it")
	flags.String(flagLogLevel, "", "debug, info, warn or error")
	return flags
}

// Load resolves the configuration from flags, environment, config file and
// defaults, in that order of precedence. The config file is read from the
// data dir as resolved from flags, env and the default; if the file itself
// sets data_dir (and no flag or env does), the new value wins but the file is
// not re-read at the new location.
func Load(opts Options) (Config, error) {
	env := envMap(opts.Environ)

	flags := NewFlags("shrinkyourshit")
	if err := flags.Parse(opts.Args); err != nil {
		return Config{}, err
	}
	src := sources{flags: flags, env: env}

	// Locate the config file using the data dir before the file is applied:
	// flag > env > default.
	dataDir := dataDirDefault()
	if flags.Changed(flagDataDir) {
		dataDir, _ = flags.GetString(flagDataDir)
	} else if v, ok := env[envDataDir]; ok && v != "" {
		dataDir = v
	}
	if err := readConfigFile(opts.fileLocation(dataDir), &src.file); err != nil {
		return Config{}, err
	}

	// Resolve the data dir with full precedence: flag > env > file > default.
	dataDir = src.string(flagDataDir, envDataDir, src.file.DataDir, dataDirDefault())

	var err error
	cfg := Config{}
	cfg.Role = src.string(flagRole, envRole, src.file.Role, RoleBoth)
	if cfg.Port, err = src.int(flagPort, envPort, src.file.Port, defaultPort()); err != nil {
		return Config{}, err
	}
	if cfg.DataDir, err = expandHome(dataDir); err != nil {
		return Config{}, err
	}
	cfg.ScratchDir = src.string(flagScratchDir, envScratchDir, src.file.ScratchDir, "")
	if cfg.ScratchDir == "" {
		cfg.ScratchDir = filepath.Join(cfg.DataDir, "scratch")
	}
	cfg.Server = src.string(flagServer, envServer, src.file.Server, "")
	cfg.Token = src.string(flagToken, envToken, src.file.Token, "")
	cfg.NodeName = src.string(flagNodeName, envNodeName, src.file.NodeName, nodeNameDefault())
	cfg.FFmpegPath = src.string(flagFFmpeg, envFFmpeg, src.file.FFmpegPath, "")
	cfg.LogLevel = src.string(flagLogLevel, envLogLevel, src.file.LogLevel, "info")

	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

// fileLocation resolves which config file to read: an explicit override, or
// config.yaml inside the given data dir.
func (o Options) fileLocation(dataDir string) string {
	if o.FileOverride != "" {
		return o.FileOverride
	}
	return filepath.Join(dataDir, "config.yaml")
}

// Validate reports values that are not usable.
func (c Config) Validate() error {
	switch c.Role {
	case RoleServer, RoleWorker, RoleBoth:
	default:
		return fmt.Errorf("invalid role %q: want server, worker or both", c.Role)
	}
	if c.Port < 1 || c.Port > 65535 {
		return fmt.Errorf("invalid port %d: want 1 to 65535", c.Port)
	}
	switch c.LogLevel {
	case "debug", "info", "warn", "error":
	default:
		return fmt.Errorf("invalid log level %q: want debug, info, warn or error", c.LogLevel)
	}
	return nil
}

// SlogLevel maps LogLevel to a slog.Level.
func (c Config) SlogLevel() (slog.Level, error) {
	switch c.LogLevel {
	case "debug":
		return slog.LevelDebug, nil
	case "info":
		return slog.LevelInfo, nil
	case "warn":
		return slog.LevelWarn, nil
	case "error":
		return slog.LevelError, nil
	default:
		return 0, fmt.Errorf("invalid log level %q", c.LogLevel)
	}
}

// FFprobePath returns the path to ffprobe, assumed beside ffmpeg. Empty means
// ffprobe should be auto-detected on PATH.
func (c Config) FFprobePath() string {
	if c.FFmpegPath == "" {
		return ""
	}
	return filepath.Join(filepath.Dir(c.FFmpegPath), "ffprobe")
}

// sources resolves each setting across flags, environment and file, in that
// order of precedence, falling back to a default.
type sources struct {
	flags *pflag.FlagSet
	env   map[string]string
	file  fileConfig
}

// string resolves one string setting: flag > env > file > default.
func (s sources) string(flagName, envKey string, fileVal *string, def string) string {
	if s.flags.Changed(flagName) {
		v, _ := s.flags.GetString(flagName)
		return v
	}
	if v, ok := s.env[envKey]; ok && v != "" {
		return v
	}
	if fileVal != nil && *fileVal != "" {
		return *fileVal
	}
	return def
}

// int resolves one int setting: flag > env > file > default.
func (s sources) int(flagName, envKey string, fileVal *int, def int) (int, error) {
	if s.flags.Changed(flagName) {
		return s.flags.GetInt(flagName)
	}
	if v, ok := s.env[envKey]; ok && v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			return 0, fmt.Errorf("invalid %s %q: %w", envKey, v, err)
		}
		return n, nil
	}
	if fileVal != nil {
		return *fileVal, nil
	}
	return def, nil
}

func defaultPort() int { return 8765 }

// dataDirDefault is ~/.shrinkyourshit outside Docker. The Docker image sets
// SHRINK_DATA_DIR=/config, which overrides this.
func dataDirDefault() string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		home = "."
	}
	return filepath.Join(home, ".shrinkyourshit")
}

// nodeNameDefault falls back to the hostname.
func nodeNameDefault() string {
	name, err := os.Hostname()
	if err != nil || name == "" {
		return "shrinkyourshit"
	}
	return name
}

// readConfigFile parses an optional YAML config file. A missing file is not
// an error.
func readConfigFile(path string, out *fileConfig) error {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("read config file: %w", err)
	}
	if err := yaml.Unmarshal(data, out); err != nil {
		return fmt.Errorf("parse config file %s: %w", path, err)
	}
	return nil
}

// expandHome resolves a leading ~ to the user home directory.
func expandHome(p string) (string, error) {
	if p == "~" || strings.HasPrefix(p, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("resolve home dir: %w", err)
		}
		return filepath.Join(home, strings.TrimPrefix(p, "~")), nil
	}
	return p, nil
}

// envMap turns KEY=VALUE pairs into a lookup map.
func envMap(environ []string) map[string]string {
	if environ == nil {
		environ = os.Environ()
	}
	m := make(map[string]string, len(environ))
	for _, kv := range environ {
		k, v, _ := strings.Cut(kv, "=")
		m[k] = v
	}
	return m
}
