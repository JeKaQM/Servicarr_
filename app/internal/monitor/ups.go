package monitor

import (
	"context"
	"fmt"
	"status/app/internal/database"
	"status/app/internal/resources"
	"strings"
	"time"
)

// UPSPowerNotifier queues a notification for a confirmed mains-power loss.
type UPSPowerNotifier interface {
	NotifyUPSLineLost(info *resources.UPSInfo) bool
}

// PollUPSPower fetches one UPS reading and applies transition-based notification logic.
func PollUPSPower(ctx context.Context, notifier UPSPowerNotifier) error {
	config, err := database.LoadResourcesUIConfig()
	if err != nil {
		return err
	}
	if config == nil || !config.Enabled || !config.UPS || strings.TrimSpace(config.NUTHost) == "" || strings.TrimSpace(config.UPSName) == "" {
		return nil
	}

	client := resources.NewNUTClient(config.NUTHost)
	if client.Address == "" {
		return fmt.Errorf("invalid NUT UPS host configuration")
	}
	fetchCtx, cancel := context.WithTimeout(ctx, 6*time.Second)
	defer cancel()
	info, err := client.FetchUPS(fetchCtx, config.UPSName)
	if err != nil {
		return err
	}
	if info == nil || info.PowerPresent == nil {
		// An unknown state is not evidence of either an outage or recovery.
		return nil
	}

	source := client.Address + "/" + strings.TrimSpace(config.UPSName)
	return processUPSPowerReading(notifier, source, info)
}

func processUPSPowerReading(notifier UPSPowerNotifier, source string, info *resources.UPSInfo) error {
	if info == nil || info.PowerPresent == nil {
		return nil
	}

	previousSource, previousPresent, previousNotified, found, err := database.LoadUPSPowerState()
	if err != nil {
		return err
	}
	present := *info.PowerPresent
	sameSource := found && previousSource == source

	if present {
		if !sameSource || !previousPresent || previousNotified {
			return database.SaveUPSPowerState(source, true, false)
		}
		return nil
	}

	needsNotification := !sameSource || previousPresent || !previousNotified
	if !needsNotification {
		return nil
	}

	notified := notifier != nil && notifier.NotifyUPSLineLost(info)
	return database.SaveUPSPowerState(source, false, notified)
}
