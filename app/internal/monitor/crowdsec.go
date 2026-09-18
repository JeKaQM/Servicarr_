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

	syncedAt := time.Now().UTC()
	total := len(remote)
	changed, syncErr := database.SyncCrowdSecDecisions(remote, total, syncedAt)
	if syncErr != nil {
		return syncErr
	}
	if changed > 0 {
		logCrowdSecSync(changed, total)
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

func logCrowdSecSync(changed, total int) {
	_ = database.InsertLog(database.LogLevelInfo, database.LogCategorySystem, "",
		"CrowdSec decisions synced",
		fmt.Sprintf("changed=%d, total=%d", changed, total))
}
