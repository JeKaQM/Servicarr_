# Deployment Guide

## Quick Start

1. Configure your `.env` file in the parent directory
2. Run: `docker-compose up -d`

## Updating to Latest Version

Run the update script:
```powershell
.\update.ps1
```

Or manually:
```powershell
docker-compose pull
docker-compose down
docker-compose up -d
```

## Data Persistence

All data is stored in the Docker volume `statusapp_data`:
- Database: `/data/uptime.db` (uptime samples, history, alerts)
- IP blocks for security
- Service state (enabled/disabled)

**Important:** The volume persists even when you:
- Update to a new image version
- Run `docker-compose down`
- Rebuild the container

The volume is only deleted if you explicitly run:
```powershell
docker-compose down -v  # DO NOT run this unless you want to delete all data!
```

## Backup Your Data

To backup the database:
```powershell
docker exec statusapp cp /data/uptime.db /data/uptime-backup.db
```

To copy it to your host:
```powershell
docker cp statusapp:/data/uptime.db ./uptime-backup.db
```

## Restore Data

To restore from backup:
```powershell
docker cp ./uptime-backup.db statusapp:/data/uptime.db
docker-compose restart
```

## View Logs

```powershell
docker-compose logs -f
```

## Access

- Local: http://localhost:4555
- Network: http://YOUR_IP:4555

Default credentials (change in `.env`):
- Username: admin
- Password: admin123
