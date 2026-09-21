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
	crowdsecAlertsPageSize = 100
	crowdsecAlertsLookback = 24 * time.Hour
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
	if hasBouncer {
		remote, fetchErr := fetchDecisionsSnapshot(ctx, client)
		if fetchErr != nil {
			syncErrors = append(syncErrors, fmt.Errorf("decisions: %w", fetchErr))
		} else {
			total := len(remote)
			changed, syncErr := database.SyncCrowdSecDecisions(remote, total, time.Now().UTC())
			if syncErr != nil {
				syncErrors = append(syncErrors, fmt.Errorf("store decisions: %w", syncErr))
			} else if changed > 0 {
				logCrowdSecSync(changed, total)
			}
		}
	}

	alertsSucceeded := false
	if hasMachine {
		alerts, fetchErr := fetchAlertsSnapshot(ctx, client)
		if fetchErr != nil {
			syncErrors = append(syncErrors, fmt.Errorf("alerts: %w", fetchErr))
		} else {
			inserted, syncErr := database.SyncCrowdSecAlerts(alerts, time.Now().UTC())
			if syncErr != nil {
				syncErrors = append(syncErrors, fmt.Errorf("store alerts: %w", syncErr))
			} else {
				alertsSucceeded = true
				if inserted > 0 {
					_ = database.InsertLog(database.LogLevelInfo, database.LogCategorySystem, "",
						"CrowdSec alerts synced", fmt.Sprintf("new=%d", inserted))
				}
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
	if !hasMachine {
		// Machine credentials own the alerts capability. Once they are removed,
		// cached detections must disappear immediately rather than looking like a
		// healthy live feed for the remainder of the 24-hour retention window.
		if _, clearErr := database.ClearCrowdSecAlerts(); clearErr != nil {
			syncErrors = append(syncErrors, fmt.Errorf("clear alerts: %w", clearErr))
		}
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

// fetchDecisionsSnapshot pulls one capped page of active decisions and
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

// fetchAlertsSnapshot pulls local scenario-detection alerts from the last
// 24 hours. Community-list/CAPI alerts are excluded: they can carry very
// large historical decision payloads and do not represent live detections
// by this LAPI. Alerts are immutable, so the DB merge dedups by ID.
func fetchAlertsSnapshot(ctx context.Context, client *crowdsec.Client) ([]models.CrowdSecAlert, error) {
	if client == nil {
		return nil, fmt.Errorf("crowdsec client unavailable")
	}
	fetchCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	includeCAPI := false
	remote, err := client.Alerts(fetchCtx, crowdsec.AlertsParams{
		// 100 is the largest page real LAPI deployments serve reliably; the
		// ent-backed pagination chokes on larger single-page limits (truncated
		// responses → "unexpected end of JSON input"). 100 matches cscli's
		// default page and is plenty for a live-activity feed.
		Limit:       crowdsecAlertsPageSize,
		Since:       crowdsecAlertsLookback,
		IncludeCAPI: &includeCAPI,
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
