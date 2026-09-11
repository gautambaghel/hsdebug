package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gautambaghel/hsdebug/internal/agent"
	"github.com/gautambaghel/hsdebug/internal/config"
)

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

func newTestServer(t *testing.T, runner agent.Runner) *Server {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("HSDEBUG_CONFIG_DIR", dir)
	t.Setenv("HSDEBUG_CONFIG", filepath.Join(dir, "config.json"))
	cfg := config.Default()
	cfg.Agent.Model = "anthropic/claude-sonnet-4-5"
	kf := filepath.Join(dir, "key")
	os.WriteFile(kf, []byte("sk-abc"), 0o600)
	cfg.Agent.CredentialRef = kf
	mgr, _ := agent.NewManager(cfg, runner)
	return New(cfg, mgr)
}

func TestHealthzEndpoint(t *testing.T) {
	s := newTestServer(t, &fakeRunner{installed: true})
	req := httptest.NewRequest("GET", "/healthz", nil)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("code = %d", rec.Code)
	}
	var body map[string]string
	json.Unmarshal(rec.Body.Bytes(), &body)
	if body["status"] != "ok" {
		t.Errorf("body = %v", body)
	}
}

func TestServicesEndpoint(t *testing.T) {
	s := newTestServer(t, &fakeRunner{installed: true})
	s.cfg.Services = append(s.cfg.Services, config.ServiceConfig{Name: "sonarr", Host: "127.0.0.1", Port: 8989})
	req := httptest.NewRequest("GET", "/api/services", nil)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("code = %d", rec.Code)
	}
	if !contains2(rec.Body.String(), "sonarr") {
		t.Errorf("body = %s", rec.Body.String())
	}
}

func TestAgentEndpoint(t *testing.T) {
	s := newTestServer(t, &fakeRunner{installed: true})
	req := httptest.NewRequest("GET", "/api/agent", nil)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	var body map[string]any
	json.Unmarshal(rec.Body.Bytes(), &body)
	if body["configured"] != true {
		t.Errorf("expected configured=true, body=%v", body)
	}
	if body["installed"] != true {
		t.Errorf("expected installed=true")
	}
}

func TestProbeEndpointSuccess(t *testing.T) {
	s := newTestServer(t, &fakeRunner{installed: true, output: []byte(`{"text":"HSDEBUG_OK"}`)})
	req := httptest.NewRequest("POST", "/api/agent/probe", nil)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Errorf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestProbeEndpointFailure(t *testing.T) {
	s := newTestServer(t, &fakeRunner{installed: true, output: []byte(`{"text":"nope"}`)})
	req := httptest.NewRequest("POST", "/api/agent/probe", nil)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503, got %d", rec.Code)
	}
}

func TestStartDebugPreflightGate(t *testing.T) {
	// Unconfigured agent (no credential) must not proceed.
	dir := t.TempDir()
	t.Setenv("HSDEBUG_CONFIG_DIR", dir)
	t.Setenv("HSDEBUG_CONFIG", filepath.Join(dir, "config.json"))
	cfg := config.Default()
	cfg.Agent.Model = "" // unconfigured
	mgr, _ := agent.NewManager(cfg, &fakeRunner{installed: true})
	s := New(cfg, mgr)
	sess := s.StartDebug(context.Background(), "sess1", nil, false)
	if sess.Status != "failed" {
		t.Errorf("expected failed session, got %q", sess.Status)
	}
}

func TestRegisterEndpointCreatesService(t *testing.T) {
	s := newTestServer(t, &fakeRunner{installed: true})
	body := strings.NewReader(`{"name":"sonarr","port":8989,"catalogId":"sonarr"}`)
	req := httptest.NewRequest("POST", "/api/services", body)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}
	if len(s.cfg.Services) != 1 || s.cfg.Services[0].Name != "sonarr" {
		t.Errorf("service not stored: %+v", s.cfg.Services)
	}
	if s.cfg.Services[0].HealthPath != "/ping" {
		t.Errorf("catalog defaults not applied: %+v", s.cfg.Services[0])
	}
}

func TestRegisterEndpointValidation(t *testing.T) {
	s := newTestServer(t, &fakeRunner{installed: true})
	req := httptest.NewRequest("POST", "/api/services", strings.NewReader(`{"name":"x"}`))
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("missing port should be 400, got %d", rec.Code)
	}
}

func TestRegisterEndpointRejectsNonLoopback(t *testing.T) {
	s := newTestServer(t, &fakeRunner{installed: true})
	body := strings.NewReader(`{"name":"x","port":80,"host":"8.8.8.8"}`)
	req := httptest.NewRequest("POST", "/api/services", body)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("non-loopback should be 400, got %d", rec.Code)
	}
}

func TestRegisterEndpointDuplicate(t *testing.T) {
	s := newTestServer(t, &fakeRunner{installed: true})
	s.cfg.Services = append(s.cfg.Services, config.ServiceConfig{Name: "dup", Port: 1})
	body := strings.NewReader(`{"name":"dup","port":1}`)
	req := httptest.NewRequest("POST", "/api/services", body)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusConflict {
		t.Errorf("duplicate should be 409, got %d", rec.Code)
	}
}

func TestScanEndpointReturnsResults(t *testing.T) {
	s := newTestServer(t, &fakeRunner{installed: true})
	req := httptest.NewRequest("POST", "/api/scan", nil)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("scan should be 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("scan response not JSON: %v", err)
	}
	if _, ok := body["detected"]; !ok {
		t.Errorf("scan response missing 'detected': %s", rec.Body.String())
	}
	if _, ok := body["added"]; !ok {
		t.Errorf("scan response missing 'added': %s", rec.Body.String())
	}
}

func TestSessionsEndpoint(t *testing.T) {
	s := newTestServer(t, &fakeRunner{installed: true, output: []byte(`{"text":"HSDEBUG_OK"}`)})
	s.StartDebug(context.Background(), "s1", nil, false) // will fail collect (no services) but records session
	req := httptest.NewRequest("GET", "/api/sessions", nil)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if !contains2(rec.Body.String(), "s1") {
		t.Errorf("session not listed: %s", rec.Body.String())
	}
}

func TestDebugEndpointGate(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HSDEBUG_CONFIG_DIR", dir)
	t.Setenv("HSDEBUG_CONFIG", filepath.Join(dir, "config.json"))
	cfg := config.Default()
	cfg.Agent.Model = "" // unconfigured -> gate should block
	mgr, _ := agent.NewManager(cfg, &fakeRunner{installed: true})
	s := New(cfg, mgr)
	req := httptest.NewRequest("POST", "/api/debug", nil)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503 from gate, got %d: %s", rec.Code, rec.Body.String())
	}
}

func contains2(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
