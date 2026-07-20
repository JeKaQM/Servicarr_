package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"status/app/internal/database"
	"status/app/internal/resources"
	"strings"
	"sync"
	"time"
)

// Cached Glances client - recreated when URL changes
var (
	glClient    *resources.Client
	glClientURL string
	glClientMu  sync.RWMutex

	nutClient     *resources.NUTClient
	nutClientAddr string
	nutClientMu   sync.RWMutex
)

// getGlancesClient returns a Glances client for the configured URL
func getGlancesClient(glancesURL string) *resources.Client {
	fullURL := normalizeGlancesURL(glancesURL)
	if fullURL == "" {
		return nil
	}

	glClientMu.RLock()
	if glClient != nil && glClientURL == fullURL {
		glClientMu.RUnlock()
		return glClient
	}
	glClientMu.RUnlock()

	// Need to create or update client
	glClientMu.Lock()
	defer glClientMu.Unlock()

	// Double-check after acquiring write lock
	if glClient != nil && glClientURL == fullURL {
		return glClient
	}

	glClient = resources.NewClient(fullURL)
	glClientURL = fullURL
	return glClient
}

func normalizeGlancesURL(glancesURL string) string {
	fullURL := strings.TrimSpace(glancesURL)
	if fullURL == "" {
		return ""
	}
	if !strings.HasPrefix(fullURL, "http://") && !strings.HasPrefix(fullURL, "https://") {
		fullURL = "http://" + fullURL
	}
	return strings.TrimSuffix(fullURL, "/") + "/api/4"
}

// getNUTClient returns a NUT client for the configured upsd address.
func getNUTClient(nutHost string) *resources.NUTClient {
	if nutHost == "" {
		return nil
	}

	addr := resources.NormalizeNUTAddress(nutHost)
	if addr == "" {
		return nil
	}

	nutClientMu.RLock()
	if nutClient != nil && nutClientAddr == addr {
		nutClientMu.RUnlock()
		return nutClient
	}
	nutClientMu.RUnlock()

	nutClientMu.Lock()
	defer nutClientMu.Unlock()

	if nutClient != nil && nutClientAddr == addr {
		return nutClient
	}

	nutClient = resources.NewNUTClient(addr)
	nutClientAddr = addr
	return nutClient
}

// HandleResources returns a normalized resource snapshot from configured sources.
// Glances provides system metrics; Network UPS Tools provides optional UPS metrics.
func HandleResources(gl *resources.Client) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Load config to get Glances URL
		cfg, err := database.LoadResourcesUIConfig()
		if err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusInternalServerError)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"error":   "config_error",
				"message": "Failed to load resources config",
			})
			return
		}

		hasGlances := cfg != nil && strings.TrimSpace(cfg.GlancesURL) != ""
		hasUPS := cfg != nil && cfg.UPS && strings.TrimSpace(cfg.NUTHost) != "" && strings.TrimSpace(cfg.UPSName) != ""
		requireUPS := r.URL.Query().Get("require") == "ups"

		// Check if resources are enabled and at least one source is configured.
		if cfg == nil || !cfg.Enabled || (!hasGlances && !hasUPS) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusServiceUnavailable)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"error":   "not_configured",
				"message": "Resources monitoring is not configured. Set Glances or NUT UPS details in admin settings.",
			})
			return
		}

		if requireUPS && !hasUPS {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusServiceUnavailable)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"error":   "ups_not_configured",
				"message": "UPS monitoring is not configured. Set NUT host:port, UPS name, and enable the UPS tile.",
			})
			return
		}

		ctx := r.Context()
		snap := resources.Snapshot{TakenAt: time.Now().UTC()}
		gotData := false
		var glancesErr error
		var upsErr error

		type glancesResult struct {
			snapshot resources.Snapshot
			err      error
		}
		type upsResult struct {
			info *resources.UPSInfo
			err  error
		}

		var glancesResults chan glancesResult
		if hasGlances {
			glancesResults = make(chan glancesResult, 1)
			go func() {
				client := getGlancesClient(cfg.GlancesURL)
				if client == nil {
					glancesResults <- glancesResult{err: errInvalidResourceConfig("invalid Glances URL configuration")}
					return
				}
				glancesSnap, err := client.FetchSnapshot(ctx)
				glancesResults <- glancesResult{snapshot: glancesSnap, err: err}
			}()
		}

		var upsResults chan upsResult
		if hasUPS {
			upsResults = make(chan upsResult, 1)
			go func() {
				client := getNUTClient(cfg.NUTHost)
				if client == nil {
					upsResults <- upsResult{err: errInvalidResourceConfig("invalid NUT UPS host configuration")}
					return
				}
				upsInfo, err := client.FetchUPS(ctx, cfg.UPSName)
				if err != nil {
					err = enrichNUTError(ctx, client, err)
				}
				upsResults <- upsResult{info: upsInfo, err: err}
			}()
		}

		if glancesResults != nil {
			result := <-glancesResults
			glancesErr = result.err
			if result.err == nil {
				snap = result.snapshot
				gotData = true
			}
		}
		if upsResults != nil {
			result := <-upsResults
			upsErr = result.err
			if result.err == nil {
				snap.UPS = result.info
				gotData = true
			}
		}

		if requireUPS && upsErr != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadGateway)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"error":    "ups_unavailable",
				"message":  upsErr.Error(),
				"taken_at": time.Now().UTC(),
			})
			return
		}

		if !gotData {
			errorName := "resources_unavailable"
			if glancesErr != nil && upsErr == nil {
				errorName = "glances_unavailable"
			} else if upsErr != nil && glancesErr == nil {
				errorName = "ups_unavailable"
			}

			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadGateway)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"error":    errorName,
				"message":  resourceErrorMessage(glancesErr, upsErr),
				"taken_at": time.Now().UTC(),
			})
			return
		}

		// The resources endpoint is public. Keep device identity and maintenance
		// diagnostics available to the authenticated connection test only.
		snap.UPS = redactPublicUPSInfo(snap.UPS)

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(snap)
	}
}

func redactPublicUPSInfo(info *resources.UPSInfo) *resources.UPSInfo {
	if info == nil {
		return nil
	}
	publicInfo := *info
	publicInfo.Manufacturer = ""
	publicInfo.Serial = ""
	publicInfo.TestResult = ""
	return &publicInfo
}

type errInvalidResourceConfig string

func (e errInvalidResourceConfig) Error() string {
	return string(e)
}

func resourceErrorMessage(glancesErr, upsErr error) string {
	switch {
	case glancesErr != nil && upsErr != nil:
		return "Glances: " + glancesErr.Error() + "; UPS: " + upsErr.Error()
	case glancesErr != nil:
		return glancesErr.Error()
	case upsErr != nil:
		return upsErr.Error()
	default:
		return "resource sources unavailable"
	}
}

func enrichNUTError(ctx context.Context, client *resources.NUTClient, err error) error {
	if err == nil || client == nil || !resources.IsUnknownUPSError(err) {
		return err
	}
	names, listErr := client.FetchUPSNames(ctx)
	if listErr != nil || len(names) == 0 {
		return err
	}
	if len(names) == 1 {
		return fmt.Errorf("%w; known UPS name: %s", err, names[0])
	}
	return fmt.Errorf("%w; known UPS names: %s", err, strings.Join(names, ", "))
}
