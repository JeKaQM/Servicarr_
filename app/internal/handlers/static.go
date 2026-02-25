package handlers

import (
	"html/template"
	"net/http"
	"os"
	"path/filepath"
	"status/app/internal/auth"
	"status/app/internal/database"
	"strings"
)

// PageData holds template data for the index page
type PageData struct {
	IsAdmin bool
	AppName string
}

// HandleIndex serves the main HTML page with conditional admin rendering
func HandleIndex(authMgr *auth.Auth) http.HandlerFunc {
	// Parse the template once at startup
	tmpl := template.Must(template.ParseFiles("web/templates/index.html"))

	return func(w http.ResponseWriter, r *http.Request) {
		// Set CSRF token cookie on every page load (for login form)
		authMgr.SetCSRFCookie(w)

		// Check if user is authenticated
		session, err := authMgr.ParseSession(r)
		isAdmin := err == nil && session != nil

		// Get app name from settings
		appName := "Service Status"
		if settings, err := database.LoadAppSettings(); err == nil && settings != nil && settings.AppName != "" {
			appName = settings.AppName
		}

		// Render template with auth state
		data := PageData{IsAdmin: isAdmin, AppName: appName}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if err := tmpl.Execute(w, data); err != nil {
			http.Error(w, "Template error", http.StatusInternalServerError)
		}
	}
}

// HandleStatic serves static files (JS, CSS, images)
func HandleStatic() http.HandlerFunc {
	// Allowed extensions and their content types
	contentTypes := map[string]string{
		".js":   "application/javascript; charset=utf-8",
		".css":  "text/css; charset=utf-8",
		".svg":  "image/svg+xml; charset=utf-8",
		".jpg":  "image/jpeg",
		".jpeg": "image/jpeg",
		".png":  "image/png",
		".html": "text/html; charset=utf-8",
	}

	// Map of allowed directory prefixes → filesystem paths
	allowedDirs := map[string]string{
		"/static/js/":     "web/static/js/",
		"/static/css/":    "web/static/css/",
		"/static/images/": "web/static/images/",
	}

	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
		w.Header().Set("Pragma", "no-cache")
		w.Header().Set("Expires", "0")

		urlPath := r.URL.Path

		// Special case: blocked.html is in templates/
		if strings.HasSuffix(urlPath, "/blocked.html") {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			http.ServeFile(w, r, "web/templates/blocked.html")
			return
		}

		// Match against allowed directories
		for prefix, fsDir := range allowedDirs {
			if !strings.HasPrefix(urlPath, prefix) {
				continue
			}

			// Extract filename (no path traversal allowed)
			filename := filepath.Base(urlPath)
			ext := strings.ToLower(filepath.Ext(filename))

			ct, allowed := contentTypes[ext]
			if !allowed {
				http.NotFound(w, r)
				return
			}

			// Verify file exists and is a regular file (prevent directory listing)
			fsPath := filepath.Join(fsDir, filename)
			info, err := os.Stat(fsPath)
			if err != nil || info.IsDir() {
				http.NotFound(w, r)
				return
			}

			w.Header().Set("Content-Type", ct)
			http.ServeFile(w, r, fsPath)
			return
		}

		http.NotFound(w, r)
	}
}

// HandleFavicon serves the favicon
func HandleFavicon() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/svg+xml; charset=utf-8")
		http.ServeFile(w, r, "web/static/images/favicon.svg")
	}
}
