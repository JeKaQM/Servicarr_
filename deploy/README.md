# Deployment Guide

Run the commands in this guide from the repository root.

## First deployment

1. Create the environment file expected by Compose:

   ```powershell
   Copy-Item .env.example .env
   ```

   Review the placeholders before starting the app. New installations can set their admin credentials in the setup wizard; environment-based installations use `AUTH_USER`, `AUTH_PASSWORD` (or `AUTH_PASSWORD_BCRYPT`) and an `AUTH_SECRET` of at least 32 characters.

2. Build and start Servicarr:

   ```powershell
   docker compose -f deploy/docker-compose.yml up -d --build
   ```

3. Verify the container and follow its startup log:

   ```powershell
   docker compose -f deploy/docker-compose.yml ps
   docker compose -f deploy/docker-compose.yml logs -f servicarr
   ```

4. Open <http://localhost:4555>. The supplied Compose file publishes port 4555 on `127.0.0.1` only. Keep that binding when a reverse proxy runs on the same host. Deliberately change the port mapping and configure a firewall if direct LAN access is required.

## Updating

Pull the repository changes, rebuild the local image, and let Compose replace the application container:

```powershell
git pull --ff-only
docker compose -f deploy/docker-compose.yml up -d --build --remove-orphans
docker compose -f deploy/docker-compose.yml ps
```

The named data volume is not removed by these commands.

## Data persistence

Application data is stored in the Docker volume `servicarr_data`; the SQLite database is `/data/uptime.db` inside the `servicarr` container. The volume survives container rebuilds, replacements and `docker compose down`.

Do not run the following command unless you intend to delete the stored application data:

```powershell
docker compose -f deploy/docker-compose.yml down -v
```

## Backup and restore

For a portable application backup, use **Admin > Settings > Export**. CrowdSec and other integration secrets are intentionally excluded and must be re-entered after an import.

For a raw SQLite backup, stop writes before copying the database:

```powershell
docker compose -f deploy/docker-compose.yml stop servicarr
docker cp servicarr:/data/uptime.db ./uptime-backup.db
docker compose -f deploy/docker-compose.yml start servicarr
```

To restore that raw database:

```powershell
docker compose -f deploy/docker-compose.yml stop servicarr
docker cp ./uptime-backup.db servicarr:/data/uptime.db
docker compose -f deploy/docker-compose.yml start servicarr
```

Confirm the application is healthy after a restore:

```powershell
docker compose -f deploy/docker-compose.yml ps
docker compose -f deploy/docker-compose.yml logs --tail 200 servicarr
```

## CrowdSec networking

The CrowdSec LAPI URL is contacted from inside the Servicarr container. `localhost` and `127.0.0.1` therefore refer to Servicarr itself, not the Docker host. Use a CrowdSec service name on a shared Docker network or a host/LAN address reachable by the container. Ensure CrowdSec's `api.server.listen_uri` is bound to that interface and allow TCP port 8080 only from the required Docker/host network.

Use HTTPS if LAPI traffic crosses an untrusted network. The **Skip TLS verification** option is intended only for a self-signed certificate on a trusted network.

The dashboard's detection charts and geography map cover Servicarr's rolling 24-hour local mirror (up to 2000 stored alerts, with at most the newest 100 requested from LAPI per server-side poll), not an all-time LAPI ledger. Future-dated alerts are excluded. The browser reads a compact projection of the complete retained mirror so map and headline totals use the same scope without transferring unused alert text every refresh. Optional server coordinates only control map destination arcs; leaving them empty keeps the destination hidden. Source locations are approximate IP geolocation. Disabling the integration pauses server-side polling; existing mirrored data remains visible and is marked as cached.
