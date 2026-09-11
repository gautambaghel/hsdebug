// Package debug orchestrates the main debugging flow: it runs a preflight gate
// (agent configured, opencode present, AI dry-run responds), collects unhealthy
// services, attaches prompts, and runs opencode to diagnose them.
package debug

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/gautambaghel/hsdebug/internal/agent"
	"github.com/gautambaghel/hsdebug/internal/config"
	"github.com/gautambaghel/hsdebug/internal/health"
	"github.com/gautambaghel/hsdebug/internal/prompts"
)

// PreflightStep is one gate in the preflight sequence.
type PreflightStep struct {
	Name   string `json:"name"`
	OK     bool   `json:"ok"`
	Detail string `json:"detail"`
}

// Preflight holds the ordered results of the gate.
type Preflight struct {
	Steps  []PreflightStep `json:"steps"`
	Passed bool            `json:"passed"`
}

// RunPreflight verifies the agent is properly configured and the AI responds
// before any real debugging proceeds. The probe is only run when enabled in
// config.
func RunPreflight(ctx context.Context, mgr *agent.Manager, cfg *config.Config) Preflight {
	pf := Preflight{Passed: true}
	add := func(name string, ok bool, detail string) {
		pf.Steps = append(pf.Steps, PreflightStep{Name: name, OK: ok, Detail: detail})
		if !ok {
			pf.Passed = false
		}
	}

	// 1. Agent configured (model + credential).
	if ok, why := mgr.Configured(); ok {
		add("agent-configured", true, why)
	} else {
		add("agent-configured", false, why)
		return pf // stop early; nothing else can pass
	}

	// 2. opencode present.
	if path, ok := mgr.Installed(); ok {
		add("opencode-present", true, path)
	} else {
		add("opencode-present", false, "opencode not found; run `hsdebug agent install`")
		return pf
	}

	// 3. AI dry-run.
	if cfg.Agent.Probe.Enabled {
		pr := mgr.Probe(ctx)
		add("ai-dry-run", pr.OK, pr.Detail)
	} else {
		add("ai-dry-run", true, "probe disabled in config")
	}
	return pf
}

// Task is a single service to debug with its attached prompt and health data.
type Task struct {
	Service config.ServiceConfig `json:"service"`
	Health  health.Result        `json:"health"`
	Prompt  string               `json:"-"`
}

// Collect gathers unhealthy services and attaches their prompts.
func Collect(cfg *config.Config, timeout time.Duration) ([]Task, error) {
	if len(cfg.Services) == 0 {
		return nil, fmt.Errorf("no services registered; run `hsdebug register` or `hsdebug scan --add` first")
	}
	results := health.CheckAll(cfg.Services, timeout)
	var tasks []Task
	for i, res := range results {
		if res.Healthy {
			continue
		}
		svc := cfg.Services[i]
		prompt, err := prompts.For(svc, cfg.PromptLibraryPath)
		if err != nil {
			return nil, err
		}
		tasks = append(tasks, Task{Service: svc, Health: res, Prompt: prompt})
	}
	return tasks, nil
}

// BuildPrompt injects service context into the base prompt.
func BuildPrompt(t Task) string {
	var b strings.Builder
	b.WriteString(t.Prompt)
	b.WriteString("\n\n---\n## Service context\n")
	fmt.Fprintf(&b, "- Name: %s\n", t.Service.Name)
	fmt.Fprintf(&b, "- Address: %s://%s:%d%s\n", t.Service.Scheme, t.Service.Host, t.Service.Port, t.Service.HealthPath)
	fmt.Fprintf(&b, "- Health URL: %s\n", t.Health.URL)
	if t.Health.Status != 0 {
		fmt.Fprintf(&b, "- Observed HTTP status: %d\n", t.Health.Status)
	}
	if t.Health.Error != "" {
		fmt.Fprintf(&b, "- Observed error: %s\n", t.Health.Error)
	}
	return b.String()
}
