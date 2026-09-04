package handlers

import (
	"encoding/json"
	"net/http"
	"status/app/internal/auth"
	"status/app/internal/database"
	"status/app/internal/security"
	"strings"
	"time"
)

type auditResponseWriter struct {
	http.ResponseWriter
	status int
}

func (w *auditResponseWriter) WriteHeader(status int) {
	if w.status != 0 {
		return
	}
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}

func (w *auditResponseWriter) Write(body []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	return w.ResponseWriter.Write(body)
}

// AuditAdminActions records explicit admin mutations and denied admin access.
// Successful background GET polling is intentionally excluded.
func AuditAdminActions(authMgr *auth.Auth, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		started := time.Now()
		actor := "anonymous"
		if session, err := authMgr.ParseSession(r); err == nil && session.U != "" {
			actor = session.U
		}

		recorder := &auditResponseWriter{ResponseWriter: w}
		next.ServeHTTP(recorder, r)
		if recorder.status == 0 {
			recorder.status = http.StatusOK
		}

		action := adminAuditAction(r)
		if action == "" && recorder.status != http.StatusUnauthorized && recorder.status != http.StatusForbidden {
			return
		}
		if action == "" {
			action = "Admin access denied"
		}
		level := database.LogLevelInfo
		if recorder.status >= 500 {
			level = database.LogLevelError
		} else if recorder.status >= 400 {
			level = database.LogLevelWarn
		}
		writeAuditLog(r, level, actor, action, map[string]any{
			"method":      r.Method,
			"path":        r.URL.Path,
			"status":      recorder.status,
			"outcome":     auditOutcome(recorder.status),
			"duration_ms": time.Since(started).Milliseconds(),
		})
	})
}

func adminAuditAction(r *http.Request) string {
	if r.Method == http.MethodGet {
		if r.URL.Path == "/api/admin/settings/export" {
			return "Database backup exported"
		}
		if r.Header.Get("X-Audit-Action") == "logs.refresh" {
			return "Logs refreshed"
		}
		return ""
	}
	path := r.URL.Path
	key := r.Method + " " + path
	actions := map[string]string{
		http.MethodPost + " /api/admin/ingest-now":              "All service cards refreshed",
		http.MethodPost + " /api/admin/reset-recent":            "Recent incidents reset",
		http.MethodPost + " /api/admin/check":                   "Service card refreshed",
		http.MethodPost + " /api/admin/toggle-monitoring":       "Service monitoring changed",
		http.MethodPost + " /api/admin/unblock":                 "IP address unblocked",
		http.MethodPost + " /api/admin/clear-blocks":            "IP blocks cleared",
		http.MethodPost + " /api/admin/alerts/config":           "Notification settings changed",
		http.MethodPost + " /api/admin/resources/config":        "Resource settings changed",
		http.MethodPost + " /api/admin/resources/test":          "Resource connection tested",
		http.MethodPost + " /api/admin/alerts/test":             "Email notification tested",
		http.MethodPost + " /api/admin/alerts/test-channel":     "Notification channel tested",
		http.MethodPost + " /api/admin/status-alerts":           "Status banner created",
		http.MethodPut + " /api/admin/status-alerts":            "Status banner changed",
		http.MethodDelete + " /api/admin/status-alerts":         "Status banner removed or hidden",
		http.MethodPost + " /api/admin/maintenance-schedules":   "Maintenance schedule changed",
		http.MethodDelete + " /api/admin/maintenance-schedules": "Maintenance schedule deleted",
		http.MethodPost + " /api/admin/settings/app-name":       "Application name changed",
		http.MethodPost + " /api/admin/settings/password":       "Administrator password changed",
		http.MethodPost + " /api/admin/settings/import":         "Database backup imported",
		http.MethodPost + " /api/admin/settings/reset":          "Database reset requested",
		http.MethodDelete + " /api/admin/logs":                  "Logs cleared",
		http.MethodPost + " /api/admin/services":                "Service created",
		http.MethodPost + " /api/admin/services/reorder":        "Services reordered",
		http.MethodPost + " /api/admin/services/test":           "Service connection tested",
		http.MethodPost + " /api/admin/whitelist":               "Whitelist entry added",
		http.MethodDelete + " /api/admin/whitelist":             "Whitelist entry removed",
		http.MethodPost + " /api/admin/blacklist":               "Blacklist entry added",
		http.MethodDelete + " /api/admin/blacklist":             "Blacklist entry removed",
	}
	if action := actions[key]; action != "" {
		return action
	}
	if strings.HasPrefix(path, "/api/admin/services/") {
		switch r.Method {
		case http.MethodPut:
			if strings.HasSuffix(path, "/visibility") {
				return "Service visibility changed"
			}
			return "Service settings changed"
		case http.MethodDelete:
			return "Service deleted"
		}
	}
	if r.Method == http.MethodPost || r.Method == http.MethodPut || r.Method == http.MethodDelete || r.Method == http.MethodPatch {
		return "Admin setting changed"
	}
	return ""
}

func auditOutcome(status int) string {
	if status >= 200 && status < 400 {
		return "success"
	}
	return "failed"
}

func writeAuditLog(r *http.Request, level, actor, message string, extra map[string]any) {
	if database.DB == nil {
		return
	}
	details := map[string]any{
		"actor": actor,
		"ip":    security.ClientIP(r),
	}
	for key, value := range extra {
		details[key] = value
	}
	encoded, err := json.Marshal(details)
	if err != nil {
		return
	}
	_ = database.InsertLog(level, database.LogCategoryAudit, "", message, string(encoded))
}
