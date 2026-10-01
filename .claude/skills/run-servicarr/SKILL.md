---
name: run-servicarr
description: Build, run and visually verify Servicarr on the local Docker Compose stack (container "servicarr" on 127.0.0.1:4555). Use when asked to run the app, test a change in the real app, take screenshots of the dashboard or admin tabs, or check startup/shutdown behaviour.
---

# Run Servicarr on the local Docker stack

`deploy/docker-compose.yml` is the owner's test stack, intended for exactly this. Use it rather than a bare `go run` when a change needs checking in the real app.

| Item | Value |
|---|---|
| Container / image | `servicarr` / `servicarr:local` |
| Data volume | `servicarr_data` (SQLite at `/data/uptime.db`) |
| URL | http://127.0.0.1:4555 (bound to localhost only) |
| Env | repo-root `.env` (compose `env_file`) |

## 1. Make sure Docker is up

```bash
docker info --format '{{.ServerVersion}}' || powershell -Command 'Start-Process "C:\Program Files\Docker\Docker\Docker Desktop.exe"'
for i in $(seq 1 60); do docker info >/dev/null 2>&1 && break; sleep 3; done
```

## 2. Before deploying a branch: keep a way back

Follow the owner's convention (`servicarr_pre_<label>_<yyyymmdd>` volumes). Stop the container first so the SQLite copy is consistent.

```bash
LABEL=mychange; DAY=$(date +%Y%m%d)
docker tag servicarr:local servicarr:pre-$LABEL
docker compose -f deploy/docker-compose.yml stop
MSYS_NO_PATHCONV=1 docker run --rm --entrypoint sh -v servicarr_data:/from:ro -v servicarr_pre_${LABEL}_$DAY:/to \
  servicarr:pre-$LABEL -c 'cp -a /from/. /to/'
```

## 3. Build and start

```bash
SERVICARR_COMMIT=$(git rev-parse --short HEAD) docker compose -f deploy/docker-compose.yml up -d --build
for i in $(seq 1 30); do curl -sf http://127.0.0.1:4555/healthz >/dev/null && break; sleep 1; done
docker logs servicarr 2>&1 | grep starting      # must show the commit you just built
```

The image build runs `go mod download`. On this machine TLS interception breaks module downloads inside containers, so builds rely on BuildKit's module cache. If you changed `go.mod` and the build fails with `x509: certificate signed by unknown authority`, report it rather than weakening TLS.

## 4. Smoke test

```bash
for p in healthz api/check "api/metrics?days=30"; do
  MSYS_NO_PATHCONV=1 curl -s -o /dev/null -w "$p -> %{http_code} %{time_total}s\n" "http://127.0.0.1:4555/$p"
done
```

Graceful shutdown (the app must be PID 1 and exit 0):

```bash
docker stop servicarr; docker inspect servicarr --format 'exit={{.State.ExitCode}}'   # expect exit=0
docker logs --tail 3 servicarr     # expect "Shutting down server..." then "Server stopped gracefully"
docker start servicarr
```

## 5. Drive the UI and take screenshots

`shoot.js` (next to this file) uses `puppeteer-core` with the installed Chrome. It captures the public page (desktop and mobile), then logs in once and captures every admin tab. Console errors and HTTP responses >= 400 are listed in the JSON it prints, and a run is only clean when that list is empty.

```bash
DRV="$TMPDIR/servicarr-browser"; mkdir -p "$DRV" && cp .claude/skills/run-servicarr/shoot.js "$DRV/"
(cd "$DRV" && [ -d node_modules/puppeteer-core ] || npm install --no-audit --no-fund puppeteer-core@24)
SVC_USER=<user> SVC_PASS=<password> node "$DRV/shoot.js" "$DRV/shots" --admin
```

Admin credentials are whatever the setup wizard was given, not `AUTH_*` from `.env`. **Ask the user for the test account; never write it into the repository (it is public).** Then open the PNGs and look at them: a blank or half-rendered page is a failure.

## 6. Roll back

```bash
docker tag servicarr:pre-$LABEL servicarr:local
docker compose -f deploy/docker-compose.yml up -d            # no --build
# data too, if needed (stop first):
MSYS_NO_PATHCONV=1 docker run --rm --entrypoint sh -v servicarr_pre_${LABEL}_$DAY:/from:ro -v servicarr_data:/to \
  servicarr:local -c 'rm -rf /to/* && cp -a /from/. /to/'
```

## Gotchas

- With `TRUSTED_PROXIES` empty, every local client (your browser, scripts, curl) reaches the app from the Docker gateway IP and shares one rate-limit bucket (public API 120/min, login 10/min). Rapid scripted reloads can hit 429.
- Three failed logins block that shared IP for 24 hours. Log in once per run with known-good credentials.
- The scheduler, UPS monitor and CrowdSec poller run against whatever is configured in the volume's database. Don't point experiments at production integrations.
