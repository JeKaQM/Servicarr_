package main

// dbgsec: diagnose CrowdSec config loading against the live DB.
// Prints only boolean/shape info — never secrets.
import (
	"context"
	"fmt"
	"os"
	"time"

	"status/app/internal/crowdsec"
	"status/app/internal/crypto"
	"status/app/internal/database"
	"status/app/internal/monitor"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: dbgsec <db-path>")
		os.Exit(1)
	}
	if err := database.Init(os.Args[1]); err != nil {
		fmt.Fprintln(os.Stderr, "init:", err)
		os.Exit(1)
	}
	settings, err := database.LoadAppSettings()
	if err != nil {
		fmt.Fprintln(os.Stderr, "settings:", err)
		os.Exit(1)
	}
	crypto.SetKey([]byte(settings.AuthSecret))

	cfg, err := database.LoadCrowdSecConfig()
	if err != nil {
		fmt.Fprintln(os.Stderr, "load:", err)
		os.Exit(1)
	}
	if cfg == nil {
		fmt.Println("config: <nil>")
		return
	}
	fmt.Printf("enabled=%v url=%s machine_id=%q\n", cfg.Enabled, cfg.LAPIURL, cfg.MachineID)
	fmt.Printf("machine_password_len=%d bouncer_key_len=%d\n", len(cfg.MachinePassword), len(cfg.BouncerAPIKey))
	fmt.Printf("has_machine_creds=%v\n", cfg.MachineID != "" && cfg.MachinePassword != "")

	if len(os.Args) > 2 && os.Args[2] == "--poll" {
		fmt.Println("--- direct client.Alerts test ---")
		client := crowdsec.NewClient(crowdsec.Config{
			BaseURL:         cfg.LAPIURL,
			BouncerKey:      cfg.BouncerAPIKey,
			MachineID:       cfg.MachineID,
			MachinePassword: cfg.MachinePassword,
		})
		fmt.Printf("hasMachineCreds=%v\n", client.HasMachineCredentials())
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		remote, err := client.Alerts(ctx, crowdsec.AlertsParams{Limit: 100})
		cancel()
		if err != nil {
			fmt.Println("direct alerts error:", err)
		} else {
			fmt.Printf("direct alerts returned: %d\n", len(remote))
			if len(remote) > 0 {
				first := remote[0]
				fmt.Printf("first: id=%v scenario=%s country=%s lat=%v lng=%v\n",
					first.ID, first.Scenario,
					func() string {
						if first.Source != nil {
							return first.Source.Country
						}
						return ""
					}(),
					func() any {
						if first.Source != nil && first.Source.Latitude != nil {
							return *first.Source.Latitude
						}
						return nil
					}(),
					func() any {
						if first.Source != nil && first.Source.Longitude != nil {
							return *first.Source.Longitude
						}
						return nil
					}())
			}
		}
		// Same query with the production limit (2000) to isolate a
		// server-side pagination bug with large limits.
		ctx2, cancel2 := context.WithTimeout(context.Background(), 20*time.Second)
		remote2, err2 := client.Alerts(ctx2, crowdsec.AlertsParams{Limit: 2000})
		cancel2()
		if err2 != nil {
			fmt.Println("limit=2000 alerts error:", err2)
		} else {
			fmt.Printf("limit=2000 alerts returned: %d\n", len(remote2))
		}
		fmt.Println("--- running PollCrowdSec ---")
		if err := monitor.PollCrowdSec(context.Background()); err != nil {
			fmt.Println("poll error:", err)
		} else {
			fmt.Println("poll ok")
		}
		state, err := database.GetCrowdSecState()
		if err != nil {
			fmt.Println("state query:", err)
		} else if state != nil {
			fmt.Printf("state: last_sync=%s last_error=%q auth_failed=%v\n",
				state.LastSync.Format(time.RFC3339), state.LastError, state.AuthFailed)
		}
		alerts, err := database.GetCrowdSecAlerts(10)
		if err != nil {
			fmt.Println("alerts query:", err)
		}
		fmt.Printf("stored alerts: %d\n", len(alerts))
	}
}
