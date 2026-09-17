package cli

import (
	"strings"
	"testing"
)

func TestDoctorReportsMissingConfig(t *testing.T) {
	tempConfig(t)
	globalOpts = Options{}
	out, err := run(t, newDoctorCmd())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "config") {
		t.Errorf("doctor should report config check: %q", out)
	}
	if !strings.Contains(out, "passed") {
		t.Errorf("doctor should print summary: %q", out)
	}
}

func TestDoctorJSON(t *testing.T) {
	tempConfig(t)
	globalOpts = Options{JSON: true, RedactProfile: "strict"}
	out, err := run(t, newDoctorCmd())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "\"checks\"") {
		t.Errorf("expected JSON checks array: %q", out)
	}
}

func TestAgentModelSetsConfig(t *testing.T) {
	tempConfig(t)
	globalOpts = Options{}
	out, err := run(t, newAgentModelCmd(), "anthropic/claude-sonnet-4-5")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "model set") {
		t.Errorf("unexpected output: %q", out)
	}
}

func TestAgentModelRejectsBadFormat(t *testing.T) {
	tempConfig(t)
	globalOpts = Options{}
	if _, err := run(t, newAgentModelCmd(), "notvalid"); err == nil {
		t.Error("expected error for model without provider/")
	}
}

func TestAgentProviderRequiresCredentialSource(t *testing.T) {
	tempConfig(t)
	globalOpts = Options{}
	if _, err := run(t, newAgentProviderCmd(), "anthropic"); err == nil {
		t.Error("expected error when no key source given")
	}
}

func TestDebugGatesWhenUnconfigured(t *testing.T) {
	tempConfig(t)
	globalOpts = Options{}
	_, err := run(t, newDebugCmd())
	if err == nil || !strings.Contains(err.Error(), "preflight failed") {
		t.Fatalf("expected preflight gate failure, got %v", err)
	}
}

func TestAgentGodModeToggle(t *testing.T) {
	tempConfig(t)
	globalOpts = Options{}
	out, err := run(t, newAgentGodModeCmd(), "on")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "god mode on") {
		t.Errorf("unexpected output enabling god mode: %q", out)
	}
	globalOpts = Options{}
	out, err = run(t, newAgentGodModeCmd())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "god mode is on") {
		t.Errorf("expected god mode to report on, got %q", out)
	}
	globalOpts = Options{}
	if _, err := run(t, newAgentGodModeCmd(), "banana"); err == nil {
		t.Error("expected error for invalid god mode arg")
	}
}

func TestDebugAcceptsMultipleServices(t *testing.T) {
	tempConfig(t)
	globalOpts = Options{}
	// Unconfigured agent still gates, but the command must accept multiple
	// positional args without an arg-count error.
	_, err := run(t, newDebugCmd(), "sonarr", "radarr")
	if err == nil || !strings.Contains(err.Error(), "preflight failed") {
		t.Fatalf("expected preflight gate failure (not arg error), got %v", err)
	}
}
