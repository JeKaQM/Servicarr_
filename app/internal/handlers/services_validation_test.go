package handlers

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"status/app/internal/models"
)

func validService() models.ServiceConfig {
	return models.ServiceConfig{
		Key: "plex", Name: "Plex", URL: "http://plex.local:32400", CheckType: "http",
		CheckInterval: 60, Timeout: 5, ExpectedMin: 200, ExpectedMax: 399,
	}
}

func TestValidateServiceConfig(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*models.ServiceConfig)
		wantErr string
	}{
		{"valid http", func(*models.ServiceConfig) {}, ""},
		{"valid tcp", func(s *models.ServiceConfig) { s.CheckType, s.URL = "tcp", "tcp://nas.local:22" }, ""},
		{"valid always up", func(s *models.ServiceConfig) { s.CheckType = "always_up" }, ""},
		{"timeout too long", func(s *models.ServiceConfig) { s.Timeout = 600 }, "Timeout"},
		{"timeout zero", func(s *models.ServiceConfig) { s.Timeout = 0 }, "Timeout"},
		{"interval too short", func(s *models.ServiceConfig) { s.CheckInterval = 1 }, "interval"},
		{"status range inverted", func(s *models.ServiceConfig) { s.ExpectedMin, s.ExpectedMax = 400, 200 }, "status range"},
		{"unknown check type", func(s *models.ServiceConfig) { s.CheckType = "icmp" }, "Check type"},
		{"http without scheme", func(s *models.ServiceConfig) { s.URL = "plex.local:32400" }, "http://"},
		{"metadata target", func(s *models.ServiceConfig) { s.URL = "http://169.254.169.254/latest" }, "metadata"},
		{"legacy key still editable", func(s *models.ServiceConfig) { s.Key = "Legacy Key From Setup" }, ""},
		{"blank name", func(s *models.ServiceConfig) { s.Name = "   " }, "required"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := validService()
			tt.mutate(&svc)
			err := validateServiceConfig(&svc)
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("error = %v, want one mentioning %q", err, tt.wantErr)
			}
		})
	}
}

func TestCreateServiceRejectsUnboundedTimeout(t *testing.T) {
	initMaintenanceHandlerDB(t)
	body := `{"name":"Slow","url":"http://slow.local","check_type":"http","timeout":600,"check_interval":60}`
	recorder := httptest.NewRecorder()
	HandleCreateService(recorder, httptest.NewRequest(http.MethodPost, "/api/admin/services", strings.NewReader(body)))
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body = %s", recorder.Code, recorder.Body.String())
	}
}

func TestCreateServiceRejectsUnsafeKeyButDerivesLongNames(t *testing.T) {
	initMaintenanceHandlerDB(t)
	unsafe := `{"key":"x\" onmouseover=\"alert(1)","name":"Bad","url":"http://bad.local","check_type":"http"}`
	recorder := httptest.NewRecorder()
	HandleCreateService(recorder, httptest.NewRequest(http.MethodPost, "/api/admin/services", strings.NewReader(unsafe)))
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("unsafe key: status = %d, want 400", recorder.Code)
	}

	longName := strings.Repeat("Very Long Service Name ", 4) // over 64 characters once slugged
	body := `{"name":"` + longName + `","url":"http://long.local","check_type":"http"}`
	recorder = httptest.NewRecorder()
	HandleCreateService(recorder, httptest.NewRequest(http.MethodPost, "/api/admin/services", strings.NewReader(body)))
	if recorder.Code != http.StatusCreated {
		t.Fatalf("long name: status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
}

func linkedServices() []models.ServiceConfig {
	return []models.ServiceConfig{
		{Key: "router", Name: "Router"},
		{Key: "nas", Name: "NAS", DependsOn: "router"},
		{Key: "plex", Name: "Plex", DependsOn: "nas"},
		{Key: "overseerr", Name: "Overseerr", DependsOn: "plex", ConnectedTo: "sonarr"},
		{Key: "sonarr", Name: "Sonarr", DependsOn: "nas"},
	}
}

func TestValidateServiceLinks(t *testing.T) {
	tests := []struct {
		name        string
		svc         models.ServiceConfig
		wantErr     string
		wantDepends string
		wantConns   string
	}{
		{"new service with links", models.ServiceConfig{Key: "radarr", Name: "Radarr", DependsOn: "nas, router", ConnectedTo: "plex"}, "", "nas,router", "plex"},
		{"duplicates and blanks are dropped", models.ServiceConfig{Key: "radarr", Name: "Radarr", DependsOn: " nas,,nas ,", ConnectedTo: ""}, "", "nas", ""},
		{"edit keeps existing links", models.ServiceConfig{Key: "plex", Name: "Plex", DependsOn: "nas"}, "", "nas", ""},
		{"self dependency", models.ServiceConfig{Key: "plex", Name: "Plex", DependsOn: "plex"}, "itself", "", ""},
		{"self connection", models.ServiceConfig{Key: "plex", Name: "Plex", ConnectedTo: "plex"}, "itself", "", ""},
		{"unknown service", models.ServiceConfig{Key: "plex", Name: "Plex", DependsOn: "gone"}, `Unknown service "gone"`, "", ""},
		{"direct loop", models.ServiceConfig{Key: "plex", Name: "Plex", DependsOn: "nas,overseerr"}, "Plex can't depend on Overseerr: Overseerr depends on Plex", "", ""},
		{"indirect loop", models.ServiceConfig{Key: "router", Name: "Router", DependsOn: "overseerr"}, "Overseerr depends on Plex, which depends on NAS, which depends on Router", "", ""},
		{"peer links never loop", models.ServiceConfig{Key: "sonarr", Name: "Sonarr", DependsOn: "nas", ConnectedTo: "overseerr"}, "", "nas", "overseerr"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := tt.svc
			err := validateServiceLinks(&svc, linkedServices())
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("validateServiceLinks() error = %v, want it to contain %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("validateServiceLinks() unexpected error: %v", err)
			}
			if svc.DependsOn != tt.wantDepends || svc.ConnectedTo != tt.wantConns {
				t.Fatalf("normalised links = %q / %q, want %q / %q", svc.DependsOn, svc.ConnectedTo, tt.wantDepends, tt.wantConns)
			}
		})
	}
}

func TestValidateServiceLinksToleratesExistingLoops(t *testing.T) {
	// Older data may already contain a loop between other services; checking
	// an unrelated service must still terminate and pass.
	all := append(linkedServices(),
		models.ServiceConfig{Key: "a", Name: "A", DependsOn: "b"},
		models.ServiceConfig{Key: "b", Name: "B", DependsOn: "a"},
	)
	svc := models.ServiceConfig{Key: "radarr", Name: "Radarr", DependsOn: "a"}
	if err := validateServiceLinks(&svc, all); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
