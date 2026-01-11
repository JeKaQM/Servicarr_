package resources

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"
)

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

	CPUPercent        *float64  `json:"cpu_percent,omitempty"`
	CPUUserPercent    *float64  `json:"cpu_user_percent,omitempty"`
	CPUSystemPercent  *float64  `json:"cpu_system_percent,omitempty"`
	CPUIOWaitPercent  *float64  `json:"cpu_iowait_percent,omitempty"`
	CPUIdlePercent    *float64  `json:"cpu_idle_percent,omitempty"`
	CPUCores          *uint64   `json:"cpu_cores,omitempty"`
	CPUPerCorePercent []float64 `json:"cpu_per_core_percent,omitempty"`

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
}

type Client struct {
	BaseURL string
	HTTP    *http.Client

	mu        sync.Mutex
	cachedAt  time.Time
	cached    Snapshot
	cacheErr  error
	cacheFor  time.Duration
	inFlight  bool
	inFlightC *sync.Cond

	// Track min/max temperature observed during this process lifetime.
	tempSeen bool
	tempMin  float64
	tempMax  float64
}

func NewClient(baseURL string) *Client {
	c := &Client{
		BaseURL:  baseURL,
		HTTP:     &http.Client{Timeout: 6 * time.Second},
		cacheFor: 5 * time.Second,
	}
	c.inFlightC = sync.NewCond(&c.mu)
	return c
}

func (c *Client) SetCacheTTL(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.cacheFor = d
}

func (c *Client) getJSON(ctx context.Context, path string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL+path, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("glances http %d", resp.StatusCode)
	}

	dec := json.NewDecoder(resp.Body)
	dec.UseNumber()
	return dec.Decode(out)
}

type glancesCPU struct {
	Total interface{} `json:"total"`
	// Some Glances builds expose "total" as a number; we keep it loose.
	User    interface{} `json:"user"`
	System  interface{} `json:"system"`
	IOWait  interface{} `json:"iowait"`
	Idle    interface{} `json:"idle"`
	CPUCore interface{} `json:"cpucore"`
}

type glancesLoad struct {
	Min1    interface{} `json:"min1"`
	Min5    interface{} `json:"min5"`
	Min15   interface{} `json:"min15"`
	CPUCore interface{} `json:"cpucore"`
}

type glancesMem struct {
	Total   interface{} `json:"total"`
	Used    interface{} `json:"used"`
	Percent interface{} `json:"percent"`
}

type glancesSwap struct {
	Total   interface{} `json:"total"`
	Used    interface{} `json:"used"`
	Percent interface{} `json:"percent"`
}

type glancesProcessCount struct {
	Total    interface{} `json:"total"`
	Running  interface{} `json:"running"`
	Sleeping interface{} `json:"sleeping"`
}

type glancesSensor struct {
	Label    interface{} `json:"label"`
	Unit     interface{} `json:"unit"`
	Value    interface{} `json:"value"`
	Warning  interface{} `json:"warning"`
	Critical interface{} `json:"critical"`
	Type     interface{} `json:"type"`
}

type glancesSystem struct {
	Hostname interface{} `json:"hostname"`
	OSName   interface{} `json:"os_name"`
	Platform interface{} `json:"platform"`
	Uptime   interface{} `json:"uptime"`
}

type glancesNet struct {
	InterfaceName interface{} `json:"interface_name"`
	// Your Glances exposes bytes_* and bytes_*_rate_per_sec
	BytesRecvRatePerSec interface{} `json:"bytes_recv_rate_per_sec"`
	BytesSentRatePerSec interface{} `json:"bytes_sent_rate_per_sec"`

	// Keep compatibility with other Glances builds
	RxRate interface{} `json:"rx_rate"`
	TxRate interface{} `json:"tx_rate"`
}

type glancesPerCPU struct {
	CPUNumber interface{} `json:"cpu_number"`
	Total     interface{} `json:"total"`
}

type glancesDiskIO struct {
	DiskName             interface{} `json:"disk_name"`
	ReadBytesRatePerSec  interface{} `json:"read_bytes_rate_per_sec"`
	WriteBytesRatePerSec interface{} `json:"write_bytes_rate_per_sec"`
}

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

func asFloatPtr(v interface{}) *float64 {
	switch x := v.(type) {
	case nil:
		return nil
	case float64:
		return &x
	case string:
		// Some APIs/decoders might return numbers as strings.
		n := json.Number(x)
		f, err := n.Float64()
		if err != nil {
			return nil
		}
		return &f
	case int:
		f := float64(x)
		return &f
	case int64:
		f := float64(x)
		return &f
	case json.Number:
		f, err := x.Float64()
		if err != nil {
			return nil
		}
		return &f
	default:
		return nil
	}
}

func asUint64Ptr(v interface{}) *uint64 {
	switch x := v.(type) {
	case nil:
		return nil
	case float64:
		if x < 0 {
			return nil
		}
		u := uint64(x)
		return &u
	case int:
		if x < 0 {
			return nil
		}
		u := uint64(x)
		return &u
	case int64:
		if x < 0 {
			return nil
		}
		u := uint64(x)
		return &u
	case json.Number:
		f, err := x.Float64()
		if err != nil || f < 0 {
			return nil
		}
		u := uint64(f)
		return &u
	default:
		return nil
	}
}

func (c *Client) FetchSnapshot(ctx context.Context) (Snapshot, error) {
	// Cache with coalescing to avoid a thundering herd.
	c.mu.Lock()
	if time.Since(c.cachedAt) < c.cacheFor {
		s := c.cached
		err := c.cacheErr
		c.mu.Unlock()
		return s, err
	}
	if c.inFlight {
		for c.inFlight {
			c.inFlightC.Wait()
		}
		// after wakeup, return whatever was cached
		s := c.cached
		err := c.cacheErr
		c.mu.Unlock()
		return s, err
	}
	c.inFlight = true
	c.mu.Unlock()

	// Always release inFlight.
	defer func() {
		c.mu.Lock()
		c.inFlight = false
		c.inFlightC.Broadcast()
		c.mu.Unlock()
	}()

	// NOTE: Glances endpoints are under /api/4/...
	var sys glancesSystem
	var cpu glancesCPU
	var load glancesLoad
	var mem glancesMem
	var swap glancesSwap
	var pc glancesProcessCount
	var sensors []glancesSensor
	var nets []glancesNet
	var percpu []glancesPerCPU
	var diskio []glancesDiskIO
	var fs []glancesFS

	// Best-effort: ignore individual errors but return a combined error if too many fail.
	errCount := 0

	if err := c.getJSON(ctx, "/system", &sys); err != nil {
		errCount++
	}
	if err := c.getJSON(ctx, "/cpu", &cpu); err != nil {
		errCount++
	}
	if err := c.getJSON(ctx, "/load", &load); err != nil {
		errCount++
	}
	if err := c.getJSON(ctx, "/mem", &mem); err != nil {
		errCount++
	}
	if err := c.getJSON(ctx, "/memswap", &swap); err != nil {
		errCount++
	}
	if err := c.getJSON(ctx, "/processcount", &pc); err != nil {
		errCount++
	}
	if err := c.getJSON(ctx, "/sensors", &sensors); err != nil {
		errCount++
	}
	if err := c.getJSON(ctx, "/network", &nets); err != nil {
		errCount++
	}
	if err := c.getJSON(ctx, "/percpu", &percpu); err != nil {
		// Optional in some builds
	}
	if err := c.getJSON(ctx, "/diskio", &diskio); err != nil {
		// Optional in some builds
	}
	if err := c.getJSON(ctx, "/fs", &fs); err != nil {
		// Optional in some builds
	}

	// If everything failed, surface an error.
	if errCount >= 7 {
		s := Snapshot{TakenAt: time.Now().UTC()}
		c.mu.Lock()
		c.cachedAt = time.Now()
		c.cached = s
		c.cacheErr = fmt.Errorf("failed to fetch glances resources")
		c.mu.Unlock()
		return s, c.cacheErr
	}

	s := Snapshot{TakenAt: time.Now().UTC()}
	if h, ok := sys.Hostname.(string); ok {
		s.Host = h
	}
	if p, ok := sys.Platform.(string); ok {
		s.Platform = p
	}
	if u := asFloatPtr(sys.Uptime); u != nil {
		s.UptimeSeconds = u
	}

	// cpu.total
	// Your Glances: {"total": 5.6, ...}
	s.CPUPercent = asFloatPtr(cpu.Total)
	if s.CPUPercent == nil {
		// some builds might nest, but common is {"total": {"total": 12.3}}
		if m, ok := cpu.Total.(map[string]any); ok {
			if inner, ok := m["total"]; ok {
				s.CPUPercent = asFloatPtr(inner)
			}
		}
	}
	s.CPUUserPercent = asFloatPtr(cpu.User)
	s.CPUSystemPercent = asFloatPtr(cpu.System)
	s.CPUIOWaitPercent = asFloatPtr(cpu.IOWait)
	s.CPUIdlePercent = asFloatPtr(cpu.Idle)
	// prefer cpu.cpucore, fallback to load.cpucore
	if cN := asUint64Ptr(cpu.CPUCore); cN != nil {
		s.CPUCores = cN
	} else if cN := asUint64Ptr(load.CPUCore); cN != nil {
		s.CPUCores = cN
	}

	// load averages
	s.Load1 = asFloatPtr(load.Min1)
	s.Load5 = asFloatPtr(load.Min5)
	s.Load15 = asFloatPtr(load.Min15)

	// mem
	s.MemTotalBytes = asUint64Ptr(mem.Total)
	s.MemUsedBytes = asUint64Ptr(mem.Used)
	s.MemPercent = asFloatPtr(mem.Percent)
	if s.MemPercent == nil && s.MemTotalBytes != nil && s.MemUsedBytes != nil && *s.MemTotalBytes > 0 {
		p := (float64(*s.MemUsedBytes) / float64(*s.MemTotalBytes)) * 100
		s.MemPercent = &p
	}

	// swap
	s.SwapTotalBytes = asUint64Ptr(swap.Total)
	s.SwapUsedBytes = asUint64Ptr(swap.Used)
	s.SwapPercent = asFloatPtr(swap.Percent)
	if s.SwapPercent == nil && s.SwapTotalBytes != nil && s.SwapUsedBytes != nil && *s.SwapTotalBytes > 0 {
		p := (float64(*s.SwapUsedBytes) / float64(*s.SwapTotalBytes)) * 100
		s.SwapPercent = &p
	}

	// process counts
	s.ProcTotal = asUint64Ptr(pc.Total)
	s.ProcRunning = asUint64Ptr(pc.Running)
	s.ProcSleeping = asUint64Ptr(pc.Sleeping)

	// temperature: take the highest core temp we can find
	var bestTemp *float64
	for _, sen := range sensors {
		// Only temperature sensors
		if t, ok := sen.Type.(string); ok {
			if t != "temperature_core" && t != "temperature" {
				continue
			}
		}
		// Prefer Celsius readings
		if u, ok := sen.Unit.(string); ok {
			if u != "C" && u != "°C" {
				continue
			}
		}
		if v := asFloatPtr(sen.Value); v != nil {
			if bestTemp == nil || *v > *bestTemp {
				bestTemp = v
			}
		}
	}
	if bestTemp != nil {
		s.TempC = bestTemp
		// Update running min/max for the process lifetime.
		c.mu.Lock()
		if !c.tempSeen {
			c.tempSeen = true
			c.tempMin = *bestTemp
			c.tempMax = *bestTemp
		} else {
			if *bestTemp < c.tempMin {
				c.tempMin = *bestTemp
			}
			if *bestTemp > c.tempMax {
				c.tempMax = *bestTemp
			}
		}
		min := c.tempMin
		max := c.tempMax
		c.mu.Unlock()
		s.TempMinC = &min
		s.TempMaxC = &max
	}

	// network: sum rates across interfaces (ignore loopback)
	var rxRate, txRate float64
	var hasRate bool
	for _, n := range nets {
		if name, ok := n.InterfaceName.(string); ok {
			if name == "lo" || name == "lo0" {
				continue
			}
		}

		// Prefer bytes_*_rate_per_sec (your schema)
		if r := asFloatPtr(n.BytesRecvRatePerSec); r != nil {
			rxRate += *r
			hasRate = true
		} else if r := asFloatPtr(n.RxRate); r != nil {
			rxRate += *r
			hasRate = true
		}

		if t := asFloatPtr(n.BytesSentRatePerSec); t != nil {
			txRate += *t
			hasRate = true
		} else if t := asFloatPtr(n.TxRate); t != nil {
			txRate += *t
			hasRate = true
		}
	}
	if hasRate {
		s.NetRxBytesPerSec = &rxRate
		s.NetTxBytesPerSec = &txRate
	}

	// per-cpu totals: order by cpu_number and export totals
	if len(percpu) > 0 {
		// We’ll store by cpu index, compacted into a slice.
		maxIdx := -1
		vals := map[int]float64{}
		for _, p := range percpu {
			idxPtr := asUint64Ptr(p.CPUNumber)
			valPtr := asFloatPtr(p.Total)
			if idxPtr == nil || valPtr == nil {
				continue
			}
			// G115: bounds check to prevent integer overflow
			if *idxPtr > uint64(^uint(0)>>1) {
				continue
			}
			idx := int(*idxPtr) // #nosec G115 -- bounds checked above
			if idx < 0 {
				continue
			}
			vals[idx] = *valPtr
			if idx > maxIdx {
				maxIdx = idx
			}
		}
		if maxIdx >= 0 {
			out := make([]float64, maxIdx+1)
			seen := false
			for i := 0; i <= maxIdx; i++ {
				if v, ok := vals[i]; ok {
					out[i] = v
					seen = true
				}
			}
			if seen {
				s.CPUPerCorePercent = out
			}
		}
	}

	// disk I/O: sum read/write throughput across disks
	if len(diskio) > 0 {
		var rd, wr float64
		var has bool
		for _, d := range diskio {
			if r := asFloatPtr(d.ReadBytesRatePerSec); r != nil {
				rd += *r
				has = true
			}
			if w := asFloatPtr(d.WriteBytesRatePerSec); w != nil {
				wr += *w
				has = true
			}
		}
		if has {
			s.DiskReadBytesPerSec = &rd
			s.DiskWriteBytesPerSec = &wr
		}
	}

	// filesystem totals: sum size/used/free across real mounts (skip container/system bind mounts)
	if len(fs) > 0 {
		var total, used, free uint64
		var has bool
		seenMnt := map[string]bool{}
		for _, f := range fs {
			mp, _ := f.MountPoint.(string)
			if mp == "" {
				continue
			}
			if seenMnt[mp] {
				continue
			}
			seenMnt[mp] = true

			// Skip typical container bind mounts and other pseudo mounts
			if strings.HasPrefix(mp, "/etc/") || strings.HasPrefix(mp, "/proc") || strings.HasPrefix(mp, "/sys") || strings.HasPrefix(mp, "/dev") {
				continue
			}

			sz := asUint64Ptr(f.Size)
			u := asUint64Ptr(f.Used)
			fr := asUint64Ptr(f.Free)
			if sz == nil || *sz == 0 {
				continue
			}
			// Sometimes free isn't present; compute if possible
			if fr == nil && u != nil {
				cfr := *sz - *u
				fr = &cfr
			}
			if u == nil && fr != nil {
				cu := *sz - *fr
				u = &cu
			}
			if u == nil || fr == nil {
				continue
			}

			total += *sz
			used += *u
			free += *fr
			has = true
		}
		if has && total > 0 {
			s.FSTotalBytes = &total
			s.FSUsedBytes = &used
			s.FSFreeBytes = &free
			p := (float64(used) / float64(total)) * 100
			s.FSUsedPercent = &p
		}
	}

	c.mu.Lock()
	c.cachedAt = time.Now()
	c.cached = s
	c.cacheErr = nil
	c.mu.Unlock()

	return s, nil
}
