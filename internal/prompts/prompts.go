// Package prompts loads debugging prompts for the agent. Built-in prompts are
// embedded from the prompts/ directory; a user-configured prompt library
// directory and per-service prompt overrides take precedence.
package prompts

import (
	"embed"
	"fmt"
	"os"
	"path/filepath"

	"github.com/gautambaghel/hsdebug/internal/config"
)

//go:embed all:library
var embedded embed.FS

// For returns the debugging prompt for a service, resolved in priority order:
//  1. inline PromptText on the service
//  2. PromptFile on the service
//  3. <PromptLibraryPath>/<catalogId>.md (user override dir)
//  4. embedded prompts/<catalogId>.md
//  5. embedded prompts/generic.md
func For(svc config.ServiceConfig, libraryPath string) (string, error) {
	if svc.PromptText != "" {
		return svc.PromptText, nil
	}
	if svc.PromptFile != "" {
		data, err := os.ReadFile(svc.PromptFile)
		if err != nil {
			return "", fmt.Errorf("reading prompt file %q: %w", svc.PromptFile, err)
		}
		return string(data), nil
	}
	id := svc.CatalogID
	if libraryPath != "" && id != "" {
		p := filepath.Join(libraryPath, id+".md")
		if data, err := os.ReadFile(p); err == nil {
			return string(data), nil
		}
	}
	if id != "" {
		if data, err := embedded.ReadFile("library/" + id + ".md"); err == nil {
			return string(data), nil
		}
	}
	data, err := embedded.ReadFile("library/generic.md")
	if err != nil {
		return "", fmt.Errorf("no prompt available and generic prompt missing: %w", err)
	}
	return string(data), nil
}

// Available lists the catalog ids that have a built-in prompt.
func Available() ([]string, error) {
	entries, err := embedded.ReadDir("library")
	if err != nil {
		return nil, err
	}
	var ids []string
	for _, e := range entries {
		name := e.Name()
		if filepath.Ext(name) == ".md" {
			ids = append(ids, name[:len(name)-len(".md")])
		}
	}
	return ids, nil
}
