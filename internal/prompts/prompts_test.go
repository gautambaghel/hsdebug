package prompts

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gautambaghel/hsdebug/internal/config"
)

func TestForInlineText(t *testing.T) {
	svc := config.ServiceConfig{PromptText: "inline prompt"}
	got, err := For(svc, "")
	if err != nil {
		t.Fatal(err)
	}
	if got != "inline prompt" {
		t.Errorf("got %q", got)
	}
}

func TestForPromptFile(t *testing.T) {
	f := filepath.Join(t.TempDir(), "p.md")
	os.WriteFile(f, []byte("file prompt"), 0o644)
	svc := config.ServiceConfig{PromptFile: f}
	got, err := For(svc, "")
	if err != nil {
		t.Fatal(err)
	}
	if got != "file prompt" {
		t.Errorf("got %q", got)
	}
}

func TestForMissingPromptFileErrors(t *testing.T) {
	svc := config.ServiceConfig{PromptFile: "/no/such/file.md"}
	if _, err := For(svc, ""); err == nil {
		t.Error("expected error for missing prompt file")
	}
}

func TestForEmbeddedByCatalogID(t *testing.T) {
	svc := config.ServiceConfig{CatalogID: "sonarr"}
	got, err := For(svc, "")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "Sonarr") {
		t.Errorf("expected sonarr prompt, got %q", got[:min(40, len(got))])
	}
}

func TestForLibraryOverride(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "sonarr.md"), []byte("OVERRIDE"), 0o644)
	svc := config.ServiceConfig{CatalogID: "sonarr"}
	got, err := For(svc, dir)
	if err != nil {
		t.Fatal(err)
	}
	if got != "OVERRIDE" {
		t.Errorf("library override should win, got %q", got)
	}
}

func TestForFallsBackToGeneric(t *testing.T) {
	svc := config.ServiceConfig{CatalogID: "unknown-service"}
	got, err := For(svc, "")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "home-server service") {
		t.Errorf("expected generic prompt, got %q", got[:min(40, len(got))])
	}
}

func TestAvailableIncludesGeneric(t *testing.T) {
	ids, err := Available()
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, id := range ids {
		if id == "generic" {
			found = true
		}
	}
	if !found {
		t.Errorf("generic should be available: %v", ids)
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
