// Package config defines the hsdebug configuration model, its default
// template, and load/save helpers. All configuration lives in a single JSON
// file (default: ~/.config/hsdebug/config.json, override via HSDEBUG_CONFIG).
package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// Redaction profiles for output masking.
const (
	RedactStrict = "strict" // default: known secrets + declared-sensitive + heuristic
	RedactKnown  = "known"  // known secret fields + declared-sensitive only
	RedactOff    = "off"    // no masking
)

// Config is the root configuration object persisted as JSON.
type Config struct {
	Version           int             `json:"version"`
	Server            ServerConfig    `json:"server"`
	Agent             AgentConfig     `json:"agent"`
	Redact            RedactConfig    `json:"redact"`
	PromptLibraryPath string          `json:"promptLibraryPath"`
	// ProbeHost is the host used when auto-registering/scanning catalog
	// services and as the default for `register`. Defaults to 127.0.0.1. Set
	// to "host.docker.internal" when hsdebug runs in a bridged container.
	// Overridable at runtime via the HSDEBUG_PROBE_HOST env var.
	ProbeHost string          `json:"probeHost"`
	Services  []ServiceConfig `json:"services"`
}

// ServerConfig controls the local HTTP API + UI service.
type ServerConfig struct {
	Host string `json:"host"` // default 127.0.0.1 (loopback only)
	Port int    `json:"port"`
}

// AgentConfig captures how the opencode agent is configured and probed.
type AgentConfig struct {
	Model      string      `json:"model"`      // provider/model, e.g. anthropic/claude-sonnet-4-5
	SmallModel string      `json:"smallModel"` // optional lightweight model
	Provider   string      `json:"provider"`   // primary provider id, e.g. anthropic
	// CredentialRef points at the secret file referenced by the isolated
	// opencode config via {file:}. Empty means credentials come from env.
	CredentialRef string     `json:"credentialRef"`
	OpencodePort  int        `json:"opencodePort"` // managed `opencode serve` port
	// GodMode elevates opencode permissions (allow-all) for debug runs. It is
	// written into the isolated opencode config's permission block and implies
	// auto-approval. Off by default.
	GodMode bool        `json:"godMode"`
	Probe   ProbeConfig `json:"probe"`
}

// ProbeConfig makes the AI dry-run (liveness) behaviour configurable.
type ProbeConfig struct {
	Enabled        bool   `json:"enabled"`        // gate debug on a successful dry-run
	Sentinel       string `json:"sentinel"`       // token the model must echo back
	TimeoutSeconds int    `json:"timeoutSeconds"` // max wait for the dry-run response
}

// RedactConfig controls output masking.
type RedactConfig struct {
	Profile string `json:"profile"` // strict | known | off
}

// ServiceConfig is a registered local service.
type ServiceConfig struct {
	Name         string `json:"name"`
	Host         string `json:"host"`         // constrained to loopback
	Port         int    `json:"port"`
	Scheme       string `json:"scheme"`       // http | https
	HealthPath   string `json:"healthPath"`   // e.g. / or /api/health
	ExpectStatus []int  `json:"expectStatus"` // acceptable HTTP status codes
	PingEnabled  bool   `json:"pingEnabled"`
	PromptFile   string `json:"promptFile,omitempty"`
	PromptText   string `json:"promptText,omitempty"`
	CatalogID    string `json:"catalogId,omitempty"` // links to prompt library entry
}

// Default returns a fresh config populated with sensible defaults.
func Default() *Config {
	return &Config{
		Version: 1,
		Server: ServerConfig{
			Host: "127.0.0.1",
			Port: 7654,
		},
		Agent: AgentConfig{
			Model:        "anthropic/claude-sonnet-4-5",
			SmallModel:   "",
			Provider:     "anthropic",
			OpencodePort: 4096,
			Probe: ProbeConfig{
				Enabled:        true,
				Sentinel:       "HSDEBUG_OK",
				TimeoutSeconds: 60,
			},
		},
		Redact:            RedactConfig{Profile: RedactStrict},
		PromptLibraryPath: "",
		ProbeHost:         "127.0.0.1",
		Services:          []ServiceConfig{},
	}
}

// Dir returns the hsdebug config directory, honoring HSDEBUG_CONFIG_DIR.
func Dir() (string, error) {
	if d := os.Getenv("HSDEBUG_CONFIG_DIR"); d != "" {
		return d, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "hsdebug"), nil
}

// Path returns the config file path, honoring HSDEBUG_CONFIG.
func Path() (string, error) {
	if p := os.Getenv("HSDEBUG_CONFIG"); p != "" {
		return p, nil
	}
	dir, err := Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "config.json"), nil
}

// OpencodeConfigPath returns the path to the isolated opencode config that
// hsdebug manages (pointed at via OPENCODE_CONFIG when invoking opencode).
func OpencodeConfigPath() (string, error) {
	dir, err := Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "opencode.json"), nil
}

// SecretsDir returns the directory where provider credential files live.
func SecretsDir() (string, error) {
	dir, err := Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "secrets"), nil
}

// Load reads the config file. If it does not exist, it returns the default
// config and reports exists=false so callers can prompt initialization.
func Load() (cfg *Config, exists bool, err error) {
	p, err := Path()
	if err != nil {
		return nil, false, err
	}
	data, err := os.ReadFile(p)
	if os.IsNotExist(err) {
		return Default(), false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("reading config %s: %w", p, err)
	}
	cfg = &Config{}
	if err := json.Unmarshal(data, cfg); err != nil {
		return nil, true, fmt.Errorf("parsing config %s: %w", p, err)
	}
	cfg.applyDefaults()
	return cfg, true, nil
}

// applyDefaults fills zero-valued fields that must never be empty.
func (c *Config) applyDefaults() {
	d := Default()
	if c.Version == 0 {
		c.Version = d.Version
	}
	if c.Server.Host == "" {
		c.Server.Host = d.Server.Host
	}
	if c.Server.Port == 0 {
		c.Server.Port = d.Server.Port
	}
	if c.Agent.OpencodePort == 0 {
		c.Agent.OpencodePort = d.Agent.OpencodePort
	}
	if c.Agent.Probe.Sentinel == "" {
		c.Agent.Probe.Sentinel = d.Agent.Probe.Sentinel
	}
	if c.Agent.Probe.TimeoutSeconds == 0 {
		c.Agent.Probe.TimeoutSeconds = d.Agent.Probe.TimeoutSeconds
	}
	if c.Redact.Profile == "" {
		c.Redact.Profile = d.Redact.Profile
	}
	if c.ProbeHost == "" {
		c.ProbeHost = d.ProbeHost
	}
	if c.Services == nil {
		c.Services = []ServiceConfig{}
	}
}

// ResolveProbeHost returns the host to use for probing/auto-registering
// services, in priority order: HSDEBUG_PROBE_HOST env var, config ProbeHost,
// then 127.0.0.1.
func (c *Config) ResolveProbeHost() string {
	if h := os.Getenv("HSDEBUG_PROBE_HOST"); h != "" {
		return h
	}
	if c.ProbeHost != "" {
		return c.ProbeHost
	}
	return "127.0.0.1"
}

// Save writes the config file, creating the directory if needed.
func (c *Config) Save() error {
	p, err := Path()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(p, data, 0o644)
}
