package cli

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/gautambaghel/hsdebug/internal/config"
)

func tempConfig(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("HSDEBUG_CONFIG_DIR", dir)
	t.Setenv("HSDEBUG_CONFIG", dir+"/config.json")
}

func run(t *testing.T, cmd *cobra.Command, args ...string) (string, error) {
	t.Helper()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cmd.SetArgs(args)
	err := cmd.Execute()
	return buf.String(), err
}

func TestRegisterRequiresNameAndPort(t *testing.T) {
	tempConfig(t)
	globalOpts = Options{}
	_, err := run(t, newRegisterCmd())
	if err == nil {
		t.Fatal("expected error without name/port")
	}
}

func TestRegisterRejectsNonLoopback(t *testing.T) {
	tempConfig(t)
	globalOpts = Options{}
	_, err := run(t, newRegisterCmd(), "--name", "x", "--port", "80", "--host", "8.8.8.8")
	if err == nil || !strings.Contains(err.Error(), "loopback") {
		t.Fatalf("expected loopback rejection, got %v", err)
	}
}

func TestRegisterPersistsAndPreventsDuplicate(t *testing.T) {
	tempConfig(t)
	globalOpts = Options{}
	if _, err := run(t, newRegisterCmd(), "--name", "sonarr", "--port", "8989", "--catalog-id", "sonarr"); err != nil {
		t.Fatalf("register: %v", err)
	}
	cfg, exists, _ := config.Load()
	if !exists || len(cfg.Services) != 1 {
		t.Fatalf("service not persisted: %+v", cfg.Services)
	}
	if cfg.Services[0].HealthPath != "/ping" {
		t.Errorf("catalog defaults not applied: %+v", cfg.Services[0])
	}
	// Duplicate should fail.
	if _, err := run(t, newRegisterCmd(), "--name", "sonarr", "--port", "8989"); err == nil {
		t.Error("duplicate registration should fail")
	}
}

func TestHealthEmptyMessage(t *testing.T) {
	tempConfig(t)
	globalOpts = Options{}
	out, err := run(t, newHealthCmd())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "no services registered") {
		t.Errorf("expected empty guidance, got %q", out)
	}
}

func TestHealthChecksRegistered(t *testing.T) {
	tempConfig(t)
	globalOpts = Options{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	}))
	defer srv.Close()
	host, port := parseURL(t, srv.URL)

	cfg := config.Default()
	cfg.Services = append(cfg.Services, config.ServiceConfig{
		Name: "web", Host: host, Port: port, Scheme: "http", HealthPath: "/", ExpectStatus: []int{200},
	})
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}
	out, err := run(t, newHealthCmd())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "web") {
		t.Errorf("expected web in output: %q", out)
	}
}

func TestHealthUnknownServiceErrors(t *testing.T) {
	tempConfig(t)
	globalOpts = Options{}
	cfg := config.Default()
	cfg.Services = append(cfg.Services, config.ServiceConfig{Name: "a", Host: "127.0.0.1", Port: 9})
	cfg.Save()
	if _, err := run(t, newHealthCmd(), "nonexistent"); err == nil {
		t.Error("unknown service filter should error")
	}
}

func TestScanNoAddIsReadOnly(t *testing.T) {
	tempConfig(t)
	globalOpts = Options{}
	// Without --add, config should not gain services even if something responds.
	if _, err := run(t, newScanCmd()); err != nil {
		t.Fatal(err)
	}
	cfg, exists, _ := config.Load()
	if exists && len(cfg.Services) != 0 {
		t.Errorf("scan without --add must not register: %+v", cfg.Services)
	}
}

func TestScanTextHintWhenNotAdded(t *testing.T) {
	r := scanResult{Added: false, Detected: []scanEntry{
		{Name: "sonarr", Port: 8989, Healthy: true},
	}}
	out := r.Text()
	if !strings.Contains(out, "scan --add") {
		t.Errorf("expected --add hint, got %q", out)
	}
}

func TestScanTextNoHintWhenAdded(t *testing.T) {
	r := scanResult{Added: true, Detected: []scanEntry{
		{Name: "sonarr", Port: 8989, Healthy: true, Added: true},
	}}
	out := r.Text()
	if strings.Contains(out, "scan --add") {
		t.Errorf("should not show hint after --add, got %q", out)
	}
	if !strings.Contains(out, "[registered]") {
		t.Errorf("expected registered marker, got %q", out)
	}
}

func TestScanTextNoHintForAlreadyRegistered(t *testing.T) {
	// Detected but with a note (already registered) should not count as "new".
	r := scanResult{Added: false, Detected: []scanEntry{
		{Name: "sonarr", Port: 8989, Healthy: true, Note: "already registered"},
	}}
	out := r.Text()
	if strings.Contains(out, "scan --add") {
		t.Errorf("no hint expected when all detections already registered, got %q", out)
	}
}

func parseURL(t *testing.T, url string) (string, int) {
	t.Helper()
	s := strings.TrimPrefix(url, "http://")
	i := strings.LastIndex(s, ":")
	p, err := strconv.Atoi(s[i+1:])
	if err != nil {
		t.Fatal(err)
	}
	return s[:i], p
}
