// Package redact masks sensitive values in structured output according to a
// profile: strict (default), known, or off.
package redact

import (
	"regexp"
	"strings"
)

const Mask = "***REDACTED***"

// Profile identifiers.
const (
	RedactStrict = "strict"
	RedactKnown  = "known"
	RedactOff    = "off"
)

// knownSecretKeys are field names always masked in strict and known profiles.
var knownSecretKeys = map[string]bool{
	"apikey":        true,
	"api_key":       true,
	"key":           true,
	"token":         true,
	"password":      true,
	"secret":        true,
	"credential":    true,
	"credentialref": true,
	"authorization": true,
	"access_token":  true,
	"refresh_token": true,
	"client_secret": true,
}

// heuristicKeyHints trigger masking in strict mode based on the field name.
var heuristicKeyHints = []string{"secret", "token", "passwd", "pass", "auth", "cred", "private"}

// credentialShape matches values that look like credentials (long opaque
// strings, sk-/sgamp- prefixes, JWT-like), used only in strict mode.
var credentialShape = regexp.MustCompile(`^(sk-[A-Za-z0-9_\-]{16,}|sgamp_[A-Za-z0-9_\-]{16,}|eyJ[A-Za-z0-9_\-]+\.[A-Za-z0-9_\-]+\.[A-Za-z0-9_\-]+|[A-Za-z0-9_\-]{40,})$`)

// Redactor applies masking based on a profile.
type Redactor struct {
	profile string
}

// New returns a Redactor. An empty or unknown profile defaults to strict.
func New(profile string) *Redactor {
	switch profile {
	case "off", "known", "strict":
	default:
		profile = "strict"
	}
	return &Redactor{profile: profile}
}

// Apply walks a decoded JSON value (maps/slices/scalars) and masks in place,
// returning the redacted value.
func (r *Redactor) Apply(v any) any {
	if r.profile == "off" {
		return v
	}
	switch t := v.(type) {
	case map[string]any:
		for k, val := range t {
			if r.shouldMaskKey(k) {
				t[k] = Mask
				continue
			}
			t[k] = r.applyValue(k, val)
		}
		return t
	case []any:
		for i, val := range t {
			t[i] = r.Apply(val)
		}
		return t
	default:
		return v
	}
}

func (r *Redactor) applyValue(key string, v any) any {
	switch t := v.(type) {
	case map[string]any, []any:
		return r.Apply(t)
	case string:
		if r.profile == "strict" && credentialShape.MatchString(t) {
			return Mask
		}
		return t
	default:
		return v
	}
}

func (r *Redactor) shouldMaskKey(key string) bool {
	lk := strings.ToLower(key)
	if knownSecretKeys[lk] {
		return true
	}
	if r.profile == "strict" {
		for _, hint := range heuristicKeyHints {
			if strings.Contains(lk, hint) {
				return true
			}
		}
	}
	return false
}
