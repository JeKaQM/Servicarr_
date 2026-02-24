package stats

import (
	"status/app/internal/cache"
	"status/app/internal/database"
	"testing"
	"time"
)

func initTestDB(t *testing.T) {
	t.Helper()
	if err := database.Init(":memory:"); err != nil {
		t.Fatalf("failed to init db: %v", err)
	}
	if err := EnsureStatsSchema(); err != nil {
		t.Fatalf("failed to init stats schema: %v", err)
	}
	cache.StatsCache = cache.New(5 * time.Minute)
	// Clear calculators between tests
	calcMu.Lock()
	calculators = make(map[string]*UptimeCalculator)
	calcMu.Unlock()
}

// --------------- EnsureStatsSchema ---------------

func TestEnsureStatsSchema(t *testing.T) {
	if err := database.Init(":memory:"); err != nil {
		t.Fatal(err)
	}
	if err := EnsureStatsSchema(); err != nil {
		t.Fatalf("EnsureStatsSchema failed: %v", err)
	}
	// Call again (idempotent)
	if err := EnsureStatsSchema(); err != nil {
		t.Fatalf("second call failed: %v", err)
	}
}

// --------------- GetCalculator / RemoveCalculator ---------------

func TestGetCalculator_Creates(t *testing.T) {
	initTestDB(t)
	calc := GetCalculator("svc1")
	if calc == nil {
		t.Fatal("expected non-nil calculator")
	}
	if calc.ServiceKey != "svc1" {
		t.Errorf("ServiceKey = %q", calc.ServiceKey)
	}
}

func TestGetCalculator_ReturnsSame(t *testing.T) {
	initTestDB(t)
	c1 := GetCalculator("svc1")
	c2 := GetCalculator("svc1")
	if c1 != c2 {
		t.Error("should return the same calculator instance")
	}
}

func TestGetCalculator_DifferentKeys(t *testing.T) {
	initTestDB(t)
	c1 := GetCalculator("svc1")
	c2 := GetCalculator("svc2")
	if c1 == c2 {
		t.Error("different keys should produce different calculators")
	}
}

func TestRemoveCalculator(t *testing.T) {
	initTestDB(t)
	GetCalculator("svc-remove")
	RemoveCalculator("svc-remove")

	// Getting it again should create a fresh one
	calc := GetCalculator("svc-remove")
	if len(calc.recentHeartbeats) != 0 {
		t.Error("new calculator should have empty heartbeats")
	}
}

func TestRemoveCalculator_NonExistent(t *testing.T) {
	initTestDB(t)
	// Should not panic
	RemoveCalculator("nonexistent")
}

// --------------- AddHeartbeat ---------------

func TestAddHeartbeat_FirstIsImportant(t *testing.T) {
	initTestDB(t)
	calc := GetCalculator("svc-hb")
	important := calc.AddHeartbeat(1, nil, 200, "")
	if !important {
		t.Error("first heartbeat should be important")
	}
}

func TestAddHeartbeat_SameStatus_NotImportant(t *testing.T) {
	initTestDB(t)
	calc := GetCalculator("svc-hb2")
	calc.AddHeartbeat(1, nil, 200, "")
	important := calc.AddHeartbeat(1, nil, 200, "")
	if important {
		t.Error("same status should not be important")
	}
}

func TestAddHeartbeat_StatusChange_Important(t *testing.T) {
	initTestDB(t)
	calc := GetCalculator("svc-hb3")
	calc.AddHeartbeat(1, nil, 200, "")
	important := calc.AddHeartbeat(0, nil, 0, "timeout")
	if !important {
		t.Error("status change should be important")
	}
}

func TestAddHeartbeat_RingBuffer_100(t *testing.T) {
	initTestDB(t)
	calc := GetCalculator("svc-ring")
	// Add 150 heartbeats, should keep only last 100
	for i := 0; i < 150; i++ {
		calc.AddHeartbeat(1, nil, 200, "")
	}
	calc.mu.RLock()
	count := len(calc.recentHeartbeats)
	calc.mu.RUnlock()
	if count != 100 {
		t.Errorf("expected 100 heartbeats, got %d", count)
	}
}

func TestAddHeartbeat_WithPing(t *testing.T) {
	initTestDB(t)
	calc := GetCalculator("svc-ping")
	ping := 42
	calc.AddHeartbeat(1, &ping, 200, "")

	hbs := calc.GetRecentHeartbeats(1)
	if len(hbs) != 1 {
		t.Fatal("expected 1 heartbeat")
	}
	if hbs[0].Ping == nil || *hbs[0].Ping != 42 {
		t.Errorf("ping = %v, want 42", hbs[0].Ping)
	}
}

func TestAddHeartbeat_InvalidatesCache(t *testing.T) {
	initTestDB(t)
	calc := GetCalculator("svc-cache")

	// Set a cached value
	cache.StatsCache.Set("uptime:svc-cache", map[string]float64{"1h": 99.9})

	calc.AddHeartbeat(1, nil, 200, "")

	// Cache should be invalidated
	_, exists := cache.StatsCache.Get("uptime:svc-cache")
	if exists {
		t.Error("cache should be invalidated after heartbeat")
	}
}

// --------------- GetRecentHeartbeats ---------------

func TestGetRecentHeartbeats_Empty(t *testing.T) {
	initTestDB(t)
	calc := GetCalculator("svc-empty")
	hbs := calc.GetRecentHeartbeats(10)
	if len(hbs) != 0 {
		t.Errorf("expected 0, got %d", len(hbs))
	}
}

func TestGetRecentHeartbeats_NewestFirst(t *testing.T) {
	initTestDB(t)
	calc := GetCalculator("svc-order")
	ping1 := 10
	calc.AddHeartbeat(1, &ping1, 200, "first")
	ping2 := 20
	calc.AddHeartbeat(1, &ping2, 200, "second")
	ping3 := 30
	calc.AddHeartbeat(0, &ping3, 0, "third")

	hbs := calc.GetRecentHeartbeats(3)
	if len(hbs) != 3 {
		t.Fatalf("expected 3, got %d", len(hbs))
	}
	// Newest first
	if hbs[0].Msg != "third" {
		t.Errorf("most recent should be 'third', got %q", hbs[0].Msg)
	}
	if hbs[2].Msg != "first" {
		t.Errorf("oldest should be 'first', got %q", hbs[2].Msg)
	}
}

func TestGetRecentHeartbeats_MoreThanAvailable(t *testing.T) {
	initTestDB(t)
	calc := GetCalculator("svc-more")
	calc.AddHeartbeat(1, nil, 200, "")
	calc.AddHeartbeat(1, nil, 200, "")

	hbs := calc.GetRecentHeartbeats(100)
	if len(hbs) != 2 {
		t.Errorf("expected 2, got %d", len(hbs))
	}
}

// --------------- GetUptime ---------------

func TestGetUptime_NoData(t *testing.T) {
	initTestDB(t)
	calc := GetCalculator("svc-no-data")
	uptime := calc.GetUptime(24 * time.Hour)
	if uptime != 100.0 {
		t.Errorf("uptime with no data = %v, want 100", uptime)
	}
}

func TestGetUptime_AllUp(t *testing.T) {
	initTestDB(t)
	// Insert sample data directly
	for i := 0; i < 10; i++ {
		ms := 50
		database.InsertSample(time.Now().Add(-time.Duration(i)*time.Minute), "svc-allup", true, 200, &ms)
	}

	calc := GetCalculator("svc-allup")
	uptime := calc.GetUptime(24 * time.Hour)
	if uptime != 100.0 {
		t.Errorf("uptime = %v, want 100", uptime)
	}
}

func TestGetUptime_AllDown(t *testing.T) {
	initTestDB(t)
	for i := 0; i < 10; i++ {
		database.InsertSample(time.Now().Add(-time.Duration(i)*time.Minute), "svc-alldown", false, 0, nil)
	}

	calc := GetCalculator("svc-alldown")
	uptime := calc.GetUptime(24 * time.Hour)
	if uptime != 0.0 {
		t.Errorf("uptime = %v, want 0", uptime)
	}
}

func TestGetUptime_Mixed(t *testing.T) {
	initTestDB(t)
	// 7 up, 3 down = 70%
	for i := 0; i < 7; i++ {
		database.InsertSample(time.Now().Add(-time.Duration(i)*time.Minute), "svc-mixed", true, 200, nil)
	}
	for i := 0; i < 3; i++ {
		database.InsertSample(time.Now().Add(-time.Duration(i+7)*time.Minute), "svc-mixed", false, 0, nil)
	}

	calc := GetCalculator("svc-mixed")
	uptime := calc.GetUptime(24 * time.Hour)
	if uptime != 70.0 {
		t.Errorf("uptime = %v, want 70", uptime)
	}
}

func TestGetUptime_CacheHit(t *testing.T) {
	initTestDB(t)
	calc := GetCalculator("svc-cached")
	// Set fake cache
	cache.StatsCache.Set("uptime:svc-cached", map[string]float64{
		(24 * time.Hour).String(): 95.5,
	})

	uptime := calc.GetUptime(24 * time.Hour)
	if uptime != 95.5 {
		t.Errorf("uptime = %v, want 95.5 (from cache)", uptime)
	}
}

// --------------- GetAverageLatency ---------------

func TestGetAverageLatency_NoData(t *testing.T) {
	initTestDB(t)
	calc := GetCalculator("svc-nolat")
	avg := calc.GetAverageLatency(24 * time.Hour)
	if avg != 0 {
		t.Errorf("avg = %v, want 0", avg)
	}
}

func TestGetAverageLatency_WithData(t *testing.T) {
	initTestDB(t)
	ms10 := 10
	ms20 := 20
	ms30 := 30
	database.InsertSample(time.Now(), "svc-lat", true, 200, &ms10)
	database.InsertSample(time.Now(), "svc-lat", true, 200, &ms20)
	database.InsertSample(time.Now(), "svc-lat", true, 200, &ms30)

	calc := GetCalculator("svc-lat")
	avg := calc.GetAverageLatency(24 * time.Hour)
	if avg != 20.0 {
		t.Errorf("avg = %v, want 20", avg)
	}
}

// --------------- GetUptimeStats ---------------

func TestGetUptimeStats(t *testing.T) {
	initTestDB(t)
	ms := 50
	database.InsertSample(time.Now(), "svc-stats", true, 200, &ms)

	stats := GetUptimeStats("svc-stats")
	if stats == nil {
		t.Fatal("expected non-nil stats")
	}
	if stats.Uptime24h != 100.0 {
		t.Errorf("uptime24h = %v, want 100", stats.Uptime24h)
	}
	if stats.AvgLatency != 50.0 {
		t.Errorf("avg_latency = %v, want 50", stats.AvgLatency)
	}
}

func TestGetUptimeStats_Cached(t *testing.T) {
	initTestDB(t)
	// Pre-populate cache
	cached := &UptimeStats{Uptime24h: 88.8}
	cache.StatsCache.Set("uptime_stats:svc-cs", cached)

	stats := GetUptimeStats("svc-cs")
	if stats.Uptime24h != 88.8 {
		t.Errorf("expected cached value 88.8, got %v", stats.Uptime24h)
	}
}

// --------------- RecordHeartbeat ---------------

func TestRecordHeartbeat(t *testing.T) {
	initTestDB(t)
	ping := 25
	RecordHeartbeat("svc-rec", true, &ping, 200, "")

	// Should be in heartbeats table
	var count int
	database.DB.QueryRow(`SELECT COUNT(*) FROM heartbeats WHERE service_key = 'svc-rec'`).Scan(&count)
	if count != 1 {
		t.Errorf("expected 1 heartbeat in DB, got %d", count)
	}

	// Should also be tracked in calculator
	calc := GetCalculator("svc-rec")
	hbs := calc.GetRecentHeartbeats(1)
	if len(hbs) != 1 {
		t.Error("expected 1 heartbeat in calculator")
	}
}

func TestRecordHeartbeat_UpdatesMinutely(t *testing.T) {
	initTestDB(t)
	ping := 50
	RecordHeartbeat("svc-min", true, &ping, 200, "")

	var up, down int
	database.DB.QueryRow(`SELECT up, down FROM stat_minutely WHERE service_key = 'svc-min'`).Scan(&up, &down)
	if up != 1 || down != 0 {
		t.Errorf("minutely stats: up=%d, down=%d, want 1, 0", up, down)
	}
}

func TestRecordHeartbeat_DownUpdatesMinutely(t *testing.T) {
	initTestDB(t)
	RecordHeartbeat("svc-min-down", false, nil, 0, "timeout")

	var up, down int
	database.DB.QueryRow(`SELECT up, down FROM stat_minutely WHERE service_key = 'svc-min-down'`).Scan(&up, &down)
	if up != 0 || down != 1 {
		t.Errorf("minutely stats: up=%d, down=%d, want 0, 1", up, down)
	}
}

// --------------- AggregateHourlyStats ---------------

func TestAggregateHourlyStats(t *testing.T) {
	initTestDB(t)
	// Insert old minutely data (> 1 hour ago)
	twoHoursAgo := time.Now().UTC().Add(-2 * time.Hour).Unix()
	database.DB.Exec(`INSERT INTO stat_minutely (service_key, timestamp, up, down, ping, ping_min, ping_max) VALUES ('svc-agg', ?, 10, 2, 45.0, 20, 80)`, twoHoursAgo)

	AggregateHourlyStats()

	var up, down int
	database.DB.QueryRow(`SELECT up, down FROM stat_hourly WHERE service_key = 'svc-agg'`).Scan(&up, &down)
	if up != 10 || down != 2 {
		t.Errorf("hourly: up=%d, down=%d, want 10, 2", up, down)
	}
}

// --------------- AggregateDailyStats ---------------

func TestAggregateDailyStats(t *testing.T) {
	initTestDB(t)
	// Insert old hourly data (> 1 day ago)
	twoDaysAgo := time.Now().UTC().Add(-48 * time.Hour).Unix()
	database.DB.Exec(`INSERT INTO stat_hourly (service_key, timestamp, up, down, ping, ping_min, ping_max) VALUES ('svc-daily', ?, 100, 5, 30.0, 10, 60)`, twoDaysAgo)

	AggregateDailyStats()

	var up, down int
	database.DB.QueryRow(`SELECT up, down FROM stat_daily WHERE service_key = 'svc-daily'`).Scan(&up, &down)
	if up != 100 || down != 5 {
		t.Errorf("daily: up=%d, down=%d, want 100, 5", up, down)
	}
}

// --------------- CleanupOldHeartbeats ---------------

func TestCleanupOldHeartbeats(t *testing.T) {
	initTestDB(t)
	// Insert recent heartbeat
	database.DB.Exec(`INSERT INTO heartbeats (service_key, status, time, msg, ping, http_status, important) VALUES ('svc-clean', 1, ?, '', 50, 200, 0)`,
		time.Now().UTC().Format(time.RFC3339))

	// Insert old non-important heartbeat (2 days ago)
	database.DB.Exec(`INSERT INTO heartbeats (service_key, status, time, msg, ping, http_status, important) VALUES ('svc-clean', 1, ?, '', 50, 200, 0)`,
		time.Now().UTC().Add(-48*time.Hour).Format(time.RFC3339))

	// Insert old important heartbeat (3 days ago - should survive)
	database.DB.Exec(`INSERT INTO heartbeats (service_key, status, time, msg, ping, http_status, important) VALUES ('svc-clean', 0, ?, 'went down', 0, 0, 1)`,
		time.Now().UTC().Add(-72*time.Hour).Format(time.RFC3339))

	// Insert very old important heartbeat (10 days ago - should be cleaned)
	database.DB.Exec(`INSERT INTO heartbeats (service_key, status, time, msg, ping, http_status, important) VALUES ('svc-clean', 0, ?, 'old', 0, 0, 1)`,
		time.Now().UTC().Add(-240*time.Hour).Format(time.RFC3339))

	CleanupOldHeartbeats()

	var count int
	database.DB.QueryRow(`SELECT COUNT(*) FROM heartbeats WHERE service_key = 'svc-clean'`).Scan(&count)
	// Should keep: recent (1) + old important within 7d (1) = 2
	if count != 2 {
		t.Errorf("expected 2 heartbeats after cleanup, got %d", count)
	}
}
