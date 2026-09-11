// Package health probes registered services for availability. All probes are
// constrained to loopback addresses.
package health

import (
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/gautambaghel/hsdebug/internal/config"
)

// Result is the outcome of a single service health check.
type Result struct {
	Name    string `json:"name"`
	Healthy bool   `json:"healthy"`
	Status  int    `json:"status,omitempty"`
	URL     string `json:"url"`
	Error   string `json:"error,omitempty"`
	Latency string `json:"latency,omitempty"`
}

// IsLoopback reports whether host resolves to a loopback address or is the
// literal localhost name. The Docker host-gateway alias "host.docker.internal"
// is also treated as loopback: when hsdebug runs in a bridged container it is
// the equivalent of the host's 127.0.0.1 for reaching co-located services.
func IsLoopback(host string) bool {
	if host == "localhost" || host == "host.docker.internal" {
		return true
	}
	ip := net.ParseIP(host)
	if ip == nil {
		// Resolve names; only allow if all addresses are loopback.
		addrs, err := net.LookupHost(host)
		if err != nil || len(addrs) == 0 {
			return false
		}
		for _, a := range addrs {
			if pip := net.ParseIP(a); pip == nil || !pip.IsLoopback() {
				return false
			}
		}
		return true
	}
	return ip.IsLoopback()
}

// Check probes a single service.
func Check(svc config.ServiceConfig, timeout time.Duration) Result {
	scheme := svc.Scheme
	if scheme == "" {
		scheme = "http"
	}
	path := svc.HealthPath
	if path != "" && !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	url := fmt.Sprintf("%s://%s:%d%s", scheme, svc.Host, svc.Port, path)
	res := Result{Name: svc.Name, URL: url}

	if !IsLoopback(svc.Host) {
		res.Error = "refusing to probe non-loopback host: " + svc.Host
		return res
	}

	client := &http.Client{Timeout: timeout}
	start := time.Now()
	resp, err := client.Get(url)
	res.Latency = time.Since(start).Round(time.Millisecond).String()
	if err != nil {
		res.Error = err.Error()
		return res
	}
	defer resp.Body.Close()
	res.Status = resp.StatusCode
	res.Healthy = statusAccepted(resp.StatusCode, svc.ExpectStatus)
	if !res.Healthy {
		res.Error = fmt.Sprintf("unexpected status %d", resp.StatusCode)
	}
	return res
}

func statusAccepted(status int, expect []int) bool {
	if len(expect) == 0 {
		return status >= 200 && status < 400
	}
	for _, e := range expect {
		if e == status {
			return true
		}
	}
	return false
}

// CheckAll probes every service concurrently and returns results in the same
// order as the input.
func CheckAll(services []config.ServiceConfig, timeout time.Duration) []Result {
	results := make([]Result, len(services))
	type item struct {
		i int
		r Result
	}
	ch := make(chan item, len(services))
	for i, s := range services {
		go func(i int, s config.ServiceConfig) {
			ch <- item{i, Check(s, timeout)}
		}(i, s)
	}
	for range services {
		it := <-ch
		results[it.i] = it.r
	}
	return results
}
