// Package server exposes the hsdebug REST API and (later) the embedded web UI.
// It binds to loopback by default and mirrors the CLI functionality: listing
// services, checking health, agent status/probe, and running debug sessions.
package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/gautambaghel/hsdebug/internal/agent"
	"github.com/gautambaghel/hsdebug/internal/catalog"
	"github.com/gautambaghel/hsdebug/internal/config"
	"github.com/gautambaghel/hsdebug/internal/debug"
	"github.com/gautambaghel/hsdebug/internal/health"
)

// Server holds runtime state for the API/UI.
type Server struct {
	cfg      *config.Config
	mgr      *agent.Manager
	mu       sync.Mutex
	sessions map[string]*Session
}

// Session represents a debug run tracked for the UI.
type Session struct {
	ID        string    `json:"id"`
	Services  []string  `json:"services"`
	Status    string    `json:"status"` // running | done | failed
	Output    string    `json:"output"`
	Error     string    `json:"error,omitempty"`
	CreatedAt time.Time `json:"createdAt"`

	// subscribers receive raw output lines for SSE streaming. Guarded by the
	// Server mutex.
	subs []chan sseEvent
}

// sseEvent is a single server-sent event: an event name and JSON/text data.
type sseEvent struct {
	Name string
	Data string
}

// New builds a Server.
func New(cfg *config.Config, mgr *agent.Manager) *Server {
	return &Server{cfg: cfg, mgr: mgr, sessions: map[string]*Session{}}
}

// Handler returns the HTTP handler with all routes registered.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/health", s.handleHealth)
	mux.HandleFunc("GET /api/services", s.handleServices)
	mux.HandleFunc("POST /api/services", s.handleRegister)
	mux.HandleFunc("POST /api/scan", s.handleScan)
	mux.HandleFunc("GET /api/agent", s.handleAgent)
	mux.HandleFunc("POST /api/agent/probe", s.handleProbe)
	mux.HandleFunc("GET /api/agent/godmode", s.handleGodModeGet)
	mux.HandleFunc("POST /api/agent/godmode", s.handleGodModeSet)
	mux.HandleFunc("GET /api/sessions", s.handleSessions)
	mux.HandleFunc("POST /api/debug", s.handleDebug)
	mux.HandleFunc("GET /api/debug/stream", s.handleDebugStream)
	mux.HandleFunc("POST /api/debug/permission", s.handlePermission)
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	if ui := UIHandler(); ui != nil {
		mux.Handle("/", ui)
	}
	return mux
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	results := health.CheckAll(s.cfg.Services, 5*time.Second)
	writeJSON(w, http.StatusOK, map[string]any{"results": results})
}

func (s *Server) handleServices(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"services": s.cfg.Services})
}

// handleRegister registers a single loopback service from a JSON body.
func (s *Server) handleRegister(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name       string `json:"name"`
		Host       string `json:"host"`
		Port       int    `json:"port"`
		Scheme     string `json:"scheme"`
		HealthPath string `json:"healthPath"`
		CatalogID  string `json:"catalogId"`
		PromptText string `json:"promptText"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON body"})
		return
	}
	if body.Name == "" || body.Port == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "name and port are required"})
		return
	}
	if body.Host == "" {
		body.Host = s.cfg.ResolveProbeHost()
	}
	if !health.IsLoopback(body.Host) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "host must be loopback (127.0.0.1/localhost)"})
		return
	}
	if body.Scheme == "" {
		body.Scheme = "http"
	}
	expect := []int{200, 301, 302, 401, 403}
	if body.CatalogID != "" {
		if e, ok := catalog.ByID(body.CatalogID); ok {
			if body.HealthPath == "" {
				body.HealthPath = e.HealthPath
			}
			expect = e.ExpectStatus
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, existing := range s.cfg.Services {
		if existing.Name == body.Name {
			writeJSON(w, http.StatusConflict, map[string]string{"error": "service " + body.Name + " already registered"})
			return
		}
	}
	svc := config.ServiceConfig{
		Name: body.Name, Host: body.Host, Port: body.Port, Scheme: body.Scheme,
		HealthPath: body.HealthPath, ExpectStatus: expect,
		PromptText: body.PromptText, CatalogID: body.CatalogID,
	}
	s.cfg.Services = append(s.cfg.Services, svc)
	if err := s.cfg.Save(); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"service": svc})
}

// handleScan probes the catalog on loopback and registers newly detected
// services (mirrors `hsdebug scan --add`).
func (s *Server) handleScan(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	existing := map[string]bool{}
	for _, sv := range s.cfg.Services {
		existing[sv.CatalogID] = true
	}
	type detected struct {
		Name      string `json:"name"`
		CatalogID string `json:"catalogId"`
		Port      int    `json:"port"`
		Healthy   bool   `json:"healthy"`
		Added     bool   `json:"added"`
		Note      string `json:"note,omitempty"`
	}
	var results []detected
	added := 0
	probeHost := s.cfg.ResolveProbeHost()
	for _, e := range catalog.Entries {
		svc := config.ServiceConfig{
			Name: e.Name, Host: probeHost, Port: e.Port, Scheme: e.Scheme,
			HealthPath: e.HealthPath, ExpectStatus: e.ExpectStatus, CatalogID: e.ID,
		}
		res := health.Check(svc, 2*time.Second)
		if res.Error != "" && res.Status == 0 {
			continue // not detected
		}
		d := detected{Name: e.Name, CatalogID: e.ID, Port: e.Port, Healthy: res.Healthy}
		if existing[e.ID] {
			d.Note = "already registered"
		} else {
			s.cfg.Services = append(s.cfg.Services, svc)
			d.Added = true
			added++
		}
		results = append(results, d)
	}
	if added > 0 {
		if err := s.cfg.Save(); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"detected": results, "added": added})
}


func (s *Server) handleAgent(w http.ResponseWriter, r *http.Request) {
	path, installed := s.mgr.Installed()
	configured, detail := s.mgr.Configured()
	writeJSON(w, http.StatusOK, map[string]any{
		"installed":  installed,
		"binaryPath": path,
		"model":      s.cfg.Agent.Model,
		"provider":   s.cfg.Agent.Provider,
		"configured": configured,
		"detail":     detail,
		"godMode":    s.mgr.GodMode(),
	})
}

// handleGodModeGet reports whether elevated permissions are enabled.
func (s *Server) handleGodModeGet(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"godMode": s.mgr.GodMode()})
}

// handleGodModeSet toggles elevated (allow-all) opencode permissions.
func (s *Server) handleGodModeSet(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Enabled bool `json:"enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON body"})
		return
	}
	if err := s.mgr.SetGodMode(body.Enabled); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"godMode": s.mgr.GodMode()})
}

func (s *Server) handleProbe(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 90*time.Second)
	defer cancel()
	pr := s.mgr.Probe(ctx)
	code := http.StatusOK
	if !pr.OK {
		code = http.StatusServiceUnavailable
	}
	writeJSON(w, code, pr)
}

func (s *Server) handleSessions(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	list := make([]*Session, 0, len(s.sessions))
	for _, sess := range s.sessions {
		list = append(list, sess)
	}
	writeJSON(w, http.StatusOK, map[string]any{"sessions": list})
}

func (s *Server) handleDebug(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Services []string `json:"services"`
		Auto     bool     `json:"auto"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	id := "sess-" + time.Now().Format("20060102-150405.000")
	sess := s.newSession(id, body.Services)

	// Run asynchronously so the UI can immediately open the SSE stream. A
	// background context is used so the run survives this HTTP request.
	go s.StartDebug(context.Background(), sess, body.Services, body.Auto)
	writeJSON(w, http.StatusAccepted, sess)
}

// handleDebugStream streams a session's output as server-sent events. Events:
//
//	line        — a raw opencode output line
//	permission  — opencode is requesting permission (JSON payload)
//	done        — final status (data is the status string)
func (s *Server) handleDebugStream(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")
	s.mu.Lock()
	sess := s.sessions[id]
	s.mu.Unlock()
	if sess == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "unknown session"})
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "streaming unsupported"})
		return
	}
	ch := make(chan sseEvent, 256)
	s.mu.Lock()
	// If the session already finished, replay nothing and just send done.
	finished := sess.Status != "running"
	if !finished {
		sess.subs = append(sess.subs, ch)
	}
	s.mu.Unlock()

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	if finished {
		fmt.Fprintf(w, "event: done\ndata: %s\n\n", sess.Status)
		flusher.Flush()
		return
	}
	for {
		select {
		case <-r.Context().Done():
			s.unsubscribe(sess, ch)
			return
		case ev, open := <-ch:
			if !open {
				return
			}
			fmt.Fprintf(w, "event: %s\ndata: %s\n\n", ev.Name, ev.Data)
			flusher.Flush()
			if ev.Name == "done" {
				return
			}
		}
	}
}

// handlePermission relays an operator's allow/deny decision to a managed
// opencode server. Interactive pass-through requires an attached `opencode
// serve` instance (POST /session/:id/permissions/:permissionID). When no server
// is attached this records the decision but cannot alter a one-shot run; god
// mode (allow-all) is the reliable elevated path in that case.
func (s *Server) handlePermission(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ID        string `json:"id"`
		RequestID string `json:"requestId"`
		Decision  string `json:"decision"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON body"})
		return
	}
	if body.Decision != "allow" && body.Decision != "deny" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "decision must be allow or deny"})
		return
	}
	err := s.mgr.RespondPermission(r.Context(), body.RequestID, body.Decision)
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// newSession creates and registers a running session.
func (s *Server) newSession(id string, names []string) *Session {
	sess := &Session{ID: id, Services: names, Status: "running", CreatedAt: time.Now()}
	s.mu.Lock()
	s.sessions[id] = sess
	s.mu.Unlock()
	return sess
}

// StartDebug runs a debug flow for the given service names, streaming opencode
// output to any SSE subscribers. It runs the preflight gate first and refuses
// if it fails.
func (s *Server) StartDebug(ctx context.Context, sess *Session, names []string, auto bool) *Session {
	pf := debug.RunPreflight(ctx, s.mgr, s.cfg)
	if !pf.Passed {
		s.finish(sess, "failed", "preflight failed: agent not ready")
		return sess
	}
	tasks, err := debug.Collect(s.cfg, 5*time.Second)
	if err != nil {
		s.finish(sess, "failed", err.Error())
		return sess
	}
	for _, t := range tasks {
		if len(names) > 0 && !contains(names, t.Service.Name) {
			continue
		}
		prompt := debug.BuildPrompt(t)
		err := s.mgr.RunStreaming(ctx, prompt, "", auto, func(line string) {
			s.appendOutput(sess, line)
			s.publish(sess, sseEvent{Name: "line", Data: line})
			if perm := detectPermission(line); perm != "" {
				s.publish(sess, sseEvent{Name: "permission", Data: perm})
			}
		})
		if err != nil {
			s.finish(sess, "failed", err.Error())
			return sess
		}
	}
	s.finish(sess, "done", "")
	return sess
}

func (s *Server) appendOutput(sess *Session, line string) {
	s.mu.Lock()
	sess.Output += line + "\n"
	s.mu.Unlock()
}

func (s *Server) publish(sess *Session, ev sseEvent) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, ch := range sess.subs {
		select {
		case ch <- ev:
		default: // drop if a slow subscriber's buffer is full
		}
	}
}

func (s *Server) unsubscribe(sess *Session, ch chan sseEvent) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, c := range sess.subs {
		if c == ch {
			sess.subs = append(sess.subs[:i], sess.subs[i+1:]...)
			break
		}
	}
}

func (s *Server) finish(sess *Session, status, errMsg string) {
	s.mu.Lock()
	sess.Status = status
	sess.Error = errMsg
	subs := sess.subs
	sess.subs = nil
	s.mu.Unlock()
	for _, ch := range subs {
		select {
		case ch <- sseEvent{Name: "done", Data: status}:
		default:
		}
		close(ch)
	}
}

// detectPermission inspects an opencode JSON event line and, if it represents a
// permission request, returns a JSON payload describing it for the UI. Returns
// "" for non-permission lines.
func detectPermission(line string) string {
	var ev map[string]any
	if err := json.Unmarshal([]byte(line), &ev); err != nil {
		return ""
	}
	t, _ := ev["type"].(string)
	if t != "permission" && t != "permission.updated" && t != "permission.asked" {
		return ""
	}
	out := map[string]any{"type": t}
	for _, k := range []string{"id", "permissionID", "title", "detail", "sessionID"} {
		if v, ok := ev[k]; ok {
			out[k] = v
		}
	}
	if _, ok := out["requestId"]; !ok {
		if v, ok := ev["permissionID"]; ok {
			out["requestId"] = v
		} else if v, ok := ev["id"]; ok {
			out["requestId"] = v
		}
	}
	b, _ := json.Marshal(out)
	return string(b)
}

func contains(s []string, v string) bool {
	for _, x := range s {
		if x == v {
			return true
		}
	}
	return false
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(v)
}
