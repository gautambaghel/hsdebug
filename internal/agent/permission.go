package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// RespondPermission relays an operator's allow/deny decision to a managed
// `opencode serve` instance via its HTTP API:
//
//	POST /session/{sessionID}/permissions/{permissionID}  body {response, remember?}
//
// This is the interactive pass-through path. It requires a running opencode
// server (configured via Agent.OpencodePort). When no server is reachable it
// returns an error; in that case god mode (allow-all) is the reliable elevated
// path for non-interactive one-shot runs.
//
// requestID is expected in the form "<sessionID>/<permissionID>"; if no slash
// is present the whole value is treated as the permission id and the caller's
// active session is assumed unknown (returns an error).
func (m *Manager) RespondPermission(ctx context.Context, requestID, decision string) error {
	port := m.cfg.Agent.OpencodePort
	if port == 0 {
		return fmt.Errorf("no managed opencode server configured; enable god mode for elevated one-shot runs")
	}
	sessionID, permID, ok := splitPermissionRef(requestID)
	if !ok {
		return fmt.Errorf("cannot route permission decision: request id %q missing session context", requestID)
	}
	response := "allow"
	if decision == "deny" {
		response = "reject"
	}
	body, _ := json.Marshal(map[string]any{"response": response})
	url := fmt.Sprintf("http://127.0.0.1:%d/session/%s/permissions/%s", port, sessionID, permID)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("reaching opencode server: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("opencode permission response: HTTP %d", resp.StatusCode)
	}
	return nil
}

func splitPermissionRef(ref string) (sessionID, permID string, ok bool) {
	if i := strings.LastIndex(ref, "/"); i > 0 {
		return ref[:i], ref[i+1:], true
	}
	return "", "", false
}
