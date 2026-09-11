package cli

import (
	"fmt"
	"net"
	"net/http"

	"github.com/spf13/cobra"

	"github.com/gautambaghel/hsdebug/internal/config"
	"github.com/gautambaghel/hsdebug/internal/health"
	"github.com/gautambaghel/hsdebug/internal/server"
)

func newServerCmd() *cobra.Command {
	var host string
	var port int
	cmd := &cobra.Command{
		Use:   "server",
		Short: "Start the hsdebug local service (REST API + web UI)",
		RunE: func(cmd *cobra.Command, args []string) error {
			mgr, cfg, err := loadManager()
			if err != nil {
				return err
			}
			if host == "" {
				host = cfg.Server.Host
			}
			if port == 0 {
				port = cfg.Server.Port
			}
			if !health.IsLoopback(host) {
				fmt.Fprintf(cmd.ErrOrStderr(),
					"warning: binding to non-loopback host %q exposes hsdebug to the network\n", host)
			}
			srv := server.New(cfg, mgr)
			addr := net.JoinHostPort(host, fmt.Sprintf("%d", port))
			fmt.Fprintf(cmd.OutOrStdout(), "hsdebug server listening on http://%s\n", addr)
			return http.ListenAndServe(addr, srv.Handler())
		},
	}
	f := cmd.Flags()
	f.StringVar(&host, "host", "", "host to bind (default from config: loopback)")
	f.IntVar(&port, "port", 0, "port to bind (default from config)")
	_ = config.Default
	return cmd
}
