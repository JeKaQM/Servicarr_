package monitor

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"status/app/internal/checker"
	"status/app/internal/crowdsec"
	"status/app/internal/database"
	"status/app/internal/models"
)

// CrowdSec sync cadence and page sizes.
const (
	// crowdsecAlertsPageSize caps a single LAPI alerts request. Real LAPI
	// deployments (ent pagination) fail on large single-page limits, so this
	// stays at cscli's default page size.
	crowdsecAlertsPageSize     = 100
	crowdsecAlertsLookback     = 24 * time.Hour
	crowdsecDecisionPageSize   = 500
	crowdsecAlertRequestBudget = 8
)

// Polls can be triggered by both the background monitor and the admin
// "Sync now" action. Keep them single-flight so they cannot race while
// converging the same snapshots. The channel form lets a cancelled caller
// stop waiting for an in-flight poll.
var crowdsecPollGate = make(chan struct{}, 1)

// crowdsecConfigChanged coalesces configuration changes until the background
// monitor consumes them. It is intentionally buffered so saving settings never
// blocks on monitor startup or shutdown.
var crowdsecConfigChanged = make(chan struct{}, 1)

// NotifyCrowdSecConfigChanged wakes the background monitor after CrowdSec
// settings are saved. Repeated saves before the monitor wakes coalesce into one
// notification.
func NotifyCrowdSecConfigChanged() {
	select {
	case crowdsecConfigChanged <- struct{}{}:
	default:
	}
}

// CrowdSecConfigChanges exposes the config-change wake signal to the monitor
// run loop. The channel is process-wide and intended to have one consumer.
func CrowdSecConfigChanges() <-chan struct{} {
	return crowdsecConfigChanged
}

type crowdsecClientConfig struct {
	baseURL            string
	bouncerKey         string
	machineID          string
	machinePassword    string
	insecureSkipVerify bool
}

var crowdsecClientCache struct {
	sync.Mutex
	key    crowdsecClientConfig
	client *crowdsec.Client
}

// cachedCrowdSecClient preserves the machine JWT and failed-login backoff
// across poll cycles. Any connection or credential change creates a fresh
// client immediately.
func cachedCrowdSecClient(config *models.CrowdSecConfig) *crowdsec.Client {
	key := crowdsecClientConfig{
		baseURL:            config.LAPIURL,
		bouncerKey:         config.BouncerAPIKey,
		machineID:          config.MachineID,
		machinePassword:    config.MachinePassword,
		insecureSkipVerify: config.TLSSkipVerify,
	}

	crowdsecClientCache.Lock()
	defer crowdsecClientCache.Unlock()
	if crowdsecClientCache.client != nil && crowdsecClientCache.key == key {
		return crowdsecClientCache.client
	}

	client := crowdsec.NewClient(crowdsec.Config{
		BaseURL:            key.baseURL,
		BouncerKey:         key.bouncerKey,
		MachineID:          key.machineID,
		MachinePassword:    key.machinePassword,
		InsecureSkipVerify: key.insecureSkipVerify,
	})
	crowdsecClientCache.key = key
	crowdsecClientCache.client = client
	return client
}

// PollCrowdSec runs one sync cycle. Decisions and alerts use independent LAPI
// authentication realms: a bouncer key enables decisions, while machine
// credentials enable alerts. A failure in one feed does not discard a
// successful update from the other, but the combined error is returned and
// persisted so both the caller and dashboard can report partial failure.
func PollCrowdSec(ctx context.Context) error {
	select {
	case crowdsecPollGate <- struct{}{}:
		defer func() { <-crowdsecPollGate }()
	case <-ctx.Done():
		return ctx.Err()
	}

	config, err := database.LoadCrowdSecConfig()
	if err != nil {
		return err
	}
	if config == nil || !config.Enabled || strings.TrimSpace(config.LAPIURL) == "" {
		return nil // disabled → no writes, no state churn
	}

	client := cachedCrowdSecClient(config)
	hasBouncer := strings.TrimSpace(config.BouncerAPIKey) != ""
	hasMachine := client.HasMachineCredentials()
	if !hasBouncer && !hasMachine {
		err := fmt.Errorf("%w: configure a bouncer key or machine credentials", crowdsec.ErrNotConfigured)
		recordCrowdSecSyncFailure(err)
		return err
	}

	var syncErrors []error
	sourceID := database.CrowdSecSourceID(config.LAPIURL, config.MachineID)
	history, err := database.GetCrowdSecHistoryStateForSource(sourceID)
	if err != nil {
		return err
	}
	if hasBouncer {
		remote, truncated, fetchErr := fetchDecisionsSnapshot(ctx, client)
		if fetchErr != nil {
			syncErrors = append(syncErrors, fmt.Errorf("decisions: %w", fetchErr))
		} else {
			total := len(remote)
			changed, syncErr := database.SyncCrowdSecDecisions(remote, total, time.Now().UTC())
			if syncErr == nil {
				history.DecisionTruncated = truncated
			}
			if syncErr != nil {
				syncErrors = append(syncErrors, fmt.Errorf("store decisions: %w", syncErr))
			} else if changed > 0 {
				logCrowdSecSync(changed, total)
			}
		}
	}

	alertsSucceeded := false
	if hasMachine {
		inserted, fetchErr := syncAlertsHistory(ctx, client, sourceID, &history, time.Now().UTC())
		if fetchErr != nil {
			syncErrors = append(syncErrors, fmt.Errorf("alerts: %w", fetchErr))
		} else {
			alertsSucceeded = true
			if inserted > 0 {
				_ = database.InsertLog(database.LogLevelInfo, database.LogCategorySystem, "",
					"CrowdSec alerts synced", fmt.Sprintf("new=%d", inserted))
			}
		}
	}
	if !hasBouncer && alertsSucceeded {
		// A removed bouncer key must not leave an old decisions snapshot looking
		// active while the machine-only alerts feed continues to sync. Clear it
		// only after the configured alerts feed succeeds, so a failed machine-only
		// poll cannot misleadingly advance last_sync.
		if _, syncErr := database.SyncCrowdSecDecisions(nil, 0, time.Now().UTC()); syncErr != nil {
			syncErrors = append(syncErrors, fmt.Errorf("clear decisions: %w", syncErr))
		}
	}
	// Credential removal disables ingestion, but must never erase history.
	if !hasBouncer && alertsSucceeded {
		history.DecisionTruncated = false
	}
	if saveErr := database.SaveCrowdSecHistoryStateForSource(sourceID, history); saveErr != nil {
		syncErrors = append(syncErrors, fmt.Errorf("store history progress: %w", saveErr))
	}

	if len(syncErrors) > 0 {
		err := errors.Join(syncErrors...)
		// Record only after all independent feeds have had a chance to succeed;
		// otherwise a later successful decisions write can mask an alerts error.
		recordCrowdSecSyncFailure(err)
		return err
	}
	return nil
}

// fetchDecisionsSnapshot pages LAPI's current decisions. A failed page leaves
// the previous snapshot intact. The local safety cap and non-progressing
// servers are explicitly reported, rather than presenting a page as a total.
func fetchDecisionsSnapshot(ctx context.Context, client *crowdsec.Client) ([]models.CrowdSecDecision, bool, error) {
	if client == nil {
		return nil, false, fmt.Errorf("crowdsec client unavailable")
	}
	fetchCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	now := time.Now().UTC()
	out := make([]models.CrowdSecDecision, 0, crowdsecDecisionPageSize)
	seen := make(map[int64]struct{})
	truncated := false
	unstablePages := false
	for offset := 0; ; offset += crowdsecDecisionPageSize {
		remote, err := client.Decisions(fetchCtx, crowdsec.DecisionsParams{Limit: crowdsecDecisionPageSize, Offset: offset})
		if err != nil {
			return nil, false, err
		}
		before := len(out)
		for _, d := range remote {
			if strings.TrimSpace(d.Value) == "" || d.ID == nil {
				continue
			}
			if _, duplicate := seen[*d.ID]; duplicate {
				// Offset pages are not a transactional snapshot. Overlap means
				// records may also have shifted past an unvisited offset.
				unstablePages = true
				continue
			}
			seen[*d.ID] = struct{}{}
			if len(out) == database.CrowdSecMaxSnapshotRows {
				truncated = true
				break
			}
			id := fmt.Sprintf("%d", *d.ID)
			created := now.UTC().Format(time.RFC3339)
			expires := d.ExpiresAt(now)
			if expires.IsZero() {
				expires = now.Add(time.Hour) // unparsable duration: assume short
			}
			out = append(out, models.CrowdSecDecision{
				DecisionID: id,
				Value:      d.Value,
				Type:       d.Type,
				Scope:      d.Scope,
				Origin:     d.Origin,
				Scenario:   d.Scenario,
				Duration:   d.Duration,
				CreatedAt:  created,
				ExpiresAt:  expires.UTC().Format(time.RFC3339),
				SyncedAt:   now.UTC().Format(time.RFC3339),
			})
		}
		if truncated || len(remote) < crowdsecDecisionPageSize {
			break
		}
		if len(out) == before || offset >= database.CrowdSecMaxSnapshotRows {
			truncated = true
			break
		}
	}
	return out, truncated || unstablePages, nil
}

func recordCrowdSecSyncFailure(err error) {
	authFailed := errors.Is(err, crowdsec.ErrAuthFailed)
	_ = database.SaveCrowdSecSyncError(checker.SanitizeError(err.Error()), authFailed)
}

// fetchAlertSlice filters by LAPI start_at, with a small boundary overlap to
// tolerate request transit time. Saturation is measured before conversion.
func fetchAlertSlice(ctx context.Context, client *crowdsec.Client, start, end time.Time) ([]models.CrowdSecAlert, bool, error) {
	if client == nil {
		return nil, false, fmt.Errorf("crowdsec client unavailable")
	}
	fetchCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	includeCAPI := false
	now := time.Now().UTC()
	remote, err := client.Alerts(fetchCtx, crowdsec.AlertsParams{
		Limit:       crowdsecAlertsPageSize,
		Since:       now.Sub(start) + 2*time.Second,
		Until:       max(time.Duration(0), now.Sub(end)-2*time.Second),
		IncludeCAPI: &includeCAPI,
	})
	if err != nil {
		return nil, false, err
	}

	out := make([]models.CrowdSecAlert, 0, len(remote))
	for _, a := range remote {
		id := ""
		if a.ID != nil {
			id = fmt.Sprintf("%d", *a.ID)
		}
		if id == "" {
			continue // non-persistable noise; LAPI IDs are always set
		}
		var events int64
		if a.EventsCount != nil {
			events = *a.EventsCount
		}
		simulated := a.Simulated != nil && *a.Simulated
		alert := models.CrowdSecAlert{
			AlertID:     id,
			Scenario:    a.Scenario,
			Message:     a.Message,
			EventsCount: events,
			StartAt:     a.StartAt.Format(time.RFC3339Nano),
			CreatedAt:   a.CreatedAt.Format(time.RFC3339Nano),
			Simulated:   simulated,
			HasDecision: len(a.Decisions) > 0,
		}
		if a.Source != nil {
			alert.SourceValue = a.Source.Value
			alert.Country = a.Source.Country
			alert.ASNumber = a.Source.ASNumber
			alert.ASName = a.Source.ASName
			if a.Source.Latitude != nil && a.Source.Longitude != nil {
				lat := float64(*a.Source.Latitude)
				lng := float64(*a.Source.Longitude)
				alert.Latitude = &lat
				alert.Longitude = &lng
			}
		}
		out = append(out, alert)
	}
	return out, len(remote) >= crowdsecAlertsPageSize, nil
}

// syncAlertsHistory always refreshes the newest alerts, then spends a bounded
// request budget catching up gaps and walking backwards through retained LAPI
// history. Full pages are subdivided by time (alerts have no offset support).
// Persisted widths ensure a crowded interval can progress across poll cycles.
func syncAlertsHistory(ctx context.Context, client *crowdsec.Client, sourceID string, state *database.CrowdSecHistoryState, now time.Time) (int, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	now = now.UTC().Truncate(time.Second)
	cutoff := now.Add(-database.CrowdSecArchiveMaxAge)
	cursor, _ := time.Parse(time.RFC3339Nano, state.CursorAt)
	live, _ := time.Parse(time.RFC3339Nano, state.LastLiveAt)
	if cursor.IsZero() {
		cursor = now
	}
	if live.IsZero() {
		live = now.Add(-crowdsecAlertsLookback)
	}
	if live.Before(cutoff) {
		live = cutoff
		state.Truncated = true
	}
	if cursor.Before(cutoff) {
		cursor = cutoff
	}
	inserted := 0
	fetch := func(start, end time.Time) (bool, error) {
		alerts, saturated, err := fetchAlertSlice(ctx, client, start, end)
		if err != nil {
			return false, err
		}
		n, err := database.SyncCrowdSecAlertsForSource(alerts, time.Now().UTC(), sourceID)
		inserted += n
		if err != nil {
			return false, err
		}
		stored, err := database.GetCrowdSecHistoryStateForSource(sourceID)
		state.Truncated = state.Truncated || stored.Truncated
		return saturated, err
	}
	// Recent data remains fresh even while a large initial backfill is pending.
	// LAPI filters scenario start time, while an alert may only be created
	// after the scenario finishes. Re-read the recent window on every poll.
	previewStart := now.Add(-crowdsecAlertsLookback)
	saturated, err := fetch(previewStart, now)
	if err != nil {
		return inserted, err
	}
	if !saturated && !live.Before(previewStart) {
		live = now
	}
	defer func() {
		state.CursorAt = cursor.Format(time.RFC3339Nano)
		state.LastLiveAt = live.Format(time.RFC3339Nano)
	}()
	for request := 1; request < crowdsecAlertRequestBudget; request++ {
		// Reserve half the budget for historical backfill while a live gap is
		// pending. Otherwise all remaining requests can inspect older history.
		catchup := live.Before(now) && request <= crowdsecAlertRequestBudget/2
		if !catchup && (!cursor.After(cutoff) || state.Complete) {
			continue
		}
		var archiveRows int
		if !catchup {
			if err := database.DB.QueryRow(`SELECT COUNT(*) FROM crowdsec_alerts`).Scan(&archiveRows); err != nil {
				return inserted, err
			}
			if archiveRows >= database.CrowdSecMaxArchivedAlerts {
				state.Truncated = true
				state.Complete = false
				continue
			}
		}
		width := time.Duration(state.BackfillSeconds) * time.Second
		if width <= 0 {
			width = database.CrowdSecArchiveMaxAge
		}
		start, end := maxTime(cutoff, cursor.Add(-width)), cursor
		if catchup {
			width = time.Duration(state.LiveSeconds) * time.Second
			if width <= 0 {
				width = crowdsecAlertsLookback
			}
			start, end = live, minTime(now, live.Add(width))
		}
		saturated, err = fetch(start, end)
		if err != nil {
			return inserted, err
		}
		span := end.Sub(start)
		if saturated && span > time.Second {
			width = max(time.Second, span/2)
		} else {
			if saturated {
				state.Truncated = true
			}
			if catchup {
				live = end
			} else {
				cursor = start
			}
			width = min(database.CrowdSecArchiveMaxAge, max(time.Second, span)*4)
		}
		if catchup {
			state.LiveSeconds = int64(width / time.Second)
		} else {
			state.BackfillSeconds = int64(width / time.Second)
		}
		if !cursor.After(cutoff) {
			state.Complete = true
		}
	}
	return inserted, nil
}

func maxTime(a, b time.Time) time.Time {
	if a.After(b) {
		return a
	}
	return b
}
func minTime(a, b time.Time) time.Time {
	if a.Before(b) {
		return a
	}
	return b
}

func logCrowdSecSync(changed, total int) {
	_ = database.InsertLog(database.LogLevelInfo, database.LogCategorySystem, "",
		"CrowdSec decisions synced",
		fmt.Sprintf("changed=%d, total=%d", changed, total))
}
