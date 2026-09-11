package catalog

import "testing"

func TestByIDFound(t *testing.T) {
	e, ok := ByID("sonarr")
	if !ok {
		t.Fatal("sonarr should be in catalog")
	}
	if e.Port != 8989 {
		t.Errorf("sonarr port = %d, want 8989", e.Port)
	}
}

func TestByIDMissing(t *testing.T) {
	if _, ok := ByID("nonexistent"); ok {
		t.Error("unknown id should not be found")
	}
}

func TestEntriesValid(t *testing.T) {
	seen := map[string]bool{}
	for _, e := range Entries {
		if e.ID == "" || e.Name == "" {
			t.Errorf("entry with empty id/name: %+v", e)
		}
		if e.Port <= 0 || e.Port > 65535 {
			t.Errorf("%s: invalid port %d", e.ID, e.Port)
		}
		if e.Scheme != "http" && e.Scheme != "https" {
			t.Errorf("%s: invalid scheme %q", e.ID, e.Scheme)
		}
		if len(e.ExpectStatus) == 0 {
			t.Errorf("%s: must declare expected status", e.ID)
		}
		if seen[e.ID] {
			t.Errorf("duplicate catalog id %q", e.ID)
		}
		seen[e.ID] = true
	}
}

func TestExpectedServicesPresent(t *testing.T) {
	want := []string{"jellyfin", "plex", "sonarr", "radarr", "bazarr", "prowlarr",
		"qbittorrent", "jellyseerr", "overseerr", "nzbget", "pihole", "watchtower"}
	for _, id := range want {
		if _, ok := ByID(id); !ok {
			t.Errorf("catalog missing required service %q", id)
		}
	}
}
