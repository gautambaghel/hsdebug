package cli

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/spf13/cobra"

	"github.com/gautambaghel/hsdebug/internal/config"
)

func newDoctorCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "doctor",
		Short: "Run self-diagnostics for hsdebug",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runDoctor(cmd, globalOpts)
		},
	}
}

// Check is a single diagnostic result.
type Check struct {
	Name    string `json:"name"`
	OK      bool   `json:"ok"`
	Detail  string `json:"detail"`
	Hint    string `json:"hint,omitempty"`
}

// DoctorReport is the full self-diagnostic output.
type DoctorReport struct {
	Checks []Check `json:"checks"`
	Passed int     `json:"passed"`
	Failed int     `json:"failed"`
}

func (r DoctorReport) Text() string {
	var b strings.Builder
	for _, c := range r.Checks {
		mark := "\u2713"
		if !c.OK {
			mark = "\u2717"
		}
		fmt.Fprintf(&b, "%s %s: %s\n", mark, c.Name, c.Detail)
		if !c.OK && c.Hint != "" {
			fmt.Fprintf(&b, "    hint: %s\n", c.Hint)
		}
	}
	fmt.Fprintf(&b, "\n%d passed, %d failed", r.Passed, r.Failed)
	return b.String()
}

func (r DoctorReport) Markdown() string {
	var b strings.Builder
	b.WriteString("# hsdebug doctor\n\n| Check | Status | Detail |\n|---|---|---|\n")
	for _, c := range r.Checks {
		status := "OK"
		if !c.OK {
			status = "FAIL"
		}
		fmt.Fprintf(&b, "| %s | %s | %s |\n", c.Name, status, c.Detail)
	}
	fmt.Fprintf(&b, "\n**%d passed, %d failed**\n", r.Passed, r.Failed)
	return b.String()
}

func runDoctor(cmd *cobra.Command, opts Options) error {
	report := DoctorReport{}
	add := func(c Check) {
		report.Checks = append(report.Checks, c)
		if c.OK {
			report.Passed++
		} else {
			report.Failed++
		}
	}

	// 1. Config presence + validity.
	cfg, exists, err := config.Load()
	switch {
	case err != nil:
		add(Check{Name: "config", OK: false, Detail: err.Error(),
			Hint: "fix or delete the config file, or run `hsdebug register` to recreate it"})
	case !exists:
		add(Check{Name: "config", OK: false, Detail: "no config file found",
			Hint: "run `hsdebug register` or `hsdebug scan` to create one"})
	default:
		p, _ := config.Path()
		add(Check{Name: "config", OK: true, Detail: fmt.Sprintf("loaded from %s (%d services)", p, len(cfg.Services))})
	}

	// 2. opencode binary present.
	if path, err := exec.LookPath("opencode"); err == nil {
		ver := opencodeVersion(path)
		add(Check{Name: "opencode", OK: true, Detail: fmt.Sprintf("found at %s (%s)", path, ver)})
	} else {
		add(Check{Name: "opencode", OK: false, Detail: "not found on PATH",
			Hint: "run `hsdebug agent install` to install opencode"})
	}

	// 3. Isolated opencode config.
	if ocp, err := config.OpencodeConfigPath(); err == nil {
		if _, statErr := os.Stat(ocp); statErr == nil {
			add(Check{Name: "agent-config", OK: true, Detail: fmt.Sprintf("isolated opencode config at %s", ocp)})
		} else {
			add(Check{Name: "agent-config", OK: false, Detail: "isolated opencode config not written yet",
				Hint: "run `hsdebug agent model <provider/model>` to configure the agent"})
		}
	}

	// 4. Model configured.
	if cfg != nil && cfg.Agent.Model != "" {
		add(Check{Name: "agent-model", OK: true, Detail: cfg.Agent.Model})
	} else {
		add(Check{Name: "agent-model", OK: false, Detail: "no model configured",
			Hint: "run `hsdebug agent model <provider/model>`"})
	}

	return opts.Render(cmd.OutOrStdout(), report)
}

func opencodeVersion(path string) string {
	out, err := exec.Command(path, "--version").Output()
	if err != nil {
		return "version unknown"
	}
	return "v" + strings.TrimSpace(string(out))
}
