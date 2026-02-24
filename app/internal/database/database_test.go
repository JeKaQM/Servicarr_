package database

import (
	"status/app/internal/models"
	"testing"
	"time"
)

func initTestDB(t *testing.T) {
	t.Helper()
	if err := Init(":memory:"); err != nil {
		t.Fatalf("failed to init test db: %v", err)
	}
}

// --------------- Init / EnsureSchema ---------------

func TestInit_InMemory(t *testing.T) {
	if err := Init(":memory:"); err != nil {
		t.Fatalf("Init failed: %v", err)
	}
	if DB == nil {
		t.Fatal("DB should be non-nil after Init")
	}
}

func TestEnsureSchema_Idempotent(t *testing.T) {
	initTestDB(t)
	// Calling EnsureSchema again should not error (CREATE IF NOT EXISTS)
	if err := EnsureSchema(); err != nil {
		t.Fatalf("second EnsureSchema call failed: %v", err)
	}
}

// --------------- InsertSample / Samples ---------------

func TestInsertSample(t *testing.T) {
	initTestDB(t)
	ms := 42
	InsertSample(time.Now(), "svc1", true, 200, &ms)

	var count int
	DB.QueryRow(`SELECT COUNT(*) FROM samples WHERE service_key = 'svc1'`).Scan(&count)
	if count != 1 {
		t.Errorf("expected 1 sample, got %d", count)
	}
}

func TestInsertSample_NilLatency(t *testing.T) {
	initTestDB(t)
	InsertSample(time.Now(), "svc2", false, 0, nil)

	var count int
	DB.QueryRow(`SELECT COUNT(*) FROM samples WHERE service_key = 'svc2'`).Scan(&count)
	if count != 1 {
		t.Errorf("expected 1 sample, got %d", count)
	}
}

func TestInsertSample_MultipleSamples(t *testing.T) {
	initTestDB(t)
	for i := 0; i < 10; i++ {
		ms := i * 10
		InsertSample(time.Now(), "svc-multi", true, 200, &ms)
	}
	var count int
	DB.QueryRow(`SELECT COUNT(*) FROM samples WHERE service_key = 'svc-multi'`).Scan(&count)
	if count != 10 {
		t.Errorf("expected 10 samples, got %d", count)
	}
}

// --------------- ServiceDisabledState ---------------

func TestGetServiceDisabledState_NotExists(t *testing.T) {
	initTestDB(t)
	disabled, err := GetServiceDisabledState("nonexistent")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if disabled {
		t.Error("should default to not disabled")
	}
}

func TestSetServiceDisabledState(t *testing.T) {
	initTestDB(t)
	if err := SetServiceDisabledState("svc1", true); err != nil {
		t.Fatalf("error: %v", err)
	}
	disabled, _ := GetServiceDisabledState("svc1")
	if !disabled {
		t.Error("should be disabled")
	}
}

func TestSetServiceDisabledState_Toggle(t *testing.T) {
	initTestDB(t)
	SetServiceDisabledState("svc1", true)
	SetServiceDisabledState("svc1", false)
	disabled, _ := GetServiceDisabledState("svc1")
	if disabled {
		t.Error("should be re-enabled")
	}
}

// --------------- Services CRUD ---------------

func sampleService(key string) *models.ServiceConfig {
	return &models.ServiceConfig{
		Key:           key,
		Name:          "Test " + key,
		URL:           "http://" + key + ".local",
		ServiceType:   "custom",
		Icon:          "server",
		CheckType:     "http",
		CheckInterval: 60,
		Timeout:       5,
		ExpectedMin:   200,
		ExpectedMax:   399,
		Visible:       true,
		DisplayOrder:  0,
	}
}

func TestCreateService(t *testing.T) {
	initTestDB(t)
	svc := sampleService("svc-create")
	id, err := CreateService(svc)
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if id == 0 {
		t.Error("expected non-zero ID")
	}
}

func TestCreateService_DuplicateKey(t *testing.T) {
	initTestDB(t)
	svc := sampleService("dup-key")
	CreateService(svc)
	_, err := CreateService(svc)
	if err == nil {
		t.Error("expected error for duplicate key")
	}
}

func TestCreateService_AutoDisplayOrder(t *testing.T) {
	initTestDB(t)
	svc1 := sampleService("svc-a")
	svc1.DisplayOrder = -1 // trigger auto-assign
	CreateService(svc1)

	svc2 := sampleService("svc-b")
	svc2.DisplayOrder = -1
	CreateService(svc2)

	got, _ := GetServiceByKey("svc-b")
	if got.DisplayOrder != 1 {
		t.Errorf("auto display_order = %d, want 1", got.DisplayOrder)
	}
}

func TestGetServiceByID(t *testing.T) {
	initTestDB(t)
	svc := sampleService("svc-byid")
	id, _ := CreateService(svc)

	got, err := GetServiceByID(int(id))
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if got.Key != "svc-byid" {
		t.Errorf("key = %q", got.Key)
	}
	if !got.Visible {
		t.Error("should be visible")
	}
}

func TestGetServiceByID_NotFound(t *testing.T) {
	initTestDB(t)
	_, err := GetServiceByID(9999)
	if err == nil {
		t.Error("expected error for non-existent ID")
	}
}

func TestGetServiceByKey(t *testing.T) {
	initTestDB(t)
	svc := sampleService("svc-bykey")
	CreateService(svc)

	got, err := GetServiceByKey("svc-bykey")
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if got.Name != "Test svc-bykey" {
		t.Errorf("name = %q", got.Name)
	}
}

func TestGetServiceByKey_NotFound(t *testing.T) {
	initTestDB(t)
	_, err := GetServiceByKey("ghost")
	if err == nil {
		t.Error("expected error for non-existent key")
	}
}

func TestGetAllServices(t *testing.T) {
	initTestDB(t)
	CreateService(sampleService("svc-a"))
	CreateService(sampleService("svc-b"))
	CreateService(sampleService("svc-c"))

	services, err := GetAllServices()
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if len(services) != 3 {
		t.Errorf("got %d services, want 3", len(services))
	}
}

func TestGetAllServices_Empty(t *testing.T) {
	initTestDB(t)
	services, err := GetAllServices()
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if len(services) != 0 {
		t.Errorf("expected 0 services, got %d", len(services))
	}
}

func TestGetVisibleServices(t *testing.T) {
	initTestDB(t)
	svcVisible := sampleService("visible-svc")
	svcVisible.Visible = true
	CreateService(svcVisible)

	svcHidden := sampleService("hidden-svc")
	svcHidden.Visible = false
	CreateService(svcHidden)

	services, err := GetVisibleServices()
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if len(services) != 1 {
		t.Errorf("got %d visible services, want 1", len(services))
	}
	if services[0].Key != "visible-svc" {
		t.Errorf("visible service key = %q", services[0].Key)
	}
}

func TestUpdateService(t *testing.T) {
	initTestDB(t)
	svc := sampleService("svc-update")
	id, _ := CreateService(svc)

	svc.ID = int(id)
	svc.Name = "Updated Name"
	svc.URL = "http://updated.local"
	svc.DependsOn = "dep-key"
	if err := UpdateService(svc); err != nil {
		t.Fatalf("error: %v", err)
	}

	got, _ := GetServiceByID(int(id))
	if got.Name != "Updated Name" {
		t.Errorf("name = %q", got.Name)
	}
	if got.URL != "http://updated.local" {
		t.Errorf("url = %q", got.URL)
	}
	if got.DependsOn != "dep-key" {
		t.Errorf("depends_on = %q", got.DependsOn)
	}
}

func TestDeleteService(t *testing.T) {
	initTestDB(t)
	svc := sampleService("svc-delete")
	id, _ := CreateService(svc)

	if err := DeleteService(int(id)); err != nil {
		t.Fatalf("error: %v", err)
	}
	_, err := GetServiceByID(int(id))
	if err == nil {
		t.Error("should not find service after delete")
	}
}

func TestDeleteService_NonExistent(t *testing.T) {
	initTestDB(t)
	// Should not error even if ID doesn't exist
	if err := DeleteService(99999); err != nil {
		t.Errorf("delete non-existent should not error: %v", err)
	}
}

func TestUpdateServiceVisibility(t *testing.T) {
	initTestDB(t)
	svc := sampleService("svc-vis")
	svc.Visible = true
	id, _ := CreateService(svc)

	UpdateServiceVisibility(int(id), false)
	got, _ := GetServiceByID(int(id))
	if got.Visible {
		t.Error("should be hidden")
	}

	UpdateServiceVisibility(int(id), true)
	got, _ = GetServiceByID(int(id))
	if !got.Visible {
		t.Error("should be visible again")
	}
}

func TestUpdateServiceOrder(t *testing.T) {
	initTestDB(t)
	id1, _ := CreateService(sampleService("svc-order-a"))
	id2, _ := CreateService(sampleService("svc-order-b"))

	orders := map[int]int{
		int(id1): 10,
		int(id2): 5,
	}
	if err := UpdateServiceOrder(orders); err != nil {
		t.Fatalf("error: %v", err)
	}

	got1, _ := GetServiceByID(int(id1))
	got2, _ := GetServiceByID(int(id2))
	if got1.DisplayOrder != 10 {
		t.Errorf("svc-a order = %d, want 10", got1.DisplayOrder)
	}
	if got2.DisplayOrder != 5 {
		t.Errorf("svc-b order = %d, want 5", got2.DisplayOrder)
	}
}

func TestGetServiceCount(t *testing.T) {
	initTestDB(t)
	CreateService(sampleService("count-a"))
	CreateService(sampleService("count-b"))

	count, err := GetServiceCount()
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if count != 2 {
		t.Errorf("count = %d, want 2", count)
	}
}

func TestGetServiceCount_Empty(t *testing.T) {
	initTestDB(t)
	count, err := GetServiceCount()
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if count != 0 {
		t.Errorf("count = %d, want 0", count)
	}
}

// --------------- AppSettings ---------------

func TestIsSetupComplete_NoSettings(t *testing.T) {
	initTestDB(t)
	complete, err := IsSetupComplete()
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if complete {
		t.Error("should not be complete with no settings")
	}
}

func TestSaveAndLoadAppSettings(t *testing.T) {
	initTestDB(t)
	settings := &models.AppSettings{
		SetupComplete: true,
		Username:      "admin",
		PasswordHash:  "$2a$10$hash",
		AuthSecret:    "secret123",
		AppName:       "My Status Page",
	}
	if err := SaveAppSettings(settings); err != nil {
		t.Fatalf("save error: %v", err)
	}

	loaded, err := LoadAppSettings()
	if err != nil {
		t.Fatalf("load error: %v", err)
	}
	if loaded.Username != "admin" {
		t.Errorf("username = %q", loaded.Username)
	}
	if loaded.AppName != "My Status Page" {
		t.Errorf("app_name = %q", loaded.AppName)
	}
	if !loaded.SetupComplete {
		t.Error("should be setup complete")
	}
}

func TestSaveAppSettings_DefaultAppName(t *testing.T) {
	initTestDB(t)
	settings := &models.AppSettings{
		SetupComplete: true,
		Username:      "admin",
		PasswordHash:  "hash",
		AuthSecret:    "sec",
		AppName:       "",
	}
	SaveAppSettings(settings)
	loaded, _ := LoadAppSettings()
	if loaded.AppName != "Service Status" {
		t.Errorf("app_name should default to 'Service Status', got %q", loaded.AppName)
	}
}

func TestSaveAppSettings_Upsert(t *testing.T) {
	initTestDB(t)
	s1 := &models.AppSettings{Username: "user1", PasswordHash: "h1", AuthSecret: "s1"}
	SaveAppSettings(s1)

	s2 := &models.AppSettings{Username: "user2", PasswordHash: "h2", AuthSecret: "s2"}
	SaveAppSettings(s2)

	loaded, _ := LoadAppSettings()
	if loaded.Username != "user2" {
		t.Errorf("should be upserted to user2, got %q", loaded.Username)
	}
}

func TestIsSetupComplete_AfterSave(t *testing.T) {
	initTestDB(t)
	SaveAppSettings(&models.AppSettings{
		SetupComplete: true,
		Username:      "admin",
		PasswordHash:  "h",
		AuthSecret:    "s",
	})
	complete, _ := IsSetupComplete()
	if !complete {
		t.Error("should be complete after setting SetupComplete=true")
	}
}

// --------------- AlertConfig ---------------

func TestLoadAlertConfig_NoRow(t *testing.T) {
	initTestDB(t)
	config, err := LoadAlertConfig()
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if config != nil {
		t.Error("expected nil when no alert_config row")
	}
}

func TestSaveAndLoadAlertConfig(t *testing.T) {
	initTestDB(t)
	cfg := &models.AlertConfig{
		Enabled:        true,
		SMTPHost:       "smtp.test.com",
		SMTPPort:       465,
		SMTPUser:       "user@test.com",
		SMTPPassword:   "pass",
		AlertEmail:     "alerts@test.com",
		FromEmail:      "from@test.com",
		StatusPageURL:  "http://status.test.com",
		SMTPSkipVerify: true,
		AlertOnDown:    true,
		AlertOnDegraded: true,
		AlertOnUp:      false,
		DiscordWebhookURL: "https://discord.com/api/webhooks/123",
		DiscordEnabled: true,
		TelegramBotToken: "bot123",
		TelegramChatID:   "456",
		TelegramEnabled:  false,
		WebhookURL:       "https://hooks.test.com/webhook",
		WebhookSecret:    "secret",
		WebhookEnabled:   true,
	}
	SaveAlertConfig(cfg)

	loaded, err := LoadAlertConfig()
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if loaded == nil {
		t.Fatal("expected non-nil config")
	}
	if loaded.SMTPHost != "smtp.test.com" {
		t.Errorf("smtp_host = %q", loaded.SMTPHost)
	}
	if loaded.SMTPPort != 465 {
		t.Errorf("smtp_port = %d", loaded.SMTPPort)
	}
	if !loaded.Enabled {
		t.Error("should be enabled")
	}
	if !loaded.DiscordEnabled {
		t.Error("discord should be enabled")
	}
	if loaded.DiscordWebhookURL != "https://discord.com/api/webhooks/123" {
		t.Errorf("discord url = %q", loaded.DiscordWebhookURL)
	}
	if loaded.WebhookSecret != "secret" {
		t.Errorf("webhook_secret = %q", loaded.WebhookSecret)
	}
	if !loaded.WebhookEnabled {
		t.Error("webhook should be enabled")
	}
	if loaded.StatusPageURL != "http://status.test.com" {
		t.Errorf("status_page_url = %q", loaded.StatusPageURL)
	}
}

func TestSaveAlertConfig_Upsert(t *testing.T) {
	initTestDB(t)
	cfg1 := &models.AlertConfig{Enabled: true, SMTPHost: "host1"}
	SaveAlertConfig(cfg1)

	cfg2 := &models.AlertConfig{Enabled: false, SMTPHost: "host2"}
	SaveAlertConfig(cfg2)

	loaded, _ := LoadAlertConfig()
	if loaded.SMTPHost != "host2" {
		t.Errorf("should be upserted to host2, got %q", loaded.SMTPHost)
	}
	if loaded.Enabled {
		t.Error("should be disabled after upsert")
	}
}

// --------------- ResourcesUIConfig ---------------

func TestLoadResourcesUIConfig_NoRow(t *testing.T) {
	initTestDB(t)
	config, err := LoadResourcesUIConfig()
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if config != nil {
		t.Error("expected nil when no row")
	}
}

func TestSaveAndLoadResourcesUIConfig(t *testing.T) {
	initTestDB(t)
	cfg := &models.ResourcesUIConfig{
		Enabled:    true,
		GlancesURL: "http://10.0.0.1:61208",
		CPU:        true,
		Memory:     true,
		Network:    true,
		Temp:       true,
		Storage:    true,
		Swap:       false,
		Load:       false,
		GPU:        true,
		Containers: true,
		Processes:  false,
		Uptime:     true,
	}
	SaveResourcesUIConfig(cfg)

	loaded, err := LoadResourcesUIConfig()
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if loaded == nil {
		t.Fatal("expected non-nil config")
	}
	if !loaded.Enabled {
		t.Error("should be enabled")
	}
	if loaded.GlancesURL != "http://10.0.0.1:61208" {
		t.Errorf("glances_url = %q", loaded.GlancesURL)
	}
	if !loaded.GPU {
		t.Error("gpu should be true")
	}
	if loaded.Swap {
		t.Error("swap should be false")
	}
	if !loaded.Uptime {
		t.Error("uptime should be true")
	}
}

// --------------- Logs ---------------

func TestInsertLog(t *testing.T) {
	initTestDB(t)
	err := InsertLog(LogLevelInfo, LogCategoryCheck, "svc1", "Service checked", "all good")
	if err != nil {
		t.Fatalf("error: %v", err)
	}

	logs, _ := GetLogs(10, "", "", "", 0)
	if len(logs) != 1 {
		t.Fatalf("got %d logs, want 1", len(logs))
	}
	if logs[0].Level != "info" {
		t.Errorf("level = %q", logs[0].Level)
	}
	if logs[0].Category != "check" {
		t.Errorf("category = %q", logs[0].Category)
	}
	if logs[0].Message != "Service checked" {
		t.Errorf("message = %q", logs[0].Message)
	}
}

func TestGetLogs_FilterByLevel(t *testing.T) {
	initTestDB(t)
	InsertLog(LogLevelInfo, LogCategoryCheck, "", "info msg", "")
	InsertLog(LogLevelError, LogCategoryCheck, "", "error msg", "")
	InsertLog(LogLevelWarn, LogCategoryCheck, "", "warn msg", "")

	logs, _ := GetLogs(10, "error", "", "", 0)
	if len(logs) != 1 {
		t.Errorf("got %d logs, want 1 error", len(logs))
	}
}

func TestGetLogs_FilterByCategory(t *testing.T) {
	initTestDB(t)
	InsertLog(LogLevelInfo, LogCategoryCheck, "", "check", "")
	InsertLog(LogLevelInfo, LogCategoryEmail, "", "email", "")
	InsertLog(LogLevelInfo, LogCategorySecurity, "", "security", "")

	logs, _ := GetLogs(10, "", "email", "", 0)
	if len(logs) != 1 {
		t.Errorf("got %d logs, want 1 email log", len(logs))
	}
}

func TestGetLogs_FilterByService(t *testing.T) {
	initTestDB(t)
	InsertLog(LogLevelInfo, LogCategoryCheck, "svc-a", "msg a", "")
	InsertLog(LogLevelInfo, LogCategoryCheck, "svc-b", "msg b", "")

	logs, _ := GetLogs(10, "", "", "svc-a", 0)
	if len(logs) != 1 {
		t.Errorf("got %d logs, want 1", len(logs))
	}
	if logs[0].Service != "svc-a" {
		t.Errorf("service = %q", logs[0].Service)
	}
}

func TestGetLogs_LimitAndOffset(t *testing.T) {
	initTestDB(t)
	for i := 0; i < 20; i++ {
		InsertLog(LogLevelInfo, LogCategoryCheck, "", "msg", "")
	}

	logs, _ := GetLogs(5, "", "", "", 0)
	if len(logs) != 5 {
		t.Errorf("got %d logs, want 5", len(logs))
	}

	logs2, _ := GetLogs(5, "", "", "", 15)
	if len(logs2) != 5 {
		t.Errorf("got %d logs with offset 15, want 5", len(logs2))
	}
}

func TestGetLogStats(t *testing.T) {
	initTestDB(t)
	InsertLog(LogLevelInfo, LogCategoryCheck, "", "info", "")
	InsertLog(LogLevelInfo, LogCategoryCheck, "", "info2", "")
	InsertLog(LogLevelError, LogCategoryCheck, "", "error", "")
	InsertLog(LogLevelWarn, LogCategoryCheck, "", "warn", "")
	InsertLog(LogLevelDebug, LogCategoryCheck, "", "debug", "")

	stats, err := GetLogStats()
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if stats.TotalLogs != 5 {
		t.Errorf("total = %d, want 5", stats.TotalLogs)
	}
	if stats.InfoCount != 2 {
		t.Errorf("info = %d, want 2", stats.InfoCount)
	}
	if stats.ErrorCount != 1 {
		t.Errorf("error = %d, want 1", stats.ErrorCount)
	}
	if stats.WarnCount != 1 {
		t.Errorf("warn = %d, want 1", stats.WarnCount)
	}
	if stats.DebugCount != 1 {
		t.Errorf("debug = %d, want 1", stats.DebugCount)
	}
}

func TestClearLogs_All(t *testing.T) {
	initTestDB(t)
	InsertLog(LogLevelInfo, LogCategoryCheck, "", "msg", "")
	InsertLog(LogLevelError, LogCategoryCheck, "", "msg", "")

	if err := ClearLogs(0); err != nil {
		t.Fatalf("error: %v", err)
	}

	var count int
	DB.QueryRow(`SELECT COUNT(*) FROM system_logs`).Scan(&count)
	if count != 0 {
		t.Errorf("expected 0 logs after clear, got %d", count)
	}
}

func TestClearLogs_OlderThanDays(t *testing.T) {
	initTestDB(t)
	// Insert a recent log
	InsertLog(LogLevelInfo, LogCategoryCheck, "", "recent", "")
	// Insert an old log (manually set timestamp)
	DB.Exec(`INSERT INTO system_logs (timestamp, level, category, service, message, details) VALUES (datetime('now', '-10 days'), 'info', 'check', '', 'old', '')`)

	ClearLogs(5) // clear logs older than 5 days

	logs, _ := GetLogs(100, "", "", "", 0)
	if len(logs) != 1 {
		t.Errorf("expected 1 log (recent), got %d", len(logs))
	}
}

func TestPruneLogs(t *testing.T) {
	initTestDB(t)
	for i := 0; i < 20; i++ {
		InsertLog(LogLevelInfo, LogCategoryCheck, "", "msg", "")
	}

	if err := PruneLogs(5); err != nil {
		t.Fatalf("error: %v", err)
	}

	var count int
	DB.QueryRow(`SELECT COUNT(*) FROM system_logs`).Scan(&count)
	if count != 5 {
		t.Errorf("expected 5 logs after prune, got %d", count)
	}
}
