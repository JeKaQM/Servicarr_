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

// Client is a Glances API client for fetching system resource data.
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

// NewClient creates a new Glances API client with the given baseURL.
func NewClient(baseURL string) *Client {
	c := &Client{
		BaseURL:  baseURL,
		HTTP:     &http.Client{Timeout: 6 * time.Second},
		cacheFor: 5 * time.Second,
	}
	c.inFlightC = sync.NewCond(&c.mu)
	return c
}

// SetCacheTTL sets the cache time-to-live for the client.
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

// FetchSnapshot fetches and caches a system resource snapshot from Glances.
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

	// GPU metrics (optional)
	var gpus []glancesGPU
	if err := c.getJSON(ctx, "/gpu", &gpus); err != nil {
		// Optional - not all systems have GPUs
	}

	// Container metrics (optional)
	var containers []glancesContainer
	if err := c.getJSON(ctx, "/containers", &containers); err != nil {
		// Optional - Docker/Podman may not be installed
	}

	// Uptime as string (optional)
	var uptimeStr string
	if err := c.getJSON(ctx, "/uptime", &uptimeStr); err != nil {
		// Optional
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

	// CPU
	s.CPUPercent = asFloatPtr(cpu.Total)
	if s.CPUPercent == nil {
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
	if cN := asUint64Ptr(cpu.CPUCore); cN != nil {
		s.CPUCores = cN
	} else if cN := asUint64Ptr(load.CPUCore); cN != nil {
		s.CPUCores = cN
	}

	// Load averages
	s.Load1 = asFloatPtr(load.Min1)
	s.Load5 = asFloatPtr(load.Min5)
	s.Load15 = asFloatPtr(load.Min15)

	// Memory
	s.MemTotalBytes = asUint64Ptr(mem.Total)
	s.MemUsedBytes = asUint64Ptr(mem.Used)
	s.MemPercent = asFloatPtr(mem.Percent)
	if s.MemPercent == nil && s.MemTotalBytes != nil && s.MemUsedBytes != nil && *s.MemTotalBytes > 0 {
		p := (float64(*s.MemUsedBytes) / float64(*s.MemTotalBytes)) * 100
		s.MemPercent = &p
	}

	// Swap
	s.SwapTotalBytes = asUint64Ptr(swap.Total)
	s.SwapUsedBytes = asUint64Ptr(swap.Used)
	s.SwapPercent = asFloatPtr(swap.Percent)
	if s.SwapPercent == nil && s.SwapTotalBytes != nil && s.SwapUsedBytes != nil && *s.SwapTotalBytes > 0 {
		p := (float64(*s.SwapUsedBytes) / float64(*s.SwapTotalBytes)) * 100
		s.SwapPercent = &p
	}

	// Process counts
	s.ProcTotal = asUint64Ptr(pc.Total)
	s.ProcRunning = asUint64Ptr(pc.Running)
	s.ProcSleeping = asUint64Ptr(pc.Sleeping)

	// Temperature: take the highest core temp
	var bestTemp *float64
	for _, sen := range sensors {
		if t, ok := sen.Type.(string); ok {
			if t != "temperature_core" && t != "temperature" {
				continue
			}
		}
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

	// Network: sum rates across interfaces (ignore loopback)
	var rxRate, txRate float64
	var hasRate bool
	for _, n := range nets {
		if name, ok := n.InterfaceName.(string); ok {
			if name == "lo" || name == "lo0" {
				continue
			}
		}
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

	// Per-CPU totals
	if len(percpu) > 0 {
		maxIdx := -1
		vals := map[int]float64{}
		for _, p := range percpu {
			idxPtr := asUint64Ptr(p.CPUNumber)
			valPtr := asFloatPtr(p.Total)
			if idxPtr == nil || valPtr == nil {
				continue
			}
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

	// Disk I/O: sum read/write throughput
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

	// Filesystem totals
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

			if strings.HasPrefix(mp, "/etc/") || strings.HasPrefix(mp, "/proc") || strings.HasPrefix(mp, "/sys") || strings.HasPrefix(mp, "/dev") {
				continue
			}

			sz := asUint64Ptr(f.Size)
			u := asUint64Ptr(f.Used)
			fr := asUint64Ptr(f.Free)
			if sz == nil || *sz == 0 {
				continue
			}
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

	// GPU metrics: take the first GPU with data
	if len(gpus) > 0 {
		for _, g := range gpus {
			if name, ok := g.Name.(string); ok && name != "" {
				s.GPUName = name
			}
			if proc := asFloatPtr(g.Proc); proc != nil {
				s.GPUPercent = proc
			}
			if memPct := asFloatPtr(g.Mem); memPct != nil {
				s.GPUMemPct = memPct
			}
			if temp := asFloatPtr(g.Temperature); temp != nil {
				s.GPUTempC = temp
			}
			if s.GPUName != "" {
				break
			}
		}
	}

	// Container metrics
	if len(containers) > 0 {
		count := uint64(len(containers))
		s.ContainerCount = &count
		var running uint64
		for _, ct := range containers {
			status, _ := ct.Status.(string)
			if status == "running" {
				running++
			}
			info := ContainerInfo{}
			if name, ok := ct.Name.(string); ok {
				info.Name = name
			}
			info.Status = status
			info.CPUPercent = asFloatPtr(ct.CPUPercent)
			if usage := asUint64Ptr(ct.MemUsage); usage != nil {
				if limit := asUint64Ptr(ct.MemLimit); limit != nil && *limit > 0 {
					pct := (float64(*usage) / float64(*limit)) * 100
					info.MemPercent = &pct
				}
			}
			s.Containers = append(s.Containers, info)
		}
		s.ContainerRunning = &running
	}

	// Uptime string
	if uptimeStr != "" {
		s.UptimeString = uptimeStr
	}

	// Process thread count
	if thread := asUint64Ptr(pc.Thread); thread != nil {
		s.ProcThreads = thread
	}

	c.mu.Lock()
	c.cachedAt = time.Now()
	c.cached = s
	c.cacheErr = nil
	c.mu.Unlock()

	return s, nil
}
