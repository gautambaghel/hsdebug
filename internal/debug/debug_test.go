package debug

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gautambaghel/hsdebug/internal/agent"
	"github.com/gautambaghel/hsdebug/internal/config"
)

// fakeRunner mirrors agent's fake for preflight probe control.
type fakeRunner struct {
	installed bool
	output    []byte
	err       error
}

func (f *fakeRunner) Look() (string, error) {
	if !f.installed {
		return "", os.ErrNotExist
	}
	return "/fake/opencode", nil
}
func (f *fakeRunner) Output(ctx context.Context, args ...string) ([]byte, error) {
	return f.output, f.err
}

func configuredCfg(t *testing.T) *config.Config {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("HSDEBUG_CONFIG_DIR", dir)
	t.Setenv("HSDEBUG_CONFIG", filepath.Join(dir, "config.json"))
	cfg := config.Default()
	cfg.Agent.Model = "anthropic/claude-sonnet-4-5"
	kf := filepath.Join(dir, "key")
	os.WriteFile(kf, []byte("sk-abc"), 0o600)
	cfg.Agent.CredentialRef = kf
	return cfg
}

func TestPreflightAllPass(t *testing.T) {
	cfg := configuredCfg(t)
	runner := &fakeRunner{installed: true, output: []byte(`{"text":"HSDEBUG_OK"}`)}
	mgr, _ := agent.NewManager(cfg, runner)
	pf := RunPreflight(context.Background(), mgr, cfg)
	if !pf.Passed {
		t.Fatalf("expected pass: %+v", pf.Steps)
	}
	if len(pf.Steps) != 3 {
		t.Errorf("expected 3 steps, got %d", len(pf.Steps))
	}
}

func TestPreflightStopsWhenUnconfigured(t *testing.T) {
	cfg := configuredCfg(t)
	cfg.Agent.Model = "" // unconfigured
	runner := &fakeRunner{installed: true, output: []byte(`{"text":"HSDEBUG_OK"}`)}
	mgr, _ := agent.NewManager(cfg, runner)
	pf := RunPreflight(context.Background(), mgr, cfg)
	if pf.Passed {
		t.Error("should fail when unconfigured")
	}
	if len(pf.Steps) != 1 {
		t.Errorf("should stop after first step, got %d", len(pf.Steps))
	}
}

func TestPreflightStopsWhenNotInstalled(t *testing.T) {
	cfg := configuredCfg(t)
	runner := &fakeRunner{installed: false}
	mgr, _ := agent.NewManager(cfg, runner)
	pf := RunPreflight(context.Background(), mgr, cfg)
	if pf.Passed {
		t.Error("should fail when opencode missing")
	}
	// agent-configured passes, opencode-present fails, stop.
	if len(pf.Steps) != 2 {
		t.Errorf("expected 2 steps, got %d", len(pf.Steps))
	}
}

func TestPreflightFailsOnBadDryRun(t *testing.T) {
	cfg := configuredCfg(t)
	runner := &fakeRunner{installed: true, output: []byte(`{"text":"wrong"}`)}
	mgr, _ := agent.NewManager(cfg, runner)
	pf := RunPreflight(context.Background(), mgr, cfg)
	if pf.Passed {
		t.Error("bad dry-run should fail preflight")
	}
	last := pf.Steps[len(pf.Steps)-1]
	if last.Name != "ai-dry-run" || last.OK {
		t.Errorf("last step should be failed dry-run: %+v", last)
	}
}

func TestPreflightProbeDisabled(t *testing.T) {
	cfg := configuredCfg(t)
	cfg.Agent.Probe.Enabled = false
	runner := &fakeRunner{installed: true}
	mgr, _ := agent.NewManager(cfg, runner)
	pf := RunPreflight(context.Background(), mgr, cfg)
	if !pf.Passed {
		t.Error("should pass with probe disabled")
	}
	last := pf.Steps[len(pf.Steps)-1]
	if !strings.Contains(last.Detail, "disabled") {
		t.Errorf("expected disabled note: %+v", last)
	}
}

func TestCollectNoServices(t *testing.T) {
	cfg := configuredCfg(t)
	if _, err := Collect(cfg, time.Second); err == nil {
		t.Error("expected error with no services")
	}
}

func TestCollectOnlyUnhealthy(t *testing.T) {
	cfg := configuredCfg(t)
	// Healthy server.
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) }))
	defer up.Close()
	uh, up2 := parse(t, up.URL)

	cfg.Services = []config.ServiceConfig{
		{Name: "healthy", Host: uh, Port: up2, Scheme: "http", HealthPath: "/", ExpectStatus: []int{200}},
		{Name: "down", Host: "127.0.0.1", Port: 1, Scheme: "http", HealthPath: "/", CatalogID: "sonarr"},
	}
	tasks, err := Collect(cfg, 2*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if len(tasks) != 1 || tasks[0].Service.Name != "down" {
		t.Fatalf("expected only 'down', got %+v", tasks)
	}
	if tasks[0].Prompt == "" {
		t.Error("prompt should be attached")
	}
}

func TestBuildPromptIncludesContext(t *testing.T) {
	task := Task{
		Service: config.ServiceConfig{Name: "sonarr", Host: "127.0.0.1", Port: 8989, Scheme: "http", HealthPath: "/ping"},
		Prompt:  "BASE PROMPT",
	}
	task.Health.URL = "http://127.0.0.1:8989/ping"
	task.Health.Error = "connection refused"
	out := BuildPrompt(task)
	if !strings.Contains(out, "BASE PROMPT") {
		t.Error("should include base prompt")
	}
	if !strings.Contains(out, "sonarr") || !strings.Contains(out, "connection refused") {
		t.Errorf("should include service context: %q", out)
	}
}

func parse(t *testing.T, url string) (string, int) {
	t.Helper()
	s := strings.TrimPrefix(url, "http://")
	i := strings.LastIndex(s, ":")
	p, _ := strconv.Atoi(s[i+1:])
	return s[:i], p
}
