package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// ProbeResult is the outcome of the AI liveness dry-run.
type ProbeResult struct {
	OK        bool   `json:"ok"`
	Model     string `json:"model"`
	Sentinel  string `json:"sentinel"`
	Responded bool   `json:"responded"`
	Detail    string `json:"detail"`
	Elapsed   string `json:"elapsed"`
}

// Probe runs a side-effect-free dry-run through opencode to confirm the model,
// provider, and credentials work end to end. It sends a prompt asking the model
// to echo the configured sentinel and checks the response contains it.
//
// The probe disables all tools (read-only, no bash/edit) so it cannot touch the
// system.
func (m *Manager) Probe(ctx context.Context) ProbeResult {
	p := m.cfg.Agent.Probe
	sentinel := p.Sentinel
	if sentinel == "" {
		sentinel = "HSDEBUG_OK"
	}
	res := ProbeResult{Model: m.cfg.Agent.Model, Sentinel: sentinel}

	if ok, why := m.Configured(); !ok {
		res.Detail = "agent not configured: " + why
		return res
	}

	timeout := time.Duration(p.TimeoutSeconds) * time.Second
	if timeout <= 0 {
		timeout = 60 * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	prompt := fmt.Sprintf("Reply with exactly this token and nothing else: %s", sentinel)
	args := []string{"run", "--format", "json"}
	if m.cfg.Agent.Model != "" {
		args = append(args, "-m", m.cfg.Agent.Model)
	}
	args = append(args, prompt)

	start := time.Now()
	out, err := m.runner.Output(ctx, args...)
	res.Elapsed = time.Since(start).Round(time.Millisecond).String()
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			res.Detail = fmt.Sprintf("dry-run timed out after %s", timeout)
			return res
		}
		res.Detail = "opencode run failed: " + err.Error()
		return res
	}

	text := extractAssistantText(out)
	res.Responded = strings.TrimSpace(text) != ""
	if strings.Contains(text, sentinel) {
		res.OK = true
		res.Detail = "AI responded with sentinel"
		return res
	}
	if res.Responded {
		res.Detail = "AI responded but sentinel not found; check model/credentials"
		return res
	}
	res.Detail = "no AI response detected in output"
	return res
}

// extractAssistantText pulls human-readable assistant text out of opencode's
// --format json output. opencode emits newline-delimited JSON events; we scan
// for text content. To be robust to schema variation, we also fall back to the
// raw output if no structured text is found.
func extractAssistantText(out []byte) string {
	var sb strings.Builder
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || !strings.HasPrefix(line, "{") {
			continue
		}
		var ev map[string]any
		if err := json.Unmarshal([]byte(line), &ev); err != nil {
			continue
		}
		collectText(ev, &sb)
	}
	if sb.Len() == 0 {
		// Fall back to raw output (covers plain-text responses).
		return string(out)
	}
	return sb.String()
}

// collectText walks an event object and appends any "text" string fields.
func collectText(v any, sb *strings.Builder) {
	switch t := v.(type) {
	case map[string]any:
		for k, val := range t {
			if k == "text" {
				if s, ok := val.(string); ok {
					sb.WriteString(s)
					sb.WriteString("\n")
				}
			}
			collectText(val, sb)
		}
	case []any:
		for _, e := range t {
			collectText(e, sb)
		}
	}
}
