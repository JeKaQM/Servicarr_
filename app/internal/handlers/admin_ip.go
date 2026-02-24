package handlers

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"status/app/internal/security"
)

// HandleListBlocks returns all blocked IPs
func HandleListBlocks() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		blocks, err := security.ListBlockedIPs()
		if err != nil {
			http.Error(w, "server error", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"blocks": blocks,
		})
	}
}

// HandleUnblockIP removes a block for a specific IP
func HandleUnblockIP() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			IP string `json:"ip"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.IP == "" {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}

		// Validate IP address
		if net.ParseIP(req.IP) == nil {
			http.Error(w, "invalid IP address", http.StatusBadRequest)
			return
		}

		if err := security.ClearIPBlock(req.IP); err != nil {
			http.Error(w, "server error", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"unblocked": req.IP,
		})
	}
}

// HandleClearAllBlocks removes all IP blocks
func HandleClearAllBlocks() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		affected, err := security.ClearAllIPBlocks()
		if err != nil {
			http.Error(w, "Failed to clear IP blocks", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"message": fmt.Sprintf("Successfully cleared %d IP blocks", affected),
			"cleared": affected,
		})
	}
}

// HandleListWhitelist returns all whitelisted IPs
func HandleListWhitelist() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		list, err := security.ListWhitelist()
		if err != nil {
			http.Error(w, "Failed to load whitelist", http.StatusInternalServerError)
			return
		}
		if list == nil {
			list = []map[string]interface{}{}
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"whitelist": list,
		})
	}
}

// HandleAddToWhitelist adds an IP to the whitelist
func HandleAddToWhitelist() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			IP   string `json:"ip"`
			Note string `json:"note"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.IP == "" {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}

		// Validate IP address or CIDR
		if net.ParseIP(req.IP) == nil {
			if _, _, err := net.ParseCIDR(req.IP); err != nil {
				http.Error(w, "invalid IP address or CIDR", http.StatusBadRequest)
				return
			}
		}

		if err := security.AddToWhitelist(req.IP, req.Note); err != nil {
			http.Error(w, "Failed to add to whitelist", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"ok": true,
			"ip": req.IP,
		})
	}
}

// HandleRemoveFromWhitelist removes an IP from the whitelist
func HandleRemoveFromWhitelist() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			IP string `json:"ip"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.IP == "" {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}

		if err := security.RemoveFromWhitelist(req.IP); err != nil {
			http.Error(w, "Failed to remove from whitelist", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"ok": true,
			"ip": req.IP,
		})
	}
}

// HandleListBlacklist returns all blacklisted IPs
func HandleListBlacklist() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		list, err := security.ListBlacklist()
		if err != nil {
			http.Error(w, "Failed to load blacklist", http.StatusInternalServerError)
			return
		}
		if list == nil {
			list = []map[string]interface{}{}
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"blacklist": list,
		})
	}
}

// HandleAddToBlacklist adds an IP to the blacklist
func HandleAddToBlacklist() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			IP        string `json:"ip"`
			Note      string `json:"note"`
			Permanent bool   `json:"permanent"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.IP == "" {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}

		// Validate IP address or CIDR
		if net.ParseIP(req.IP) == nil {
			if _, _, err := net.ParseCIDR(req.IP); err != nil {
				http.Error(w, "invalid IP address or CIDR", http.StatusBadRequest)
				return
			}
		}

		if err := security.AddToBlacklist(req.IP, req.Note, req.Permanent); err != nil {
			http.Error(w, "Failed to add to blacklist", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"ok":        true,
			"ip":        req.IP,
			"permanent": req.Permanent,
		})
	}
}

// HandleRemoveFromBlacklist removes an IP from the blacklist
func HandleRemoveFromBlacklist() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			IP string `json:"ip"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.IP == "" {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}

		if err := security.RemoveFromBlacklist(req.IP); err != nil {
			http.Error(w, "Failed to remove from blacklist", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"ok": true,
			"ip": req.IP,
		})
	}
}
