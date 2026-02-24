package handlers

import (
	"encoding/json"
	"net/http"
	"status/app/internal/auth"
	"status/app/internal/database"
	"strings"

	"golang.org/x/crypto/bcrypt"
)

// HandleUpdateAppName handles app name change requests
func HandleUpdateAppName() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		var req struct {
			AppName string `json:"app_name"`
		}

		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "Invalid request"})
			return
		}

		appName := strings.TrimSpace(req.AppName)
		if appName == "" {
			appName = "Service Status"
		}
		if len(appName) > 50 {
			appName = appName[:50]
		}

		// Load current settings
		settings, err := database.LoadAppSettings()
		if err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusInternalServerError)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "Failed to load settings"})
			return
		}

		// Update app name
		settings.AppName = appName
		if err := database.SaveAppSettings(settings); err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusInternalServerError)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "Failed to save app name"})
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"success": true, "app_name": appName})
	}
}

// HandleChangePassword handles password change requests
func HandleChangePassword(authMgr *auth.Auth) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		var req struct {
			CurrentPassword string `json:"current_password"`
			NewPassword     string `json:"new_password"`
		}

		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, `{"error": "invalid request"}`, http.StatusBadRequest)
			return
		}

		if req.CurrentPassword == "" || req.NewPassword == "" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "Both current and new password are required"})
			return
		}

		if len(req.NewPassword) < 8 {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "New password must be at least 8 characters"})
			return
		}

		// Load current settings
		settings, err := database.LoadAppSettings()
		if err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusInternalServerError)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "Failed to load settings"})
			return
		}

		// Verify current password
		if err := bcrypt.CompareHashAndPassword([]byte(settings.PasswordHash), []byte(req.CurrentPassword)); err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "Current password is incorrect"})
			return
		}

		// Hash new password
		newHash, err := bcrypt.GenerateFromPassword([]byte(req.NewPassword), bcrypt.DefaultCost)
		if err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusInternalServerError)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "Failed to hash password"})
			return
		}

		// Update password in database
		settings.PasswordHash = string(newHash)
		if err := database.SaveAppSettings(settings); err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusInternalServerError)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "Failed to save new password"})
			return
		}

		// Reload auth manager with new credentials
		authMgr.Reload(settings.Username, []byte(settings.PasswordHash), []byte(settings.AuthSecret))

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"success": true})
	}
}
