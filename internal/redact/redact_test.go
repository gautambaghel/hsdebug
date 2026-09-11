package redact

import (
	"encoding/json"
	"testing"
)

func decode(t *testing.T, s string) any {
	t.Helper()
	var v any
	if err := json.Unmarshal([]byte(s), &v); err != nil {
		t.Fatal(err)
	}
	return v
}

func TestStrictMasksKnownKeys(t *testing.T) {
	in := decode(t, `{"apiKey":"abc","name":"sonarr","password":"p"}`)
	out := New(RedactStrict).Apply(in).(map[string]any)
	if out["apiKey"] != Mask {
		t.Error("apiKey should be masked")
	}
	if out["password"] != Mask {
		t.Error("password should be masked")
	}
	if out["name"] != "sonarr" {
		t.Error("name should not be masked")
	}
}

func TestStrictMasksHeuristicKeys(t *testing.T) {
	in := decode(t, `{"my_auth_thing":"x","refreshCredential":"y","label":"z"}`)
	out := New(RedactStrict).Apply(in).(map[string]any)
	if out["my_auth_thing"] != Mask {
		t.Error("auth heuristic key should be masked")
	}
	if out["refreshCredential"] != Mask {
		t.Error("cred heuristic key should be masked")
	}
	if out["label"] != "z" {
		t.Error("label should not be masked")
	}
}

func TestStrictMasksCredentialShapedValues(t *testing.T) {
	in := decode(t, `{"note":"sk-abcdefghijklmnopqrstuvwxyz012345","plain":"hello"}`)
	out := New(RedactStrict).Apply(in).(map[string]any)
	if out["note"] != Mask {
		t.Errorf("credential-shaped value should be masked, got %v", out["note"])
	}
	if out["plain"] != "hello" {
		t.Error("plain short value should not be masked")
	}
}

func TestKnownProfileSkipsHeuristics(t *testing.T) {
	in := decode(t, `{"apiKey":"secret","my_auth_thing":"x","note":"sk-abcdefghijklmnopqrstuvwxyz012345"}`)
	out := New(RedactKnown).Apply(in).(map[string]any)
	if out["apiKey"] != Mask {
		t.Error("known key still masked in known profile")
	}
	if out["my_auth_thing"] != "x" {
		t.Error("heuristic key must NOT be masked in known profile")
	}
	if out["note"] != "sk-abcdefghijklmnopqrstuvwxyz012345" {
		t.Error("shape heuristic must NOT apply in known profile")
	}
}

func TestOffProfileMasksNothing(t *testing.T) {
	in := decode(t, `{"apiKey":"secret"}`)
	out := New(RedactOff).Apply(in).(map[string]any)
	if out["apiKey"] != "secret" {
		t.Error("off profile must not mask")
	}
}

func TestNestedAndArrays(t *testing.T) {
	in := decode(t, `{"services":[{"name":"a","token":"t1"},{"name":"b","token":"t2"}]}`)
	out := New(RedactStrict).Apply(in).(map[string]any)
	arr := out["services"].([]any)
	for _, e := range arr {
		m := e.(map[string]any)
		if m["token"] != Mask {
			t.Errorf("nested token should be masked: %v", m)
		}
		if m["name"] == Mask {
			t.Error("nested name should not be masked")
		}
	}
}

func TestUnknownProfileDefaultsToStrict(t *testing.T) {
	r := New("bogus")
	in := decode(t, `{"password":"x"}`)
	out := r.Apply(in).(map[string]any)
	if out["password"] != Mask {
		t.Error("unknown profile should behave as strict")
	}
}
