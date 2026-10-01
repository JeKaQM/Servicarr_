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
