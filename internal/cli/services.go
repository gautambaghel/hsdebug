package cli

import (
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/gautambaghel/hsdebug/internal/catalog"
	"github.com/gautambaghel/hsdebug/internal/config"
	"github.com/gautambaghel/hsdebug/internal/health"
)

// ---- register ----

func newRegisterCmd() *cobra.Command {
	var (
		name       string
		port       int
		host       string
		scheme     string
		healthPath string
		promptFile string
		promptText string
		catalogID  string
	)
	cmd := &cobra.Command{
		Use:   "register",
		Short: "Register a local service on loopback",
		RunE: func(cmd *cobra.Command, args []string) error {
			if name == "" || port == 0 {
				return fmt.Errorf("--name and --port are required")
			}
			if host == "" {
				host = "127.0.0.1"
			}
			if !health.IsLoopback(host) {
				return fmt.Errorf("host %q is not loopback; only localhost/127.0.0.1 services may be registered", host)
			}
			if scheme == "" {
				scheme = "http"
			}
			expect := []int{200, 301, 302, 401, 403}
			if catalogID != "" {
				if e, ok := catalog.ByID(catalogID); ok {
					if healthPath == "" {
						healthPath = e.HealthPath
					}
					expect = e.ExpectStatus
				}
			}
			cfg, _, err := config.Load()
			if err != nil {
				return err
			}
			for _, s := range cfg.Services {
				if s.Name == name {
					return fmt.Errorf("service %q already registered", name)
				}
			}
			svc := config.ServiceConfig{
				Name: name, Host: host, Port: port, Scheme: scheme,
				HealthPath: healthPath, ExpectStatus: expect,
				PromptFile: promptFile, PromptText: promptText, CatalogID: catalogID,
			}
			cfg.Services = append(cfg.Services, svc)
			if err := cfg.Save(); err != nil {
				return err
			}
			return globalOpts.Render(cmd.OutOrStdout(), registerResult{Service: svc})
		},
	}
	f := cmd.Flags()
	f.StringVar(&name, "name", "", "service name (required)")
	f.IntVar(&port, "port", 0, "service port (required)")
	f.StringVar(&host, "host", "127.0.0.1", "loopback host")
	f.StringVar(&scheme, "scheme", "http", "http or https")
	f.StringVar(&healthPath, "health-path", "", "health check path")
	f.StringVar(&promptFile, "prompt-file", "", "path to a debugging prompt file")
	f.StringVar(&promptText, "prompt-text", "", "inline debugging prompt")
	f.StringVar(&catalogID, "catalog-id", "", "link to a built-in catalog entry")
	return cmd
}

type registerResult struct {
	Service config.ServiceConfig `json:"service"`
}

func (r registerResult) Text() string {
	return fmt.Sprintf("registered %s (%s://%s:%d%s)", r.Service.Name,
		r.Service.Scheme, r.Service.Host, r.Service.Port, r.Service.HealthPath)
}
func (r registerResult) Markdown() string { return "**Registered** `" + r.Service.Name + "`" }

// ---- scan ----

func newScanCmd() *cobra.Command {
	var addAll bool
	cmd := &cobra.Command{
		Use:   "scan",
		Short: "Auto-detect commonly used home-server services on loopback",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, _, err := config.Load()
			if err != nil {
				return err
			}
			existing := map[string]bool{}
			for _, s := range cfg.Services {
				existing[s.CatalogID] = true
			}
			result := scanResult{}
			for _, e := range catalog.Entries {
				svc := config.ServiceConfig{
					Name: e.Name, Host: "127.0.0.1", Port: e.Port, Scheme: e.Scheme,
					HealthPath: e.HealthPath, ExpectStatus: e.ExpectStatus, CatalogID: e.ID,
				}
				res := health.Check(svc, 2*time.Second)
				detected := res.Error == "" || res.Status != 0
				if !detected {
					continue
				}
				entry := scanEntry{Name: e.Name, CatalogID: e.ID, Port: e.Port, Healthy: res.Healthy}
				if existing[e.ID] {
					entry.Note = "already registered"
				} else if addAll {
					cfg.Services = append(cfg.Services, svc)
					entry.Added = true
				}
				result.Detected = append(result.Detected, entry)
			}
			if addAll {
				if err := cfg.Save(); err != nil {
					return err
				}
			}
			result.Added = addAll
			return globalOpts.Render(cmd.OutOrStdout(), result)
		},
	}
	cmd.Flags().BoolVar(&addAll, "add", false, "register all newly detected services")
	return cmd
}

type scanEntry struct {
	Name      string `json:"name"`
	CatalogID string `json:"catalogId"`
	Port      int    `json:"port"`
	Healthy   bool   `json:"healthy"`
	Added     bool   `json:"added"`
	Note      string `json:"note,omitempty"`
}

type scanResult struct {
	Detected []scanEntry `json:"detected"`
	Added    bool        `json:"added"`
}

func (r scanResult) Text() string {
	if len(r.Detected) == 0 {
		return "no known services detected on loopback"
	}
	var b strings.Builder
	newDetections := 0
	for _, e := range r.Detected {
		mark := "\u2717"
		if e.Healthy {
			mark = "\u2713"
		}
		fmt.Fprintf(&b, "%s %s :%d", mark, e.Name, e.Port)
		if e.Added {
			b.WriteString(" [registered]")
		} else if e.Note != "" {
			fmt.Fprintf(&b, " [%s]", e.Note)
		} else {
			newDetections++
		}
		b.WriteString("\n")
	}
	if !r.Added && newDetections > 0 {
		fmt.Fprintf(&b, "\n%d service(s) detected but NOT registered. Re-run with `hsdebug scan --add` to register them.", newDetections)
	}
	return strings.TrimRight(b.String(), "\n")
}

func (r scanResult) Markdown() string {
	var b strings.Builder
	b.WriteString("| Service | Port | Healthy | Note |\n|---|---|---|---|\n")
	for _, e := range r.Detected {
		note := e.Note
		if e.Added {
			note = "registered"
		}
		fmt.Fprintf(&b, "| %s | %d | %v | %s |\n", e.Name, e.Port, e.Healthy, note)
	}
	return b.String()
}

// ---- health ----

func newHealthCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "health [service]",
		Short: "Check health of one or all registered services",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, exists, err := config.Load()
			if err != nil {
				return err
			}
			if !exists || len(cfg.Services) == 0 {
				return globalOpts.Render(cmd.OutOrStdout(), healthResult{Empty: true})
			}
			services := cfg.Services
			if len(args) == 1 {
				var filtered []config.ServiceConfig
				for _, s := range cfg.Services {
					if strings.EqualFold(s.Name, args[0]) || strings.EqualFold(s.CatalogID, args[0]) {
						filtered = append(filtered, s)
					}
				}
				if len(filtered) == 0 {
					return fmt.Errorf("no registered service matches %q", args[0])
				}
				services = filtered
			}
			results := health.CheckAll(services, 5*time.Second)
			return globalOpts.Render(cmd.OutOrStdout(), healthResult{Results: results})
		},
	}
	return cmd
}

type healthResult struct {
	Empty   bool            `json:"empty"`
	Results []health.Result `json:"results,omitempty"`
}

func (r healthResult) Text() string {
	if r.Empty {
		return "no services registered. Run `hsdebug register` or `hsdebug scan --add` first."
	}
	var b strings.Builder
	for _, res := range r.Results {
		mark := "\u2717"
		if res.Healthy {
			mark = "\u2713"
		}
		fmt.Fprintf(&b, "%s %s", mark, res.Name)
		if res.Status != 0 {
			fmt.Fprintf(&b, " (%d)", res.Status)
		}
		if !res.Healthy && res.Error != "" {
			fmt.Fprintf(&b, " - %s", res.Error)
		}
		b.WriteString("\n")
	}
	return strings.TrimRight(b.String(), "\n")
}

func (r healthResult) Markdown() string {
	if r.Empty {
		return "_No services registered._"
	}
	var b strings.Builder
	b.WriteString("| Service | Healthy | Status | Detail |\n|---|---|---|---|\n")
	for _, res := range r.Results {
		fmt.Fprintf(&b, "| %s | %v | %d | %s |\n", res.Name, res.Healthy, res.Status, res.Error)
	}
	return b.String()
}
