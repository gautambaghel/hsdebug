package cli

import (
	"context"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/gautambaghel/hsdebug/internal/agent"
	"github.com/gautambaghel/hsdebug/internal/config"
)

func newAgentCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "agent",
		Short: "Configure the opencode agent (model, provider, credentials)",
	}
	cmd.AddCommand(
		newAgentStatusCmd(),
		newAgentInstallCmd(),
		newAgentModelCmd(),
		newAgentProviderCmd(),
		newAgentProbeCmd(),
	)
	return cmd
}

func newAgentInstallCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "install",
		Short: "Install opencode if it is not already present",
		RunE: func(cmd *cobra.Command, args []string) error {
			mgr, _, err := loadManager()
			if err != nil {
				return err
			}
			if path, ok := mgr.Installed(); ok {
				return globalOpts.Render(cmd.OutOrStdout(), simpleMsg{"opencode already installed at " + path})
			}
			path, err := mgr.Install(cmd.Context())
			if err != nil {
				return err
			}
			return globalOpts.Render(cmd.OutOrStdout(), simpleMsg{"opencode installed at " + path})
		},
	}
}

func loadManager() (*agent.Manager, *config.Config, error) {
	cfg, _, err := config.Load()
	if err != nil {
		return nil, nil, err
	}
	mgr, err := agent.NewManager(cfg, nil)
	if err != nil {
		return nil, nil, err
	}
	return mgr, cfg, nil
}

func newAgentStatusCmd() *cobra.Command {
	var probe bool
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show agent configuration and readiness",
		RunE: func(cmd *cobra.Command, args []string) error {
			mgr, cfg, err := loadManager()
			if err != nil {
				return err
			}
			path, installed := mgr.Installed()
			configured, why := mgr.Configured()
			st := agentStatus{
				Installed:   installed,
				BinaryPath:  path,
				Model:       cfg.Agent.Model,
				Provider:    cfg.Agent.Provider,
				Configured:  configured,
				ConfigDetail: why,
			}
			if probe {
				pr := mgr.Probe(context.Background())
				st.Probe = &pr
			}
			return globalOpts.Render(cmd.OutOrStdout(), st)
		},
	}
	cmd.Flags().BoolVar(&probe, "probe", false, "run an AI dry-run to confirm the model responds")
	return cmd
}

type agentStatus struct {
	Installed    bool                `json:"installed"`
	BinaryPath   string              `json:"binaryPath,omitempty"`
	Model        string              `json:"model"`
	Provider     string              `json:"provider"`
	Configured   bool                `json:"configured"`
	ConfigDetail string              `json:"configDetail"`
	Probe        *agent.ProbeResult  `json:"probe,omitempty"`
}

func (s agentStatus) Text() string {
	var b strings.Builder
	fmt.Fprintf(&b, "opencode installed: %v\n", s.Installed)
	if s.BinaryPath != "" {
		fmt.Fprintf(&b, "binary: %s\n", s.BinaryPath)
	}
	fmt.Fprintf(&b, "model: %s\n", s.Model)
	fmt.Fprintf(&b, "provider: %s\n", s.Provider)
	fmt.Fprintf(&b, "configured: %v (%s)", s.Configured, s.ConfigDetail)
	if s.Probe != nil {
		fmt.Fprintf(&b, "\nAI dry-run: %v (%s, %s)", s.Probe.OK, s.Probe.Detail, s.Probe.Elapsed)
	}
	return b.String()
}

func (s agentStatus) Markdown() string {
	return fmt.Sprintf("**Agent**: model `%s`, provider `%s`, configured=%v", s.Model, s.Provider, s.Configured)
}

func newAgentModelCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "model <provider/model>",
		Short: "Set the agent model and regenerate the isolated opencode config",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			mgr, _, err := loadManager()
			if err != nil {
				return err
			}
			if err := mgr.SetModel(args[0]); err != nil {
				return err
			}
			return globalOpts.Render(cmd.OutOrStdout(), simpleMsg{"model set to " + args[0]})
		},
	}
}

func newAgentProviderCmd() *cobra.Command {
	var key, keyEnv, keyFile string
	cmd := &cobra.Command{
		Use:   "provider <id>",
		Short: "Configure provider credentials (non-interactive bypass)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			mgr, _, err := loadManager()
			if err != nil {
				return err
			}
			if err := mgr.SetProviderKey(args[0], key, keyEnv, keyFile); err != nil {
				return err
			}
			return globalOpts.Render(cmd.OutOrStdout(), simpleMsg{"provider " + args[0] + " configured"})
		},
	}
	f := cmd.Flags()
	f.StringVar(&key, "key", "", "API key value (stored 0600 and referenced via {file:})")
	f.StringVar(&keyEnv, "key-env", "", "name of an env var holding the key (referenced via {env:})")
	f.StringVar(&keyFile, "key-file", "", "path to an existing key file (referenced via {file:})")
	return cmd
}

func newAgentProbeCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "probe",
		Short: "Run an AI dry-run to confirm the model responds",
		RunE: func(cmd *cobra.Command, args []string) error {
			mgr, _, err := loadManager()
			if err != nil {
				return err
			}
			pr := mgr.Probe(context.Background())
			if err := globalOpts.Render(cmd.OutOrStdout(), probeMsg{pr}); err != nil {
				return err
			}
			if !pr.OK {
				return fmt.Errorf("AI dry-run failed: %s", pr.Detail)
			}
			return nil
		},
	}
}

type simpleMsg struct {
	Message string `json:"message"`
}

func (s simpleMsg) Text() string     { return s.Message }
func (s simpleMsg) Markdown() string { return s.Message }

type probeMsg struct {
	agent.ProbeResult
}

func (p probeMsg) Text() string {
	mark := "\u2717"
	if p.OK {
		mark = "\u2713"
	}
	return fmt.Sprintf("%s AI dry-run: %s (%s)", mark, p.Detail, p.Elapsed)
}
func (p probeMsg) Markdown() string {
	return fmt.Sprintf("AI dry-run: **%v** — %s", p.OK, p.Detail)
}
