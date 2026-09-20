package agent

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/gautambaghel/hsdebug/internal/config"
)

func TestPermissionPolicy(t *testing.T) {
	off := permissionPolicy(false)
	if off["bash"] != "ask" || off["edit"] != "ask" {
		t.Errorf("expected ask defaults when god mode off, got %v", off)
	}
	on := permissionPolicy(true)
	if on["bash"] != "allow" || on["edit"] != "allow" || on["webfetch"] != "allow" {
		t.Errorf("expected allow-all when god mode on, got %v", on)
	}
}

func TestSetGodModeWritesPermissionBlock(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HSDEBUG_CONFIG_DIR", dir)
	t.Setenv("HSDEBUG_CONFIG", filepath.Join(dir, "config.json"))

	cfg := config.Default()
	cfg.Agent.Model = "anthropic/claude-sonnet-4-5"
	cfg.Agent.Provider = "anthropic"
	mgr, err := NewManager(cfg, &fakeRunner{installed: true})
	if err != nil {
		t.Fatal(err)
	}

	if err := mgr.SetGodMode(true); err != nil {
		t.Fatal(err)
	}
	if !mgr.GodMode() {
		t.Error("GodMode() should report true after enabling")
	}

	ocp, _ := config.OpencodeConfigPath()
	data, err := os.ReadFile(ocp)
	if err != nil {
		t.Fatal(err)
	}
	var oc OpencodeConfig
	if err := json.Unmarshal(data, &oc); err != nil {
		t.Fatal(err)
	}
	if oc.Permission["bash"] != "allow" {
		t.Errorf("expected bash=allow in written config, got %v", oc.Permission)
	}

	if err := mgr.SetGodMode(false); err != nil {
		t.Fatal(err)
	}
	data, _ = os.ReadFile(ocp)
	_ = json.Unmarshal(data, &oc)
	if oc.Permission["bash"] != "ask" {
		t.Errorf("expected bash=ask after disabling god mode, got %v", oc.Permission)
	}
}

func TestSplitPermissionRef(t *testing.T) {
	if sid, pid, ok := splitPermissionRef("ses_abc/perm_123"); !ok || sid != "ses_abc" || pid != "perm_123" {
		t.Errorf("unexpected split: %q %q %v", sid, pid, ok)
	}
	if _, _, ok := splitPermissionRef("perm_only"); ok {
		t.Error("expected split to fail without session context")
	}
}
