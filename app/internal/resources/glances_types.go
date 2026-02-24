package resources

// glancesCPU represents the CPU response from Glances API
type glancesCPU struct {
	Total   interface{} `json:"total"`
	User    interface{} `json:"user"`
	System  interface{} `json:"system"`
	IOWait  interface{} `json:"iowait"`
	Idle    interface{} `json:"idle"`
	CPUCore interface{} `json:"cpucore"`
}

// glancesLoad represents the load average response from Glances API
type glancesLoad struct {
	Min1    interface{} `json:"min1"`
	Min5    interface{} `json:"min5"`
	Min15   interface{} `json:"min15"`
	CPUCore interface{} `json:"cpucore"`
}

// glancesMem represents the memory response from Glances API
type glancesMem struct {
	Total   interface{} `json:"total"`
	Used    interface{} `json:"used"`
	Percent interface{} `json:"percent"`
}

// glancesSwap represents the swap response from Glances API
type glancesSwap struct {
	Total   interface{} `json:"total"`
	Used    interface{} `json:"used"`
	Percent interface{} `json:"percent"`
}

// glancesProcessCount represents the process count response from Glances API
type glancesProcessCount struct {
	Total    interface{} `json:"total"`
	Running  interface{} `json:"running"`
	Sleeping interface{} `json:"sleeping"`
	Thread   interface{} `json:"thread"`
}

// glancesSensor represents a sensor reading from Glances API
type glancesSensor struct {
	Label    interface{} `json:"label"`
	Unit     interface{} `json:"unit"`
	Value    interface{} `json:"value"`
	Warning  interface{} `json:"warning"`
	Critical interface{} `json:"critical"`
	Type     interface{} `json:"type"`
}

// glancesSystem represents the system info response from Glances API
type glancesSystem struct {
	Hostname interface{} `json:"hostname"`
	OSName   interface{} `json:"os_name"`
	Platform interface{} `json:"platform"`
	Uptime   interface{} `json:"uptime"`
}

// glancesNet represents a network interface from Glances API
type glancesNet struct {
	InterfaceName       interface{} `json:"interface_name"`
	BytesRecvRatePerSec interface{} `json:"bytes_recv_rate_per_sec"`
	BytesSentRatePerSec interface{} `json:"bytes_sent_rate_per_sec"`
	RxRate              interface{} `json:"rx_rate"`
	TxRate              interface{} `json:"tx_rate"`
}

// glancesPerCPU represents per-core CPU usage from Glances API
type glancesPerCPU struct {
	CPUNumber interface{} `json:"cpu_number"`
	Total     interface{} `json:"total"`
}

// glancesDiskIO represents disk I/O stats from Glances API
type glancesDiskIO struct {
	DiskName             interface{} `json:"disk_name"`
	ReadBytesRatePerSec  interface{} `json:"read_bytes_rate_per_sec"`
	WriteBytesRatePerSec interface{} `json:"write_bytes_rate_per_sec"`
}

// glancesFS represents a filesystem entry from Glances API
type glancesFS struct {
	DeviceName interface{} `json:"device_name"`
	FSType     interface{} `json:"fs_type"`
	MountPoint interface{} `json:"mnt_point"`
	Options    interface{} `json:"options"`
	Size       interface{} `json:"size"`
	Used       interface{} `json:"used"`
	Free       interface{} `json:"free"`
	Percent    interface{} `json:"percent"`
}

// glancesGPU represents GPU stats from Glances API
type glancesGPU struct {
	GPUID       interface{} `json:"gpu_id"`
	Name        interface{} `json:"name"`
	Mem         interface{} `json:"mem"`
	Proc        interface{} `json:"proc"`
	Temperature interface{} `json:"temperature"`
}

// glancesContainer represents container stats from Glances API
type glancesContainer struct {
	Name       interface{} `json:"name"`
	Status     interface{} `json:"status"`
	CPUPercent interface{} `json:"cpu_percent"`
	MemUsage   interface{} `json:"memory_usage"`
	MemLimit   interface{} `json:"memory_limit"`
}
