package config

import (
	"os"
	"path/filepath"
	"testing"
)

// withTempConfigDir points HSDEBUG_CONFIG_DIR/HSDEBUG_CONFIG at a temp dir.
func withTempConfigDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("HSDEBUG_CONFIG_DIR", dir)
	t.Setenv("HSDEBUG_CONFIG", filepath.Join(dir, "config.json"))
	return dir
}

func TestDefaultHasSaneValues(t *testing.T) {
	d := Default()
	if d.Server.Host != "127.0.0.1" {
		t.Errorf("default host = %q, want loopback", d.Server.Host)
	}
	if d.Agent.Probe.Sentinel == "" {
		t.Error("default sentinel must not be empty")
	}
	if d.Agent.Probe.TimeoutSeconds <= 0 {
		t.Error("default probe timeout must be positive")
	}
	if !d.Agent.Probe.Enabled {
		t.Error("probe should be enabled by default")
	}
	if d.Redact.Profile != RedactStrict {
		t.Errorf("default redact profile = %q, want strict", d.Redact.Profile)
	}
}

func TestLoadMissingReturnsDefault(t *testing.T) {
	withTempConfigDir(t)
	cfg, exists, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if exists {
		t.Error("exists should be false when no file present")
	}
	if cfg.Server.Port != Default().Server.Port {
		t.Error("missing config should equal default")
	}
}

func TestSaveThenLoadRoundTrip(t *testing.T) {
	withTempConfigDir(t)
	cfg := Default()
	cfg.Services = append(cfg.Services, ServiceConfig{
		Name: "sonarr", Host: "127.0.0.1", Port: 8989, Scheme: "http",
		HealthPath: "/ping", ExpectStatus: []int{200}, CatalogID: "sonarr",
	})
	cfg.Agent.Model = "anthropic/claude-opus-4-5"
	if err := cfg.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}

	got, exists, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !exists {
		t.Fatal("exists should be true after Save")
	}
	if len(got.Services) != 1 || got.Services[0].Name != "sonarr" {
		t.Errorf("services not persisted: %+v", got.Services)
	}
	if got.Agent.Model != "anthropic/claude-opus-4-5" {
		t.Errorf("model = %q", got.Agent.Model)
	}
}

func TestApplyDefaultsFillsZeroValues(t *testing.T) {
	withTempConfigDir(t)
	p, _ := Path()
	// Minimal config missing most fields.
	if err := os.WriteFile(p, []byte(`{"version":1}`), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, _, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Server.Host == "" || cfg.Server.Port == 0 {
		t.Error("server defaults not applied")
	}
	if cfg.Agent.Probe.Sentinel == "" || cfg.Agent.Probe.TimeoutSeconds == 0 {
		t.Error("probe defaults not applied")
	}
	if cfg.Redact.Profile == "" {
		t.Error("redact default not applied")
	}
	if cfg.Services == nil {
		t.Error("services should be non-nil")
	}
}

func TestLoadInvalidJSONReturnsError(t *testing.T) {
	withTempConfigDir(t)
	p, _ := Path()
	os.WriteFile(p, []byte(`{not json`), 0o644)
	_, exists, err := Load()
	if err == nil {
		t.Fatal("expected parse error")
	}
	if !exists {
		t.Error("exists should be true when file present but invalid")
	}
}

func TestPathHonorsEnvOverride(t *testing.T) {
	custom := filepath.Join(t.TempDir(), "custom.json")
	t.Setenv("HSDEBUG_CONFIG", custom)
	p, err := Path()
	if err != nil {
		t.Fatal(err)
	}
	if p != custom {
		t.Errorf("Path = %q, want %q", p, custom)
	}
}

func TestSaveCreatesDirAndTrailingNewline(t *testing.T) {
	dir := t.TempDir()
	nested := filepath.Join(dir, "a", "b")
	t.Setenv("HSDEBUG_CONFIG_DIR", nested)
	t.Setenv("HSDEBUG_CONFIG", filepath.Join(nested, "config.json"))
	if err := Default().Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}
	p, _ := Path()
	data, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	if len(data) == 0 || data[len(data)-1] != '\n' {
		t.Error("saved config should end with newline")
	}
}
