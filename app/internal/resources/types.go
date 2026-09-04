package resources

import "time"

// Snapshot is a normalized, UI-friendly view of system resources.
// Values are best-effort based on Glances API v4.
//
// All percentages are 0..100.
// All bytes values are raw bytes.
//
// If a field is nil, it means the source metric was unavailable.
type Snapshot struct {
	TakenAt time.Time `json:"taken_at"`

	Host     string `json:"host"`
	Platform string `json:"platform"`

	UptimeSeconds *float64 `json:"uptime_seconds,omitempty"`
	UptimeString  string   `json:"uptime_string,omitempty"`

	CPUPercent            *float64               `json:"cpu_percent,omitempty"`
	CPUUserPercent        *float64               `json:"cpu_user_percent,omitempty"`
	CPUSystemPercent      *float64               `json:"cpu_system_percent,omitempty"`
	CPUIOWaitPercent      *float64               `json:"cpu_iowait_percent,omitempty"`
	CPUIdlePercent        *float64               `json:"cpu_idle_percent,omitempty"`
	CPUCores              *uint64                `json:"cpu_cores,omitempty"`
	CPUAvgTempC           *float64               `json:"cpu_avg_temp_c,omitempty"`
	CPUPerCorePercent     []float64              `json:"cpu_per_core_percent,omitempty"`
	CPUCoreMetrics        []CPUCoreMetric        `json:"cpu_core_metrics,omitempty"`
	CPUTemperatureSensors []CPUTemperatureSensor `json:"cpu_temperature_sensors,omitempty"`

	Load1  *float64 `json:"load_1,omitempty"`
	Load5  *float64 `json:"load_5,omitempty"`
	Load15 *float64 `json:"load_15,omitempty"`

	MemUsedBytes  *uint64  `json:"mem_used_bytes,omitempty"`
	MemTotalBytes *uint64  `json:"mem_total_bytes,omitempty"`
	MemPercent    *float64 `json:"mem_percent,omitempty"`

	SwapUsedBytes  *uint64  `json:"swap_used_bytes,omitempty"`
	SwapTotalBytes *uint64  `json:"swap_total_bytes,omitempty"`
	SwapPercent    *float64 `json:"swap_percent,omitempty"`

	ProcTotal    *uint64 `json:"proc_total,omitempty"`
	ProcRunning  *uint64 `json:"proc_running,omitempty"`
	ProcSleeping *uint64 `json:"proc_sleeping,omitempty"`
	ProcThreads  *uint64 `json:"proc_threads,omitempty"`

	TempC    *float64 `json:"temp_c,omitempty"`
	TempMinC *float64 `json:"temp_min_c,omitempty"`
	TempMaxC *float64 `json:"temp_max_c,omitempty"`

	NetRxBytesPerSec *float64 `json:"net_rx_bytes_per_sec,omitempty"`
	NetTxBytesPerSec *float64 `json:"net_tx_bytes_per_sec,omitempty"`

	DiskReadBytesPerSec  *float64 `json:"disk_read_bytes_per_sec,omitempty"`
	DiskWriteBytesPerSec *float64 `json:"disk_write_bytes_per_sec,omitempty"`

	FSTotalBytes  *uint64  `json:"fs_total_bytes,omitempty"`
	FSUsedBytes   *uint64  `json:"fs_used_bytes,omitempty"`
	FSFreeBytes   *uint64  `json:"fs_free_bytes,omitempty"`
	FSUsedPercent *float64 `json:"fs_used_percent,omitempty"`

	// GPU metrics
	GPUName    string   `json:"gpu_name,omitempty"`
	GPUPercent *float64 `json:"gpu_percent,omitempty"`
	GPUMemPct  *float64 `json:"gpu_mem_percent,omitempty"`
	GPUTempC   *float64 `json:"gpu_temp_c,omitempty"`

	// Container metrics
	ContainerCount   *uint64         `json:"container_count,omitempty"`
	ContainerRunning *uint64         `json:"container_running,omitempty"`
	Containers       []ContainerInfo `json:"containers,omitempty"`

	// UPS metrics from Network UPS Tools (NUT)
	UPS *UPSInfo `json:"ups,omitempty"`
}

// CPUCoreMetric holds the measurements Glances reports for one logical CPU.
// A nil reading means the host did not expose that measurement for this CPU.
type CPUCoreMetric struct {
	Index       uint64   `json:"index"`
	LoadPercent *float64 `json:"load_percent,omitempty"`
	TempC       *float64 `json:"temp_c,omitempty"`
}

// CPUTemperatureSensor is a CPU thermal reading that is not tied to one core.
type CPUTemperatureSensor struct {
	Label string  `json:"label"`
	TempC float64 `json:"temp_c"`
}

// ContainerInfo holds basic container stats
type ContainerInfo struct {
	Name       string   `json:"name"`
	Status     string   `json:"status"`
	CPUPercent *float64 `json:"cpu_percent,omitempty"`
	MemPercent *float64 `json:"mem_percent,omitempty"`
}

// UPSInfo is a normalized, UI-friendly view of a NUT UPS.
type UPSInfo struct {
	Name string `json:"name"`

	Manufacturer string `json:"manufacturer,omitempty"`
	Model        string `json:"model,omitempty"`
	Serial       string `json:"serial,omitempty"`

	Status       string `json:"status,omitempty"`
	StatusText   string `json:"status_text,omitempty"`
	PowerPresent *bool  `json:"power_present,omitempty"`

	BatteryChargePercent     *float64 `json:"battery_charge_percent,omitempty"`
	BatteryRuntimeSeconds    *float64 `json:"battery_runtime_seconds,omitempty"`
	BatteryRuntimeLowSeconds *float64 `json:"battery_runtime_low_seconds,omitempty"`
	BatteryVoltage           *float64 `json:"battery_voltage,omitempty"`
	BatteryVoltageNominal    *float64 `json:"battery_voltage_nominal,omitempty"`
	BatteryType              string   `json:"battery_type,omitempty"`

	LoadPercent          *float64 `json:"load_percent,omitempty"`
	OutputPowerWatt      *float64 `json:"output_power_watt,omitempty"`
	OutputPowerEstimated bool     `json:"output_power_estimated,omitempty"`
	InputVoltage         *float64 `json:"input_voltage,omitempty"`
	InputVoltageNominal  *float64 `json:"input_voltage_nominal,omitempty"`
	RealPowerNominalWatt *float64 `json:"realpower_nominal_watt,omitempty"`

	TestResult string `json:"test_result,omitempty"`
}
