// Package agent manages the opencode agent that hsdebug drives: detecting and
// installing the binary, writing an isolated opencode config (model, provider,
// credentials via {file:} reference), and running a managed `opencode serve`
// instance that debug sessions attach to.
package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/gautambaghel/hsdebug/internal/config"
)

// Runner abstracts execution of the opencode binary so it can be faked in
// tests.
type Runner interface {
	// Look reports the resolved path of the opencode binary, or an error.
	Look() (string, error)
	// Output runs opencode with args and returns combined stdout.
	Output(ctx context.Context, args ...string) ([]byte, error)
}

// ExecRunner is the production Runner using os/exec.
type ExecRunner struct {
	Env []string // extra environment (e.g. OPENCODE_CONFIG=...)
}

func (r ExecRunner) Look() (string, error) { return exec.LookPath("opencode") }

func (r ExecRunner) Output(ctx context.Context, args ...string) ([]byte, error) {
	path, err := r.Look()
	if err != nil {
		return nil, err
	}
	cmd := exec.CommandContext(ctx, path, args...)
	cmd.Env = append(os.Environ(), r.Env...)
	return cmd.Output()
}

// Manager coordinates agent configuration and operations.
type Manager struct {
	cfg    *config.Config
	runner Runner
}

// NewManager returns a Manager. If runner is nil, an ExecRunner configured with
// the isolated opencode config path is used.
func NewManager(cfg *config.Config, runner Runner) (*Manager, error) {
	if runner == nil {
		ocp, err := config.OpencodeConfigPath()
		if err != nil {
			return nil, err
		}
		runner = ExecRunner{Env: []string{"OPENCODE_CONFIG=" + ocp}}
	}
	return &Manager{cfg: cfg, runner: runner}, nil
}

// Installed reports whether the opencode binary is available.
func (m *Manager) Installed() (string, bool) {
	path, err := m.runner.Look()
	if err != nil {
		return "", false
	}
	return path, true
}

// InstallScript is the official one-line installer for opencode.
const InstallScript = "curl -fsSL https://opencode.ai/install | bash"

// Install runs the official opencode installer via a shell. It is a no-op that
// returns the already-installed path if opencode is present.
func (m *Manager) Install(ctx context.Context) (string, error) {
	if path, ok := m.Installed(); ok {
		return path, nil
	}
	cmd := exec.CommandContext(ctx, "bash", "-c", InstallScript)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("opencode install failed: %w", err)
	}
	path, ok := m.Installed()
	if !ok {
		return "", fmt.Errorf("opencode still not found on PATH after install; you may need to restart your shell")
	}
	return path, nil
}

// OpencodeConfig is the subset of opencode's config schema hsdebug writes.
type OpencodeConfig struct {
	Schema     string                    `json:"$schema"`
	Model      string                    `json:"model,omitempty"`
	SmallModel string                    `json:"small_model,omitempty"`
	Provider   map[string]ProviderBlock  `json:"provider,omitempty"`
	Permission map[string]any            `json:"permission,omitempty"`
}

type ProviderBlock struct {
	Options map[string]any `json:"options,omitempty"`
}

// SetModel updates the model (and provider inferred from provider/model) in
// hsdebug config and regenerates the isolated opencode config.
func (m *Manager) SetModel(model string) error {
	if !strings.Contains(model, "/") {
		return fmt.Errorf("model must be in provider/model form, got %q", model)
	}
	m.cfg.Agent.Model = model
	m.cfg.Agent.Provider = strings.SplitN(model, "/", 2)[0]
	if err := m.cfg.Save(); err != nil {
		return err
	}
	return m.WriteOpencodeConfig()
}

// SetProviderKey stores an API key for a provider and references it from the
// isolated opencode config via {file:}. This is the non-interactive "bypass"
// path that avoids `opencode auth login`.
//
// Exactly one of key, keyEnv, or keyFile should be non-empty.
func (m *Manager) SetProviderKey(provider, key, keyEnv, keyFile string) error {
	if provider == "" {
		return fmt.Errorf("provider is required")
	}
	switch {
	case key != "":
		secretsDir, err := config.SecretsDir()
		if err != nil {
			return err
		}
		if err := os.MkdirAll(secretsDir, 0o700); err != nil {
			return err
		}
		path := filepath.Join(secretsDir, provider)
		if err := os.WriteFile(path, []byte(strings.TrimSpace(key)), 0o600); err != nil {
			return err
		}
		m.cfg.Agent.CredentialRef = path
	case keyFile != "":
		if _, err := os.Stat(keyFile); err != nil {
			return fmt.Errorf("key file %q: %w", keyFile, err)
		}
		m.cfg.Agent.CredentialRef = keyFile
	case keyEnv != "":
		// Reference an env var; store the reference marker.
		m.cfg.Agent.CredentialRef = "env:" + keyEnv
	default:
		return fmt.Errorf("one of --key, --key-file, or --key-env is required")
	}
	m.cfg.Agent.Provider = provider
	if err := m.cfg.Save(); err != nil {
		return err
	}
	return m.WriteOpencodeConfig()
}

// permissionPolicy returns the opencode permission block. In god mode every
// tool is allowed; otherwise potentially destructive tools require an explicit
// ask (surfaced to the operator via the UI or --auto).
func permissionPolicy(godMode bool) map[string]any {
	if godMode {
		return map[string]any{"edit": "allow", "bash": "allow", "webfetch": "allow"}
	}
	return map[string]any{"edit": "ask", "bash": "ask", "webfetch": "allow"}
}

// SetGodMode toggles elevated permissions and regenerates the isolated opencode
// config so the change takes effect on the next run.
func (m *Manager) SetGodMode(enabled bool) error {
	m.cfg.Agent.GodMode = enabled
	if err := m.cfg.Save(); err != nil {
		return err
	}
	return m.WriteOpencodeConfig()
}

// GodMode reports whether elevated permissions are enabled.
func (m *Manager) GodMode() bool { return m.cfg.Agent.GodMode }

// apiKeyField maps a provider id to the option key opencode expects. Most
// providers use "apiKey".
func apiKeyField(provider string) string { return "apiKey" }

// credentialSubstitution returns the {file:}/{env:} substitution string for the
// stored credential reference, or "" if none.
func (m *Manager) credentialSubstitution() string {
	ref := m.cfg.Agent.CredentialRef
	switch {
	case ref == "":
		return ""
	case strings.HasPrefix(ref, "env:"):
		return "{env:" + strings.TrimPrefix(ref, "env:") + "}"
	default:
		return "{file:" + ref + "}"
	}
}

// WriteOpencodeConfig regenerates the isolated opencode config from hsdebug
// config.
func (m *Manager) WriteOpencodeConfig() error {
	ocp, err := config.OpencodeConfigPath()
	if err != nil {
		return err
	}
	oc := OpencodeConfig{
		Schema:     "https://opencode.ai/config.json",
		Model:      m.cfg.Agent.Model,
		SmallModel: m.cfg.Agent.SmallModel,
	}
	if sub := m.credentialSubstitution(); sub != "" && m.cfg.Agent.Provider != "" {
		oc.Provider = map[string]ProviderBlock{
			m.cfg.Agent.Provider: {Options: map[string]any{apiKeyField(m.cfg.Agent.Provider): sub}},
		}
	}
	oc.Permission = permissionPolicy(m.cfg.Agent.GodMode)
	if err := os.MkdirAll(filepath.Dir(ocp), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(oc, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(ocp, append(data, '\n'), 0o644)
}

// Configured reports whether the agent has a model and a resolvable credential.
func (m *Manager) Configured() (bool, string) {
	if m.cfg.Agent.Model == "" {
		return false, "no model configured (run `hsdebug agent model <provider/model>`)"
	}
	ref := m.cfg.Agent.CredentialRef
	switch {
	case ref == "":
		// Allow env-based provider keys discovered by opencode itself.
		if envKeySet(m.cfg.Agent.Provider) {
			return true, "using provider env credentials"
		}
		return false, "no provider credential configured (run `hsdebug agent provider <id> --key ...`)"
	case strings.HasPrefix(ref, "env:"):
		if os.Getenv(strings.TrimPrefix(ref, "env:")) == "" {
			return false, "referenced env var " + strings.TrimPrefix(ref, "env:") + " is empty"
		}
		return true, "using env credential"
	default:
		data, err := os.ReadFile(ref)
		if err != nil || len(strings.TrimSpace(string(data))) == 0 {
			return false, "credential file missing or empty: " + ref
		}
		return true, "using file credential"
	}
}

// envKeySet reports whether a well-known provider env var is set.
func envKeySet(provider string) bool {
	candidates := map[string][]string{
		"anthropic": {"ANTHROPIC_API_KEY"},
		"openai":    {"OPENAI_API_KEY"},
		"gemini":    {"GEMINI_API_KEY", "GOOGLE_API_KEY"},
		"opencode":  {"OPENCODE_API_KEY"},
	}
	for _, v := range candidates[provider] {
		if os.Getenv(v) != "" {
			return true
		}
	}
	return false
}
