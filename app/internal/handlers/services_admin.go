package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"status/app/internal/checker"
	"status/app/internal/crypto"
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
	serveServices(w, r, false)
}

// HandleGetAdminServices must only be registered behind authentication.
func HandleGetAdminServices(w http.ResponseWriter, r *http.Request) {
	serveServices(w, r, true)
}

func serveServices(w http.ResponseWriter, r *http.Request, isAdmin bool) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

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

	// Don't expose API tokens or internal URLs to non-admin
	if !isAdmin {
		filterPublicRelationships(services)
		for i := range services {
			services[i].APIToken = ""
			services[i].URL = ""
		}
	} else {
		// Admin sees masked tokens, never plaintext
		for i := range services {
			services[i].APIToken = crypto.MaskToken(services[i].APIToken)
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(services)
}

// Public relationships may only refer to services present in the public view.
func filterPublicRelationships(services []models.ServiceConfig) {
	visible := make(map[string]bool, len(services))
	for _, service := range services {
		visible[service.Key] = true
	}
	filter := func(value string) string {
		keys := make([]string, 0)
		for _, key := range strings.Split(value, ",") {
			key = strings.TrimSpace(key)
			if visible[key] {
				keys = append(keys, key)
			}
		}
		return strings.Join(keys, ",")
	}
	for i := range services {
		services[i].DependsOn = filter(services[i].DependsOn)
		services[i].ConnectedTo = filter(services[i].ConnectedTo)
	}
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
	if !serviceKeyPattern.MatchString(s.Key) {
		http.Error(w, "Service key must start with a letter or digit and use only lowercase letters, digits, hyphens or underscores (max 64)", http.StatusBadRequest)
		return
	}
	if err := validateServiceConfig(&s); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if !checkServiceLinks(w, &s) {
		return
	}

	id, err := database.CreateService(&s)
	if err != nil {
		http.Error(w, "Failed to create service: "+err.Error(), http.StatusInternalServerError)
		return
	}

	s.ID = int(id)
	// Mask token before sending response — never expose plaintext
	s.APIToken = crypto.MaskToken(s.APIToken)
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

	// If token is empty or looks like a masked value, keep the existing token
	if s.APIToken == "" || strings.HasPrefix(s.APIToken, "\u2022") {
		s.APIToken = existing.APIToken
	}
	if err := validateServiceConfig(&s); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if !checkServiceLinks(w, &s) {
		return
	}

	if err := database.UpdateService(&s); err != nil {
		http.Error(w, "Failed to update service", http.StatusInternalServerError)
		return
	}

	// Mask token before sending response — never expose plaintext
	s.APIToken = crypto.MaskToken(s.APIToken)
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
	// Keep keys within serviceKeyPattern's length for long names.
	if len(key) > 64 {
		key = strings.TrimRight(key[:64], "-")
	}
	return key
}

// serviceKeyPattern matches keys the UI generates from service names. Keys
// appear in element IDs and URLs, so arbitrary characters are not accepted.
var serviceKeyPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,63}$`)

// validateServiceConfig enforces the same bounds as the service form. Without
// them a single service with a very long timeout could stall every scheduler
// tick and public status refresh while its check waits. Keys are checked only
// on create: they are immutable afterwards, and services from the setup wizard
// or older backups may use keys that predate serviceKeyPattern.
func validateServiceConfig(s *models.ServiceConfig) error {
	s.Name = strings.TrimSpace(s.Name)
	s.URL = strings.TrimSpace(s.URL)
	switch {
	case s.Name == "" || s.URL == "":
		return errors.New("Name and URL are required")
	case len(s.Name) > 100:
		return errors.New("Name must be at most 100 characters")
	case s.CheckInterval < 10 || s.CheckInterval > 3600:
		return errors.New("Check interval must be between 10 and 3600 seconds")
	case s.Timeout < 1 || s.Timeout > 60:
		return errors.New("Timeout must be between 1 and 60 seconds")
	case s.ExpectedMin < 100 || s.ExpectedMax > 599 || s.ExpectedMin > s.ExpectedMax:
		return errors.New("Expected status range must be within 100-599 with min <= max")
	}
	switch s.CheckType {
	case "http":
		if !strings.HasPrefix(s.URL, "http://") && !strings.HasPrefix(s.URL, "https://") {
			return errors.New("HTTP checks need a URL starting with http:// or https://")
		}
		if err := checker.ValidateURLTarget(s.URL); err != nil {
			return err
		}
	case "tcp", "dns", "always_up":
	default:
		return errors.New("Check type must be http, tcp, dns or always_up")
	}
	return nil
}

// splitServiceKeys parses a comma-separated key list, dropping blanks and
// duplicates while keeping the original order.
func splitServiceKeys(value string) []string {
	keys := make([]string, 0)
	seen := make(map[string]bool)
	for _, key := range strings.Split(value, ",") {
		key = strings.TrimSpace(key)
		if key == "" || seen[key] {
			continue
		}
		seen[key] = true
		keys = append(keys, key)
	}
	return keys
}

// validateServiceLinks normalises depends_on and connected_to and rejects
// links to the service itself, to unknown services, and dependency loops.
// Loops matter beyond the topology view: alerts are suppressed while an
// upstream dependency is down, so two services that depend on each other
// would silence each other's alerts when both fail.
func validateServiceLinks(s *models.ServiceConfig, all []models.ServiceConfig) error {
	names := make(map[string]string, len(all))
	deps := make(map[string][]string, len(all))
	for _, other := range all {
		if other.Key == s.Key {
			continue
		}
		names[other.Key] = other.Name
		deps[other.Key] = splitServiceKeys(other.DependsOn)
	}
	nameOf := func(key string) string {
		if key == s.Key {
			return s.Name
		}
		return names[key]
	}
	normalise := func(field, value string) (string, error) {
		keys := splitServiceKeys(value)
		for _, key := range keys {
			if key == s.Key {
				return "", fmt.Errorf("%s can't be linked to itself in %s", s.Name, field)
			}
			if _, ok := names[key]; !ok {
				return "", fmt.Errorf("Unknown service %q in %s", key, field)
			}
		}
		return strings.Join(keys, ","), nil
	}

	var err error
	if s.DependsOn, err = normalise("depends on", s.DependsOn); err != nil {
		return err
	}
	if s.ConnectedTo, err = normalise("connected to", s.ConnectedTo); err != nil {
		return err
	}
	for _, dep := range splitServiceKeys(s.DependsOn) {
		path := dependencyPath(deps, dep, s.Key)
		if path == nil {
			continue
		}
		parts := []string{nameOf(path[0]) + " depends on " + nameOf(path[1])}
		for _, key := range path[2:] {
			parts = append(parts, "which depends on "+nameOf(key))
		}
		return fmt.Errorf("%s can't depend on %s: %s", s.Name, nameOf(dep), strings.Join(parts, ", "))
	}
	return nil
}

// checkServiceLinks validates s's links against the stored services and writes
// the error response itself; it reports whether the request may continue.
func checkServiceLinks(w http.ResponseWriter, s *models.ServiceConfig) bool {
	all, err := database.GetAllServices()
	if err != nil {
		http.Error(w, "Failed to load services", http.StatusInternalServerError)
		return false
	}
	if err := validateServiceLinks(s, all); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return false
	}
	return true
}

// dependencyPath returns the chain of keys from "from" to "target" following
// depends_on links, or nil when target is not reachable.
func dependencyPath(deps map[string][]string, from, target string) []string {
	prev := map[string]string{from: ""}
	queue := []string{from}
	for len(queue) > 0 {
		key := queue[0]
		queue = queue[1:]
		for _, next := range deps[key] {
			if _, seen := prev[next]; seen {
				continue
			}
			prev[next] = key
			if next == target {
				path := []string{next}
				for at := key; at != ""; at = prev[at] {
					path = append([]string{at}, path...)
				}
				return path
			}
			queue = append(queue, next)
		}
	}
	return nil
}

// HandleTestServiceConnection tests if a service URL is reachable
func HandleTestServiceConnection(w http.ResponseWriter, r *http.Request) {
	var req struct {
		URL         string `json:"url"`
		APIToken    string `json:"api_token"`
		CheckType   string `json:"check_type"`
		Timeout     int    `json:"timeout"`
		ServiceType string `json:"service_type"`
		ServiceID   int    `json:"service_id"`
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

	// If no token provided but a service_id is set, load the stored token
	if req.APIToken == "" && req.ServiceID > 0 {
		if svc, err := database.GetServiceByID(req.ServiceID); err == nil && svc != nil {
			req.APIToken = svc.APIToken // already decrypted by GetServiceByID
		}
	}

	timeout := req.Timeout
	if timeout <= 0 {
		timeout = 5
	}

	result := testServiceConnection(req.URL, req.APIToken, req.CheckType, req.ServiceType, timeout)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

// testServiceConnection shares the scheduler's target validation, redirect rules,
// credential handling, and timeouts so a preview exercises the real check.
func testServiceConnection(url, apiToken, checkType, serviceType string, timeout int) map[string]any {
	if timeout <= 0 {
		timeout = 5
	}
	if timeout > 60 {
		timeout = 60
	}
	ok, code, ms, errText := checker.Check(checker.CheckOptions{
		URL: url, APIToken: apiToken, CheckType: checkType, ServiceType: serviceType,
		Timeout: time.Duration(timeout) * time.Second, ExpectedMin: 200, ExpectedMax: 399,
	})
	result := map[string]any{"success": ok, "status_code": code, "latency_ms": ms}
	if errText != "" {
		result["error"] = checker.SanitizeError(errText)
	} else if !ok {
		result["error"] = "Unexpected status code: " + strconv.Itoa(code)
	}
	if code != 0 {
		result["status"] = strconv.Itoa(code) + " " + http.StatusText(code)
	} else if ok {
		result["status"] = "Connection successful"
	}
	return result
}
