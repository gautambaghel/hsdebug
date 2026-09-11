package cli

import (
	"github.com/spf13/cobra"

	"github.com/gautambaghel/hsdebug/internal/config"
	"github.com/gautambaghel/hsdebug/internal/redact"
)

// globalOpts holds the parsed global output flags, shared across commands.
var globalOpts Options

// NewRootCmd builds the hsdebug command tree.
func NewRootCmd(version string) *cobra.Command {
	root := &cobra.Command{
		Use:   "hsdebug",
		Short: "Home server debugger agent",
		Long: "hsdebug registers local home-server services, checks their health, " +
			"and uses the opencode agent to debug the unhealthy ones.",
		Version:       version,
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	pf := root.PersistentFlags()
	pf.BoolVar(&globalOpts.JSON, "json", false, "output as JSON")
	pf.BoolVar(&globalOpts.Markdown, "markdown", false, "output as Markdown")
	pf.StringVar(&globalOpts.JQ, "jq", "", "apply a jq expression to JSON output (implies --json)")
	pf.BoolVar(&globalOpts.NoRedact, "no-redact", false, "disable output redaction for this command")

	root.PersistentPreRunE = func(cmd *cobra.Command, args []string) error {
		// Resolve the redaction profile from config (env override handled in
		// config layer later); fall back to strict when config is absent.
		cfg, _, err := config.Load()
		if err == nil && cfg != nil {
			globalOpts.RedactProfile = cfg.Redact.Profile
		} else {
			globalOpts.RedactProfile = redact.RedactStrict
		}
		return nil
	}

	root.AddCommand(
		newServerCmd(),
		newRegisterCmd(),
		newScanCmd(),
		newHealthCmd(),
		newAgentCmd(),
		newDebugCmd(),
		newDoctorCmd(),
	)
	return root
}
