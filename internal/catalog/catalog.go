// Package catalog holds the built-in list of commonly used home-server
// services with their default ports and health-check endpoints. Ports and
// endpoints are sourced from each application's well-known defaults.
package catalog

// Entry describes a known home-server service.
type Entry struct {
	ID           string // stable catalog id (matches prompt library file)
	Name         string // display name
	Port         int    // default port on loopback
	Scheme       string // http | https
	HealthPath   string // path probed for availability
	ExpectStatus []int  // acceptable HTTP status codes (200 or auth-redirects)
}

// ok200 is the common "healthy" set: 200 OK, plus auth redirects/401 that
// still indicate the service is up and responding.
var okUp = []int{200, 301, 302, 401, 403}

// Entries is the built-in catalog.
var Entries = []Entry{
	{ID: "jellyfin", Name: "Jellyfin", Port: 8096, Scheme: "http", HealthPath: "/health", ExpectStatus: []int{200}},
	{ID: "plex", Name: "Plex", Port: 32400, Scheme: "http", HealthPath: "/identity", ExpectStatus: []int{200, 401}},
	{ID: "sonarr", Name: "Sonarr", Port: 8989, Scheme: "http", HealthPath: "/ping", ExpectStatus: []int{200}},
	{ID: "radarr", Name: "Radarr", Port: 7878, Scheme: "http", HealthPath: "/ping", ExpectStatus: []int{200}},
	{ID: "bazarr", Name: "Bazarr", Port: 6767, Scheme: "http", HealthPath: "/", ExpectStatus: okUp},
	{ID: "prowlarr", Name: "Prowlarr", Port: 9696, Scheme: "http", HealthPath: "/ping", ExpectStatus: []int{200}},
	{ID: "qbittorrent", Name: "qBittorrent", Port: 8080, Scheme: "http", HealthPath: "/", ExpectStatus: okUp},
	{ID: "jellyseerr", Name: "Jellyseerr", Port: 5055, Scheme: "http", HealthPath: "/api/v1/status", ExpectStatus: []int{200}},
	{ID: "overseerr", Name: "Overseerr", Port: 5055, Scheme: "http", HealthPath: "/api/v1/status", ExpectStatus: []int{200}},
	{ID: "nzbget", Name: "NZBGet", Port: 6789, Scheme: "http", HealthPath: "/", ExpectStatus: okUp},
	{ID: "pihole", Name: "Pi-hole", Port: 80, Scheme: "http", HealthPath: "/admin/", ExpectStatus: okUp},
	{ID: "watchtower", Name: "Watchtower", Port: 8080, Scheme: "http", HealthPath: "/v1/metrics", ExpectStatus: []int{200, 401}},
}

// ByID returns the catalog entry with the given id, if present.
func ByID(id string) (Entry, bool) {
	for _, e := range Entries {
		if e.ID == id {
			return e, true
		}
	}
	return Entry{}, false
}
