package health

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gautambaghel/hsdebug/internal/config"
)

func TestIsLoopback(t *testing.T) {
	cases := map[string]bool{
		"127.0.0.1": true,
		"::1":       true,
		"localhost": true,
		"host.docker.internal": true,
		"10.0.0.5":  false,
		"192.168.1.10": false,
		"8.8.8.8":   false,
	}
	for host, want := range cases {
		if got := IsLoopback(host); got != want {
			t.Errorf("IsLoopback(%q) = %v, want %v", host, got, want)
		}
	}
}

// newLoopbackServer starts a test server and returns its host+port (loopback).
func newLoopbackServer(t *testing.T, handler http.HandlerFunc) (host string, port int) {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	// srv.URL like http://127.0.0.1:port
	trimmed := strings.TrimPrefix(srv.URL, "http://")
	h, p, err := splitHostPort(trimmed)
	if err != nil {
		t.Fatal(err)
	}
	return h, p
}

func splitHostPort(s string) (string, int, error) {
	i := strings.LastIndex(s, ":")
	host := s[:i]
	port, err := strconv.Atoi(s[i+1:])
	return host, port, err
}

func TestCheckHealthy(t *testing.T) {
	host, port := newLoopbackServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/ping" {
			w.WriteHeader(200)
			return
		}
		w.WriteHeader(404)
	})
	svc := config.ServiceConfig{Name: "t", Host: host, Port: port, Scheme: "http", HealthPath: "/ping", ExpectStatus: []int{200}}
	res := Check(svc, 2*time.Second)
	if !res.Healthy {
		t.Errorf("expected healthy, got %+v", res)
	}
	if res.Status != 200 {
		t.Errorf("status = %d", res.Status)
	}
}

func TestCheckUnhealthyStatus(t *testing.T) {
	host, port := newLoopbackServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
	})
	svc := config.ServiceConfig{Name: "t", Host: host, Port: port, Scheme: "http", HealthPath: "/", ExpectStatus: []int{200}}
	res := Check(svc, 2*time.Second)
	if res.Healthy {
		t.Error("500 should be unhealthy")
	}
	if res.Error == "" {
		t.Error("expected error detail")
	}
}

func TestCheckRefusesNonLoopback(t *testing.T) {
	svc := config.ServiceConfig{Name: "t", Host: "8.8.8.8", Port: 80, Scheme: "http", HealthPath: "/"}
	res := Check(svc, time.Second)
	if res.Healthy {
		t.Error("non-loopback must not be healthy")
	}
	if !strings.Contains(res.Error, "loopback") {
		t.Errorf("expected loopback refusal, got %q", res.Error)
	}
}

func TestCheckConnectionRefused(t *testing.T) {
	svc := config.ServiceConfig{Name: "t", Host: "127.0.0.1", Port: 1, Scheme: "http", HealthPath: "/"}
	res := Check(svc, time.Second)
	if res.Healthy {
		t.Error("unreachable port must be unhealthy")
	}
	if res.Error == "" {
		t.Error("expected connection error")
	}
}

func TestStatusAcceptedDefaultRange(t *testing.T) {
	if !statusAccepted(204, nil) {
		t.Error("2xx should be accepted by default")
	}
	if statusAccepted(500, nil) {
		t.Error("5xx should not be accepted by default")
	}
}

func TestCheckAllPreservesOrder(t *testing.T) {
	host, port := newLoopbackServer(t, func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) })
	svcs := []config.ServiceConfig{
		{Name: "a", Host: host, Port: port, HealthPath: "/", ExpectStatus: []int{200}},
		{Name: "b", Host: "127.0.0.1", Port: 1, HealthPath: "/"},
		{Name: "c", Host: host, Port: port, HealthPath: "/", ExpectStatus: []int{200}},
	}
	res := CheckAll(svcs, time.Second)
	if len(res) != 3 {
		t.Fatalf("got %d results", len(res))
	}
	if res[0].Name != "a" || res[1].Name != "b" || res[2].Name != "c" {
		t.Errorf("order not preserved: %v %v %v", res[0].Name, res[1].Name, res[2].Name)
	}
	if !res[0].Healthy || res[1].Healthy || !res[2].Healthy {
		t.Error("unexpected health results")
	}
}
