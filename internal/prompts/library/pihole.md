# Debugging Pi-hole

Pi-hole is a network-wide DNS sinkhole. The web admin defaults to port 80 at
`/admin/`. The DNS service listens on port 53.

You are diagnosing why the local Pi-hole instance is unhealthy. Work through:

1. Is the `pihole-FTL` service running? (`pihole status`, `systemctl status
   pihole-FTL`, or `docker ps`.)
2. Is port 53 already in use by `systemd-resolved` or another resolver? This is
   the most common failure.
3. Does the web admin (`lighttpd` or the FTL web server) respond on `/admin/`?
4. Check `/var/log/pihole/FTL.log` and `pihole-FTL.log` for bind errors.
5. Verify gravity database integrity (`pihole -g` may need to run).
6. Check disk space and permissions on `/etc/pihole`.

Report the **cause** and **resolution** separately. Avoid destructive changes
without approval.
