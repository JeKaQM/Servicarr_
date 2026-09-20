package monitor

import (
	"context"
	"errors"
	"fmt"
	"strings"
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
	crowdsecAlertsPageSize = 100
)

// PollCrowdSec runs one sync cycle: fetch decisions (and alerts when machine
// credentials are configured), then converge the snapshot table. Returns
// an error for the caller's deduped logging; sync failures are also
// persisted to crowdsec_state so the dashboard can show WHY.
func PollCrowdSec(ctx context.Context) error {
	config, err := database.LoadCrowdSecConfig()
	if err != nil {
		return err
	}
	if config == nil || !config.Enabled || strings.TrimSpace(config.LAPIURL) == "" {
		return nil // disabled → no writes, no state churn
	}

	client := crowdsec.NewClient(crowdsec.Config{
		BaseURL:            config.LAPIURL,
		BouncerKey:         config.BouncerAPIKey,
		MachineID:          config.MachineID,
		MachinePassword:    config.MachinePassword,
		InsecureSkipVerify: config.TLSSkipVerify,
	})

	remote, err := fetchDecisionsSnapshot(ctx, client)
	if err != nil {
		recordCrowdSecSyncFailure(err)
		return err
	}

	// Alerts (scenario detections — includes scans that produced no decision)
	// need machine credentials. Missing credentials are not an error: the
	// decisions dashboard works on the bouncer key alone. A failed alerts
	// fetch also must not block decision syncing: the decisions dashboard
	// stays functional even when the machine login is misconfigured.
	alertsInserted := 0
	alertsSyncErr := error(nil)
	if client.HasMachineCredentials() {
		alerts, alertsErr := fetchAlertsSnapshot(ctx, client)
		if alertsErr != nil {
			alertsSyncErr = alertsErr
		} else if len(alerts) > 0 {
			syncedAt := time.Now().UTC()
			alertsInserted, alertsErr = database.SyncCrowdSecAlerts(alerts, syncedAt)
			if alertsErr != nil {
				return alertsErr
			}
		}
	}

	syncedAt := time.Now().UTC()
	total := len(remote)
	changed, syncErr := database.SyncCrowdSecDecisions(remote, total, syncedAt)
	if syncErr != nil {
		return syncErr
	}
	// Record alerts-feed failures AFTER the decisions state save so the
	// successful decisions sync cannot erase them (state order matters).
	if alertsSyncErr != nil {
		recordCrowdSecSyncFailure(alertsSyncErr)
	}
	if changed > 0 {
		logCrowdSecSync(changed, total)
	}
	if alertsInserted > 0 {
		_ = database.InsertLog(database.LogLevelInfo, database.LogCategorySystem, "",
			"CrowdSec alerts synced", fmt.Sprintf("new=%d", alertsInserted))
	}
	return nil
}

// fetchDecisionsSnapshot pulls the latest page of active decisions and
// converts them to the snapshot model. The LAPI list endpoint paginates
// server-side; we deliberately keep one page (cap 500) — community
// blocklist totals can exceed 15k entries and fetching all would be
// megabytes per poll.
func fetchDecisionsSnapshot(ctx context.Context, client *crowdsec.Client) ([]models.CrowdSecDecision, error) {
	if client == nil {
		return nil, fmt.Errorf("crowdsec client unavailable")
	}
	fetchCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	remote, err := client.Decisions(fetchCtx, crowdsec.DecisionsParams{
		Limit: database.CrowdSecMaxSnapshotRows,
	})
	if err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	out := make([]models.CrowdSecDecision, 0, len(remote))
	for _, d := range remote {
		if strings.TrimSpace(d.Value) == "" {
			continue
		}
		id := ""
		if d.ID != nil {
			id = fmt.Sprintf("%d", *d.ID)
		}
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
	// Decision IDs must be unique in the snapshot; LAPI IDs are unique,
	// but a defensive dedup guards against odd API responses.
	seen := make(map[string]struct{}, len(out))
	deduped := out[:0]
	for _, d := range out {
		if d.DecisionID == "" {
			continue
		}
		if _, dup := seen[d.DecisionID]; dup {
			continue
		}
		seen[d.DecisionID] = struct{}{}
		deduped = append(deduped, d)
	}
	return deduped, nil
}

func recordCrowdSecSyncFailure(err error) {
	authFailed := errors.Is(err, crowdsec.ErrAuthFailed)
	_ = database.SaveCrowdSecSyncError(checker.SanitizeError(err.Error()), authFailed)
}

// fetchAlertsSnapshot pulls recent scenario-detection alerts (LAPI applies
// its own `since` sliding window server-side). Alerts are immutable, so
// the DB merge dedups by ID. Machine credentials required.
func fetchAlertsSnapshot(ctx context.Context, client *crowdsec.Client) ([]models.CrowdSecAlert, error) {
	if client == nil {
		return nil, fmt.Errorf("crowdsec client unavailable")
	}
	fetchCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	remote, err := client.Alerts(fetchCtx, crowdsec.AlertsParams{
		// 100 is the largest page real LAPI deployments serve reliably; the
		// ent-backed pagination chokes on larger single-page limits (truncated
		// responses → "unexpected end of JSON input"). 100 matches cscli's
		// default page and is plenty for a live-activity feed.
		Limit: crowdsecAlertsPageSize,
	})
	if err != nil {
		return nil, err
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
			StartAt:     a.StartAt.Format(time.RFC3339),
			CreatedAt:   a.CreatedAt.Format(time.RFC3339),
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
	return out, nil
}

func logCrowdSecSync(changed, total int) {
	_ = database.InsertLog(database.LogLevelInfo, database.LogCategorySystem, "",
		"CrowdSec decisions synced",
		fmt.Sprintf("changed=%d, total=%d", changed, total))
}
