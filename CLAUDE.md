# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

Servicarr is a self-hosted status page: a single Go binary (module `status`, entry point `app/main.go`, packages under `app/internal/`) serving a vanilla-JS frontend from `web/`, storing everything in SQLite, deployed with Docker. The GitHub repository is public.

## Commands

```bash
go run ./app                                   # serves http://localhost:4555 (set INSECURE_DEV=true for plain-HTTP cookies)
go build ./app/... && go vet ./app/...
go test -count=1 -p 1 ./app/...                # whole Go suite
go test -count=1 -run TestImportBackupIsAtomic ./app/internal/handlers/   # one test
npx jest --ci                                  # JS suite (tests/js)
npx jest --ci tests/js/logs.test.js            # one JS file
staticcheck ./app/...                          # ST1005 (capitalised error strings) findings are pre-existing
govulncheck ./app/...
```

- Use `-p 1` on Windows: when several test binaries run at once, httptest-based tests stall about 30s and fail with `Client.Timeout exceeded`. They pass serially.
- CI runs `go test -race`, which needs cgo. Locally, run it in a Linux container with the host module cache. TLS interception on this machine breaks module downloads inside containers, hence `GOPROXY=off`:
  ```bash
  docker run --rm -v "$PWD:/src:ro" -v "$HOME/go/pkg/mod:/go/pkg/mod:ro" -w /src \
    -e GOPROXY=off -e GOFLAGS="-buildvcs=false -mod=readonly" golang:1.26.8-bookworm go test -race -count=1 ./app/...
  ```
- CI rejects unformatted Go. `.gitattributes` forces LF; a checkout made before it existed may still have CRLF files, which `gofmt -l` reports even though the committed blobs are clean.
- To run or visually check the app in the local Docker stack, use the `run-servicarr` project skill (`.claude/skills/run-servicarr`).
- Versioning: `app/internal/buildinfo/VERSION`, `package.json` and `package-lock.json` must hold the same version (CI checks). Each successful CI run on `main` publishes a release and `ghcr.io/jekaqm/servicarr` image.

## Architecture

**Startup** (`app/main.go`): load env config, `database.Init` (runs `EnsureSchema` migrations), `stats.EnsureStatsSchema`, build the auth manager from `app_settings` (or a placeholder until the setup wizard completes), start the UPS and CrowdSec monitors and the check scheduler, build the asset bundles, serve. SIGTERM drains requests, flushes queued notifications (`alerts.WaitForDeliveries`) and closes the DB. In Docker the binary runs as PID 1 via `setpriv` (`deploy/entrypoint.sh`) so it receives SIGTERM.

**Request pipeline** (`handlers/routes.go`): everything is wrapped by `security.SecureHeaders` (CSP, 70 MB body cap, rejects cross-origin mutations). Under it:
- Setup routes work only until setup completes. `SetupRequiredMiddleware` redirects everything else to `/setup` until then.
- Public routes go through `api` with `ratelimit.APILimiter`.
- Admin routes (`/api/admin/*`) go through `authAPI`. Each handler is wrapped in `authMgr.RequireAuth` (HMAC session cookie, plus double-submit CSRF on non-GET), and the whole mux sits behind `AuditAdminActions` and `invalidatePublicCacheOnMutation`.
- `/api/admin/services/test` is deliberately reachable without auth before setup completes.

**Monitoring loop**: the scheduler checks due services concurrently (`checker.CheckAll`, max 8), then records results sequentially in display order. That order matters: `monitor.FailureTracker` confirms an outage after 2 consecutive failures, and `alerts.Manager.CheckAndSendAlerts` suppresses alerts when a `depends_on` upstream is down. Alert state lives in `service_status_history`, and deliveries go through per-channel serial queues. Active maintenance windows (`maintenance.MonitoringSuppressed`) skip checks, samples, incidents and alerts. "Degraded" always means `models.IsDegraded` (latency > `DegradedLatencyMS`).

**Public endpoints are shared, not per-visitor**: every dashboard polls `/api/check`, which runs *live* checks, plus `/api/metrics` every 15s. Results are computed once per TTL via `cache.PublicCache.GetOrLoad` (single-flight) and cleared on any admin or setup mutation. New public endpoints doing real work should follow the same pattern.

**Database rules** (`app/internal/database`):
- The pool has exactly one connection (`MaxOpenConns(1)`). Inside a transaction, never call a helper that uses `database.DB`: it waits forever for the connection the transaction holds. Use the `...With(q database.Querier, ...)` variants, as `handlers/backup_import.go` does, and don't issue queries while a `*sql.Rows` is still open.
- Schema changes go in `EnsureSchema` (`schema.go`): `CREATE ... IF NOT EXISTS` for new tables and `addMissingColumns` for new columns. There are no numbered migrations; `SchemaVersion` is informational. Stats tables (`stat_*`, `heartbeats`) are created by `stats.EnsureStatsSchema`, which handler tests must call before touching them.
- `samples.taken_at` is UTC RFC3339 text and range queries compare it as text, so normalise any other format before inserting. `system_logs.timestamp` uses SQLite `datetime('now')`.
- Retention: samples 400 days, minutely/hourly/daily stats and heartbeats are pruned in `stats/aggregation.go`, and logs follow `database.DefaultLogRetention` (check logs capped separately so they cannot evict audit/security history).
- Backup export/import (`settings_backup.go`, `backup_import.go`) excludes secrets and runs import in one transaction. Persisting a new setting means updating export, import and `HandleResetDatabase`'s table list.

**Secrets**: `app_settings.auth_secret` keys session HMACs (which also cover the password hash, so changing the password revokes sessions) and, via SHA-256, the AES-GCM key that encrypts service API tokens and CrowdSec credentials at rest (`crypto`). Outbound requests to user-supplied targets go through `checker`'s transport, which re-validates the resolved IP at dial time (cloud metadata blocked, LAN allowed) and refuses to forward credentials across origins on redirect.

**Frontend** (`web/`): no build step and no ES modules.
- `handlers/bundle.go` concatenates files *in list order* into four bundles at startup, so a new JS/CSS file must be added there (a missing file is fatal). The admin bundles are served only to authenticated sessions.
- All bundle files share one global scope. Notable homes: `j()` (fetch wrapper with CSRF) is at the bottom of `resources.js`; the main `refresh()` polling loop is in `day-detail.js`; `escapeHtml` (escapes quotes, safe for attributes) is in `utils.js`.
- The CSP forbids inline scripts and handlers: wire events with delegated `data-action` listeners.
- Jest tests evaluate the real source files into jsdom (`tests/js/test-helpers.js` rewrites top-level `const`/`let` to `var`).
- Design tokens and component conventions live in the `servicarr-frontend` project skill.

## Testing conventions

- Handler and database tests call `database.Init(":memory:")` per test. Clear `cache.PublicCache` in setups that hit public endpoints, or a previous test's cached response leaks in.
- Alerts tests wait for `WaitForDeliveries` before replacing the global DB; queued deliveries write logs through it.

## Local, untracked directories

`.worktrees/` (git worktrees with full repo copies), `AI Instructions/` (older architecture notes, may be stale) and `mock-lapi/` (a CrowdSec LAPI mock) are local only. Exclude them from searches. `servicarr-backup*.json`, `*.db` files and `.env` contain real data and are gitignored.
