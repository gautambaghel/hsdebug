// Package cli provides shared command-line plumbing: global output flags
// (--json, --markdown, --jq), redaction, and rendering helpers used by all
// hsdebug subcommands.
package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/itchyny/gojq"

	"github.com/gautambaghel/hsdebug/internal/redact"
)

// OutputFormat enumerates supported output renderings.
type OutputFormat int

const (
	FormatText OutputFormat = iota
	FormatJSON
	FormatMarkdown
)

// Options are the global output/redaction flags shared by every command.
type Options struct {
	JSON     bool
	Markdown bool
	JQ       string // jq expression; implies JSON
	NoRedact bool   // disable masking for this command
	// RedactProfile is resolved from config/env and may be overridden by
	// NoRedact. Set by the command layer before rendering.
	RedactProfile string
}

// Format resolves the effective output format from the flags.
func (o Options) Format() OutputFormat {
	if o.JQ != "" || o.JSON {
		return FormatJSON
	}
	if o.Markdown {
		return FormatMarkdown
	}
	return FormatText
}

// Renderable is data that can present itself as text and markdown. JSON is
// derived from the value's own JSON marshaling.
type Renderable interface {
	Text() string
	Markdown() string
}

// Render writes the value to w according to the options.
func (o Options) Render(w io.Writer, v Renderable) error {
	switch o.Format() {
	case FormatJSON:
		return o.renderJSON(w, v)
	case FormatMarkdown:
		fmt.Fprintln(w, v.Markdown())
		return nil
	default:
		fmt.Fprintln(w, v.Text())
		return nil
	}
}

func (o Options) renderJSON(w io.Writer, v any) error {
	// Round-trip through JSON so redaction operates on a generic tree.
	raw, err := json.Marshal(v)
	if err != nil {
		return err
	}
	var tree any
	if err := json.Unmarshal(raw, &tree); err != nil {
		return err
	}

	profile := o.RedactProfile
	if o.NoRedact {
		profile = redact.RedactOff
	}
	tree = redact.New(profile).Apply(tree)

	if o.JQ != "" {
		return o.applyJQ(w, tree)
	}

	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(tree)
}

const RedactOffProfile = "off"

func (o Options) applyJQ(w io.Writer, tree any) error {
	query, err := gojq.Parse(o.JQ)
	if err != nil {
		return fmt.Errorf("invalid --jq expression: %w", err)
	}
	iter := query.Run(tree)
	var out []string
	for {
		v, ok := iter.Next()
		if !ok {
			break
		}
		if err, ok := v.(error); ok {
			return fmt.Errorf("jq: %w", err)
		}
		b, err := json.MarshalIndent(v, "", "  ")
		if err != nil {
			return err
		}
		out = append(out, string(b))
	}
	fmt.Fprintln(w, strings.Join(out, "\n"))
	return nil
}
