package handlers

import (
	"html/template"
	"net/http"
	"status/app/internal/auth"
	"strings"
)

// PageData holds template data for the index page
type PageData struct {
	IsAdmin bool
}

// HandleIndex serves the main HTML page with conditional admin rendering
func HandleIndex(authMgr *auth.Auth) http.HandlerFunc {
	// Parse the template once at startup
	tmpl := template.Must(template.ParseFiles("web/templates/index.html"))
	
	return func(w http.ResponseWriter, r *http.Request) {
		// Set CSRF token cookie on every page load (for login form)
		_, _ = authMgr.SetCSRFCookie(w)
		
		// Check if user is authenticated
		session, err := authMgr.ParseSession(r)
		isAdmin := err == nil && session != nil
		
		// Render template with auth state
		data := PageData{IsAdmin: isAdmin}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if err := tmpl.Execute(w, data); err != nil {
			http.Error(w, "Template error", http.StatusInternalServerError)
		}
	}
}

// HandleStatic serves static files (JS, CSS, images)
func HandleStatic() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Set cache control headers to prevent caching
		w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
		w.Header().Set("Pragma", "no-cache")
		w.Header().Set("Expires", "0")

		switch {
		case strings.HasSuffix(r.URL.Path, "/app.js"):
			w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
			http.ServeFile(w, r, "web/static/js/app.js")
		case strings.HasSuffix(r.URL.Path, "/blocks.js"):
			w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
			http.ServeFile(w, r, "web/static/js/blocks.js")
		case strings.HasSuffix(r.URL.Path, "/utils.js"):
			w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
			http.ServeFile(w, r, "web/static/js/utils.js")
		case strings.HasSuffix(r.URL.Path, "/main.css"):
			w.Header().Set("Content-Type", "text/css; charset=utf-8")
			http.ServeFile(w, r, "web/static/css/main.css")
		case strings.HasSuffix(r.URL.Path, "/blocks.css"):
			w.Header().Set("Content-Type", "text/css; charset=utf-8")
			http.ServeFile(w, r, "web/static/css/blocks.css")
		case strings.HasSuffix(r.URL.Path, "/blocked.html"):
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			http.ServeFile(w, r, "web/templates/blocked.html")
		case strings.HasSuffix(r.URL.Path, "/plex.svg"):
			w.Header().Set("Content-Type", "image/svg+xml; charset=utf-8")
			http.ServeFile(w, r, "web/static/images/plex.svg")
		case strings.HasSuffix(r.URL.Path, "/overseerr.svg"):
			w.Header().Set("Content-Type", "image/svg+xml; charset=utf-8")
			http.ServeFile(w, r, "web/static/images/overseerr.svg")
		case strings.HasSuffix(r.URL.Path, "/server.svg"):
			w.Header().Set("Content-Type", "image/svg+xml; charset=utf-8")
			http.ServeFile(w, r, "web/static/images/server.svg")
		case strings.HasSuffix(r.URL.Path, "/favicon.svg"):
			w.Header().Set("Content-Type", "image/svg+xml; charset=utf-8")
			http.ServeFile(w, r, "web/static/images/favicon.svg")
		case strings.HasSuffix(r.URL.Path, "/servicarr-icon.jpg"):
			w.Header().Set("Content-Type", "image/jpeg")
			http.ServeFile(w, r, "web/static/images/servicarr-icon.jpg")
		default:
			http.NotFound(w, r)
		}
	}
}

// HandleFavicon serves the favicon
func HandleFavicon() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/x-icon")
		http.ServeFile(w, r, "web/static/images/favicon.ico")
	}
}
