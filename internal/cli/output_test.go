package cli

import (
	"bytes"
	"strings"
	"testing"
)

type sample struct {
	Name   string `json:"name"`
	APIKey string `json:"apiKey"`
}

func (s sample) Text() string     { return "name=" + s.Name }
func (s sample) Markdown() string { return "# " + s.Name }

func TestFormatResolution(t *testing.T) {
	if (Options{}).Format() != FormatText {
		t.Error("default should be text")
	}
	if (Options{JSON: true}).Format() != FormatJSON {
		t.Error("--json => JSON")
	}
	if (Options{Markdown: true}).Format() != FormatMarkdown {
		t.Error("--markdown => markdown")
	}
	if (Options{JQ: ".name"}).Format() != FormatJSON {
		t.Error("--jq implies JSON")
	}
}

func TestRenderText(t *testing.T) {
	var buf bytes.Buffer
	if err := (Options{}).Render(&buf, sample{Name: "x"}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "name=x") {
		t.Errorf("text render: %q", buf.String())
	}
}

func TestRenderMarkdown(t *testing.T) {
	var buf bytes.Buffer
	(Options{Markdown: true}).Render(&buf, sample{Name: "x"})
	if !strings.Contains(buf.String(), "# x") {
		t.Errorf("markdown render: %q", buf.String())
	}
}

func TestRenderJSONRedactsByDefault(t *testing.T) {
	var buf bytes.Buffer
	opts := Options{JSON: true, RedactProfile: "strict"}
	if err := opts.Render(&buf, sample{Name: "x", APIKey: "secret"}); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(buf.String(), "secret") {
		t.Errorf("apiKey should be redacted: %s", buf.String())
	}
	if !strings.Contains(buf.String(), "REDACTED") {
		t.Errorf("expected redaction marker: %s", buf.String())
	}
}

func TestRenderNoRedact(t *testing.T) {
	var buf bytes.Buffer
	opts := Options{JSON: true, RedactProfile: "strict", NoRedact: true}
	opts.Render(&buf, sample{Name: "x", APIKey: "secret"})
	if !strings.Contains(buf.String(), "secret") {
		t.Errorf("--no-redact should keep value: %s", buf.String())
	}
}

func TestRenderJQ(t *testing.T) {
	var buf bytes.Buffer
	opts := Options{JQ: ".name", RedactProfile: "off"}
	if err := opts.Render(&buf, sample{Name: "sonarr", APIKey: "k"}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "sonarr") {
		t.Errorf("jq output: %q", buf.String())
	}
}

func TestRenderJQInvalid(t *testing.T) {
	var buf bytes.Buffer
	opts := Options{JQ: ".[", RedactProfile: "off"}
	if err := opts.Render(&buf, sample{Name: "x"}); err == nil {
		t.Error("invalid jq should error")
	}
}
