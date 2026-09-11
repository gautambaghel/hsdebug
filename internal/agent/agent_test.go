package agent

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gautambaghel/hsdebug/internal/config"
)

// fakeRunner implements Runner for tests.
type fakeRunner struct {
	installed bool
	output    []byte
	err       error
	gotArgs   []string
}

func (f *fakeRunner) Look() (string, error) {
	if !f.installed {
		return "", os.ErrNotExist
	}
	return "/fake/opencode", nil
}
func (f *fakeRunner) Output(ctx context.Context, args ...string) ([]byte, error) {
	f.gotArgs = args
	return f.output, f.err
}

func tempEnv(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("HSDEBUG_CONFIG_DIR", dir)
	t.Setenv("HSDEBUG_CONFIG", filepath.Join(dir, "config.json"))
}

func TestSetModelValidatesFormat(t *testing.T) {
	tempEnv(t)
	m, _ := NewManager(config.Default(), &fakeRunner{installed: true})
	if err := m.SetModel("justmodel"); err == nil {
		t.Error("model without provider/ should fail")
	}
	if err := m.SetModel("anthropic/claude-sonnet-4-5"); err != nil {
		t.Fatalf("valid model: %v", err)
	}
	if m.cfg.Agent.Provider != "anthropic" {
		t.Errorf("provider inferred = %q", m.cfg.Agent.Provider)
	}
}

func TestSetProviderKeyWritesSecretFileAndReference(t *testing.T) {
	tempEnv(t)
	cfg := config.Default()
	m, _ := NewManager(cfg, &fakeRunner{installed: true})
	if err := m.SetProviderKey("anthropic", "sk-secretvalue", "", ""); err != nil {
		t.Fatalf("SetProviderKey: %v", err)
	}
	// Secret file exists with 0600 and contains the key.
	ref := cfg.Agent.CredentialRef
	info, err := os.Stat(ref)
	if err != nil {
		t.Fatalf("secret file: %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Errorf("secret perms = %v, want 0600", info.Mode().Perm())
	}
	data, _ := os.ReadFile(ref)
	if strings.TrimSpace(string(data)) != "sk-secretvalue" {
		t.Errorf("secret content = %q", string(data))
	}

	// Isolated opencode config references it via {file:}.
	ocp, _ := config.OpencodeConfigPath()
	ocData, _ := os.ReadFile(ocp)
	var oc OpencodeConfig
	json.Unmarshal(ocData, &oc)
	sub := oc.Provider["anthropic"].Options["apiKey"].(string)
	if !strings.HasPrefix(sub, "{file:") || !strings.Contains(sub, ref) {
		t.Errorf("opencode config should reference key file, got %q", sub)
	}
}

func TestSetProviderKeyEnvReference(t *testing.T) {
	tempEnv(t)
	cfg := config.Default()
	m, _ := NewManager(cfg, &fakeRunner{installed: true})
	if err := m.SetProviderKey("openai", "", "OPENAI_API_KEY", ""); err != nil {
		t.Fatal(err)
	}
	ocp, _ := config.OpencodeConfigPath()
	data, _ := os.ReadFile(ocp)
	if !strings.Contains(string(data), "{env:OPENAI_API_KEY}") {
		t.Errorf("expected env substitution, got %s", data)
	}
}

func TestSetProviderKeyRequiresOne(t *testing.T) {
	tempEnv(t)
	m, _ := NewManager(config.Default(), &fakeRunner{installed: true})
	if err := m.SetProviderKey("anthropic", "", "", ""); err == nil {
		t.Error("expected error when no credential source given")
	}
}

func TestConfiguredStates(t *testing.T) {
	tempEnv(t)
	cfg := config.Default()
	cfg.Agent.Model = ""
	m, _ := NewManager(cfg, &fakeRunner{installed: true})
	if ok, _ := m.Configured(); ok {
		t.Error("no model should be unconfigured")
	}

	cfg.Agent.Model = "anthropic/claude-sonnet-4-5"
	cfg.Agent.CredentialRef = ""
	if ok, _ := m.Configured(); ok {
		t.Error("no credential should be unconfigured")
	}

	// File credential present.
	f := filepath.Join(t.TempDir(), "key")
	os.WriteFile(f, []byte("sk-abc"), 0o600)
	cfg.Agent.CredentialRef = f
	if ok, _ := m.Configured(); !ok {
		t.Error("file credential should be configured")
	}

	// Empty file credential is not configured.
	empty := filepath.Join(t.TempDir(), "empty")
	os.WriteFile(empty, []byte("  "), 0o600)
	cfg.Agent.CredentialRef = empty
	if ok, _ := m.Configured(); ok {
		t.Error("empty credential file should be unconfigured")
	}
}

func TestInstalled(t *testing.T) {
	tempEnv(t)
	m, _ := NewManager(config.Default(), &fakeRunner{installed: false})
	if _, ok := m.Installed(); ok {
		t.Error("should report not installed")
	}
	m2, _ := NewManager(config.Default(), &fakeRunner{installed: true})
	if _, ok := m2.Installed(); !ok {
		t.Error("should report installed")
	}
}
