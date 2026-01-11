package handlers

import (
	"encoding/json"
	"net/http"
	"status/app/internal/resources"
	"time"
)

// HandleResources returns a normalized snapshot of system resources from Glances.
// It is designed to be called frequently by the frontend.
func HandleResources(gl *resources.Client) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// A bit shorter than the client's HTTP timeout.
		ctx := r.Context()
		// Fetch is cached inside the client.
		snap, err := gl.FetchSnapshot(ctx)
		if err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadGateway)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"error":    "glances_unavailable",
				"message":  err.Error(),
				"taken_at": time.Now().UTC(),
			})
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(snap)
	}
}
