<div align="center">
  <img src="web/static/images/favicon.svg" alt="Servicarr" width="80">
  
# Servicarr

**Live Demo**: [https://status.jenodoescode.com](https://status.jenodoescode.com)
</div>

A lightweight, self-hosted status page that monitors your services and displays real-time uptime. Built with Go and vanilla JavaScript, deployed via Docker.

## Features

- **Service Monitoring** — HTTP, TCP, DNS and "always up" health checks with configurable per-service intervals and timeouts
- **Service Relationships** — Define `depends_on` (hierarchical) and `connected_to` (peer) relationships with visual matrix view
- **Setup Wizard** — First-run wizard to configure credentials, add services and optionally import a database backup
- **20+ Service Templates** — Pre-built templates for Plex, Sonarr, Radarr, Jellyfin, Nextcloud, Home Assistant, Pi-hole and more
- **Uptime Bars** — 30-day visual uptime history per service with daily granularity; click any day for hour-by-hour breakdown
- **Matrix View** — Network topology visualisation with dependency arcs, connected-to links and status lines
- **System Resources** — Live CPU, RAM, disk, GPU, swap, network, containers, processes and uptime via [Glances](https://github.com/nicolargo/glances), plus UPS status, automatic mains-loss warnings and transition-based email alerts via Network UPS Tools
- **CrowdSec Integration** — Mirror active CrowdSec decisions (bans, captchas) into an admin dashboard via the Local API, with encrypted credential storage, configurable sync intervals, connection testing and a live sync badge
- **Multi-Channel Alerts** — SMTP, webhook, Discord and Telegram notifications
- **Status Alerts** — Manual banners, one-time windows and flexible daily/weekly maintenance schedules with automatic monitoring and uptime suppression
- **Admin Panel** — Manage services, view logs, reorder cards, toggle monitoring, import/export database
- **Security** — CSRF protection, CSP headers (no unsafe-inline for scripts), HSTS, IP-based rate limiting, auto-blocking after failed logins, IP whitelist/blacklist, SSRF protection, request body size limits
- **Responsive** — Mobile-optimised layout with touch-friendly uptime tooltips
- **Logging** — Structured internal logs (info/warn/error) with search, filtering and auto-pruning
- **Docker Ready** — Multi-stage build, non-root container, SQLite storage

## Quick Start

### Prerequisites

- Docker and Docker Compose
- Or: Go 1.26.8+ for local development

### Using Docker (Recommended)

1. **Clone the repository**
   ```bash
   git clone https://github.com/JeKaQM/Servicarr_.git
   cd Servicarr_
   ```

2. **Create a `.env` file** in the project root (Compose expects the file; the setup wizard handles most settings):

   ```bash
   cp .env.example .env
   ```

   Review the placeholders before using environment-based credentials. A minimal example is:

   ```env
   PORT=4555
   UNBLOCK_TOKEN=<a-secure-random-string>
   ```

3. **Start the application**
   ```bash
   docker compose -f deploy/docker-compose.yml up -d
   ```

4. **Open** http://localhost:4555 — the setup wizard will guide you through first-time configuration.

### Running Locally

1. **Run the application**
   ```bash
   go run ./app
   ```

2. **Open** http://localhost:4555

## Versioning

Servicarr uses semantic versions. The release-line version is stored in `app/internal/buildinfo/VERSION` and embedded into local Go builds. Keep `package.json` and `package-lock.json` aligned; CI rejects a mismatch between the application and package versions.

```bash
go run ./app --version
docker exec servicarr status --version
```

Every successful CI run for a push to `main` creates a GitHub Release and an immutable `ghcr.io/jekaqm/servicarr:<version>` image. Patch versions advance once per first-parent `main` commit. To start a new major or minor release line, update `app/internal/buildinfo/VERSION`, `package.json`, and `package-lock.json` together; that merge uses the requested version and subsequent merges continue its patch sequence.

Release images also update the `latest`, major, and major/minor container tags when the released commit is still the current `main`. Docker builds embed the UTC build time and accept `APP_VERSION` and `VCS_REF` build arguments. Compose uses the source version and accepts `SERVICARR_COMMIT` for the commit value. The running version, commit, build time, Go runtime, SQLite version, database schema, and recent deployment history are available under **Admin > Settings**. Each startup is also written to the system log and persisted in the database.

## Configuration

All settings are stored in SQLite after the setup wizard completes. The following environment variables can still be set:

| Variable | Default | Description |
|----------|---------|-------------|
| `PORT` | `4555` | HTTP listen port |
| `DB_PATH` | `./uptime.db` | SQLite database path (the Docker image sets this to `/data/uptime.db`) |
| `POLL_SECONDS` | `60` | Scheduler polling interval |
| `ENABLE_SCHEDULER` | `true` | Run background health checks |
| `INSECURE_DEV` | `false` | Set to `true` only for local HTTP development (disables Secure cookie flag) |
| `UNBLOCK_TOKEN` | — | Secret token for the self-unblock endpoint |
| `SESSION_MAX_AGE_SECONDS` | `86400` | Session cookie lifetime in seconds |
| `TRUSTED_PROXIES` | empty | Comma-separated IPs/CIDRs of trusted reverse proxy peers; empty ignores forwarded client IP headers |
| `STATUS_PAGE_URL` | — | Public URL included in alert emails |

> **Production deployment**: Always run behind a reverse proxy (nginx, Caddy, Cloudflare Tunnel) that terminates TLS. The application sets `Strict-Transport-Security`, `X-Frame-Options: DENY`, and strict CSP headers automatically.

## Default Credentials

Credentials are normally set during the setup wizard. For the environment fallback, configure:

- **Username**: `AUTH_USER` (the Docker image defaults to `admin`)
- **Password**: Set via `AUTH_PASSWORD` (or provide a bcrypt hash in `AUTH_PASSWORD_BCRYPT`)

## Scheduled Maintenance

Under **Admin > Banners**, choose a one-time window with calendar start/end dates, a daily repeat, or a weekly repeat on any combination of weekdays. One-time windows can span any number of days or stay active with **No end date** until disabled or deleted. Recurring durations accept minutes, hours, days or weeks without the old 24-hour/one-week limits (only a numeric overflow guard of approximately 292 years remains). Existing Monday 02:55–03:25 `Europe/London` rules are preserved and remain editable.

Times use the schedule's IANA timezone, independent of the browser timezone. During a daylight-saving change, missing one-time clock times are rejected, missing recurring occurrences are skipped, and repeated clock times use their first occurrence. API clients can supply an explicit RFC3339 offset to select the second occurrence. Existing schedules and their new fields survive database backup/restore. While a schedule with monitoring suppression is active, Servicarr shows a maintenance banner and skips service checks, failure tracking, incidents, alert dispatch, heartbeats, samples, and uptime updates.

Outside maintenance, a confirmed service failure creates an automatic critical-outage banner. Once all affected services recover, it is replaced by a restoration banner for 24 hours while performance is monitored. The outage banner only states that an alert was sent when at least one configured notification channel was actually queued.

UPS mains-loss email uses the SMTP recipient configured under **Admin > Notifications**. One email is queued per confirmed outage; NUT connection failures and unknown UPS states do not trigger it.

## CrowdSec

Servicarr can mirror a [CrowdSec](https://crowdsec.net) Local API (LAPI) into the admin-only dashboard under **Admin > CrowdSec**. The two credential types unlock separate feeds:

| Credential | CrowdSec capability | Servicarr panel |
|------------|---------------------|-----------------|
| Bouncer API key | Read `/v1/decisions` | Active bans, captchas and other decisions |
| Machine ID and password | Log in through `/v1/watchers/login` and read `/v1/alerts` | 24-hour detection feed, map and alert statistics |

Configure either credential for its corresponding feed, or configure both for the complete dashboard.

1. On the CrowdSec host, create a bouncer key if you want the active-decisions panel:

   ```bash
   sudo cscli bouncers add servicarr
   ```

   Copy the generated API key when it is displayed.

2. Create machine credentials if you want the live detection feed:

   ```bash
   sudo cscli machines add servicarr --auto -f -
   ```

   The `-f -` form prints the generated YAML to standard output instead of replacing CrowdSec's own `/etc/crowdsec/local_api_credentials.yaml`. Use the printed `login` as the machine ID and the printed `password` as the machine password. Because this command runs on the LAPI host with `--auto`, the machine is registered and validated immediately.

3. Under **Admin > CrowdSec**, enter the LAPI root URL (for example, `http://10.0.0.5:8080`; a trailing `/v1` is accepted and stripped), add the credentials, choose a sync interval from 10 to 3600 seconds, and select **Test Connection** before saving.

The LAPI URL is resolved from inside the Servicarr container. Do not use `localhost` or `127.0.0.1` unless CrowdSec runs in that same container. Put both containers on a shared Docker network and use the CrowdSec service name, or use a host/LAN address that the Servicarr container can reach. CrowdSec's LAPI commonly listens on loopback by default; configure its `api.server.listen_uri` for the intended Docker/LAN interface and restrict TCP port 8080 at the firewall to the Servicarr host or container network. Use HTTPS when the connection crosses an untrusted network. **Skip TLS verification** is only for a self-signed certificate on a trusted network and should remain off otherwise.

Dashboard behaviour and limits:

- The dashboard refreshes while the CrowdSec tab is visible. **Sync Now** forces an immediate server-side poll.
- Each decision sync stores at most 500 active decisions from the page returned by LAPI. This is a capped snapshot, not a count of every decision held by LAPI.
- Each alert sync requests at most 100 local (non-CAPI) alerts from the previous 24 hours. Alerts are deduplicated by LAPI ID, and the local history is capped at 2000 rows. On a very busy LAPI, the feed therefore represents the newest detections rather than an exhaustive event ledger.
- The UI reads the local SQLite snapshot, so the most recently synced data remains available during a short LAPI outage. The badge records the last successful sync and distinguishes authentication failures from other errors.
- Cloud metadata endpoints are rejected from the LAPI URL, redirects are not followed, and credentials are encrypted at rest. Secrets are never returned by the API and are excluded from database backups.

## Notifications

The master enable switch, event filters and public dashboard URL under **Admin > Notifications** apply to all configured channels. Unavailable, degraded, outage-recovery and degraded-recovery events can be selected independently; degraded recovery is off by default. Repeated checks in the same state do not send duplicate alerts.

Discord embeds show the service, new/previous status, observation time and available check type, HTTP response code, latency and time in the previous unhealthy state. The title links to your public dashboard. Set a custom sender name or enable silent delivery, and use the scenario selector to test unavailable, degraded, recovery and test messages. Mentions are disabled; monitor URLs and raw check errors are excluded from notification details. Save configuration before testing. Failed deliveries now report an error, and Discord rate limits receive bounded retries.

## Security

When upgrading behind a reverse proxy, set `TRUSTED_PROXIES` to the actual proxy IPs or CIDRs and restart Servicarr. For a local proxy this might be `127.0.0.1/32,::1/128`; a Docker proxy needs its actual network peer address. Only trusted peers can supply forwarded client IPs, and forwarded chains are evaluated from right to left. With the default empty setting, rate limits and blocks use the direct peer IP. Configure the proxy to overwrite client-supplied forwarded headers.

Password changes invalidate existing sessions. Public service and history endpoints omit hidden services and private connection details. The CI pipeline checks reachable Go vulnerabilities and high-severity npm advisories, and its aggregate status fails if a required CI job fails or is skipped.

- **Rate limiting**: Login 10/min, public API 120/min, health-check 30/min, setup/unblock 5/min per IP
- **Auto-blocking**: 3 failed login attempts → 24-hour IP block (cleared on successful login)
- **IP whitelist / blacklist**: Managed from the admin panel
- **CSRF tokens**: Double-submit cookie pattern on all state-changing requests
- **Session auth**: HMAC-SHA256 signed cookies with `HttpOnly`, `SameSite=Lax`, `Secure` flags; crypto/rand generated secret
- **CSP headers**: Strict Content-Security-Policy (no `unsafe-inline` for scripts)
- **HSTS**: `Strict-Transport-Security` header on all responses
- **SSRF protection**: Cloud metadata endpoints (169.254.169.254, metadata.google.internal) blocked in health-check URLs
- **Request limits**: 70 MB global body size limit; minimum 8-character passwords
- **Slowloris protection**: `ReadHeaderTimeout` set on the HTTP server
- **Graceful shutdown**: SIGINT/SIGTERM signal handling with connection draining
- **SQLite hardening**: WAL mode, busy timeout, single-connection pool
- **Non-root container**: Docker image runs as unprivileged `servicarr` user; port bound to `127.0.0.1`
- **Self-unblock**: `POST /api/self-unblock` with `{"token": "<UNBLOCK_TOKEN>"}` to remove your own IP block

## Project Structure

```
Servicarr_/
├── app/
│   ├── main.go                 # Entry point, scheduler, server
│   └── internal/
│       ├── alerts/             # SMTP email alerting
│       ├── auth/               # Session / HMAC auth
│       ├── cache/              # TTL in-memory cache
│       ├── checker/            # HTTP / TCP / DNS health checks
│       ├── config/             # Env-based configuration
│       ├── database/           # SQLite schema + CRUD
│       ├── handlers/           # HTTP handlers + routes
│       ├── maintenance/        # Recurring schedule evaluation
│       ├── models/             # Data structures
│       ├── monitor/            # Consecutive-failure tracker
│       ├── ratelimit/          # Token-bucket rate limiter
│       ├── resources/          # Glances API v4 client
│       ├── security/           # IP blocking, CSP, middleware
│       └── stats/              # Heartbeat recording + aggregation
├── web/
│   ├── static/                 # CSS, JS, images
│   └── templates/              # Go HTML templates
├── deploy/
│   ├── Dockerfile              # Multi-stage Go → Debian slim
│   └── docker-compose.yml      # Production compose file
├── go.mod
└── README.md
```

## API Reference

### Public Endpoints

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/api/check` | Live status of all services |
| `GET` | `/api/metrics?days=30` | Historical uptime data (daily buckets) |
| `GET` | `/api/metrics?hours=24` | Historical uptime data (hourly buckets) |
| `GET` | `/api/metrics/day-detail` | Hour-by-hour breakdown for a specific day |
| `GET` | `/api/uptime?service=KEY` | Pre-computed uptime stats |
| `GET` | `/api/heartbeats?service=KEY` | Recent heartbeats |
| `GET` | `/api/resources` | System resource snapshot (Glances and optional NUT UPS) |
| `GET` | `/api/resources/config` | Resources UI tile visibility |
| `GET` | `/api/services` | Visible service list |
| `GET` | `/api/services/templates` | Available service templates |
| `GET` | `/api/status-alerts` | Active maintenance/incident banners |

### Authentication Endpoints

| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/api/login` | Authenticate and receive session cookie |
| `POST` | `/api/logout` | Clear session cookie |
| `GET` | `/api/me` | Current authenticated user info |
| `POST` | `/api/self-unblock` | Remove own IP block (requires `UNBLOCK_TOKEN`) |

### Setup Endpoints (first-run only)

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/api/setup/status` | Check if setup is complete |
| `POST` | `/api/setup` | Complete initial setup (credentials + settings) |
| `POST` | `/api/setup/service` | Add a service during setup |
| `POST` | `/api/setup/import` | Import database backup during setup |

### Admin — Service Management (require auth)

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/api/admin/services` | List all services (includes hidden) |
| `POST` | `/api/admin/services` | Create a new service |
| `PUT` | `/api/admin/services/{id}` | Update a service |
| `DELETE` | `/api/admin/services/{id}` | Delete a service |
| `PUT` | `/api/admin/services/{id}/visibility` | Toggle service visibility |
| `POST` | `/api/admin/services/reorder` | Reorder service cards |
| `POST` | `/api/admin/services/test` | Test service connection |
| `POST` | `/api/admin/toggle-monitoring` | Enable/disable monitoring for a service |
| `POST` | `/api/admin/ingest-now` | Force an immediate health-check cycle |
| `POST` | `/api/admin/check` | Run admin health check |
| `POST` | `/api/admin/reset-recent` | Reset recent check data |

### Admin — Alerts & Notifications (require auth)

| Method | Path | Description |
|--------|------|-------------|
| `GET/POST` | `/api/admin/alerts/config` | Get/save alert channel settings |
| `POST` | `/api/admin/alerts/test` | Send test email notification |
| `POST` | `/api/admin/alerts/test-channel` | Test any notification channel |
| `POST` | `/api/admin/resources/test` | Test Glances or NUT without saving settings |
| `GET/POST/DELETE` | `/api/admin/status-alerts` | Manage maintenance/incident banners |
| `GET/POST/DELETE` | `/api/admin/maintenance-schedules` | Manage recurring maintenance windows |

### Admin — Settings (require auth)

| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/api/admin/settings/app-name` | Update application name |
| `POST` | `/api/admin/settings/password` | Change admin password |
| `GET` | `/api/admin/settings/export` | Download database backup |
| `POST` | `/api/admin/settings/import` | Import database backup |
| `POST` | `/api/admin/settings/reset` | Factory reset the database |
| `GET/POST` | `/api/admin/resources/config` | Get/save resources tile settings |

### Admin — Security & Logs (require auth)

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/api/admin/blocks` | List blocked IPs |
| `POST` | `/api/admin/unblock` | Unblock a specific IP |
| `POST` | `/api/admin/clear-blocks` | Clear all IP blocks |
| `GET/POST/DELETE` | `/api/admin/whitelist` | Manage IP whitelist |
| `GET/POST/DELETE` | `/api/admin/blacklist` | Manage IP blacklist |
| `GET` | `/api/admin/logs` | Query structured logs |
| `DELETE` | `/api/admin/logs` | Clear all logs |
| `GET` | `/api/admin/logs/stats` | Log statistics summary |

### Admin — CrowdSec (require auth)

| Method | Path | Description |
|--------|------|-------------|
| `GET/POST` | `/api/admin/crowdsec/config` | Get the masked configuration or save connection settings |
| `GET` | `/api/admin/crowdsec/status` | Last sync, error state and local decision count |
| `GET` | `/api/admin/crowdsec/decisions?active=true` | Read the local decision snapshot; `active=true` excludes expired rows |
| `GET` | `/api/admin/crowdsec/alerts?limit=50` | Read recent local alerts; `limit` may be 1–2000 |
| `GET` | `/api/admin/crowdsec/stats` | Read 24-hour detection and active-decision aggregates |
| `POST` | `/api/admin/crowdsec/sync-now` | Run an immediate sync |
| `POST` | `/api/admin/crowdsec/test` | Test LAPI reachability and the supplied credential realms without saving |

## Development

### Building the Docker image

```bash
docker build -f deploy/Dockerfile -t servicarr:latest .
```

### Running tests

```bash
go test ./...
npm test -- --runInBand
```

The Go and JavaScript suites cover the backend packages, API handlers, resource clients, security middleware, and browser-side dashboard logic.

## Troubleshooting

**App won't start?**
- Check your `.env` file exists or environment variables are set
- Ensure port 4555 is not in use: `lsof -i :4555`
- Check container logs: `docker logs servicarr`

**Services show as down?**
- Verify service URLs are correct and reachable from inside the container
- For *arr apps and services behind API keys, set the API token in the service config
- Check firewall and Docker network connectivity

**Resources section shows UNAVAILABLE?**
- Ensure [Glances](https://github.com/nicolargo/glances) is running and accessible from the container
- Configure the Glances host:port in **Admin → Resources**
- Check that Glances API v4 is enabled (default port 61208)
- For UPS details, ensure NUT `upsd` is reachable from the container (default port 3493)
- Configure the NUT host:port and UPS name in **Admin → Resources**. For `upsc apc`, the UPS name is `apc`; `upsc -l` lists names exposed by `upsd`

**CrowdSec live view is empty or reports an unreachable LAPI?**
- Confirm the LAPI URL is reachable from the Servicarr container. Inside Docker, `localhost` is Servicarr, not the Docker host or another container.
- If LAPI only listens on `127.0.0.1:8080`, change CrowdSec's `api.server.listen_uri` to an interface reachable on the intended Docker/LAN network, then restrict access with a firewall.
- Run **Test Connection**. A bouncer key validates the decisions feed; machine credentials validate the alerts feed. The live detection view requires the latter.
- Select **Sync Now**, then inspect `docker compose -f deploy/docker-compose.yml logs --tail 200 servicarr` if the badge still shows an error.
- A working but empty alert feed can simply mean LAPI has no local alerts in the last 24 hours. Imported CAPI/community-blocklist alerts are deliberately omitted from this live view.
- Keep **Skip TLS verification** disabled unless you intentionally use a self-signed certificate on a trusted network.

**Login not working?**
- Clear browser cookies and retry
- If your IP is blocked, use the self-unblock endpoint with your `UNBLOCK_TOKEN`
- Blocked IPs are stored in the database and persist across restarts

**Uptime bars are all grey?**
- This is normal on a fresh install — data accumulates once the scheduler runs
- Wait a few minutes for the first data points to appear
- Verify `ENABLE_SCHEDULER=true` (default)

## License

MIT

## Author

Created by jekaq
