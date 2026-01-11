package handlers

import (
	"encoding/json"
	"log"
	"net/http"
	"status/app/internal/auth"
	"status/app/internal/security"
	"time"

	"golang.org/x/crypto/bcrypt"
)

// HandleWhoAmI returns current authentication status
func HandleWhoAmI(authMgr *auth.Auth) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		type resp struct {
			Authenticated bool   `json:"authenticated"`
			User          string `json:"user,omitempty"`
		}
		me := resp{Authenticated: false}

		if s, err := authMgr.ParseSession(r); err == nil {
			me.Authenticated = true
			me.User = s.U
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(me)
	}
}

// HandleLogin authenticates a user
func HandleLogin(authMgr *auth.Auth) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		type creds struct {
			Username string `json:"username"`
			Password string `json:"password"`
		}
		var c creds
		if err := json.NewDecoder(r.Body).Decode(&c); err != nil {
			log.Printf("login: decode error: %v", err)
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}

		ip := security.ClientIP(r)
		if security.IsIPBlocked(ip) {
			log.Printf("login: IP blocked: %s", ip)
			http.Error(w, "access denied - too many failed attempts", http.StatusForbidden)
			return
		}

		if c.Username != authMgr.User {
			log.Printf("login: wrong username from %s", ip)
			security.LogFailedLoginAttempt(ip)
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		if bcrypt.CompareHashAndPassword(authMgr.Hash, []byte(c.Password)) != nil {
			log.Printf("login: wrong password for user %s from %s", c.Username, ip)
			security.LogFailedLoginAttempt(ip)
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		log.Printf("login: success for user %s from %s", c.Username, ip)
		_ = authMgr.MakeSessionCookie(w, c.Username, time.Duration(authMgr.SessionMaxAgeS)*time.Second)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": true})
	}
}

// HandleLogout logs out the current user
func HandleLogout(authMgr *auth.Auth) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		authMgr.ClearSessionCookie(w)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": true})
	}
}
