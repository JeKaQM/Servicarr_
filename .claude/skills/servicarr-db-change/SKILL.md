---
name: servicarr-db-change
description: Checklist for changing Servicarr's SQLite data layer safely - new tables or columns, queries on large tables, indexes, retention, transactions, and backup export/import compatibility. Use before editing app/internal/database, app/internal/stats, schema.go, or anything that writes several rows at once.
---

# Changing Servicarr's data layer

SQLite runs with **one pooled connection** (`MaxOpenConns(1)`), WAL and `busy_timeout=5000`. Most data-layer bugs in this repo come from forgetting that.

## Hard rules

1. **No `database.DB` inside a transaction.** The transaction owns the only connection, so any helper that uses `DB` blocks forever. Thread the transaction through `database.Querier`, adding a `FooWith(q Querier, ...)` variant and keeping `Foo(...)` as a wrapper (see `CreateServiceWith`, `SaveAlertConfigWith`).
2. **Close rows before the next query.** Collect results into a slice, close the rows, then write. The aggregation code in `stats/aggregation.go` does this for the same reason.
3. **Multi-statement writes are transactional.** A delete-then-reinsert or "clear table" path must use one transaction and return errors, never `_, _ = DB.Exec`. Model new ones on `importBackup` and `deleteAllRows`.
4. **Big deletes run in batches** (`DELETE ... WHERE id IN (SELECT id ... LIMIT n)` in a loop, as in `PruneSamples`) so the scheduler can write between batches.

## Schema changes

- New table: add `CREATE TABLE IF NOT EXISTS` in `EnsureSchema` (`database/schema.go`). Statistics tables belong in `stats.EnsureStatsSchema`.
- New column: add a `columnSpec` to the `addMissingColumns` list. It checks `pragma_table_info` and reports real failures. Never add another error-ignoring `ALTER TABLE`.
- Order matters: a column must exist before any migration that copies the table (see the CrowdSec archive rebuild).
- Bump `SchemaVersion` when the persistent layout changes; backups record it and refuse newer schemas.
- Timestamps compared as text must share one format: `samples.taken_at` and `heartbeats.time` are UTC RFC3339 (`time.RFC3339`), `system_logs.timestamp` is SQLite `datetime('now')`. Normalise anything imported.

## Queries and indexes

- Check the plan on realistic data before and after: `EXPLAIN QUERY PLAN SELECT ...`. A `SCAN` of `samples`, `heartbeats` or `crowdsec_alerts` on a polled path is a bug.
- Per-service range queries use the composite indexes `samples(service_key, taken_at)` and `heartbeats(service_key, time)`. Prefer extending a composite index to adding a parallel single-column one.
- Anonymous-facing endpoints run on every dashboard poll. Serve them through `cache.PublicCache.GetOrLoad` rather than querying per request.

## Retention

Every append-only table needs a bound. Current policy: samples 400 days (longer than the 365-day maximum dashboard window), `stat_minutely` 1 day, `stat_hourly` 30 days, `stat_daily` 365 days, heartbeats 1 day (important ones 7 days), logs per `database.DefaultLogRetention`, CrowdSec alerts capped in `database/crowdsec.go`. A new table must get its pruning in the same change.

## Backup compatibility

`HandleExportDatabase` / `importBackup` define the portable backup format.
- New user-facing configuration must be added to both, with tests in `system_info_test.go` or `backup_import_test.go` that round-trip it.
- Secrets never go into exports.
- Every new table also goes into `HandleResetDatabase`'s list.

## Prove it on real data

Unit tests use `:memory:`. Also run the migration against copies of real databases: the repo root's local `*.db` files, and the Docker volume (`docker cp servicarr:/data/uptime.db` plus `-wal`). Copy them into a temp directory and use a throwaway env-gated test you delete afterwards:

```go
func TestZZMigrateRealCopy(t *testing.T) {
	path := os.Getenv("SERVICARR_MIGRATION_DB")
	if path == "" {
		t.Skip()
	}
	if err := Init(path); err != nil { // runs EnsureSchema on the copy
		t.Fatal(err)
	}
}
```

Never point it at the live file. Finally, run the suite with the race detector (command in CLAUDE.md), because data-layer code is shared by the scheduler, the monitors and the HTTP handlers.
