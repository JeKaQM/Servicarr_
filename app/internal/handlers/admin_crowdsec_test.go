package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"status/app/internal/crypto"
	"status/app/internal/database"
)

func initCrowdSecHandlerDB(t *testing.T) {
	t.Helper()
	if err := database.Init(":memory:"); err != nil {
		t.Fatal(err)
	}
	crypto.SetKey([]byte("test-encryption-key-for-crowdsec"))
}

func TestGetCrowdSecConfig_Defaults(t *testing.T) {
	initCrowdSecHandlerDB(t)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/admin/crowdsec/config", nil)
	HandleGetCrowdSecConfig().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d", recorder.Code)
	}
	var response struct {
		Enabled            bool   `json:"enabled"`
		LAPIURL            string `json:"lapi_url"`
		PollIntervalS      int    `json:"poll_interval_seconds"`
		MachinePasswordSet bool   `json:"machine_password_configured"`
		BouncerKeySet      bool   `json:"bouncer_api_key_configured"`
	}
	if err := json.NewDecoder(recorder.Body).Decode(&response); err != nil {
		t.Fatal(err)
	}
	if response.Enabled || response.LAPIURL != "" || response.MachinePasswordSet || response.BouncerKeySet {
		t.Fatalf("unexpected defaults: %+v", response)
	}
	if response.PollIntervalS != 30 {
		t.Errorf("default poll interval = %d, want 30", response.PollIntervalS)
	}
}

func TestSaveCrowdSecConfig_ValidatesURL(t *testing.T) {
	initCrowdSecHandlerDB(t)
	body := `{"enabled":true,"lapi_url":"http://169.254.169.254","bouncer_api_key":"k","poll_interval_seconds":30}`
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/admin/crowdsec/config", strings.NewReader(body))
	HandleSaveCrowdSecConfig().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("cloud metadata URL must be rejected, status = %d", recorder.Code)
	}
}

func TestSaveCrowdSecConfig_RequiresCredentialWhenEnabled(t *testing.T) {
	initCrowdSecHandlerDB(t)
	body := `{"enabled":true,"lapi_url":"http://10.0.0.5:8080","poll_interval_seconds":30}`
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/admin/crowdsec/config", strings.NewReader(body))
	HandleSaveCrowdSecConfig().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("enable without credentials must be rejected, status = %d", recorder.Code)
	}
}

func TestSaveCrowdSecConfig_ValidatesMapHome(t *testing.T) {
	cases := []struct {
		name string
		lat  string
		lng  string
		want int
	}{
		{"valid coords", "51.5074", "-0.1278", http.StatusOK},
		{"latitude out of range", "91", "0", http.StatusBadRequest},
		{"latitude below range", "-90.5", "0", http.StatusBadRequest},
		{"longitude out of range", "0", "181", http.StatusBadRequest},
		{"longitude below range", "0", "-180.5", http.StatusBadRequest},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			initCrowdSecHandlerDB(t)
			body := `{"enabled":false,"lapi_url":"http://10.0.0.5:8080","bouncer_api_key":"k","poll_interval_seconds":30,"map_home_latitude":` + tc.lat + `,"map_home_longitude":` + tc.lng + `}`
			recorder := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodPost, "/api/admin/crowdsec/config", strings.NewReader(body))
			HandleSaveCrowdSecConfig().ServeHTTP(recorder, request)
			if recorder.Code != tc.want {
				t.Fatalf("status = %d, want %d (body %s)", recorder.Code, tc.want, recorder.Body.String())
			}
			if tc.want == http.StatusOK {
				cfg, err := database.LoadCrowdSecConfig()
				if err != nil || cfg == nil {
					t.Fatalf("load: %v %v", cfg, err)
				}
				if cfg.MapHomeLat != parseTestFloat(t, tc.lat) || cfg.MapHomeLng != parseTestFloat(t, tc.lng) {
					t.Errorf("map home roundtrip failed: (%v, %v)", cfg.MapHomeLat, cfg.MapHomeLng)
				}
			}
		})
	}
}

func parseTestFloat(t *testing.T, s string) float64 {
	t.Helper()
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		t.Fatal(err)
	}
	return v
}

func TestSaveCrowdSecConfig_Roundtrip_MasksSecrets(t *testing.T) {
	initCrowdSecHandlerDB(t)
	body := `{"enabled":true,"lapi_url":"http://10.0.0.5:8080/v1","bouncer_api_key":"secret-key","machine_id":"m1","machine_password":"mp","poll_interval_seconds":45}`
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/admin/crowdsec/config", strings.NewReader(body))
	HandleSaveCrowdSecConfig().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("save status = %d, body %s", recorder.Code, recorder.Body.String())
	}

	// The stored URL must be normalized (/v1 stripped).
	cfg, err := database.LoadCrowdSecConfig()
	if err != nil || cfg == nil {
		t.Fatalf("load: %v %v", cfg, err)
	}
	if cfg.LAPIURL != "http://10.0.0.5:8080" {
		t.Errorf("URL normalization failed: %q", cfg.LAPIURL)
	}
	if cfg.BouncerAPIKey != "secret-key" || cfg.MachinePassword != "mp" {
		t.Errorf("secret roundtrip failed: %+v", cfg)
	}

	// GET must never return secrets — only *_configured flags.
	recorder = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodGet, "/api/admin/crowdsec/config", nil)
	HandleGetCrowdSecConfig().ServeHTTP(recorder, request)
	bodyBytes := recorder.Body.String()
	if strings.Contains(bodyBytes, "secret-key") || strings.Contains(bodyBytes, "mp") {
		t.Errorf("GET response leaks secrets: %s", bodyBytes)
	}
	var response struct {
		BouncerKeySet      bool `json:"bouncer_api_key_configured"`
		MachinePasswordSet bool `json:"machine_password_configured"`
	}
	if err := json.NewDecoder(strings.NewReader(bodyBytes)).Decode(&response); err != nil {
		t.Fatal(err)
	}
	if !response.BouncerKeySet || !response.MachinePasswordSet {
		t.Errorf("configured flags missing: %s", bodyBytes)
	}
}

func TestSaveCrowdSecConfig_PreservesEmptySecrets(t *testing.T) {
	initCrowdSecHandlerDB(t)
	// Save with a key.
	first := `{"enabled":true,"lapi_url":"http://10.0.0.5:8080","bouncer_api_key":"secret-key","poll_interval_seconds":30}`
	recorder := httptest.NewRecorder()
	HandleSaveCrowdSecConfig().ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/api/admin/crowdsec/config", strings.NewReader(first)))
	if recorder.Code != http.StatusOK {
		t.Fatalf("first save status = %d", recorder.Code)
	}

	// Second save with EMPTY key and no clear flag must preserve the stored key.
	second := `{"enabled":true,"lapi_url":"http://10.0.0.5:8080","poll_interval_seconds":60}`
	recorder = httptest.NewRecorder()
	HandleSaveCrowdSecConfig().ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/api/admin/crowdsec/config", strings.NewReader(second)))
	if recorder.Code != http.StatusOK {
		t.Fatalf("second save status = %d: %s", recorder.Code, recorder.Body.String())
	}

	cfg, err := database.LoadCrowdSecConfig()
	if err != nil || cfg == nil {
		t.Fatalf("load: %v %v", cfg, err)
	}
	if cfg.BouncerAPIKey != "secret-key" {
		t.Errorf("preserve-on-empty failed: key = %q", cfg.BouncerAPIKey)
	}
	if cfg.PollIntervalS != 60 {
		t.Errorf("interval not updated: %d", cfg.PollIntervalS)
	}
}

func TestSaveCrowdSecConfig_ClearFlagsRemoveSecrets(t *testing.T) {
	initCrowdSecHandlerDB(t)
	first := `{"enabled":true,"lapi_url":"http://10.0.0.5:8080","bouncer_api_key":"secret-key","poll_interval_seconds":30}`
	HandleSaveCrowdSecConfig().ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "/api/admin/crowdsec/config", strings.NewReader(first)))

	clear := `{"enabled":false,"lapi_url":"http://10.0.0.5:8080","clear_bouncer_api_key":true,"poll_interval_seconds":30}`
	recorder := httptest.NewRecorder()
	HandleSaveCrowdSecConfig().ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/api/admin/crowdsec/config", strings.NewReader(clear)))
	if recorder.Code != http.StatusOK {
		t.Fatalf("clear save status = %d", recorder.Code)
	}

	cfg, err := database.LoadCrowdSecConfig()
	if err != nil || cfg == nil {
		t.Fatalf("load: %v %v", cfg, err)
	}
	if cfg.BouncerAPIKey != "" {
		t.Errorf("clear flag did not remove key: %q", cfg.BouncerAPIKey)
	}
}

func TestSaveCrowdSecConfig_ClampsInterval(t *testing.T) {
	initCrowdSecHandlerDB(t)
	body := `{"enabled":true,"lapi_url":"http://10.0.0.5:8080","bouncer_api_key":"k","poll_interval_seconds":5}`
	recorder := httptest.NewRecorder()
	HandleSaveCrowdSecConfig().ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/api/admin/crowdsec/config", strings.NewReader(body)))
	if recorder.Code != http.StatusOK {
		t.Fatalf("save status = %d", recorder.Code)
	}
	cfg, err := database.LoadCrowdSecConfig()
	if err != nil || cfg == nil {
		t.Fatalf("load: %v %v", cfg, err)
	}
	if cfg.PollIntervalS != 10 {
		t.Errorf("interval not clamped to 10: %d", cfg.PollIntervalS)
	}
}

func TestGetCrowdSecDecisions_ReturnsNonNilArray(t *testing.T) {
	initCrowdSecHandlerDB(t)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/admin/crowdsec/decisions", nil)
	HandleGetCrowdSecDecisions().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d", recorder.Code)
	}
	// Empty result must marshal as [], not null.
	if strings.Contains(recorder.Body.String(), ": null") {
		t.Errorf("null decisions array: %s", recorder.Body.String())
	}
}
