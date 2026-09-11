package agent

import (
	"bufio"
	"context"
	"io"
	"os"
	"os/exec"

	"github.com/gautambaghel/hsdebug/internal/config"
)

// Verbosity controls how much debug output is surfaced.
type Verbosity int

const (
	Quiet   Verbosity = iota // cause + resolution only
	Normal                   // progress
	Verbose                  // full JSON events
)

// RunStreaming runs `opencode run` for a prompt and streams output line-by-line
// to the provided callback. It attaches to a managed opencode server when
// attachURL is non-empty. Permissions are governed by the isolated config plus
// the auto flag.
//
// This uses os/exec directly (not the Runner interface) because it needs live
// streaming; the Runner interface is for buffered/fakeable calls like the probe.
func (m *Manager) RunStreaming(ctx context.Context, prompt, attachURL string, auto bool, onLine func(string)) error {
	path, err := m.runner.Look()
	if err != nil {
		return err
	}
	args := []string{"run", "--format", "json"}
	if m.cfg.Agent.Model != "" {
		args = append(args, "-m", m.cfg.Agent.Model)
	}
	if attachURL != "" {
		args = append(args, "--attach", attachURL)
	}
	if auto {
		args = append(args, "--auto")
	}
	args = append(args, prompt)

	ocp, _ := config.OpencodeConfigPath()
	cmd := exec.CommandContext(ctx, path, args...)
	cmd.Env = append(os.Environ(), "OPENCODE_CONFIG="+ocp)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	cmd.Stderr = cmd.Stdout
	if err := cmd.Start(); err != nil {
		return err
	}
	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		onLine(scanner.Text())
	}
	if err := scanner.Err(); err != nil && err != io.EOF {
		_ = cmd.Wait()
		return err
	}
	return cmd.Wait()
}
