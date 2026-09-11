package agent

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/gautambaghel/hsdebug/internal/config"
)

func configuredManager(t *testing.T, runner Runner) (*Manager, *config.Config) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("HSDEBUG_CONFIG_DIR", dir)
	t.Setenv("HSDEBUG_CONFIG", filepath.Join(dir, "config.json"))
	cfg := config.Default()
	cfg.Agent.Model = "anthropic/claude-sonnet-4-5"
	keyFile := filepath.Join(dir, "key")
	os.WriteFile(keyFile, []byte("sk-abc"), 0o600)
	cfg.Agent.CredentialRef = keyFile
	m, err := NewManager(cfg, runner)
	if err != nil {
		t.Fatal(err)
	}
	return m, cfg
}

func TestProbeSuccessWithSentinel(t *testing.T) {
	runner := &fakeRunner{installed: true,
		output: []byte(`{"type":"message","text":"HSDEBUG_OK"}`)}
	m, _ := configuredManager(t, runner)
	pr := m.Probe(context.Background())
	if !pr.OK {
		t.Errorf("expected OK, got %+v", pr)
	}
	if !pr.Responded {
		t.Error("should mark responded")
	}
	// Verify probe used run --format json and the model flag.
	joined := ""
	for _, a := range runner.gotArgs {
		joined += a + " "
	}
	if !contains(runner.gotArgs, "run") || !contains(runner.gotArgs, "--format") || !contains(runner.gotArgs, "json") {
		t.Errorf("probe args missing run/json: %v", runner.gotArgs)
	}
	if !contains(runner.gotArgs, "-m") {
		t.Errorf("probe should pass model: %v", joined)
	}
}

func TestProbeSentinelMismatch(t *testing.T) {
	runner := &fakeRunner{installed: true,
		output: []byte(`{"type":"message","text":"hello there"}`)}
	m, _ := configuredManager(t, runner)
	pr := m.Probe(context.Background())
	if pr.OK {
		t.Error("mismatch should not be OK")
	}
	if !pr.Responded {
		t.Error("should still mark responded")
	}
}

func TestProbeNoResponse(t *testing.T) {
	runner := &fakeRunner{installed: true, output: []byte("")}
	m, _ := configuredManager(t, runner)
	pr := m.Probe(context.Background())
	if pr.OK || pr.Responded {
		t.Errorf("empty output should be no response: %+v", pr)
	}
}

func TestProbeRunError(t *testing.T) {
	runner := &fakeRunner{installed: true, err: errors.New("boom")}
	m, _ := configuredManager(t, runner)
	pr := m.Probe(context.Background())
	if pr.OK {
		t.Error("run error should fail probe")
	}
	if pr.Detail == "" {
		t.Error("expected error detail")
	}
}

func TestProbeUnconfiguredShortCircuits(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HSDEBUG_CONFIG_DIR", dir)
	t.Setenv("HSDEBUG_CONFIG", filepath.Join(dir, "config.json"))
	cfg := config.Default()
	cfg.Agent.Model = "" // unconfigured
	runner := &fakeRunner{installed: true, output: []byte("HSDEBUG_OK")}
	m, _ := NewManager(cfg, runner)
	pr := m.Probe(context.Background())
	if pr.OK {
		t.Error("unconfigured agent must not pass probe")
	}
	if runner.gotArgs != nil {
		t.Error("probe should not invoke opencode when unconfigured")
	}
}

func TestProbeCustomSentinel(t *testing.T) {
	runner := &fakeRunner{installed: true,
		output: []byte(`{"text":"PONG-42"}`)}
	m, cfg := configuredManager(t, runner)
	cfg.Agent.Probe.Sentinel = "PONG-42"
	pr := m.Probe(context.Background())
	if !pr.OK {
		t.Errorf("custom sentinel should match: %+v", pr)
	}
	if pr.Sentinel != "PONG-42" {
		t.Errorf("sentinel = %q", pr.Sentinel)
	}
}

func TestExtractAssistantTextFallback(t *testing.T) {
	// Non-JSON output falls back to raw.
	got := extractAssistantText([]byte("plain HSDEBUG_OK text"))
	if got == "" {
		t.Error("should fall back to raw output")
	}
}

func TestExtractAssistantTextNested(t *testing.T) {
	out := []byte(`{"parts":[{"text":"part one"},{"text":"HSDEBUG_OK"}]}`)
	got := extractAssistantText(out)
	if !contains2(got, "HSDEBUG_OK") {
		t.Errorf("nested text not extracted: %q", got)
	}
}

func contains(s []string, want string) bool {
	for _, v := range s {
		if v == want {
			return true
		}
	}
	return false
}
func contains2(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || (len(sub) > 0 && indexOf(s, sub) >= 0))
}
func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
