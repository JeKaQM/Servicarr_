package handlers

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"status/app/internal/database"
	"status/app/internal/models"
)

// ServiceTemplates defines presets for popular services
var ServiceTemplates = []models.ServiceTemplate{
	{
		Type:          "plex",
		Name:          "Plex",
		Icon:          "plex",
		IconURL:       "https://raw.githubusercontent.com/walkxcode/dashboard-icons/main/svg/plex.svg",
		DefaultURL:    "http://localhost:32400",
		CheckType:     "http",
		URLSuffix:     "/identity",
		RequiresToken: true,
		TokenHeader:   "X-Plex-Token",
		HelpText:      "Enter your Plex server URL and token. The token can be found in Plex settings.",
	},
	{
		Type:          "overseerr",
		Name:          "Overseerr",
		Icon:          "overseerr",
		IconURL:       "https://raw.githubusercontent.com/walkxcode/dashboard-icons/main/svg/overseerr.svg",
		DefaultURL:    "http://localhost:5055",
		CheckType:     "http",
		URLSuffix:     "/api/v1/status",
		RequiresToken: false,
		HelpText:      "Enter your Overseerr URL. No API key required for status check.",
	},
	{
		Type:          "jellyfin",
		Name:          "Jellyfin",
		Icon:          "jellyfin",
		IconURL:       "https://raw.githubusercontent.com/walkxcode/dashboard-icons/main/svg/jellyfin.svg",
		DefaultURL:    "http://localhost:8096",
		CheckType:     "http",
		URLSuffix:     "/System/Ping",
		RequiresToken: false,
		HelpText:      "Enter your Jellyfin server URL.",
	},
	{
		Type:          "emby",
		Name:          "Emby",
		Icon:          "emby",
		IconURL:       "https://raw.githubusercontent.com/walkxcode/dashboard-icons/main/svg/emby.svg",
		DefaultURL:    "http://localhost:8096",
		CheckType:     "http",
		URLSuffix:     "/System/Ping",
		RequiresToken: true,
		TokenHeader:   "X-Emby-Token",
		HelpText:      "Enter your Emby server URL and API key.",
	},
	{
		Type:          "sonarr",
		Name:          "Sonarr",
		Icon:          "sonarr",
		IconURL:       "https://raw.githubusercontent.com/walkxcode/dashboard-icons/main/svg/sonarr.svg",
		DefaultURL:    "http://localhost:8989",
		CheckType:     "http",
		URLSuffix:     "/api/v3/system/status",
		RequiresToken: true,
		TokenHeader:   "X-Api-Key",
		HelpText:      "Enter your Sonarr URL and API key from Settings > General.",
	},
	{
		Type:          "radarr",
		Name:          "Radarr",
		Icon:          "radarr",
		IconURL:       "https://raw.githubusercontent.com/walkxcode/dashboard-icons/main/svg/radarr.svg",
		DefaultURL:    "http://localhost:7878",
		CheckType:     "http",
		URLSuffix:     "/api/v3/system/status",
		RequiresToken: true,
		TokenHeader:   "X-Api-Key",
		HelpText:      "Enter your Radarr URL and API key from Settings > General.",
	},
	{
		Type:          "prowlarr",
		Name:          "Prowlarr",
		Icon:          "prowlarr",
		IconURL:       "https://raw.githubusercontent.com/walkxcode/dashboard-icons/main/svg/prowlarr.svg",
		DefaultURL:    "http://localhost:9696",
		CheckType:     "http",
		URLSuffix:     "/api/v1/system/status",
		RequiresToken: true,
		TokenHeader:   "X-Api-Key",
		HelpText:      "Enter your Prowlarr URL and API key from Settings > General.",
	},
	{
		Type:          "lidarr",
		Name:          "Lidarr",
		Icon:          "lidarr",
		IconURL:       "https://raw.githubusercontent.com/walkxcode/dashboard-icons/main/svg/lidarr.svg",
		DefaultURL:    "http://localhost:8686",
		CheckType:     "http",
		URLSuffix:     "/api/v1/system/status",
		RequiresToken: true,
		TokenHeader:   "X-Api-Key",
		HelpText:      "Enter your Lidarr URL and API key from Settings > General.",
	},
	{
		Type:          "readarr",
		Name:          "Readarr",
		Icon:          "readarr",
		IconURL:       "https://raw.githubusercontent.com/walkxcode/dashboard-icons/main/svg/readarr.svg",
		DefaultURL:    "http://localhost:8787",
		CheckType:     "http",
		URLSuffix:     "/api/v1/system/status",
		RequiresToken: true,
		TokenHeader:   "X-Api-Key",
		HelpText:      "Enter your Readarr URL and API key from Settings > General.",
	},
	{
		Type:          "bazarr",
		Name:          "Bazarr",
		Icon:          "bazarr",
		IconURL:       "https://raw.githubusercontent.com/walkxcode/dashboard-icons/main/svg/bazarr.svg",
		DefaultURL:    "http://localhost:6767",
		CheckType:     "http",
		URLSuffix:     "/api/system/status",
		RequiresToken: true,
		TokenHeader:   "X-API-KEY",
		HelpText:      "Enter your Bazarr URL and API key from Settings > General.",
	},
	{
		Type:          "tautulli",
		Name:          "Tautulli",
		Icon:          "tautulli",
		IconURL:       "https://raw.githubusercontent.com/walkxcode/dashboard-icons/main/svg/tautulli.svg",
		DefaultURL:    "http://localhost:8181",
		CheckType:     "http",
		URLSuffix:     "/api/v2?cmd=status",
		RequiresToken: true,
		TokenHeader:   "apikey",
		HelpText:      "Enter your Tautulli URL and API key from Settings > Web Interface.",
	},
	{
		Type:          "sabnzbd",
		Name:          "SABnzbd",
		Icon:          "sabnzbd",
		IconURL:       "https://raw.githubusercontent.com/walkxcode/dashboard-icons/main/svg/sabnzbd.svg",
		DefaultURL:    "http://localhost:8080",
		CheckType:     "http",
		URLSuffix:     "/api?mode=version",
		RequiresToken: true,
		TokenHeader:   "apikey",
		HelpText:      "Enter your SABnzbd URL and API key from Config > General.",
	},
	{
		Type:          "qbittorrent",
		Name:          "qBittorrent",
		Icon:          "qbittorrent",
		IconURL:       "https://raw.githubusercontent.com/walkxcode/dashboard-icons/main/svg/qbittorrent.svg",
		DefaultURL:    "http://localhost:8080",
		CheckType:     "http",
		URLSuffix:     "/api/v2/app/version",
		RequiresToken: false,
		HelpText:      "Enter your qBittorrent Web UI URL.",
	},
	{
		Type:          "transmission",
		Name:          "Transmission",
		Icon:          "transmission",
		IconURL:       "https://raw.githubusercontent.com/walkxcode/dashboard-icons/main/svg/transmission.svg",
		DefaultURL:    "http://localhost:9091",
		CheckType:     "http",
		URLSuffix:     "/transmission/web/",
		RequiresToken: false,
		HelpText:      "Enter your Transmission Web UI URL.",
	},
	{
		Type:          "homeassistant",
		Name:          "Home Assistant",
		Icon:          "homeassistant",
		IconURL:       "https://raw.githubusercontent.com/walkxcode/dashboard-icons/main/svg/home-assistant.svg",
		DefaultURL:    "http://localhost:8123",
		CheckType:     "http",
		URLSuffix:     "/api/",
		RequiresToken: true,
		TokenHeader:   "Authorization",
		HelpText:      "Enter your Home Assistant URL and Long-Lived Access Token (prefix with 'Bearer ').",
	},
	{
		Type:          "pihole",
		Name:          "Pi-hole",
		Icon:          "pihole",
		IconURL:       "https://raw.githubusercontent.com/walkxcode/dashboard-icons/main/svg/pi-hole.svg",
		DefaultURL:    "http://localhost:80",
		CheckType:     "http",
		URLSuffix:     "/admin/api.php",
		RequiresToken: false,
		HelpText:      "Enter your Pi-hole URL.",
	},
	{
		Type:          "portainer",
		Name:          "Portainer",
		Icon:          "portainer",
		IconURL:       "https://raw.githubusercontent.com/walkxcode/dashboard-icons/main/svg/portainer.svg",
		DefaultURL:    "http://localhost:9000",
		CheckType:     "http",
		URLSuffix:     "/api/system/status",
		RequiresToken: false,
		HelpText:      "Enter your Portainer URL.",
	},
	{
		Type:          "server",
		Name:          "Server",
		Icon:          "server",
		IconURL:       "",
		DefaultURL:    "tcp://localhost:22",
		CheckType:     "tcp",
		URLSuffix:     "",
		RequiresToken: false,
		HelpText:      "Enter a TCP address to check (e.g., tcp://192.168.1.1:22 for SSH).",
	},
	{
		Type:          "website",
		Name:          "Website",
		Icon:          "globe",
		IconURL:       "",
		DefaultURL:    "https://example.com",
		CheckType:     "http",
		URLSuffix:     "",
		RequiresToken: false,
		HelpText:      "Enter any HTTP/HTTPS URL to monitor.",
	},
	{
		Type:          "custom",
		Name:          "Custom Service",
		Icon:          "custom",
		IconURL:       "",
		DefaultURL:    "http://localhost:8080",
		CheckType:     "http",
		URLSuffix:     "",
		RequiresToken: false,
		HelpText:      "Configure a custom service with your own settings.",
	},
}

// HandleGetServiceTemplates returns all available service templates
func HandleGetServiceTemplates(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(ServiceTemplates)
}

// HandleGetServices returns all services (admin: all, public: visible only)
func HandleGetServices(w http.ResponseWriter, r *http.Request) {
	// Check if admin
	isAdmin := r.URL.Query().Get("admin") == "true"

	var services []models.ServiceConfig
	var err error

	if isAdmin {
		services, err = database.GetAllServices()
	} else {
		services, err = database.GetVisibleServices()
	}

	if err != nil {
		http.Error(w, "Failed to load services", http.StatusInternalServerError)
		return
	}

	// Don't expose API tokens to non-admin
	if !isAdmin {
		for i := range services {
			services[i].APIToken = ""
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(services)
}

// HandleCreateService creates a new service
func HandleCreateService(w http.ResponseWriter, r *http.Request) {
	var s models.ServiceConfig
	if err := json.NewDecoder(r.Body).Decode(&s); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Validate required fields
	if s.Name == "" || s.URL == "" {
		http.Error(w, "Name and URL are required", http.StatusBadRequest)
		return
	}

	// Generate key from name if not provided
	if s.Key == "" {
		s.Key = generateServiceKey(s.Name)
	}

	// Check for duplicate key
	existing, _ := database.GetServiceByKey(s.Key)
	if existing != nil {
		http.Error(w, "A service with this key already exists", http.StatusConflict)
		return
	}

	// Set defaults
	if s.ServiceType == "" {
		s.ServiceType = "custom"
	}
	if s.CheckType == "" {
		s.CheckType = "http"
	}
	if s.CheckInterval == 0 {
		s.CheckInterval = 60
	}
	if s.Timeout == 0 {
		s.Timeout = 5
	}
	if s.ExpectedMin == 0 {
		s.ExpectedMin = 200
	}
	if s.ExpectedMax == 0 {
		s.ExpectedMax = 399
	}
	s.Visible = true
	// Auto-append to the end of the list
	s.DisplayOrder = -1

	id, err := database.CreateService(&s)
	if err != nil {
		http.Error(w, "Failed to create service: "+err.Error(), http.StatusInternalServerError)
		return
	}

	s.ID = int(id)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(s)
}

// HandleUpdateService updates an existing service
func HandleUpdateService(w http.ResponseWriter, r *http.Request) {
	// Get ID from query param (set by router)
	idStr := r.URL.Query().Get("_id")
	if idStr == "" {
		http.Error(w, "Missing service ID", http.StatusBadRequest)
		return
	}

	id, err := strconv.Atoi(idStr)
	if err != nil {
		http.Error(w, "Invalid service ID", http.StatusBadRequest)
		return
	}

	var s models.ServiceConfig
	if err := json.NewDecoder(r.Body).Decode(&s); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Ensure ID matches
	s.ID = id

	// Validate required fields
	if s.Name == "" || s.URL == "" {
		http.Error(w, "Name and URL are required", http.StatusBadRequest)
		return
	}

	// Check service exists
	existing, err := database.GetServiceByID(id)
	if err != nil || existing == nil {
		http.Error(w, "Service not found", http.StatusNotFound)
		return
	}

	// Keep the original key
	s.Key = existing.Key
	// Preserve display order (reordering handled separately)
	s.DisplayOrder = existing.DisplayOrder

	if err := database.UpdateService(&s); err != nil {
		http.Error(w, "Failed to update service", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(s)
}

// HandleDeleteService deletes a service
func HandleDeleteService(w http.ResponseWriter, r *http.Request) {
	// Get ID from query param (set by router)
	idStr := r.URL.Query().Get("_id")
	if idStr == "" {
		http.Error(w, "Missing service ID", http.StatusBadRequest)
		return
	}

	id, err := strconv.Atoi(idStr)
	if err != nil {
		http.Error(w, "Invalid service ID", http.StatusBadRequest)
		return
	}

	// Check service exists
	existing, err := database.GetServiceByID(id)
	if err != nil || existing == nil {
		http.Error(w, "Service not found", http.StatusNotFound)
		return
	}

	if err := database.DeleteService(id); err != nil {
		http.Error(w, "Failed to delete service", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// HandleToggleServiceVisibility toggles service visibility
func HandleToggleServiceVisibility(w http.ResponseWriter, r *http.Request) {
	// Get ID from query param (set by router)
	idStr := r.URL.Query().Get("_id")
	if idStr == "" {
		http.Error(w, "Missing service ID", http.StatusBadRequest)
		return
	}

	id, err := strconv.Atoi(idStr)
	if err != nil {
		http.Error(w, "Invalid service ID", http.StatusBadRequest)
		return
	}

	var req struct {
		Visible bool `json:"visible"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if err := database.UpdateServiceVisibility(id, req.Visible); err != nil {
		http.Error(w, "Failed to update visibility", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

// HandleReorderServices updates the display order of services
func HandleReorderServices(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Orders map[int]int `json:"orders"` // map of service ID to display order
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if err := database.UpdateServiceOrder(req.Orders); err != nil {
		http.Error(w, "Failed to update order", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

// generateServiceKey creates a URL-safe key from a service name
func generateServiceKey(name string) string {
	// Convert to lowercase
	key := strings.ToLower(name)
	// Replace spaces and special chars with hyphens
	reg := regexp.MustCompile(`[^a-z0-9]+`)
	key = reg.ReplaceAllString(key, "-")
	// Trim hyphens from ends
	key = strings.Trim(key, "-")
	return key
}

// HandleTestServiceConnection tests if a service URL is reachable
func HandleTestServiceConnection(w http.ResponseWriter, r *http.Request) {
	var req struct {
		URL         string `json:"url"`
		APIToken    string `json:"api_token"`
		CheckType   string `json:"check_type"`
		Timeout     int    `json:"timeout"`
		ServiceType string `json:"service_type"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.URL == "" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]any{
			"success": false,
			"error":   "URL is required",
		})
		return
	}

	timeout := req.Timeout
	if timeout <= 0 {
		timeout = 5
	}

	result := testServiceConnection(req.URL, req.APIToken, req.CheckType, req.ServiceType, timeout)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

// testServiceConnection performs the actual connection test
func testServiceConnection(url, apiToken, checkType, serviceType string, timeout int) map[string]any {
	client := &http.Client{
		Timeout: time.Duration(timeout) * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			// Allow redirects but limit them
			if len(via) >= 10 {
				return http.ErrUseLastResponse
			}
			return nil
		},
	}

	// Always-up demo check
	if checkType == "always_up" || checkType == "demo" {
		return map[string]any{
			"success":     true,
			"status":      "Always up",
			"status_code": 200,
			"latency_ms":  0,
		}
	}

	// Handle TCP checks
	if checkType == "tcp" || strings.HasPrefix(url, "tcp://") {
		return testTCPConnection(url, timeout)
	}

	// Handle DNS checks
	if checkType == "dns" || strings.HasPrefix(url, "dns://") {
		return testDNSConnection(url, timeout)
	}

	// HTTP/HTTPS check
	start := time.Now()

	// For Plex, append token as query parameter
	testURL := url
	if apiToken != "" && serviceType == "plex" {
		if strings.Contains(testURL, "?") {
			testURL += "&X-Plex-Token=" + apiToken
		} else {
			testURL += "?X-Plex-Token=" + apiToken
		}
	}

	req, err := http.NewRequest("GET", testURL, nil)
	if err != nil {
		return map[string]any{
			"success": false,
			"error":   "Invalid URL: " + err.Error(),
		}
	}

	// Add common headers
	req.Header.Set("User-Agent", "Servicarr/1.0")
	req.Header.Set("Accept", "application/json")

	// Add API token based on service type
	if apiToken != "" {
		switch serviceType {
		case "plex":
			// Plex uses X-Plex-Token (already added as query param, but also add header)
			req.Header.Set("X-Plex-Token", apiToken)
		case "sonarr", "radarr", "lidarr", "readarr", "prowlarr", "bazarr":
			// *arr services use X-Api-Key
			req.Header.Set("X-Api-Key", apiToken)
		case "overseerr", "jellyseerr":
			// Overseerr/Jellyseerr use X-Api-Key
			req.Header.Set("X-Api-Key", apiToken)
		case "tautulli":
			// Tautulli uses apikey query param - append to URL
			if strings.Contains(req.URL.String(), "?") {
				req.URL.RawQuery += "&apikey=" + apiToken
			} else {
				req.URL.RawQuery = "apikey=" + apiToken
			}
		case "jellyfin", "emby":
			// Jellyfin/Emby use X-Emby-Token or api_key param
			req.Header.Set("X-Emby-Token", apiToken)
		default:
			// Generic: try common patterns
			req.Header.Set("X-Api-Key", apiToken)
			req.Header.Set("Authorization", "Bearer "+apiToken)
		}
	}

	resp, err := client.Do(req)
	latency := time.Since(start).Milliseconds()

	if err != nil {
		return map[string]any{
			"success":    false,
			"error":      "Connection failed: " + err.Error(),
			"latency_ms": latency,
		}
	}
	defer resp.Body.Close()

	// Consider 2xx and 3xx as success
	success := resp.StatusCode >= 200 && resp.StatusCode < 400

	result := map[string]any{
		"success":     success,
		"status_code": resp.StatusCode,
		"status":      resp.Status,
		"latency_ms":  latency,
	}

	if !success {
		result["error"] = "Unexpected status code: " + resp.Status
	}

	return result
}

// testTCPConnection tests a TCP connection
func testTCPConnection(url string, timeout int) map[string]any {
	// Parse TCP URL (tcp://host:port)
	address := strings.TrimPrefix(url, "tcp://")

	start := time.Now()
	conn, err := net.DialTimeout("tcp", address, time.Duration(timeout)*time.Second)
	latency := time.Since(start).Milliseconds()

	if err != nil {
		return map[string]any{
			"success":    false,
			"error":      "TCP connection failed: " + err.Error(),
			"latency_ms": latency,
		}
	}
	conn.Close()

	return map[string]any{
		"success":    true,
		"status":     "TCP port open",
		"latency_ms": latency,
	}
}

// testDNSConnection tests a DNS lookup
func testDNSConnection(url string, timeout int) map[string]any {
	// Parse DNS URL (dns://hostname)
	hostname := strings.TrimPrefix(url, "dns://")

	start := time.Now()
	addrs, err := net.LookupHost(hostname)
	latency := time.Since(start).Milliseconds()

	if err != nil {
		return map[string]any{
			"success":    false,
			"error":      "DNS lookup failed: " + err.Error(),
			"latency_ms": latency,
		}
	}

	if len(addrs) == 0 {
		return map[string]any{
			"success":    false,
			"error":      "DNS lookup returned no addresses",
			"latency_ms": latency,
		}
	}

	return map[string]any{
		"success":    true,
		"status":     fmt.Sprintf("Resolved to %s", strings.Join(addrs, ", ")),
		"latency_ms": latency,
	}
}
