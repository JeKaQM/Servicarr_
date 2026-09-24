package database

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"status/app/internal/crypto"
	"status/app/internal/models"
)

// CrowdSecMaxSnapshotRows bounds the complete paged decision snapshot.
const CrowdSecMaxSnapshotRows = 100000

const CrowdSecMaxDecisionResponseRows = 2000

// CrowdSecMaxAlertRows caps a single alerts API response/map render.
const CrowdSecMaxAlertRows = 2000

// The archive is bounded independently of the rendered feed. All-time means
// all retained rows, not unlimited local disk growth.
const CrowdSecMaxArchivedAlerts = 100000
const CrowdSecArchiveMaxAge = 365 * 24 * time.Hour

// CrowdSecSourceID is stable for a LAPI URL + machine identity, without
// incorporating a credential. It prevents alert-ID reuse across connections.
func CrowdSecSourceID(lapiURL, machineID string) string {
	if strings.TrimSpace(lapiURL) == "" && strings.TrimSpace(machineID) == "" {
		return "legacy"
	}
	sum := sha256.Sum256([]byte(strings.TrimRight(strings.TrimSpace(lapiURL), "/") + "\x00" + strings.TrimSpace(machineID)))
	return hex.EncodeToString(sum[:16])
}

func CurrentCrowdSecSourceID() (string, error) {
	var url, machineID string
	err := DB.QueryRow(`SELECT lapi_url, lapi_machine_id FROM crowdsec_config WHERE id = 1`).Scan(&url, &machineID)
	if err == sql.ErrNoRows {
		return "legacy", nil
	}
	if err != nil {
		return "", err
	}
	return CrowdSecSourceID(url, machineID), nil
}

// LoadCrowdSecConfig loads CrowdSec LAPI configuration. Returns (nil, nil)
// when no row exists (first run). A decryption failure is returned so a
// settings save cannot silently replace an unreadable stored secret.
func LoadCrowdSecConfig() (*models.CrowdSecConfig, error) {
	var config models.CrowdSecConfig
	var enabled, skipVerify int
	var encPassword, encKey string
	err := DB.QueryRow(`SELECT enabled, COALESCE(lapi_url, ''), COALESCE(lapi_machine_id, ''),
		COALESCE(lapi_machine_password, ''), COALESCE(bouncer_api_key, ''), poll_interval, tls_skip_verify,
		COALESCE(map_home_lat, 0), COALESCE(map_home_lng, 0)
		FROM crowdsec_config WHERE id = 1`).Scan(
		&enabled, &config.LAPIURL, &config.MachineID,
		&encPassword, &encKey, &config.PollIntervalS, &skipVerify,
		&config.MapHomeLat, &config.MapHomeLng)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	config.Enabled = enabled != 0
	config.TLSSkipVerify = skipVerify != 0
	config.MachinePassword, err = decryptCrowdSecSecret("machine password", encPassword)
	if err != nil {
		return nil, err
	}
	config.BouncerAPIKey, err = decryptCrowdSecSecret("bouncer API key", encKey)
	if err != nil {
		return nil, err
	}
	return &config, nil
}

func decryptCrowdSecSecret(name, stored string) (string, error) {
	if stored == "" {
		return "", nil
	}
	plain, err := crypto.Decrypt(stored)
	if err != nil {
		return "", fmt.Errorf("crowdsec: decrypt %s: %w", name, err)
	}
	return plain, nil
}

// SaveCrowdSecConfig saves CrowdSec LAPI configuration (upsert singleton).
// Secrets must be encrypted before persisting; a failed encryption fails
// the save rather than storing plaintext (LAPI credentials are high-privilege).
func SaveCrowdSecConfig(config *models.CrowdSecConfig) error {
	config.LAPIURL = strings.TrimSpace(config.LAPIURL)
	config.MachineID = strings.TrimSpace(config.MachineID)

	encPassword, err := crypto.Encrypt(config.MachinePassword)
	if err != nil {
		return fmt.Errorf("crowdsec: machine password not encrypted: %w", err)
	}
	encKey, err := crypto.Encrypt(config.BouncerAPIKey)
	if err != nil {
		return fmt.Errorf("crowdsec: bouncer API key not encrypted: %w", err)
	}

	enabled := boolInt(config.Enabled)
	skipVerify := boolInt(config.TLSSkipVerify)
	_, err = DB.Exec(`INSERT INTO crowdsec_config
		(id, enabled, lapi_url, lapi_machine_id, lapi_machine_password, bouncer_api_key, poll_interval, tls_skip_verify, map_home_lat, map_home_lng, updated_at)
		VALUES (1, ?, ?, ?, ?, ?, ?, ?, ?, ?, datetime('now'))
		ON CONFLICT(id) DO UPDATE SET
			enabled=?, lapi_url=?, lapi_machine_id=?, lapi_machine_password=?, bouncer_api_key=?,
			poll_interval=?, tls_skip_verify=?, map_home_lat=excluded.map_home_lat,
			map_home_lng=excluded.map_home_lng, updated_at=datetime('now')`,
		enabled, config.LAPIURL, config.MachineID, encPassword, encKey, config.PollIntervalS, skipVerify, config.MapHomeLat, config.MapHomeLng,
		enabled, config.LAPIURL, config.MachineID, encPassword, encKey, config.PollIntervalS, skipVerify)
	return err
}

// CrowdSecState is the poller-owned sync state singleton.
type CrowdSecState struct {
	LastSync      time.Time // zero = never synced successfully
	LastError     string
	AuthFailed    bool
	DecisionCount int
}

// GetCrowdSecState loads the sync state singleton. Returns (nil, nil) when
// no row exists yet.
func GetCrowdSecState() (*CrowdSecState, error) {
	var state CrowdSecState
	var lastSync sql.NullString
	var authFailed int
	err := DB.QueryRow(`SELECT last_sync, last_error, auth_failed, decision_count
		FROM crowdsec_state WHERE id = 1`).Scan(&lastSync, &state.LastError, &authFailed, &state.DecisionCount)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	state.AuthFailed = authFailed != 0
	if lastSync.Valid && lastSync.String != "" {
		parsed, err := time.Parse(time.RFC3339Nano, lastSync.String)
		if err != nil {
			return nil, err
		}
		state.LastSync = parsed
	}
	return &state, nil
}

// saveCrowdSecSyncTx advances sync state. last_sync means "last SUCCESSFUL
// sync" and is never touched by error paths.
func saveCrowdSecSyncTx(tx *sql.Tx, lastSync time.Time, totalCount int) error {
	_, err := tx.Exec(`INSERT INTO crowdsec_state (id, last_sync, last_error, auth_failed, decision_count, updated_at)
		VALUES (1, ?, '', 0, ?, datetime('now'))
		ON CONFLICT(id) DO UPDATE SET last_sync=excluded.last_sync,
			last_error='', auth_failed=0, decision_count=excluded.decision_count, updated_at=datetime('now')`,
		lastSync.UTC().Format(time.RFC3339), totalCount)
	return err
}

// SaveCrowdSecSyncError records a failed sync. The conditional upsert avoids
// rewriting the same persistent error on every poll cycle.
func SaveCrowdSecSyncError(errText string, authFailed bool) error {
	_, err := DB.Exec(`INSERT INTO crowdsec_state (id, last_sync, last_error, auth_failed, decision_count, updated_at)
		VALUES (1, NULL, ?, ?, 0, datetime('now'))
		ON CONFLICT(id) DO UPDATE SET last_error=excluded.last_error,
			auth_failed=excluded.auth_failed, updated_at=datetime('now')
		WHERE crowdsec_state.last_error IS NOT excluded.last_error
			OR crowdsec_state.auth_failed != excluded.auth_failed`,
		errText, boolInt(authFailed))
	return err
}

// SyncCrowdSecDecisions converges the snapshot table to remote (the current
// active set, already capped by the caller). Read-first diff, then one
// short transaction — steady state performs ZERO decision writes, so WAL
// churn is proportional to actual decision churn, not poll frequency.
// Network I/O must complete BEFORE this runs: the single connection pool
// means an open transaction stalls every handler in the app.
// Returns the number of changed rows.
func SyncCrowdSecDecisions(remote []models.CrowdSecDecision, totalCount int, syncedAt time.Time) (int, error) {
	rows, err := DB.Query(`SELECT decision_id, expires_at FROM crowdsec_decisions`)
	if err != nil {
		return 0, err
	}
	local := make(map[string]string, 512) // decision_id -> expires_at
	for rows.Next() {
		var id, expires string
		if err := rows.Scan(&id, &expires); err != nil {
			rows.Close()
			return 0, err
		}
		local[id] = expires
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return 0, err
	}

	remoteIDs := make(map[string]struct{}, len(remote))
	var upserts []models.CrowdSecDecision
	for _, d := range remote {
		remoteIDs[d.DecisionID] = struct{}{}
		if exp, ok := local[d.DecisionID]; !ok || exp != d.ExpiresAt {
			// New decision, or its expiry changed (LAPI extended it).
			upserts = append(upserts, d)
		}
	}
	var removed []string
	for id := range local {
		if _, ok := remoteIDs[id]; !ok {
			removed = append(removed, id)
		}
	}

	if len(upserts) == 0 && len(removed) == 0 {
		// Zero-write fast path: only advance sync state.
		tx, err := DB.Begin()
		if err != nil {
			return 0, err
		}
		defer tx.Rollback()
		if err := saveCrowdSecSyncTx(tx, syncedAt, totalCount); err != nil {
			return 0, err
		}
		return 0, tx.Commit()
	}

	tx, err := DB.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	if len(removed) > 0 {
		del, err := tx.Prepare(`DELETE FROM crowdsec_decisions WHERE decision_id = ?`)
		if err != nil {
			return 0, err
		}
		defer del.Close()
		for _, id := range removed {
			if _, err := del.Exec(id); err != nil {
				return 0, err
			}
		}
	}
	if len(upserts) > 0 {
		up, err := tx.Prepare(`INSERT INTO crowdsec_decisions
			(decision_id, value, type, scope, origin, scenario, duration, simulated, created_at, expires_at, synced_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT(decision_id) DO UPDATE SET value=excluded.value, type=excluded.type,
				scope=excluded.scope, origin=excluded.origin, scenario=excluded.scenario,
				duration=excluded.duration, simulated=excluded.simulated,
				expires_at=excluded.expires_at, synced_at=excluded.synced_at`)
		if err != nil {
			return 0, err
		}
		defer up.Close()
		for _, d := range upserts {
			if _, err := up.Exec(d.DecisionID, d.Value, d.Type, d.Scope, d.Origin, d.Scenario,
				d.Duration, boolInt(d.Simulated), d.CreatedAt, d.ExpiresAt, d.SyncedAt); err != nil {
				return 0, err
			}
		}
	}

	// Belt-and-braces cap (self-heals fetch-cap regressions).
	if _, err := tx.Exec(`DELETE FROM crowdsec_decisions WHERE decision_id NOT IN (
		SELECT decision_id FROM crowdsec_decisions ORDER BY created_at DESC, decision_id ASC LIMIT ?)`,
		CrowdSecMaxSnapshotRows); err != nil {
		return 0, err
	}

	if err := saveCrowdSecSyncTx(tx, syncedAt, totalCount); err != nil {
		return 0, err
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return len(upserts) + len(removed), nil
}

// GetCrowdSecDecisions returns snapshot rows, newest first, optionally
// filtered to still-valid decisions. Expiry compares RFC3339 strings via
// parameter — never datetime('now'), which is not lexicographically
// comparable with RFC3339.
func GetCrowdSecDecisions(activeOnly bool, limit int) ([]models.CrowdSecDecision, error) {
	if limit <= 0 || limit > CrowdSecMaxDecisionResponseRows {
		limit = CrowdSecMaxDecisionResponseRows
	}
	query := `SELECT decision_id, value, type, scope, origin, scenario, duration, simulated, created_at, expires_at, synced_at
		FROM crowdsec_decisions`
	args := []any{}
	if activeOnly {
		query += ` WHERE expires_at > ?`
		args = append(args, time.Now().UTC().Format(time.RFC3339))
	}
	query += ` ORDER BY created_at DESC, decision_id ASC LIMIT ?`
	args = append(args, limit)

	rows, err := DB.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []models.CrowdSecDecision{} // non-nil so handlers marshal [] not null
	for rows.Next() {
		var d models.CrowdSecDecision
		var simulated int
		if err := rows.Scan(&d.DecisionID, &d.Value, &d.Type, &d.Scope, &d.Origin, &d.Scenario,
			&d.Duration, &simulated, &d.CreatedAt, &d.ExpiresAt, &d.SyncedAt); err != nil {
			return nil, err
		}
		d.Simulated = simulated != 0
		out = append(out, d)
	}
	return out, rows.Err()
}

// GetCrowdSecSnapshotCount returns the number of stored snapshot rows.
func GetCrowdSecSnapshotCount() (int, error) {
	var n int
	err := DB.QueryRow(`SELECT COUNT(*) FROM crowdsec_decisions`).Scan(&n)
	return n, err
}

func CountCrowdSecDecisions(activeOnly bool) (int, error) {
	query := `SELECT COUNT(*) FROM crowdsec_decisions`
	args := []any{}
	if activeOnly {
		query += ` WHERE expires_at > ?`
		args = append(args, time.Now().UTC().Format(time.RFC3339))
	}
	var count int
	err := DB.QueryRow(query, args...).Scan(&count)
	return count, err
}

// SyncCrowdSecAlerts merges fetched alerts into the snapshot table.
// Append-only: new alert IDs are inserted, existing IDs are skipped (alerts
// are immutable events in LAPI — no updates). Runs in one short transaction
// with the archive age/row prune so the table stays bounded. Network I/O must
// complete BEFORE this runs (MaxOpenConns(1)).
func SyncCrowdSecAlerts(alerts []models.CrowdSecAlert, syncedAt time.Time) (int, error) {
	sourceID, err := CurrentCrowdSecSourceID()
	if err != nil {
		return 0, err
	}
	return SyncCrowdSecAlertsForSource(alerts, syncedAt, sourceID)
}

func SyncCrowdSecAlertsForSource(alerts []models.CrowdSecAlert, syncedAt time.Time, sourceID string) (int, error) {
	syncedAt = syncedAt.UTC()
	cutoffTime := syncedAt.Add(-CrowdSecArchiveMaxAge)
	tx, err := DB.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`INSERT INTO crowdsec_history_state (source_id) VALUES (?)
		ON CONFLICT(source_id) DO NOTHING`, sourceID); err != nil {
		return 0, err
	}

	ins, err := tx.Prepare(`INSERT INTO crowdsec_alerts
		(source_id, alert_id, scenario, message, source_value, country, as_number, as_name,
		 latitude, longitude, events_count, start_at, created_at, has_decision, simulated, synced_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(source_id, alert_id) DO NOTHING`)
	if err != nil {
		return 0, err
	}
	defer ins.Close()

	synced := syncedAt.Format(time.RFC3339Nano)
	inserted := 0
	for _, a := range alerts {
		if a.AlertID == "" {
			continue
		}
		createdAt, err := time.Parse(time.RFC3339Nano, a.CreatedAt)
		if err != nil || !createdAt.After(cutoffTime) || createdAt.After(syncedAt) {
			continue
		}
		res, err := ins.Exec(sourceID, a.AlertID, a.Scenario, a.Message, a.SourceValue, a.Country,
			a.ASNumber, a.ASName, nullableFloat(a.Latitude), nullableFloat(a.Longitude),
			a.EventsCount, a.StartAt, createdAt.UTC().Format(time.RFC3339Nano),
			boolInt(a.HasDecision), boolInt(a.Simulated), synced)
		if err != nil {
			return 0, err
		}
		if n, _ := res.RowsAffected(); n > 0 {
			inserted++
		}
	}

	// Prune records outside the archive window as well as excess rows. This
	// also runs for an empty response so the archive remains bounded.
	cutoff := cutoffTime.Format(time.RFC3339Nano)
	if _, err := tx.Exec(`UPDATE crowdsec_history_state SET truncated = 1, complete = 0
		WHERE source_id IN (SELECT DISTINCT source_id FROM crowdsec_alerts
			WHERE julianday(created_at) <= julianday(?))`, cutoff); err != nil {
		return 0, err
	}
	if _, err := tx.Exec(`DELETE FROM crowdsec_alerts WHERE julianday(created_at) IS NULL
		OR julianday(created_at) <= julianday(?) OR julianday(created_at) > julianday(?)`,
		cutoff, synced); err != nil {
		return 0, err
	}
	var archiveRows int64
	if err := tx.QueryRow(`SELECT COUNT(*) FROM crowdsec_alerts`).Scan(&archiveRows); err != nil {
		return 0, err
	}
	if archiveRows > CrowdSecMaxArchivedAlerts {
		// Global cap may remove rows from several configured sources. Mark all
		// known sources partial so switching back cannot claim full coverage.
		if _, err := tx.Exec(`UPDATE crowdsec_history_state SET truncated = 1, complete = 0`); err != nil {
			return 0, err
		}
		if _, err := tx.Exec(`DELETE FROM crowdsec_alerts WHERE (source_id, alert_id) NOT IN (
			SELECT source_id, alert_id FROM crowdsec_alerts
			ORDER BY julianday(created_at) DESC, source_id ASC, alert_id ASC LIMIT ?)`,
			CrowdSecMaxArchivedAlerts); err != nil {
			return 0, err
		}
	}

	return inserted, tx.Commit()
}

// ClearCrowdSecAlerts explicitly removes the local archive.
// Removing machine credentials must not call this destructive operation.
func ClearCrowdSecAlerts() (int64, error) {
	result, err := DB.Exec(`DELETE FROM crowdsec_alerts`)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

// CrowdSecHistoryState tracks which absolute start-time boundary has been
// completely inspected in LAPI. It is separate from the observed oldest row:
// a gap in alerts must not be mistaken for the end of history.
type CrowdSecHistoryState struct {
	CursorAt          string
	LastLiveAt        string
	Complete          bool
	Truncated         bool
	DecisionTruncated bool
	BackfillSeconds   int64
	LiveSeconds       int64
}

func GetCrowdSecHistoryState() (CrowdSecHistoryState, error) {
	sourceID, err := CurrentCrowdSecSourceID()
	if err != nil {
		return CrowdSecHistoryState{}, err
	}
	return GetCrowdSecHistoryStateForSource(sourceID)
}

func GetCrowdSecHistoryStateForSource(sourceID string) (CrowdSecHistoryState, error) {
	var state CrowdSecHistoryState
	var complete, truncated, decisionTruncated int
	err := DB.QueryRow(`SELECT cursor_at, last_live_at, complete, truncated, decision_truncated, backfill_seconds, live_seconds
		FROM crowdsec_history_state WHERE source_id = ?`, sourceID).Scan(
		&state.CursorAt, &state.LastLiveAt, &complete, &truncated, &decisionTruncated, &state.BackfillSeconds, &state.LiveSeconds)
	if err == sql.ErrNoRows {
		return state, nil
	}
	state.Complete = complete != 0
	state.Truncated = truncated != 0
	state.DecisionTruncated = decisionTruncated != 0
	return state, err
}

func SaveCrowdSecHistoryState(state CrowdSecHistoryState) error {
	sourceID, err := CurrentCrowdSecSourceID()
	if err != nil {
		return err
	}
	return SaveCrowdSecHistoryStateForSource(sourceID, state)
}

func SaveCrowdSecHistoryStateForSource(sourceID string, state CrowdSecHistoryState) error {
	_, err := DB.Exec(`INSERT INTO crowdsec_history_state
		(source_id, cursor_at, last_live_at, complete, truncated, decision_truncated, backfill_seconds, live_seconds)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(source_id) DO UPDATE SET cursor_at = excluded.cursor_at,
		last_live_at = excluded.last_live_at, complete = excluded.complete,
		truncated = MAX(crowdsec_history_state.truncated, excluded.truncated), decision_truncated = excluded.decision_truncated,
		backfill_seconds = excluded.backfill_seconds, live_seconds = excluded.live_seconds`,
		sourceID, state.CursorAt, state.LastLiveAt,
		boolInt(state.Complete), boolInt(state.Truncated), boolInt(state.DecisionTruncated), state.BackfillSeconds, state.LiveSeconds)
	return err
}

// CrowdSecHistoryCoverage describes the local archive, not LAPI's unknown
// pre-retention lifetime. Complete means backfill inspected the retained
// time window; Truncated separately records retention or saturated slices.
type CrowdSecHistoryCoverage struct {
	Oldest            string
	Complete          bool
	Truncated         bool
	DecisionTruncated bool
}

func GetCrowdSecHistoryCoverage() (CrowdSecHistoryCoverage, error) {
	state, err := GetCrowdSecHistoryState()
	if err != nil {
		return CrowdSecHistoryCoverage{}, err
	}
	sourceID, err := CurrentCrowdSecSourceID()
	if err != nil {
		return CrowdSecHistoryCoverage{}, err
	}
	coverage := CrowdSecHistoryCoverage{Complete: state.Complete, Truncated: state.Truncated,
		DecisionTruncated: state.DecisionTruncated}
	err = DB.QueryRow(`SELECT created_at FROM crowdsec_alerts
		WHERE source_id = ? ORDER BY julianday(created_at) ASC, alert_id ASC LIMIT 1`, sourceID).Scan(&coverage.Oldest)
	if err == sql.ErrNoRows {
		return coverage, nil
	}
	return coverage, err
}

// CrowdSecWindow is a single captured clock boundary shared by one query.
// For all retained data, Start is the oldest row (or End when empty).
type CrowdSecWindow struct {
	Start time.Time
	End   time.Time
	Hours int // zero for all retained data
}

func NewCrowdSecWindow(now time.Time, hours int, all bool, oldest string) CrowdSecWindow {
	now = now.UTC().Truncate(time.Second)
	if all {
		start := now
		if parsed, err := time.Parse(time.RFC3339Nano, oldest); err == nil && parsed.Before(now) {
			// julianday() has millisecond precision; one second keeps the
			// earliest stored row inside the strict lower-bound predicate.
			start = parsed.UTC().Add(-time.Second)
		}
		return CrowdSecWindow{Start: start, End: now}
	}
	if hours < 1 || hours > 8760 {
		hours = 24
	}
	return CrowdSecWindow{Start: now.Add(-time.Duration(hours) * time.Hour), End: now, Hours: hours}
}

// GetCrowdSecAlerts returns the newest alerts from the live 24-hour window.
func GetCrowdSecAlerts(limit int) ([]models.CrowdSecAlert, error) {
	return getCrowdSecAlerts(limit, false, NewCrowdSecWindow(time.Now(), 24, false, ""))
}

// GetCrowdSecCompactAlerts omits message/start_at at the database read so
// dashboard polling does not repeatedly load unused, potentially large text.
func GetCrowdSecCompactAlerts(limit int) ([]models.CrowdSecAlert, error) {
	return getCrowdSecAlerts(limit, true, NewCrowdSecWindow(time.Now(), 24, false, ""))
}

func GetCrowdSecAlertsWindow(limit int, compact bool, window CrowdSecWindow) ([]models.CrowdSecAlert, error) {
	return getCrowdSecAlerts(limit, compact, window)
}

func CountCrowdSecAlertsWindow(window CrowdSecWindow) (int64, error) {
	sourceID, err := CurrentCrowdSecSourceID()
	if err != nil {
		return 0, err
	}
	var count int64
	err = DB.QueryRow(`SELECT COUNT(*) FROM crowdsec_alerts
		WHERE source_id = ? AND julianday(created_at) > julianday(?)
		AND julianday(created_at) <= julianday(?)`,
		sourceID, window.Start.Format(time.RFC3339Nano), window.End.Format(time.RFC3339Nano)).Scan(&count)
	return count, err
}

func getCrowdSecAlerts(limit int, compact bool, window CrowdSecWindow) ([]models.CrowdSecAlert, error) {
	if limit <= 0 || limit > CrowdSecMaxAlertRows {
		limit = 50
	}
	now := window.End
	cutoff := window.Start
	sourceID, err := CurrentCrowdSecSourceID()
	if err != nil {
		return nil, err
	}
	columns := `alert_id, scenario, source_value, country,
		as_number, as_name, latitude, longitude, events_count, created_at, has_decision, simulated`
	if !compact {
		columns = `alert_id, scenario, message, source_value, country,
			as_number, as_name, latitude, longitude, events_count, start_at, created_at, has_decision, simulated`
	}
	rows, err := DB.Query(`SELECT `+columns+`
		FROM crowdsec_alerts WHERE source_id = ? AND julianday(created_at) > julianday(?)
			AND julianday(created_at) <= julianday(?)
		ORDER BY julianday(created_at) DESC, alert_id ASC LIMIT ?`,
		sourceID, cutoff.Format(time.RFC3339Nano), now.Format(time.RFC3339Nano), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []models.CrowdSecAlert{} // non-nil so handlers marshal [] not null
	for rows.Next() {
		var a models.CrowdSecAlert
		var hasDecision, simulated int
		var lat, lng sql.NullFloat64
		if compact {
			if err := rows.Scan(&a.AlertID, &a.Scenario, &a.SourceValue, &a.Country,
				&a.ASNumber, &a.ASName, &lat, &lng, &a.EventsCount, &a.CreatedAt,
				&hasDecision, &simulated); err != nil {
				return nil, err
			}
		} else {
			if err := rows.Scan(&a.AlertID, &a.Scenario, &a.Message, &a.SourceValue, &a.Country,
				&a.ASNumber, &a.ASName, &lat, &lng, &a.EventsCount, &a.StartAt, &a.CreatedAt,
				&hasDecision, &simulated); err != nil {
				return nil, err
			}
		}
		createdAt, err := time.Parse(time.RFC3339Nano, a.CreatedAt)
		if err != nil || !createdAt.After(cutoff) || createdAt.After(now) {
			continue
		}
		if lat.Valid {
			v := lat.Float64
			a.Latitude = &v
		}
		if lng.Valid {
			v := lng.Float64
			a.Longitude = &v
		}
		a.HasDecision = hasDecision != 0
		a.Simulated = simulated != 0
		out = append(out, a)
		if len(out) == limit {
			break
		}
	}
	return out, rows.Err()
}

// nullableFloat converts an optional coordinate to a driver value.
func nullableFloat(v *float64) any {
	if v == nil {
		return nil
	}
	return *v
}

const crowdSecStatsTopRows = 10

type crowdSecLabelCount struct {
	label string
	count int64
}

// GetCrowdSecStats aggregates the bounded local CrowdSec mirrors. The alert
// metrics default to a rolling 24-hour window; active-decision metrics
// describe the current snapshot rather than a historical total.
func GetCrowdSecStats() (*models.CrowdSecStats, error) {
	return getCrowdSecStatsAt(time.Now().UTC())
}

func GetCrowdSecStatsWindow(window CrowdSecWindow) (*models.CrowdSecStats, error) {
	return getCrowdSecStatsWindow(window)
}

// getCrowdSecStatsAt is the deterministic implementation behind
// GetCrowdSecStats. Keeping the clock at the boundary makes rolling-window
// behavior directly testable and ensures every aggregate uses the same now.
func getCrowdSecStatsAt(now time.Time) (*models.CrowdSecStats, error) {
	return getCrowdSecStatsWindow(NewCrowdSecWindow(now, 24, false, ""))
}

func crowdSecBucketSize(span time.Duration) time.Duration {
	switch {
	case span <= 48*time.Hour:
		return time.Hour
	case span <= 60*24*time.Hour:
		return 24 * time.Hour
	case span <= 180*24*time.Hour:
		return 7 * 24 * time.Hour
	default:
		return 30 * 24 * time.Hour
	}
}

func getCrowdSecStatsWindow(window CrowdSecWindow) (*models.CrowdSecStats, error) {
	now := window.End.UTC()
	windowStart := window.Start.UTC()
	bucketSize := crowdSecBucketSize(now.Sub(windowStart))
	bucketCount := int(math.Ceil(float64(now.Sub(windowStart)) / float64(bucketSize)))
	if bucketCount < 1 {
		bucketCount = 1
	}
	coverage, err := GetCrowdSecHistoryCoverage()
	if err != nil {
		return nil, err
	}
	sourceID, err := CurrentCrowdSecSourceID()
	if err != nil {
		return nil, err
	}
	bucketUnit := "hour"
	if bucketSize >= 24*time.Hour {
		bucketUnit = "day"
	}
	stats := &models.CrowdSecStats{
		WindowStart:           windowStart.Format(time.RFC3339),
		WindowEnd:             now.Format(time.RFC3339),
		WindowHours:           window.Hours,
		BucketUnit:            bucketUnit,
		BucketSeconds:         int64(bucketSize / time.Second),
		HistoryOldest:         coverage.Oldest,
		HistoryComplete:       coverage.Complete,
		HistoryTruncated:      coverage.Truncated,
		DecisionTruncated:     coverage.DecisionTruncated,
		Hourly:                make([]models.CrowdSecHourlyCount, bucketCount),
		Countries:             []models.CrowdSecCountryCount{},
		Scenarios:             []models.CrowdSecScenarioCount{},
		Networks:              []models.CrowdSecNetworkCount{},
		Sources:               []models.CrowdSecSourceCount{},
		ActiveDecisionTypes:   []models.CrowdSecDecisionTypeCount{},
		ActiveDecisionOrigins: []models.CrowdSecDecisionOriginCount{},
	}
	for i := range stats.Hourly {
		stats.Hourly[i].Start = windowStart.Add(time.Duration(i) * bucketSize).Format(time.RFC3339)
	}

	tx, err := DB.BeginTx(context.Background(), &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	countries := make(map[string]int64)
	scenarios := make(map[string]int64)
	networks := make(map[string]int64)
	sources := make(map[string]int64)
	uniqueSources := make(map[string]struct{})

	rows, err := tx.Query(`SELECT scenario, source_value, country, as_number, as_name,
		latitude, longitude, events_count, created_at, has_decision, simulated
		FROM crowdsec_alerts WHERE source_id = ? AND julianday(created_at) > julianday(?)
		AND julianday(created_at) <= julianday(?)`,
		sourceID, windowStart.Format(time.RFC3339Nano), now.Format(time.RFC3339Nano))
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var scenario, source, country, asNumber, asName, createdAt string
		var latitude, longitude sql.NullFloat64
		var events int64
		var hasDecision, simulated int
		if err := rows.Scan(&scenario, &source, &country, &asNumber, &asName,
			&latitude, &longitude, &events, &createdAt, &hasDecision, &simulated); err != nil {
			rows.Close()
			return nil, err
		}

		created, err := time.Parse(time.RFC3339Nano, createdAt)
		if err != nil {
			continue // malformed timestamps cannot be placed truthfully in the window
		}
		created = created.UTC()
		if !created.After(windowStart) || created.After(now) {
			continue
		}

		stats.Alerts24h++
		if hasDecision != 0 {
			stats.AlertsWithDecision++
		}
		if simulated != 0 {
			stats.SimulatedAlerts24h++
		}
		if validCrowdSecCoordinates(latitude, longitude) {
			stats.GeolocatedAlerts24h++
		}
		if events > 0 {
			stats.ReportedEvents24h += events
		}

		trimmedSource := strings.TrimSpace(source)
		if trimmedSource != "" {
			uniqueSources[trimmedSource] = struct{}{}
		} else {
			trimmedSource = "unknown"
		}
		sources[trimmedSource]++

		country = strings.ToUpper(strings.TrimSpace(country))
		if country == "" {
			country = "??"
		}
		countries[country]++

		scenario = strings.TrimSpace(scenario)
		if scenario == "" {
			scenario = "unknown"
		}
		scenarios[scenario]++

		asNumber = strings.ToUpper(strings.TrimSpace(asNumber))
		asName = strings.TrimSpace(asName)
		if asNumber == "" && asName == "" {
			asNumber, asName = "unknown", "unknown"
		}
		networks[asNumber+"\x00"+asName]++

		bucket := int(created.Sub(windowStart) / bucketSize)
		// The end of the rolling window is inclusive so an alert timestamped
		// exactly at now is represented by the final bucket.
		if bucket == bucketCount && created.Equal(now) {
			bucket--
		}
		if bucket >= 0 && bucket < len(stats.Hourly) {
			stats.Hourly[bucket].Detections++
			if hasDecision != 0 {
				stats.Hourly[bucket].WithDecision++
			}
			if events > 0 {
				stats.Hourly[bucket].ReportedEvents += events
			}
		}
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}

	typeCounts := make(map[string]int64)
	originCounts := make(map[string]int64)
	drows, err := tx.Query(`SELECT type, origin, expires_at FROM crowdsec_decisions`)
	if err != nil {
		return nil, err
	}
	for drows.Next() {
		var decisionType, origin, expiresAt string
		if err := drows.Scan(&decisionType, &origin, &expiresAt); err != nil {
			drows.Close()
			return nil, err
		}
		expires, err := time.Parse(time.RFC3339Nano, expiresAt)
		if err != nil || !expires.UTC().After(now) {
			continue
		}
		stats.ActiveDecisions++
		decisionType = strings.ToLower(strings.TrimSpace(decisionType))
		if decisionType == "" {
			decisionType = "unknown"
		}
		origin = strings.ToLower(strings.TrimSpace(origin))
		if origin == "" {
			origin = "unknown"
		}
		typeCounts[decisionType]++
		originCounts[origin]++
	}
	if err := drows.Err(); err != nil {
		drows.Close()
		return nil, err
	}
	if err := drows.Close(); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}

	stats.UniqueSources24h = int64(len(uniqueSources))
	stats.TotalAlertsInWindow = stats.Alerts24h
	if stats.Alerts24h > 0 {
		stats.DecisionActionRatePercent = math.Round(
			(float64(stats.AlertsWithDecision)/float64(stats.Alerts24h))*1000,
		) / 10
	}

	stats.Countries, stats.CountriesOtherCount = topCrowdSecCountries(countries, crowdSecStatsTopRows)
	if len(stats.Countries) > 0 {
		stats.TopCountry = stats.Countries[0].Country
		stats.TopCountryCount = stats.Countries[0].Count
	}
	stats.Scenarios, stats.ScenariosOtherCount = topCrowdSecScenarios(scenarios, crowdSecStatsTopRows)
	if len(stats.Scenarios) > 0 {
		stats.TopScenario = stats.Scenarios[0].Scenario
	}
	stats.Networks, stats.NetworksOtherCount = topCrowdSecNetworks(networks, crowdSecStatsTopRows)
	stats.Sources, stats.SourcesOtherCount = topCrowdSecSources(sources, crowdSecStatsTopRows)

	for _, item := range rankCrowdSecLabels(typeCounts) {
		stats.ActiveDecisionTypes = append(stats.ActiveDecisionTypes, models.CrowdSecDecisionTypeCount{
			Type: item.label, Count: item.count,
		})
	}
	for _, item := range rankCrowdSecLabels(originCounts) {
		stats.ActiveDecisionOrigins = append(stats.ActiveDecisionOrigins, models.CrowdSecDecisionOriginCount{
			Origin: item.label, Count: item.count,
		})
	}

	return stats, nil
}

func validCrowdSecCoordinates(latitude, longitude sql.NullFloat64) bool {
	return latitude.Valid && longitude.Valid &&
		!math.IsNaN(latitude.Float64) && !math.IsInf(latitude.Float64, 0) &&
		!math.IsNaN(longitude.Float64) && !math.IsInf(longitude.Float64, 0) &&
		latitude.Float64 >= -90 && latitude.Float64 <= 90 &&
		longitude.Float64 >= -180 && longitude.Float64 <= 180
}

func rankCrowdSecLabels(counts map[string]int64) []crowdSecLabelCount {
	ranked := make([]crowdSecLabelCount, 0, len(counts))
	for label, count := range counts {
		ranked = append(ranked, crowdSecLabelCount{label: label, count: count})
	}
	sort.Slice(ranked, func(i, j int) bool {
		if ranked[i].count != ranked[j].count {
			return ranked[i].count > ranked[j].count
		}
		return ranked[i].label < ranked[j].label
	})
	return ranked
}

func topCrowdSecLabels(counts map[string]int64, limit int) ([]crowdSecLabelCount, int64) {
	ranked := rankCrowdSecLabels(counts)
	if limit < 0 {
		limit = 0
	}
	if limit > len(ranked) {
		limit = len(ranked)
	}
	var other int64
	for _, item := range ranked[limit:] {
		other += item.count
	}
	return ranked[:limit], other
}

func topCrowdSecCountries(counts map[string]int64, limit int) ([]models.CrowdSecCountryCount, int64) {
	ranked, other := topCrowdSecLabels(counts, limit)
	out := make([]models.CrowdSecCountryCount, 0, len(ranked))
	for _, item := range ranked {
		out = append(out, models.CrowdSecCountryCount{Country: item.label, Count: item.count})
	}
	return out, other
}

func topCrowdSecScenarios(counts map[string]int64, limit int) ([]models.CrowdSecScenarioCount, int64) {
	ranked, other := topCrowdSecLabels(counts, limit)
	out := make([]models.CrowdSecScenarioCount, 0, len(ranked))
	for _, item := range ranked {
		out = append(out, models.CrowdSecScenarioCount{Scenario: item.label, Count: item.count})
	}
	return out, other
}

func topCrowdSecSources(counts map[string]int64, limit int) ([]models.CrowdSecSourceCount, int64) {
	ranked, other := topCrowdSecLabels(counts, limit)
	out := make([]models.CrowdSecSourceCount, 0, len(ranked))
	for _, item := range ranked {
		out = append(out, models.CrowdSecSourceCount{Source: item.label, Count: item.count})
	}
	return out, other
}

func topCrowdSecNetworks(counts map[string]int64, limit int) ([]models.CrowdSecNetworkCount, int64) {
	ranked, other := topCrowdSecLabels(counts, limit)
	out := make([]models.CrowdSecNetworkCount, 0, len(ranked))
	for _, item := range ranked {
		asNumber, asName, _ := strings.Cut(item.label, "\x00")
		out = append(out, models.CrowdSecNetworkCount{
			ASNumber: asNumber,
			ASName:   asName,
			Count:    item.count,
		})
	}
	return out, other
}
