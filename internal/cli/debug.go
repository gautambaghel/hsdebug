package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/gautambaghel/hsdebug/internal/agent"
	"github.com/gautambaghel/hsdebug/internal/debug"
)

func newDebugCmd() *cobra.Command {
	var (
		quiet   bool
		verbose bool
		auto    bool
		attach  string
	)
	cmd := &cobra.Command{
		Use:   "debug [service]",
		Short: "Debug unhealthy services with the opencode agent",
		Long: "Runs a preflight gate (agent configured, opencode present, AI dry-run), " +
			"then collects unhealthy services and runs the agent to diagnose them.",
		RunE: func(cmd *cobra.Command, args []string) error {
			mgr, cfg, err := loadManager()
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()

			// --- Preflight gate ---
			pf := debug.RunPreflight(cmd.Context(), mgr, cfg)
			if !quiet && globalOpts.Format() == FormatText {
				for _, s := range pf.Steps {
					mark := "\u2717"
					if s.OK {
						mark = "\u2713"
					}
					fmt.Fprintf(out, "%s %s: %s\n", mark, s.Name, s.Detail)
				}
			}
			if !pf.Passed {
				return fmt.Errorf("preflight failed; agent is not ready. Configure it with `hsdebug agent model/provider` and verify with `hsdebug agent probe`")
			}

			// --- Collect unhealthy services ---
			tasks, err := debug.Collect(cfg, 5*time.Second)
			if err != nil {
				return err
			}
			if len(args) == 1 {
				var filtered []debug.Task
				for _, t := range tasks {
					if strings.EqualFold(t.Service.Name, args[0]) || strings.EqualFold(t.Service.CatalogID, args[0]) {
						filtered = append(filtered, t)
					}
				}
				tasks = filtered
			}
			if len(tasks) == 0 {
				return globalOpts.Render(out, simpleMsg{"all services healthy; nothing to debug"})
			}

			verbosity := agent.Normal
			if quiet {
				verbosity = agent.Quiet
			}
			if verbose {
				verbosity = agent.Verbose
			}

			// --- Run agent per task ---
			report := debugReport{}
			for _, t := range tasks {
				prompt := debug.BuildPrompt(t)
				sess := debugSession{Service: t.Service.Name, HealthURL: t.Health.URL}
				var lastText strings.Builder
				err := mgr.RunStreaming(cmd.Context(), prompt, attach, auto, func(line string) {
					if verbosity == agent.Verbose {
						fmt.Fprintln(out, line)
					}
					if txt := extractText(line); txt != "" {
						lastText.WriteString(txt)
						if verbosity == agent.Normal {
							fmt.Fprint(out, txt)
						}
					}
				})
				sess.Output = strings.TrimSpace(lastText.String())
				if err != nil {
					sess.Error = err.Error()
				}
				report.Sessions = append(report.Sessions, sess)
			}
			if globalOpts.Format() != FormatText {
				return globalOpts.Render(out, report)
			}
			return nil
		},
	}
	f := cmd.Flags()
	f.BoolVar(&quiet, "quiet", false, "only show cause and resolution")
	f.BoolVarP(&verbose, "verbose", "v", false, "show full JSON agent events")
	f.BoolVar(&auto, "auto", false, "auto-approve agent permissions not explicitly denied")
	f.StringVar(&attach, "attach", "", "attach to a running opencode server URL")
	return cmd
}

type debugSession struct {
	Service   string `json:"service"`
	HealthURL string `json:"healthUrl"`
	Output    string `json:"output"`
	Error     string `json:"error,omitempty"`
}

type debugReport struct {
	Sessions []debugSession `json:"sessions"`
}

func (r debugReport) Text() string {
	var b strings.Builder
	for _, s := range r.Sessions {
		fmt.Fprintf(&b, "== %s ==\n%s\n", s.Service, s.Output)
		if s.Error != "" {
			fmt.Fprintf(&b, "error: %s\n", s.Error)
		}
	}
	return b.String()
}
func (r debugReport) Markdown() string {
	var b strings.Builder
	for _, s := range r.Sessions {
		fmt.Fprintf(&b, "## %s\n\n%s\n\n", s.Service, s.Output)
	}
	return b.String()
}

// extractText pulls a text field from a single opencode JSON event line.
func extractText(line string) string {
	line = strings.TrimSpace(line)
	if !strings.HasPrefix(line, "{") {
		return ""
	}
	var ev map[string]any
	if err := json.Unmarshal([]byte(line), &ev); err != nil {
		return ""
	}
	if t, ok := ev["text"].(string); ok {
		return t
	}
	return ""
}

var _ = context.Background
