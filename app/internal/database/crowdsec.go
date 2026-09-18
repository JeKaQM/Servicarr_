package database

import (
	"database/sql"
	"fmt"
	"log"
	"strings"
	"time"

	"status/app/internal/crypto"
	"status/app/internal/models"
)

// CrowdSecMaxSnapshotRows caps the decisions snapshot table. The true LAPI
// total can be much higher (community blocklists reach 15k+); the dashboard
// shows "latest N of total".
const CrowdSecMaxSnapshotRows = 500

// LoadCrowdSecConfig loads CrowdSec LAPI configuration. Returns (nil, nil)
// when no row exists (first run). Secrets are decrypted; on decryption
// failure the field is blanked (ciphertext is never surfaced) and the
// handler-side preserve-on-empty merge protects the stored value.
func LoadCrowdSecConfig() (*models.CrowdSecConfig, error) {
	var config models.CrowdSecConfig
	var enabled, skipVerify int
	var encPassword, encKey string
	err := DB.QueryRow(`SELECT enabled, COALESCE(lapi_url, ''), COALESCE(lapi_machine_id, ''),
		COALESCE(lapi_machine_password, ''), COALESCE(bouncer_api_key, ''), poll_interval, tls_skip_verify
		FROM crowdsec_config WHERE id = 1`).Scan(
		&enabled, &config.LAPIURL, &config.MachineID,
		&encPassword, &encKey, &config.PollIntervalS, &skipVerify)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	config.Enabled = enabled != 0
	config.TLSSkipVerify = skipVerify != 0
	config.MachinePassword = decryptCrowdSecSecret("machine password", encPassword)
	config.BouncerAPIKey = decryptCrowdSecSecret("bouncer API key", encKey)
	return &config, nil
}

func decryptCrowdSecSecret(name, stored string) string {
	if stored == "" {
		return ""
	}
	plain, err := crypto.Decrypt(stored)
	if err != nil {
		log.Printf("Warning: failed to decrypt CrowdSec %s: %v", name, err)
		return ""
	}
	return plain
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
		(id, enabled, lapi_url, lapi_machine_id, lapi_machine_password, bouncer_api_key, poll_interval, tls_skip_verify, updated_at)
		VALUES (1, ?, ?, ?, ?, ?, ?, ?, datetime('now'))
		ON CONFLICT(id) DO UPDATE SET
			enabled=?, lapi_url=?, lapi_machine_id=?, lapi_machine_password=?, bouncer_api_key=?,
			poll_interval=?, tls_skip_verify=?, updated_at=datetime('now')`,
		enabled, config.LAPIURL, config.MachineID, encPassword, encKey, config.PollIntervalS, skipVerify,
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

// SaveCrowdSecSyncError records a failed sync. Callers dedup in memory so a
// persistent outage does not write every cycle (runUPSMonitor pattern).
func SaveCrowdSecSyncError(errText string, authFailed bool) error {
	_, err := DB.Exec(`INSERT INTO crowdsec_state (id, last_sync, last_error, auth_failed, decision_count, updated_at)
		VALUES (1, NULL, ?, ?, 0, datetime('now'))
		ON CONFLICT(id) DO UPDATE SET last_error=excluded.last_error,
			auth_failed=excluded.auth_failed, updated_at=datetime('now')`,
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
				created_at=excluded.created_at, expires_at=excluded.expires_at, synced_at=excluded.synced_at`)
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
	if limit <= 0 || limit > CrowdSecMaxSnapshotRows {
		limit = CrowdSecMaxSnapshotRows
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
