package handlers

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"status/app/internal/database"
	"status/app/internal/models"
	"status/app/internal/resources"
)

func TestHandleGetPublicResourcesUIConfig_RedactsConnectionDetails(t *testing.T) {
	if err := database.Init(":memory:"); err != nil {
		t.Fatalf("database init: %v", err)
	}
	if err := database.SaveResourcesUIConfig(&models.ResourcesUIConfig{
		Enabled:    true,
		GlancesURL: "http://10.0.0.2:61208",
		NUTHost:    "10.0.0.3:3493",
		UPSName:    "apc",
		CPU:        true,
		UPS:        true,
	}); err != nil {
		t.Fatalf("save resources config: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/resources/config", nil)
	rr := httptest.NewRecorder()
	HandleGetPublicResourcesUIConfig().ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", rr.Code, rr.Body.String())
	}
	var body map[string]any
	if err := json.NewDecoder(rr.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if _, ok := body["glances_url"]; ok {
		t.Fatal("public config exposed glances_url")
	}
	if _, ok := body["nut_host"]; ok {
		t.Fatal("public config exposed nut_host")
	}
	if _, ok := body["ups_name"]; ok {
		t.Fatal("public config exposed ups_name")
	}
	if body["glances_configured"] != true || body["ups_configured"] != true {
		t.Fatalf("configured flags = %#v", body)
	}
}

func TestHandleTestResourcesConnection_UPSDoesNotPersistFormValues(t *testing.T) {
	if err := database.Init(":memory:"); err != nil {
		t.Fatalf("database init: %v", err)
	}
	original := &models.ResourcesUIConfig{Enabled: false, GlancesURL: "old-host:61208", CPU: true}
	if err := database.SaveResourcesUIConfig(original); err != nil {
		t.Fatalf("save original config: %v", err)
	}

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer listener.Close()

	done := make(chan error, 1)
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			done <- err
			return
		}
		defer conn.Close()
		reader := bufio.NewReader(conn)
		line, err := reader.ReadString('\n')
		if err != nil {
			done <- err
			return
		}
		if strings.TrimSpace(line) != "LIST VAR apc" {
			done <- fmt.Errorf("command = %q", line)
			return
		}
		_, err = fmt.Fprint(conn, "BEGIN LIST VAR apc\nVAR apc device.serial \"admin-visible-serial\"\nVAR apc ups.status \"OL\"\nEND LIST VAR apc\n")
		done <- err
	}()

	payload, err := json.Marshal(map[string]string{
		"source":   "ups",
		"nut_host": listener.Addr().String(),
		"ups_name": "apc",
	})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/admin/resources/test", bytes.NewReader(payload))
	rr := httptest.NewRecorder()
	HandleTestResourcesConnection().ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", rr.Code, rr.Body.String())
	}
	var tested resources.Snapshot
	if err := json.NewDecoder(rr.Body).Decode(&tested); err != nil {
		t.Fatalf("decode test response: %v", err)
	}
	if tested.UPS == nil || tested.UPS.Serial != "admin-visible-serial" {
		t.Fatalf("authenticated connection test omitted UPS diagnostics: %#v", tested.UPS)
	}
	stored, err := database.LoadResourcesUIConfig()
	if err != nil {
		t.Fatalf("load resources config: %v", err)
	}
	if stored.Enabled || stored.GlancesURL != "old-host:61208" || stored.NUTHost != "" || stored.UPSName != "" || stored.UPS {
		t.Fatalf("connection test persisted form values: %#v", stored)
	}
	if err := <-done; err != nil {
		t.Fatalf("server: %v", err)
	}
}

func TestHandleTestResourcesConnection_GlancesRequiresResourceData(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{}`)
	}))
	defer server.Close()

	payload, err := json.Marshal(map[string]string{
		"source":      "glances",
		"glances_url": server.URL,
	})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/admin/resources/test", bytes.NewReader(payload))
	rr := httptest.NewRecorder()
	HandleTestResourcesConnection().ServeHTTP(rr, req)

	if rr.Code != http.StatusBadGateway {
		t.Fatalf("status = %d body = %s", rr.Code, rr.Body.String())
	}
	var body map[string]any
	if err := json.NewDecoder(rr.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body["error"] != "glances_invalid_response" {
		t.Fatalf("error = %v, want glances_invalid_response", body["error"])
	}
}

func TestHandleResources_UPSOnly(t *testing.T) {
	if err := database.Init(":memory:"); err != nil {
		t.Fatalf("database init: %v", err)
	}

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer listener.Close()

	done := make(chan error, 1)
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			done <- err
			return
		}
		defer conn.Close()

		reader := bufio.NewReader(conn)
		line, err := reader.ReadString('\n')
		if err != nil {
			done <- err
			return
		}
		if strings.TrimSpace(line) != "LIST VAR apc" {
			done <- fmt.Errorf("command = %q", line)
			return
		}

		_, err = fmt.Fprint(conn, strings.Join([]string{
			`BEGIN LIST VAR apc`,
			`VAR apc battery.charge "92"`,
			`VAR apc battery.runtime "1200"`,
			`VAR apc device.mfr "Example Power Systems"`,
			`VAR apc device.model "Back-UPS BX750MI"`,
			`VAR apc device.serial "TEST-SERIAL-0001"`,
			`VAR apc ups.status "OL"`,
			`VAR apc ups.test.result "Done and passed"`,
			`END LIST VAR apc`,
			"",
		}, "\n"))
		done <- err
	}()

	cfg := &models.ResourcesUIConfig{
		Enabled: true,
		NUTHost: listener.Addr().String(),
		UPSName: "apc",
		UPS:     true,
	}
	if err := database.SaveResourcesUIConfig(cfg); err != nil {
		t.Fatalf("save resources config: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/resources", nil)
	rr := httptest.NewRecorder()
	HandleResources(nil).ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", rr.Code, rr.Body.String())
	}

	body := rr.Body.Bytes()
	var snap resources.Snapshot
	if err := json.NewDecoder(bytes.NewReader(body)).Decode(&snap); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if snap.UPS == nil {
		t.Fatal("expected UPS data")
	}
	if snap.UPS.Model != "Back-UPS BX750MI" {
		t.Fatalf("model = %q", snap.UPS.Model)
	}
	if snap.UPS.BatteryChargePercent == nil || *snap.UPS.BatteryChargePercent != 92 {
		t.Fatalf("charge = %v, want 92", snap.UPS.BatteryChargePercent)
	}
	if snap.UPS.PowerPresent == nil || !*snap.UPS.PowerPresent {
		t.Fatalf("power_present = %v, want true", snap.UPS.PowerPresent)
	}
	if snap.UPS.Manufacturer != "" || snap.UPS.Serial != "" || snap.UPS.TestResult != "" {
		t.Fatalf("public response exposed UPS identity details: %#v", snap.UPS)
	}
	if strings.Contains(string(body), "TEST-SERIAL-0001") || strings.Contains(string(body), "Done and passed") {
		t.Fatalf("public response contains private UPS diagnostics: %s", body)
	}

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("server: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("mock NUT server did not complete")
	}
}

func TestHandleResources_RequireUPSReturnsUPSErrorWhenGlancesSucceeds(t *testing.T) {
	if err := database.Init(":memory:"); err != nil {
		t.Fatalf("database init: %v", err)
	}

	glances := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/4/system":
			fmt.Fprint(w, `{"hostname":"test-host","platform":"Linux"}`)
		case "/api/4/cpu":
			fmt.Fprint(w, `{"total":10}`)
		case "/api/4/load":
			fmt.Fprint(w, `{"min1":0.1}`)
		case "/api/4/mem":
			fmt.Fprint(w, `{"total":1000,"used":500,"percent":50}`)
		case "/api/4/memswap":
			fmt.Fprint(w, `{"total":0,"used":0,"percent":0}`)
		case "/api/4/processcount":
			fmt.Fprint(w, `{"total":1}`)
		case "/api/4/sensors", "/api/4/network", "/api/4/percpu", "/api/4/diskio", "/api/4/fs", "/api/4/gpu", "/api/4/containers":
			fmt.Fprint(w, `[]`)
		case "/api/4/uptime":
			fmt.Fprint(w, `"1:00"`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer glances.Close()

	closedListener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen closed port: %v", err)
	}
	closedAddr := closedListener.Addr().String()
	_ = closedListener.Close()

	cfg := &models.ResourcesUIConfig{
		Enabled:    true,
		GlancesURL: glances.URL,
		NUTHost:    closedAddr,
		UPSName:    "apc",
		CPU:        true,
		UPS:        true,
	}
	if err := database.SaveResourcesUIConfig(cfg); err != nil {
		t.Fatalf("save resources config: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/resources?require=ups", nil)
	rr := httptest.NewRecorder()
	HandleResources(nil).ServeHTTP(rr, req)

	if rr.Code != http.StatusBadGateway {
		t.Fatalf("status = %d body = %s", rr.Code, rr.Body.String())
	}

	var body map[string]any
	if err := json.NewDecoder(rr.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body["error"] != "ups_unavailable" {
		t.Fatalf("error = %v, want ups_unavailable", body["error"])
	}
	if body["message"] == "" {
		t.Fatal("expected useful UPS error message")
	}
}

func TestHandleResources_RequireUPSIncludesKnownUPSNames(t *testing.T) {
	if err := database.Init(":memory:"); err != nil {
		t.Fatalf("database init: %v", err)
	}

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer listener.Close()

	done := make(chan error, 1)
	go func() {
		for i := 0; i < 2; i++ {
			conn, err := listener.Accept()
			if err != nil {
				done <- err
				return
			}

			reader := bufio.NewReader(conn)
			line, err := reader.ReadString('\n')
			if err != nil {
				conn.Close()
				done <- err
				return
			}

			switch strings.TrimSpace(line) {
			case "LIST VAR ups":
				_, err = fmt.Fprint(conn, "ERR UNKNOWN-UPS\n")
			case "LIST UPS":
				_, err = fmt.Fprint(conn, strings.Join([]string{
					`BEGIN LIST UPS`,
					`UPS apc "Back-UPS BX750MI"`,
					`END LIST UPS`,
					"",
				}, "\n"))
			default:
				err = fmt.Errorf("command = %q", line)
			}
			conn.Close()
			if err != nil {
				done <- err
				return
			}
		}
		done <- nil
	}()

	cfg := &models.ResourcesUIConfig{
		Enabled: true,
		NUTHost: listener.Addr().String(),
		UPSName: "ups",
		UPS:     true,
	}
	if err := database.SaveResourcesUIConfig(cfg); err != nil {
		t.Fatalf("save resources config: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/resources?require=ups", nil)
	rr := httptest.NewRecorder()
	HandleResources(nil).ServeHTTP(rr, req)

	if rr.Code != http.StatusBadGateway {
		t.Fatalf("status = %d body = %s", rr.Code, rr.Body.String())
	}

	var body map[string]any
	if err := json.NewDecoder(rr.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	want := `NUT does not know UPS "ups"; known UPS name: apc`
	if body["message"] != want {
		t.Fatalf("message = %v, want %q", body["message"], want)
	}

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("server: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("mock NUT server did not complete")
	}
}
