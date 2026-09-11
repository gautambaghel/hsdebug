# Debugging Sonarr

Sonarr is a PVR for Usenet and BitTorrent that manages TV series. It defaults to
port 8989 and exposes `/ping` for health.

You are diagnosing why the local Sonarr instance is unhealthy. Work through:

1. Confirm the process/container is running and listening on its port.
2. Check the Sonarr logs (typically under the config directory `logs/`) for
   startup errors, database corruption, or migration failures.
3. Verify the SQLite database (`sonarr.db`) is not locked or corrupted.
4. Check disk space and permissions on the config and media directories.
5. Confirm connectivity to configured indexers (via Prowlarr) and download
   clients (qBittorrent/NZBGet).
6. If behind a reverse proxy, verify the URL base and that `/ping` returns 200.

Report the most likely root cause and the concrete resolution steps. Do not make
destructive changes without explicit approval.
