package resources

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

// --------------- asFloatPtr tests ---------------

func TestAsFloatPtr_Nil(t *testing.T) {
	if got := asFloatPtr(nil); got != nil {
		t.Fatalf("expected nil, got %v", *got)
	}
}

func TestAsFloatPtr_Float64(t *testing.T) {
	got := asFloatPtr(float64(3.14))
	if got == nil || *got != 3.14 {
		t.Fatalf("expected 3.14, got %v", got)
	}
}

func TestAsFloatPtr_Float64_Zero(t *testing.T) {
	got := asFloatPtr(float64(0))
	if got == nil || *got != 0 {
		t.Fatalf("expected 0, got %v", got)
	}
}

func TestAsFloatPtr_Float64_Negative(t *testing.T) {
	got := asFloatPtr(float64(-42.5))
	if got == nil || *got != -42.5 {
		t.Fatalf("expected -42.5, got %v", got)
	}
}

func TestAsFloatPtr_String_ValidNumber(t *testing.T) {
	got := asFloatPtr("123.456")
	if got == nil || *got != 123.456 {
		t.Fatalf("expected 123.456, got %v", got)
	}
}

func TestAsFloatPtr_String_Invalid(t *testing.T) {
	got := asFloatPtr("not-a-number")
	if got != nil {
		t.Fatalf("expected nil for invalid string, got %v", *got)
	}
}

func TestAsFloatPtr_String_Empty(t *testing.T) {
	got := asFloatPtr("")
	if got != nil {
		t.Fatalf("expected nil for empty string, got %v", *got)
	}
}

func TestAsFloatPtr_Int(t *testing.T) {
	got := asFloatPtr(int(42))
	if got == nil || *got != 42.0 {
		t.Fatalf("expected 42, got %v", got)
	}
}

func TestAsFloatPtr_Int_Negative(t *testing.T) {
	got := asFloatPtr(int(-7))
	if got == nil || *got != -7.0 {
		t.Fatalf("expected -7, got %v", got)
	}
}

func TestAsFloatPtr_Int64(t *testing.T) {
	got := asFloatPtr(int64(999))
	if got == nil || *got != 999.0 {
		t.Fatalf("expected 999, got %v", got)
	}
}

func TestAsFloatPtr_JSONNumber_Valid(t *testing.T) {
	got := asFloatPtr(json.Number("78.9"))
	if got == nil || *got != 78.9 {
		t.Fatalf("expected 78.9, got %v", got)
	}
}

func TestAsFloatPtr_JSONNumber_Invalid(t *testing.T) {
	got := asFloatPtr(json.Number("xyz"))
	if got != nil {
		t.Fatalf("expected nil for invalid json.Number, got %v", *got)
	}
}

func TestAsFloatPtr_UnknownType(t *testing.T) {
	got := asFloatPtr(struct{}{})
	if got != nil {
		t.Fatalf("expected nil for unknown type, got %v", *got)
	}
}

func TestAsFloatPtr_Bool(t *testing.T) {
	got := asFloatPtr(true)
	if got != nil {
		t.Fatalf("expected nil for bool, got %v", *got)
	}
}

// --------------- asUint64Ptr tests ---------------

func TestAsUint64Ptr_Nil(t *testing.T) {
	if got := asUint64Ptr(nil); got != nil {
		t.Fatalf("expected nil, got %v", *got)
	}
}

func TestAsUint64Ptr_Float64_Positive(t *testing.T) {
	got := asUint64Ptr(float64(100))
	if got == nil || *got != 100 {
		t.Fatalf("expected 100, got %v", got)
	}
}

func TestAsUint64Ptr_Float64_Zero(t *testing.T) {
	got := asUint64Ptr(float64(0))
	if got == nil || *got != 0 {
		t.Fatalf("expected 0, got %v", got)
	}
}

func TestAsUint64Ptr_Float64_Negative(t *testing.T) {
	got := asUint64Ptr(float64(-1))
	if got != nil {
		t.Fatalf("expected nil for negative float64, got %v", *got)
	}
}

func TestAsUint64Ptr_Float64_Large(t *testing.T) {
	got := asUint64Ptr(float64(1e18))
	if got == nil || *got != uint64(1e18) {
		t.Fatalf("expected 1e18, got %v", got)
	}
}

func TestAsUint64Ptr_Int_Positive(t *testing.T) {
	got := asUint64Ptr(int(42))
	if got == nil || *got != 42 {
		t.Fatalf("expected 42, got %v", got)
	}
}

func TestAsUint64Ptr_Int_Negative(t *testing.T) {
	got := asUint64Ptr(int(-5))
	if got != nil {
		t.Fatalf("expected nil for negative int, got %v", *got)
	}
}

func TestAsUint64Ptr_Int64_Positive(t *testing.T) {
	got := asUint64Ptr(int64(999))
	if got == nil || *got != 999 {
		t.Fatalf("expected 999, got %v", got)
	}
}

func TestAsUint64Ptr_Int64_Negative(t *testing.T) {
	got := asUint64Ptr(int64(-10))
	if got != nil {
		t.Fatalf("expected nil for negative int64, got %v", *got)
	}
}

func TestAsUint64Ptr_JSONNumber_Valid(t *testing.T) {
	got := asUint64Ptr(json.Number("256"))
	if got == nil || *got != 256 {
		t.Fatalf("expected 256, got %v", got)
	}
}

func TestAsUint64Ptr_JSONNumber_Negative(t *testing.T) {
	got := asUint64Ptr(json.Number("-100"))
	if got != nil {
		t.Fatalf("expected nil for negative json.Number, got %v", *got)
	}
}

func TestAsUint64Ptr_JSONNumber_Invalid(t *testing.T) {
	got := asUint64Ptr(json.Number("abc"))
	if got != nil {
		t.Fatalf("expected nil for invalid json.Number, got %v", *got)
	}
}

func TestAsUint64Ptr_UnknownType(t *testing.T) {
	got := asUint64Ptr("some string")
	if got != nil {
		t.Fatalf("expected nil for string type, got %v", *got)
	}
}

// --------------- NewClient tests ---------------

func TestNewClient_Defaults(t *testing.T) {
	c := NewClient("http://localhost:61208/api/4")
	if c.BaseURL != "http://localhost:61208/api/4" {
		t.Fatalf("wrong BaseURL: %s", c.BaseURL)
	}
	if c.HTTP == nil {
		t.Fatal("HTTP client is nil")
	}
	if c.HTTP.Timeout != 6*time.Second {
		t.Fatalf("unexpected timeout: %v", c.HTTP.Timeout)
	}
	if c.cacheFor != 5*time.Second {
		t.Fatalf("unexpected cacheFor: %v", c.cacheFor)
	}
	if c.inFlightC == nil {
		t.Fatal("inFlightC condition is nil")
	}
}

func TestNewClient_EmptyURL(t *testing.T) {
	c := NewClient("")
	if c.BaseURL != "" {
		t.Fatalf("expected empty BaseURL, got %s", c.BaseURL)
	}
}

// --------------- SetCacheTTL tests ---------------

func TestSetCacheTTL(t *testing.T) {
	c := NewClient("http://test")
	c.SetCacheTTL(10 * time.Second)
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.cacheFor != 10*time.Second {
		t.Fatalf("expected 10s, got %v", c.cacheFor)
	}
}

func TestSetCacheTTL_Zero(t *testing.T) {
	c := NewClient("http://test")
	c.SetCacheTTL(0)
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.cacheFor != 0 {
		t.Fatalf("expected 0, got %v", c.cacheFor)
	}
}

// --------------- getJSON tests ---------------

func TestGetJSON_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Accept") != "application/json" {
			t.Error("missing Accept header")
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"total": 42.5, "user": 10.1}`)
	}))
	defer srv.Close()

	c := NewClient(srv.URL)
	var result map[string]json.Number
	err := c.getJSON(context.Background(), "/test", &result)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result["total"].String() != "42.5" {
		t.Fatalf("unexpected total: %s", result["total"])
	}
}

func TestGetJSON_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	c := NewClient(srv.URL)
	var result map[string]interface{}
	err := c.getJSON(context.Background(), "/fail", &result)
	if err == nil {
		t.Fatal("expected error for HTTP 500")
	}
}

func TestGetJSON_InvalidJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `not json at all`)
	}))
	defer srv.Close()

	c := NewClient(srv.URL)
	var result map[string]interface{}
	err := c.getJSON(context.Background(), "/bad", &result)
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestGetJSON_InvalidURL(t *testing.T) {
	c := NewClient("http://[::1]:namedport")
	var result map[string]interface{}
	err := c.getJSON(context.Background(), "/test", &result)
	if err == nil {
		t.Fatal("expected error for invalid URL")
	}
}

// --------------- FetchSnapshot tests ---------------

// glancesMockServer creates a test server that mimics the Glances API.
func glancesMockServer(t *testing.T, overrides map[string]interface{}) *httptest.Server {
	t.Helper()
	// Default responses for each Glances endpoint
	defaults := map[string]interface{}{
		"/system": map[string]interface{}{
			"hostname": "test-host",
			"platform": "Linux",
			"uptime":   12345.6,
		},
		"/cpu": map[string]interface{}{
			"total":   55.5,
			"user":    30.2,
			"system":  15.1,
			"iowait":  5.0,
			"idle":    44.5,
			"cpucore": 8,
		},
		"/load": map[string]interface{}{
			"min1":    1.5,
			"min5":    1.2,
			"min15":   0.9,
			"cpucore": 8,
		},
		"/mem": map[string]interface{}{
			"total":   16000000000,
			"used":    8000000000,
			"percent": 50.0,
		},
		"/memswap": map[string]interface{}{
			"total":   4000000000,
			"used":    1000000000,
			"percent": 25.0,
		},
		"/processcount": map[string]interface{}{
			"total":    250,
			"running":  5,
			"sleeping": 240,
			"thread":   1200,
		},
		"/sensors": []map[string]interface{}{
			{"label": "Core 0", "unit": "C", "value": 55.0, "type": "temperature_core"},
			{"label": "Core 1", "unit": "C", "value": 60.0, "type": "temperature_core"},
		},
		"/network": []map[string]interface{}{
			{
				"interface_name":          "eth0",
				"bytes_recv_rate_per_sec": 1000.0,
				"bytes_sent_rate_per_sec": 500.0,
			},
			{
				"interface_name":          "lo",
				"bytes_recv_rate_per_sec": 100.0,
				"bytes_sent_rate_per_sec": 100.0,
			},
		},
		"/percpu": []map[string]interface{}{
			{"cpu_number": 0, "total": 50.0},
			{"cpu_number": 1, "total": 60.0},
		},
		"/diskio": []map[string]interface{}{
			{
				"disk_name":                "sda",
				"read_bytes_rate_per_sec":  2000.0,
				"write_bytes_rate_per_sec": 1000.0,
			},
		},
		"/fs": []map[string]interface{}{
			{
				"device_name": "/dev/sda1",
				"fs_type":     "ext4",
				"mnt_point":   "/",
				"size":        100000000000,
				"used":        40000000000,
				"free":        60000000000,
			},
		},
		"/gpu":        []map[string]interface{}{},
		"/containers": []map[string]interface{}{},
		"/uptime":     "1 day, 2:30:00",
	}

	// Apply overrides
	for k, v := range overrides {
		defaults[k] = v
	}

	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		data, ok := defaults[r.URL.Path]
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		if data == nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if s, ok := data.(string); ok {
			// for /uptime which returns a plain JSON string
			json.NewEncoder(w).Encode(s)
		} else {
			json.NewEncoder(w).Encode(data)
		}
	}))
}

func TestFetchSnapshot_FullHappyPath(t *testing.T) {
	srv := glancesMockServer(t, nil)
	defer srv.Close()

	c := NewClient(srv.URL)
	c.SetCacheTTL(0) // disable caching for test

	snap, err := c.FetchSnapshot(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// System info
	if snap.Host != "test-host" {
		t.Errorf("host = %q, want test-host", snap.Host)
	}
	if snap.Platform != "Linux" {
		t.Errorf("platform = %q, want Linux", snap.Platform)
	}
	if snap.UptimeSeconds == nil || *snap.UptimeSeconds != 12345.6 {
		t.Errorf("uptime = %v, want 12345.6", snap.UptimeSeconds)
	}
	if snap.UptimeString != "1 day, 2:30:00" {
		t.Errorf("uptime string = %q, want '1 day, 2:30:00'", snap.UptimeString)
	}

	// CPU
	if snap.CPUPercent == nil || *snap.CPUPercent != 55.5 {
		t.Errorf("cpu total = %v, want 55.5", snap.CPUPercent)
	}
	if snap.CPUUserPercent == nil || *snap.CPUUserPercent != 30.2 {
		t.Errorf("cpu user = %v, want 30.2", snap.CPUUserPercent)
	}
	if snap.CPUCores == nil || *snap.CPUCores != 8 {
		t.Errorf("cpu cores = %v, want 8", snap.CPUCores)
	}
	if snap.CPUIdlePercent == nil || *snap.CPUIdlePercent != 44.5 {
		t.Errorf("cpu idle = %v, want 44.5", snap.CPUIdlePercent)
	}

	// Load
	if snap.Load1 == nil || *snap.Load1 != 1.5 {
		t.Errorf("load1 = %v, want 1.5", snap.Load1)
	}
	if snap.Load5 == nil || *snap.Load5 != 1.2 {
		t.Errorf("load5 = %v, want 1.2", snap.Load5)
	}
	if snap.Load15 == nil || *snap.Load15 != 0.9 {
		t.Errorf("load15 = %v, want 0.9", snap.Load15)
	}

	// Memory
	if snap.MemPercent == nil || *snap.MemPercent != 50.0 {
		t.Errorf("mem pct = %v, want 50", snap.MemPercent)
	}
	if snap.MemTotalBytes == nil || *snap.MemTotalBytes != 16000000000 {
		t.Errorf("mem total = %v", snap.MemTotalBytes)
	}
	if snap.MemUsedBytes == nil || *snap.MemUsedBytes != 8000000000 {
		t.Errorf("mem used = %v", snap.MemUsedBytes)
	}

	// Swap
	if snap.SwapPercent == nil || *snap.SwapPercent != 25.0 {
		t.Errorf("swap pct = %v, want 25", snap.SwapPercent)
	}

	// Processes
	if snap.ProcTotal == nil || *snap.ProcTotal != 250 {
		t.Errorf("proc total = %v, want 250", snap.ProcTotal)
	}
	if snap.ProcRunning == nil || *snap.ProcRunning != 5 {
		t.Errorf("proc running = %v, want 5", snap.ProcRunning)
	}
	if snap.ProcSleeping == nil || *snap.ProcSleeping != 240 {
		t.Errorf("proc sleeping = %v, want 240", snap.ProcSleeping)
	}
	if snap.ProcThreads == nil || *snap.ProcThreads != 1200 {
		t.Errorf("proc threads = %v, want 1200", snap.ProcThreads)
	}

	// Temperature (highest core is 60.0)
	if snap.TempC == nil || *snap.TempC != 60.0 {
		t.Errorf("temp = %v, want 60", snap.TempC)
	}
	if snap.TempMinC == nil || snap.TempMaxC == nil {
		t.Error("temp min/max should be set")
	}

	// Network (eth0 only, loopback excluded)
	if snap.NetRxBytesPerSec == nil || *snap.NetRxBytesPerSec != 1000 {
		t.Errorf("net rx = %v, want 1000", snap.NetRxBytesPerSec)
	}
	if snap.NetTxBytesPerSec == nil || *snap.NetTxBytesPerSec != 500 {
		t.Errorf("net tx = %v, want 500", snap.NetTxBytesPerSec)
	}

	// Per-CPU
	if len(snap.CPUPerCorePercent) != 2 {
		t.Errorf("percpu length = %d, want 2", len(snap.CPUPerCorePercent))
	} else {
		if snap.CPUPerCorePercent[0] != 50.0 {
			t.Errorf("percpu[0] = %v, want 50", snap.CPUPerCorePercent[0])
		}
		if snap.CPUPerCorePercent[1] != 60.0 {
			t.Errorf("percpu[1] = %v, want 60", snap.CPUPerCorePercent[1])
		}
	}

	// Disk I/O
	if snap.DiskReadBytesPerSec == nil || *snap.DiskReadBytesPerSec != 2000.0 {
		t.Errorf("disk read = %v, want 2000", snap.DiskReadBytesPerSec)
	}
	if snap.DiskWriteBytesPerSec == nil || *snap.DiskWriteBytesPerSec != 1000.0 {
		t.Errorf("disk write = %v, want 1000", snap.DiskWriteBytesPerSec)
	}

	// Filesystem
	if snap.FSTotalBytes == nil || *snap.FSTotalBytes != 100000000000 {
		t.Errorf("fs total = %v", snap.FSTotalBytes)
	}
	if snap.FSUsedBytes == nil || *snap.FSUsedBytes != 40000000000 {
		t.Errorf("fs used = %v", snap.FSUsedBytes)
	}
	if snap.FSFreeBytes == nil || *snap.FSFreeBytes != 60000000000 {
		t.Errorf("fs free = %v", snap.FSFreeBytes)
	}
	if snap.FSUsedPercent == nil || *snap.FSUsedPercent != 40.0 {
		t.Errorf("fs pct = %v, want 40", snap.FSUsedPercent)
	}

	if snap.TakenAt.IsZero() {
		t.Error("TakenAt should not be zero")
	}
}

func TestFetchSnapshot_CacheHit(t *testing.T) {
	callCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/system":
			fmt.Fprint(w, `{"hostname":"h"}`)
		case "/cpu":
			fmt.Fprint(w, `{"total":10}`)
		case "/load":
			fmt.Fprint(w, `{"min1":1}`)
		case "/mem":
			fmt.Fprint(w, `{"total":100,"used":50,"percent":50}`)
		case "/memswap":
			fmt.Fprint(w, `{"total":0}`)
		case "/processcount":
			fmt.Fprint(w, `{"total":1}`)
		case "/sensors":
			fmt.Fprint(w, `[]`)
		case "/network":
			fmt.Fprint(w, `[]`)
		case "/percpu":
			fmt.Fprint(w, `[]`)
		case "/diskio":
			fmt.Fprint(w, `[]`)
		case "/fs":
			fmt.Fprint(w, `[]`)
		case "/gpu":
			fmt.Fprint(w, `[]`)
		case "/containers":
			fmt.Fprint(w, `[]`)
		case "/uptime":
			fmt.Fprint(w, `"1:00"`)
		default:
			w.WriteHeader(404)
		}
	}))
	defer srv.Close()

	c := NewClient(srv.URL)
	c.SetCacheTTL(5 * time.Second)

	s1, err := c.FetchSnapshot(context.Background())
	if err != nil {
		t.Fatalf("first fetch: %v", err)
	}

	beforeSecondCall := callCount

	s2, err := c.FetchSnapshot(context.Background())
	if err != nil {
		t.Fatalf("second fetch: %v", err)
	}

	// The second call should not have made additional HTTP requests
	if callCount != beforeSecondCall {
		t.Errorf("cache miss: made %d additional HTTP calls", callCount-beforeSecondCall)
	}

	if s1.Host != s2.Host {
		t.Error("cached snapshot differs")
	}
}

func TestFetchSnapshot_CacheExpiry(t *testing.T) {
	fetchCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/system" {
			fetchCount++
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/system":
			fmt.Fprintf(w, `{"hostname":"h%d"}`, fetchCount)
		default:
			fmt.Fprint(w, `{}`)
		}
	}))
	defer srv.Close()

	c := NewClient(srv.URL)
	c.SetCacheTTL(50 * time.Millisecond)

	s1, _ := c.FetchSnapshot(context.Background())
	if s1.Host != "h1" {
		t.Fatalf("first fetch host = %q, want h1", s1.Host)
	}

	// Wait for cache to expire
	time.Sleep(100 * time.Millisecond)

	s2, _ := c.FetchSnapshot(context.Background())
	if s2.Host != "h2" {
		t.Fatalf("second fetch host = %q, want h2 (cache should have expired)", s2.Host)
	}
}

func TestFetchSnapshot_Coalescing(t *testing.T) {
	callCount := 0
	var mu sync.Mutex
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/system" {
			mu.Lock()
			callCount++
			mu.Unlock()
			time.Sleep(100 * time.Millisecond) // Slow endpoint
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/system":
			fmt.Fprint(w, `{"hostname":"h"}`)
		default:
			fmt.Fprint(w, `{}`)
		}
	}))
	defer srv.Close()

	c := NewClient(srv.URL)
	c.SetCacheTTL(0) // TTL=0 means every request is a cache miss,
	// but in-flight coalescing should still work

	var wg sync.WaitGroup
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			c.FetchSnapshot(context.Background())
		}()
	}
	wg.Wait()

	mu.Lock()
	defer mu.Unlock()
	// With coalescing, /system should be called far fewer than 5 times
	// (ideally 1, but timing may cause a second call)
	if callCount > 3 {
		t.Errorf("expected coalesced calls, got %d /system calls for 5 concurrent fetches", callCount)
	}
}

func TestFetchSnapshot_AllEndpointsFail(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	c := NewClient(srv.URL)
	c.SetCacheTTL(0)

	_, err := c.FetchSnapshot(context.Background())
	if err == nil {
		t.Fatal("expected error when all endpoints fail")
	}
}

func TestFetchSnapshot_MemPercentFallback(t *testing.T) {
	// When percent is nil but total and used are available, should calculate
	srv := glancesMockServer(t, map[string]interface{}{
		"/mem": map[string]interface{}{
			"total": 2000,
			"used":  500,
			// no "percent" field
		},
	})
	defer srv.Close()

	c := NewClient(srv.URL)
	c.SetCacheTTL(0)

	snap, err := c.FetchSnapshot(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if snap.MemPercent == nil {
		t.Fatal("MemPercent should be computed from total/used")
	}
	if *snap.MemPercent != 25.0 {
		t.Errorf("MemPercent = %v, want 25", *snap.MemPercent)
	}
}

func TestFetchSnapshot_SwapPercentFallback(t *testing.T) {
	srv := glancesMockServer(t, map[string]interface{}{
		"/memswap": map[string]interface{}{
			"total": 4000,
			"used":  1000,
			// no "percent" field
		},
	})
	defer srv.Close()

	c := NewClient(srv.URL)
	c.SetCacheTTL(0)

	snap, err := c.FetchSnapshot(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if snap.SwapPercent == nil {
		t.Fatal("SwapPercent should be computed from total/used")
	}
	if *snap.SwapPercent != 25.0 {
		t.Errorf("SwapPercent = %v, want 25", *snap.SwapPercent)
	}
}

func TestFetchSnapshot_TemperatureTracking(t *testing.T) {
	// First fetch: temp 50, second fetch: temp 70 → min=50, max=70
	callNum := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/sensors":
			callNum++
			if callNum <= 1 {
				// First fetch's sensors request
				fmt.Fprint(w, `[{"label":"Core 0","unit":"C","value":50.0,"type":"temperature_core"}]`)
			} else {
				fmt.Fprint(w, `[{"label":"Core 0","unit":"C","value":70.0,"type":"temperature_core"}]`)
			}
		case "/system":
			fmt.Fprint(w, `{"hostname":"h"}`)
		default:
			fmt.Fprint(w, `{}`)
		}
	}))
	defer srv.Close()

	c := NewClient(srv.URL)
	c.SetCacheTTL(0)

	s1, _ := c.FetchSnapshot(context.Background())
	if s1.TempC == nil || *s1.TempC != 50.0 {
		t.Fatalf("first temp = %v, want 50", s1.TempC)
	}

	s2, _ := c.FetchSnapshot(context.Background())
	if s2.TempC == nil || *s2.TempC != 70.0 {
		t.Fatalf("second temp = %v, want 70", s2.TempC)
	}
	if s2.TempMinC == nil || *s2.TempMinC != 50.0 {
		t.Errorf("temp min = %v, want 50", s2.TempMinC)
	}
	if s2.TempMaxC == nil || *s2.TempMaxC != 70.0 {
		t.Errorf("temp max = %v, want 70", s2.TempMaxC)
	}
}

func TestFetchSnapshot_NetworkSumsMultipleInterfaces(t *testing.T) {
	srv := glancesMockServer(t, map[string]interface{}{
		"/network": []map[string]interface{}{
			{"interface_name": "eth0", "bytes_recv_rate_per_sec": 1000.0, "bytes_sent_rate_per_sec": 500.0},
			{"interface_name": "eth1", "bytes_recv_rate_per_sec": 2000.0, "bytes_sent_rate_per_sec": 1000.0},
			{"interface_name": "lo", "bytes_recv_rate_per_sec": 999.0, "bytes_sent_rate_per_sec": 999.0},
		},
	})
	defer srv.Close()

	c := NewClient(srv.URL)
	c.SetCacheTTL(0)

	snap, _ := c.FetchSnapshot(context.Background())
	if snap.NetRxBytesPerSec == nil || *snap.NetRxBytesPerSec != 3000 {
		t.Errorf("net rx = %v, want 3000 (sum of eth0+eth1, not lo)", snap.NetRxBytesPerSec)
	}
	if snap.NetTxBytesPerSec == nil || *snap.NetTxBytesPerSec != 1500 {
		t.Errorf("net tx = %v, want 1500", snap.NetTxBytesPerSec)
	}
}

func TestFetchSnapshot_NetworkFallbackRxTxRate(t *testing.T) {
	// When bytes_recv_rate_per_sec is absent, fall back to rx_rate
	srv := glancesMockServer(t, map[string]interface{}{
		"/network": []map[string]interface{}{
			{"interface_name": "eth0", "rx_rate": 800.0, "tx_rate": 400.0},
		},
	})
	defer srv.Close()

	c := NewClient(srv.URL)
	c.SetCacheTTL(0)

	snap, _ := c.FetchSnapshot(context.Background())
	if snap.NetRxBytesPerSec == nil || *snap.NetRxBytesPerSec != 800 {
		t.Errorf("net rx = %v, want 800 (from rx_rate fallback)", snap.NetRxBytesPerSec)
	}
	if snap.NetTxBytesPerSec == nil || *snap.NetTxBytesPerSec != 400 {
		t.Errorf("net tx = %v, want 400", snap.NetTxBytesPerSec)
	}
}

func TestFetchSnapshot_GPUMetrics(t *testing.T) {
	srv := glancesMockServer(t, map[string]interface{}{
		"/gpu": []map[string]interface{}{
			{"gpu_id": 0, "name": "GeForce RTX 3080", "proc": 75.5, "mem": 60.2, "temperature": 72.0},
		},
	})
	defer srv.Close()

	c := NewClient(srv.URL)
	c.SetCacheTTL(0)

	snap, _ := c.FetchSnapshot(context.Background())
	if snap.GPUName != "GeForce RTX 3080" {
		t.Errorf("gpu name = %q", snap.GPUName)
	}
	if snap.GPUPercent == nil || *snap.GPUPercent != 75.5 {
		t.Errorf("gpu pct = %v, want 75.5", snap.GPUPercent)
	}
	if snap.GPUMemPct == nil || *snap.GPUMemPct != 60.2 {
		t.Errorf("gpu mem = %v, want 60.2", snap.GPUMemPct)
	}
	if snap.GPUTempC == nil || *snap.GPUTempC != 72.0 {
		t.Errorf("gpu temp = %v, want 72", snap.GPUTempC)
	}
}

func TestFetchSnapshot_ContainerMetrics(t *testing.T) {
	srv := glancesMockServer(t, map[string]interface{}{
		"/containers": []map[string]interface{}{
			{"name": "nginx", "status": "running", "cpu_percent": 5.0, "memory_usage": 100000, "memory_limit": 500000},
			{"name": "redis", "status": "running", "cpu_percent": 2.0, "memory_usage": 50000, "memory_limit": 200000},
			{"name": "old-app", "status": "exited"},
		},
	})
	defer srv.Close()

	c := NewClient(srv.URL)
	c.SetCacheTTL(0)

	snap, _ := c.FetchSnapshot(context.Background())
	if snap.ContainerCount == nil || *snap.ContainerCount != 3 {
		t.Errorf("container count = %v, want 3", snap.ContainerCount)
	}
	if snap.ContainerRunning == nil || *snap.ContainerRunning != 2 {
		t.Errorf("running = %v, want 2", snap.ContainerRunning)
	}
	if len(snap.Containers) != 3 {
		t.Fatalf("containers len = %d, want 3", len(snap.Containers))
	}
	// Check first container
	if snap.Containers[0].Name != "nginx" {
		t.Errorf("container 0 name = %q, want nginx", snap.Containers[0].Name)
	}
	if snap.Containers[0].CPUPercent == nil || *snap.Containers[0].CPUPercent != 5.0 {
		t.Errorf("container 0 cpu = %v, want 5", snap.Containers[0].CPUPercent)
	}
	// Memory percent should be calculated: 100000/500000 * 100 = 20%
	if snap.Containers[0].MemPercent == nil || *snap.Containers[0].MemPercent != 20.0 {
		t.Errorf("container 0 mem%% = %v, want 20", snap.Containers[0].MemPercent)
	}
}

func TestFetchSnapshot_FSExcludesSystemMounts(t *testing.T) {
	srv := glancesMockServer(t, map[string]interface{}{
		"/fs": []map[string]interface{}{
			{"mnt_point": "/", "size": 1000, "used": 400, "free": 600},
			{"mnt_point": "/etc/host", "size": 500, "used": 200, "free": 300},     // excluded
			{"mnt_point": "/proc", "size": 500, "used": 0, "free": 500},           // excluded
			{"mnt_point": "/sys/firmware", "size": 100, "used": 100, "free": 0},   // excluded
			{"mnt_point": "/dev/shm", "size": 200, "used": 50, "free": 150},       // excluded
			{"mnt_point": "/home", "size": 2000, "used": 1000, "free": 1000},
		},
	})
	defer srv.Close()

	c := NewClient(srv.URL)
	c.SetCacheTTL(0)

	snap, _ := c.FetchSnapshot(context.Background())
	// Only / and /home should be included: total=3000, used=1400, free=1600
	if snap.FSTotalBytes == nil || *snap.FSTotalBytes != 3000 {
		t.Errorf("fs total = %v, want 3000", snap.FSTotalBytes)
	}
	if snap.FSUsedBytes == nil || *snap.FSUsedBytes != 1400 {
		t.Errorf("fs used = %v, want 1400", snap.FSUsedBytes)
	}
	if snap.FSFreeBytes == nil || *snap.FSFreeBytes != 1600 {
		t.Errorf("fs free = %v, want 1600", snap.FSFreeBytes)
	}
}

func TestFetchSnapshot_FSDedupMountPoints(t *testing.T) {
	srv := glancesMockServer(t, map[string]interface{}{
		"/fs": []map[string]interface{}{
			{"mnt_point": "/", "size": 1000, "used": 400, "free": 600},
			{"mnt_point": "/", "size": 1000, "used": 400, "free": 600}, // duplicate
		},
	})
	defer srv.Close()

	c := NewClient(srv.URL)
	c.SetCacheTTL(0)

	snap, _ := c.FetchSnapshot(context.Background())
	if snap.FSTotalBytes == nil || *snap.FSTotalBytes != 1000 {
		t.Errorf("fs total = %v, want 1000 (deduped)", snap.FSTotalBytes)
	}
}

func TestFetchSnapshot_FSComputesMissingFree(t *testing.T) {
	// When free is nil but size and used are available
	srv := glancesMockServer(t, map[string]interface{}{
		"/fs": []map[string]interface{}{
			{"mnt_point": "/", "size": 1000, "used": 300},
		},
	})
	defer srv.Close()

	c := NewClient(srv.URL)
	c.SetCacheTTL(0)

	snap, _ := c.FetchSnapshot(context.Background())
	if snap.FSFreeBytes == nil || *snap.FSFreeBytes != 700 {
		t.Errorf("fs free = %v, want 700 (computed from size-used)", snap.FSFreeBytes)
	}
}

func TestFetchSnapshot_FSComputesMissingUsed(t *testing.T) {
	// When used is nil but size and free are available
	srv := glancesMockServer(t, map[string]interface{}{
		"/fs": []map[string]interface{}{
			{"mnt_point": "/", "size": 1000, "free": 400},
		},
	})
	defer srv.Close()

	c := NewClient(srv.URL)
	c.SetCacheTTL(0)

	snap, _ := c.FetchSnapshot(context.Background())
	if snap.FSUsedBytes == nil || *snap.FSUsedBytes != 600 {
		t.Errorf("fs used = %v, want 600 (computed from size-free)", snap.FSUsedBytes)
	}
}

func TestFetchSnapshot_DiskIOSumsMultipleDisks(t *testing.T) {
	srv := glancesMockServer(t, map[string]interface{}{
		"/diskio": []map[string]interface{}{
			{"disk_name": "sda", "read_bytes_rate_per_sec": 1000.0, "write_bytes_rate_per_sec": 500.0},
			{"disk_name": "sdb", "read_bytes_rate_per_sec": 2000.0, "write_bytes_rate_per_sec": 1500.0},
		},
	})
	defer srv.Close()

	c := NewClient(srv.URL)
	c.SetCacheTTL(0)

	snap, _ := c.FetchSnapshot(context.Background())
	if snap.DiskReadBytesPerSec == nil || *snap.DiskReadBytesPerSec != 3000 {
		t.Errorf("disk read = %v, want 3000", snap.DiskReadBytesPerSec)
	}
	if snap.DiskWriteBytesPerSec == nil || *snap.DiskWriteBytesPerSec != 2000 {
		t.Errorf("disk write = %v, want 2000", snap.DiskWriteBytesPerSec)
	}
}

func TestFetchSnapshot_CPUPercentFromNestedMap(t *testing.T) {
	// Some Glances versions return total as {"total": 55.5} instead of 55.5
	srv := glancesMockServer(t, map[string]interface{}{
		"/cpu": map[string]interface{}{
			"total": map[string]interface{}{"total": 77.3},
		},
	})
	defer srv.Close()

	c := NewClient(srv.URL)
	c.SetCacheTTL(0)

	snap, _ := c.FetchSnapshot(context.Background())
	if snap.CPUPercent == nil || math.Abs(*snap.CPUPercent-77.3) > 0.01 {
		t.Errorf("cpu pct = %v, want 77.3 (from nested map)", snap.CPUPercent)
	}
}

func TestFetchSnapshot_CPUCoresFallbackFromLoad(t *testing.T) {
	// When cpu.cpucore is nil, fall back to load.cpucore
	srv := glancesMockServer(t, map[string]interface{}{
		"/cpu":  map[string]interface{}{"total": 50},
		"/load": map[string]interface{}{"min1": 1.0, "cpucore": 4},
	})
	defer srv.Close()

	c := NewClient(srv.URL)
	c.SetCacheTTL(0)

	snap, _ := c.FetchSnapshot(context.Background())
	if snap.CPUCores == nil || *snap.CPUCores != 4 {
		t.Errorf("cpu cores = %v, want 4 (from load fallback)", snap.CPUCores)
	}
}

func TestFetchSnapshot_NoSensorsData(t *testing.T) {
	srv := glancesMockServer(t, map[string]interface{}{
		"/sensors": []map[string]interface{}{},
	})
	defer srv.Close()

	c := NewClient(srv.URL)
	c.SetCacheTTL(0)

	snap, _ := c.FetchSnapshot(context.Background())
	if snap.TempC != nil {
		t.Errorf("temp should be nil when no sensors, got %v", *snap.TempC)
	}
}

func TestFetchSnapshot_SensorFiltersByType(t *testing.T) {
	// Only temperature_core and temperature types should be used
	srv := glancesMockServer(t, map[string]interface{}{
		"/sensors": []map[string]interface{}{
			{"label": "Fan", "unit": "rpm", "value": 3000.0, "type": "fan_speed"},
			{"label": "Core 0", "unit": "C", "value": 45.0, "type": "temperature_core"},
		},
	})
	defer srv.Close()

	c := NewClient(srv.URL)
	c.SetCacheTTL(0)

	snap, _ := c.FetchSnapshot(context.Background())
	if snap.TempC == nil || *snap.TempC != 45.0 {
		t.Errorf("temp = %v, want 45 (fan should be ignored)", snap.TempC)
	}
}

func TestFetchSnapshot_EmptyPerCPU(t *testing.T) {
	srv := glancesMockServer(t, map[string]interface{}{
		"/percpu": []map[string]interface{}{},
	})
	defer srv.Close()

	c := NewClient(srv.URL)
	c.SetCacheTTL(0)

	snap, _ := c.FetchSnapshot(context.Background())
	if snap.CPUPerCorePercent != nil {
		t.Errorf("percpu should be nil for empty data, got %v", snap.CPUPerCorePercent)
	}
}

func TestFetchSnapshot_EmptyContainers(t *testing.T) {
	srv := glancesMockServer(t, map[string]interface{}{
		"/containers": []map[string]interface{}{},
	})
	defer srv.Close()

	c := NewClient(srv.URL)
	c.SetCacheTTL(0)

	snap, _ := c.FetchSnapshot(context.Background())
	if snap.ContainerCount != nil {
		t.Errorf("container count should be nil for empty, got %v", *snap.ContainerCount)
	}
}
