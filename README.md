<div align="center">
  <img src="web/static/images/favicon.svg" alt="Servicarr" width="80">
  
# Servicarr

**Live Demo**: [https://status.jenodoescode.com](https://status.jenodoescode.com)
</div>

A lightweight, self-hosted status page that monitors your services and displays real-time uptime. Built with Go and vanilla JavaScript, deployed via Docker.

## Features

- **Service Monitoring** — HTTP, TCP, DNS and "always up" health checks with configurable intervals and timeouts
- **Setup Wizard** — First-run wizard to configure credentials, add services and optionally import a database backup
- **20+ Service Templates** — Pre-built templates for Plex, Sonarr, Radarr, Jellyfin, Nextcloud, Home Assistant, Pi-hole and more
- **Uptime Bars** — 30-day visual uptime history per service with daily granularity
- **System Resources** — Live CPU, RAM, disk, GPU, swap, network, containers, processes and uptime via [Glances](https://github.com/nicolargo/glances)
- **Email Alerts** — SMTP notifications when services go down or recover
- **Status Alerts** — Public maintenance/incident banners
- **Admin Panel** — Manage services, view logs, reorder cards, toggle monitoring, import/export database
- **Security** — IP-based rate limiting, automatic blocking after failed logins, IP whitelist/blacklist, CSRF protection, CSP headers
- **Responsive** — Mobile-optimised layout with touch-friendly uptime tooltips
- **Logging** — Structured internal logs (info/warn/error) with search, filtering and auto-pruning
- **Docker Ready** — Multi-stage build, single container, SQLite storage

## Quick Start

### Prerequisites

- Docker and Docker Compose
- Or: Go 1.25+ for local development

### Using Docker (Recommended)

1. **Clone the repository**
   ```bash
   git clone https://github.com/JeKaQM/Servicarr_.git
   cd Servicarr_
   ```

2. **Create a `.env` file** in the project root (optional — the setup wizard handles most settings):
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

## Configuration

All settings are stored in SQLite after the setup wizard completes. The following environment variables can still be set:

| Variable | Default | Description |
|----------|---------|-------------|
| `PORT` | `4555` | HTTP listen port |
| `DB_PATH` | `data/status.db` | SQLite database path |
| `POLL_SECONDS` | `60` | Scheduler polling interval |
| `ENABLE_SCHEDULER` | `true` | Run background health checks |
| `INSECURE_DEV` | `true` | Set to `false` when behind HTTPS |
| `UNBLOCK_TOKEN` | — | Secret token for the self-unblock endpoint |
| `SESSION_MAX_AGE` | `86400` | Session cookie lifetime in seconds |
| `STATUS_PAGE_URL` | — | Public URL included in alert emails |

## Default Credentials

Set during the setup wizard. Defaults if using env-based config:

- **Username**: `admin`
- **Password**: Set via `ADMIN_PASSWORD` env var

## Security

- **Rate limiting**: Login 20/min, public API 120/min, health-check 30/min per IP
- **Auto-blocking**: Configurable failed-login threshold + duration
- **IP whitelist / blacklist**: Managed from the admin panel
- **CSRF tokens**: Required on all state-changing requests
- **Session auth**: HMAC-signed cookies with configurable TTL
- **CSP headers**: Strict Content-Security-Policy on all responses
- **Self-unblock**: `POST /api/self-unblock?token=<UNBLOCK_TOKEN>` to remove your own IP block

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
| `GET` | `/api/uptime?service=KEY` | Pre-computed uptime stats |
| `GET` | `/api/heartbeats?service=KEY` | Recent heartbeats |
| `GET` | `/api/resources` | System resource snapshot (Glances) |
| `GET` | `/api/resources/config` | Resources UI tile visibility |
| `GET` | `/api/services` | Visible service list |
| `GET` | `/api/status-alerts` | Active maintenance/incident banners |

### Admin Endpoints (require auth)

| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/api/admin/toggle-monitoring` | Enable/disable monitoring for a service |
| `POST` | `/api/admin/ingest-now` | Force an immediate health-check cycle |
| `POST` | `/api/admin/services` | Create a new service |
| `PUT`  | `/api/admin/services/{id}` | Update a service |
| `DELETE` | `/api/admin/services/{id}` | Delete a service |
| `POST` | `/api/admin/services/reorder` | Reorder service cards |
| `GET/POST` | `/api/admin/alerts/config` | Get/save email alert settings |
| `GET/POST` | `/api/admin/resources/config` | Get/save resources tile settings |
| `POST` | `/api/admin/settings/password` | Change admin password |
| `GET` | `/api/admin/settings/export` | Download database backup |
| `POST` | `/api/admin/settings/import` | Import database backup |
| `GET` | `/api/admin/logs` | Query structured logs |

## Development

### Building the Docker image

```bash
docker build -f deploy/Dockerfile -t servicarr:latest .
```

### Running tests

```bash
go test ./...
```

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
