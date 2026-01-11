const REFRESH_MS = 15000;
let DAYS = 30;
let resourcesConfig = null; // Cache the config
const $ = (s,r=document) => r.querySelector(s);
const $$ = (s,r=document) => Array.from(r.querySelectorAll(s));
const fmtMs = ms => ms==null ? '—' : ms+' ms';
const cls = (ok, status, degraded) => {
  if (!ok) return 'pill down'; // Down = red
  if (degraded) return 'pill warn'; // Degraded = amber
  return 'pill ok'; // Up = green
};

function fmtBytes(n) {
  if (n == null || isNaN(n)) return '—';
  const units = ['B','KB','MB','GB','TB'];
  let v = Number(n);
  let i = 0;
  while (v >= 1024 && i < units.length - 1) {
    v /= 1024;
    i++;
  }
  const digits = i === 0 ? 0 : (i >= 3 ? 2 : 1);
  return `${v.toFixed(digits)} ${units[i]}`;
}

function fmtRateBps(n) {
  if (n == null || isNaN(n)) return '—';
  return `${fmtBytes(n)}/s`;
}

function fmtPct(n) {
  if (n == null || isNaN(n)) return '—';
  return `${Number(n).toFixed(0)}%`;
}

function fmtFloat(n, digits = 2) {
  if (n == null || isNaN(n)) return '—';
  return Number(n).toFixed(digits);
}

function fmtTempC(n) {
  if (n == null || isNaN(n)) return '—';
  return `${Number(n).toFixed(0)}°C`;
}

function setResText(id, value) {
  const el = document.getElementById(id);
  if (el) el.textContent = value;
}

function setResClass(id, clsName) {
  const el = document.getElementById(id);
  if (el) el.className = clsName;
}

function meterClassForPct(p) {
  if (p == null || isNaN(p)) return '';
  const n = Number(p);
  if (n >= 90) return 'bad';
  if (n >= 75) return 'warn';
  return '';
}

function setMeter(id, pct) {
  const el = document.getElementById(id);
  if (!el) return;
  if (pct == null || isNaN(pct)) {
    el.style.width = '0%';
    el.classList.remove('warn', 'bad');
    return;
  }
  const p = Math.max(0, Math.min(100, Number(pct)));
  el.style.width = `${p}%`;
  el.classList.remove('warn', 'bad');
  const clsName = meterClassForPct(p);
  if (clsName) el.classList.add(clsName);
}

function applyResourcesVisibility(config) {
  const section = document.getElementById('card-resources');
  if (!section || !config) return;

  // Cache the config for use elsewhere
  resourcesConfig = config;

  const enabled = config.enabled !== false;
  
  // For each tile: if enabled in config, remove 'hidden' class; otherwise ensure it has 'hidden'
  const tiles = $$('.resource-tile', section);
  tiles.forEach(t => {
    const kind = t.getAttribute('data-kind');
    let show = false;
    if (kind === 'cpu') show = config.cpu !== false;
    else if (kind === 'mem') show = config.memory !== false;
    else if (kind === 'net') show = config.network !== false;
    else if (kind === 'temp') show = config.temp !== false;
    else if (kind === 'storage') show = config.storage !== false;
    
    if (show) {
      t.classList.remove('hidden');
    } else {
      t.classList.add('hidden');
    }
  });

  // Show/hide the entire section
  if (enabled) {
    section.classList.remove('hidden');
  } else {
    section.classList.add('hidden');
  }
}

async function loadResourcesConfig() {
  try {
    // Public endpoint so the dashboard can respect admin settings without being logged in.
    // Add timestamp to prevent any browser caching
    const timestamp = Date.now();
    const cfg = await j(`/api/resources/config?_=${timestamp}`);
    applyResourcesVisibility(cfg);

    // If admin form exists (admin view), hydrate it too.
    if ($('#resourcesEnabled')) {
      $('#resourcesEnabled').checked = cfg.enabled !== false;
      $('#resourcesCPU').checked = cfg.cpu !== false;
      $('#resourcesMemory').checked = cfg.memory !== false;
      $('#resourcesNetwork').checked = cfg.network !== false;
      $('#resourcesTemp').checked = cfg.temp !== false;
      if ($('#resourcesStorage')) $('#resourcesStorage').checked = cfg.storage !== false;
    }
  } catch (err) {
    // If the public endpoint isn't available for some reason, try the admin endpoint
    // (will work when logged in).
    try {
      const cfg = await j('/api/admin/resources/config');
      applyResourcesVisibility(cfg);
      if ($('#resourcesEnabled')) {
        $('#resourcesEnabled').checked = cfg.enabled !== false;
        $('#resourcesCPU').checked = cfg.cpu !== false;
        $('#resourcesMemory').checked = cfg.memory !== false;
        $('#resourcesNetwork').checked = cfg.network !== false;
        $('#resourcesTemp').checked = cfg.temp !== false;
        if ($('#resourcesStorage')) $('#resourcesStorage').checked = cfg.storage !== false;
      }
    } catch (_) {
      // Both endpoints failed (likely rate limit when spamming refresh)
      // DON'T apply any defaults - keep the current visibility state
      // Only apply defaults if we have no config cached yet (first load failure)
      if (!resourcesConfig) {
        applyResourcesVisibility({
          enabled: false,
          cpu: false,
          memory: false,
          network: false,
          temp: false,
          storage: false
        });
      }
    }
  }
}

async function saveResourcesConfig() {
  const statusEl = $('#resourcesStatus');
  const btn = $('#saveResources');
  if (!btn) return;

  const config = {
    enabled: $('#resourcesEnabled').checked,
    cpu: $('#resourcesCPU').checked,
    memory: $('#resourcesMemory').checked,
    network: $('#resourcesNetwork').checked,
    temp: $('#resourcesTemp').checked,
    storage: $('#resourcesStorage') ? $('#resourcesStorage').checked : true,
  };

  await handleButtonAction(
    btn,
    async () => {
      await j('/api/admin/resources/config', {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
          'X-CSRF-Token': getCsrf()
        },
        body: JSON.stringify(config)
      });

      // Apply immediately on the public page.
      applyResourcesVisibility(config);

      if (statusEl) {
        statusEl.textContent = 'Resources settings saved successfully';
        statusEl.className = 'status-message success';
        statusEl.classList.remove('hidden');
        setTimeout(() => statusEl.classList.add('hidden'), 3000);
      }
    },
    'Resources settings saved'
  );
}

async function refreshResources() {
  const pill = document.getElementById('resources-pill');
  const section = document.getElementById('card-resources');

  // If the entire section is hidden by admin config, skip the fetch.
  if (section && section.classList.contains('hidden')) {
    return;
  }

  // Check which tiles are actually visible (not hidden)
  const cpuTile = document.querySelector('#card-resources .resource-tile[data-kind="cpu"]');
  const memTile = document.querySelector('#card-resources .resource-tile[data-kind="mem"]');
  const tempTile = document.querySelector('#card-resources .resource-tile[data-kind="temp"]');
  const netTile = document.querySelector('#card-resources .resource-tile[data-kind="net"]');
  const storageTile = document.querySelector('#card-resources .resource-tile[data-kind="storage"]');

  const cpuEnabled = cpuTile && !cpuTile.classList.contains('hidden');
  const memEnabled = memTile && !memTile.classList.contains('hidden');
  const tempEnabled = tempTile && !tempTile.classList.contains('hidden');
  const netEnabled = netTile && !netTile.classList.contains('hidden');
  const storageEnabled = storageTile && !storageTile.classList.contains('hidden');

  // If ALL tiles are disabled, don't fetch data at all
  if (!cpuEnabled && !memEnabled && !tempEnabled && !netEnabled && !storageEnabled) {
    if (pill) {
      pill.textContent = 'DISABLED';
      pill.className = 'pill';
    }
    return;
  }

  try {
    const snap = await j('/api/resources');

    if (cpuEnabled) {
      setResText('res-cpu', fmtPct(snap.cpu_percent));
      setMeter('meter-cpu', snap.cpu_percent);
    }

    // CPU detail: cores + breakdown when available
    let cpuDetail = '—';
    if (Array.isArray(snap.cpu_per_core_percent) && snap.cpu_per_core_percent.length) {
      // Example: C0 12% • C1 6% • C2 18% ...
      cpuDetail = snap.cpu_per_core_percent
        .map((v, i) => `C${i} ${fmtPct(v)}`)
        .join(' • ');
    } else if (snap.cpu_percent == null) {
      cpuDetail = 'CPU usage unavailable';
    } else {
      // fallback: show cores + avg/max when we have at least an average
      const bits = [];
      if (snap.cpu_cores != null) bits.push(`${snap.cpu_cores} cores`);
      bits.push(`Avg ${fmtPct(snap.cpu_percent)}`);
      cpuDetail = bits.join(' — ');
    }
    if (cpuEnabled) {
      setResText('res-cpu-detail', cpuDetail);
    }

    if (memEnabled) {
      setResText('res-mem', fmtPct(snap.mem_percent));
      setMeter('meter-mem', snap.mem_percent);
      setResText('res-mem-detail', (snap.mem_used_bytes != null && snap.mem_total_bytes != null)
        ? `${fmtBytes(snap.mem_used_bytes)} / ${fmtBytes(snap.mem_total_bytes)}`
        : '—');
    }

    // Temperature
    if (tempEnabled) {
      setResText('res-temp', fmtTempC(snap.temp_c));
      setResText('res-temp-min', fmtTempC(snap.temp_min_c));
      setResText('res-temp-max', fmtTempC(snap.temp_max_c));
      setResText('res-temp-detail', (snap.temp_c == null)
        ? 'Temp unavailable'
        : '');
    }

    if (netEnabled) {
      setResText('res-net-rx', fmtRateBps(snap.net_rx_bytes_per_sec));
      setResText('res-net-tx', fmtRateBps(snap.net_tx_bytes_per_sec));
      const rx = snap.net_rx_bytes_per_sec == null ? 0 : Number(snap.net_rx_bytes_per_sec);
      const tx = snap.net_tx_bytes_per_sec == null ? 0 : Number(snap.net_tx_bytes_per_sec);
      const netSum = (snap.net_rx_bytes_per_sec == null && snap.net_tx_bytes_per_sec == null)
        ? '—'
        : fmtRateBps(rx + tx);
      setResText('res-net', netSum);
      setResText('res-net-detail', (snap.net_rx_bytes_per_sec == null && snap.net_tx_bytes_per_sec == null)
        ? 'Network metrics unavailable'
        : 'Live throughput');

      // Disk I/O (optional)
      setResText('res-io-rd', fmtRateBps(snap.disk_read_bytes_per_sec));
      setResText('res-io-wr', fmtRateBps(snap.disk_write_bytes_per_sec));
    }

    // Storage tile (optional)
    if (storageEnabled) {
      setResText('res-storage', fmtPct(snap.fs_used_percent));
      setMeter('meter-storage', snap.fs_used_percent);
      setResText('res-storage-detail', (snap.fs_used_bytes != null && snap.fs_total_bytes != null)
        ? `${fmtBytes(snap.fs_used_bytes)} / ${fmtBytes(snap.fs_total_bytes)}`
        : 'Storage metrics unavailable');

      setResText('res-storage-used', (snap.fs_used_bytes != null) ? fmtBytes(snap.fs_used_bytes) : '—');
      setResText('res-storage-free', (snap.fs_free_bytes != null) ? fmtBytes(snap.fs_free_bytes) : '—');
    }

    // Pill status based on availability and enabled metrics
    if (pill) {
      const hasAny = (snap.cpu_percent != null) || (snap.mem_percent != null) || (snap.temp_c != null) || (snap.net_rx_bytes_per_sec != null) || (snap.net_tx_bytes_per_sec != null);
      pill.textContent = hasAny ? 'LIVE' : 'PARTIAL';
      pill.className = hasAny ? 'pill ok' : 'pill warn';
    }
  } catch (e) {
    if (pill) {
      pill.textContent = 'UNAVAILABLE';
      pill.className = 'pill warn';
    }
    // Only reset meters for enabled tiles
    if (cpuEnabled) setMeter('meter-cpu', null);
    if (memEnabled) setMeter('meter-mem', null);
    if (tempEnabled) {
      setResText('res-temp', '—');
      setResText('res-temp-min', '—');
      setResText('res-temp-max', '—');
      setResText('res-temp-detail', 'Temp unavailable');
    }
    if (netEnabled) {
      setResText('res-net', '—');
      setResText('res-net-detail', 'Network metrics unavailable');
      setResText('res-io-rd', '—');
      setResText('res-io-wr', '—');
    }
    if (storageEnabled) {
      setMeter('meter-storage', null);
      setResText('res-storage', '—');
      setResText('res-storage-detail', 'Storage metrics unavailable');
      setResText('res-storage-used', '—');
      setResText('res-storage-free', '—');
    }
  }
}

async function j(u,opts) {
  const controller = new AbortController();
  const timeoutId = setTimeout(() => controller.abort(), 10000); // 10 second timeout
  
  try {
    const fetchOpts = Object.assign({
      cache:'no-store',
      credentials:'include',
      signal: controller.signal
    }, opts||{});
    
    const r = await fetch(u, fetchOpts);
    clearTimeout(timeoutId);
    
    // Read response body first, before checking ok
    let result;
    const ct = r.headers.get('content-type')||'';
    try {
      result = ct.includes('json') ? await r.json() : await r.text();
    } catch (parseErr) {
      console.error(`Failed to parse response from ${u}:`, parseErr);
      throw new Error(`Failed to parse response: ${parseErr.message}`);
    }
    
    if(!r.ok) {
      const err = new Error('HTTP '+r.status);
      err.status = r.status;
      err.resp = r;
      err.body = result;
      throw err;
    }
    
    return result;
  } catch (err) {
    clearTimeout(timeoutId);
    
    if (err.name === 'AbortError') {
      throw new Error('Request timeout - check your connection');
    }
    throw err;
  }
}

function updCard(id,data) {
  const el = document.getElementById(id);
  if (!el) {
    console.error('Card element not found:', id);
    return;
  }
  
  const pill = $('.pill',el);
  const k = $('.kpi',el);
  const h = $('.kpirow .label',el); // More specific selector for status label
  const toggle = $('.monitorToggle', el);
  
  if (!pill || !k || !h) {
    console.error('Required elements not found in card:', id);
    return;
  }
  
  // Set checkbox state based on disabled flag from server
  if (toggle) {
    toggle.checked = !data.disabled;
  }
  
  if (data.disabled) {
    pill.textContent = 'DISABLED';
    pill.className = 'pill warn';
    k.textContent = '—';
    h.textContent = 'Monitoring disabled';
    return;
  }

  if (data.degraded) {
    pill.textContent = 'DEGRADED';
  } else {
    pill.textContent = data.ok ? 'UP' : 'DOWN';
  }
  pill.className = cls(data.ok, data.status, data.degraded);
  k.textContent = fmtMs(data.ms);
  h.textContent = data.status ? ('HTTP '+data.status) : 'no response';
  
  // Update last check time
  const lastCheckEl = $(`#last-check-${id.split('-').pop()}`);
  if (lastCheckEl) {
    const now = new Date();
    lastCheckEl.textContent = now.toLocaleTimeString('en-US', { hour: '2-digit', minute: '2-digit' });
  }
}

async function toggleMonitoring(card, enabled) {
  const key = card.getAttribute('data-key');
  try {
    await j('/api/admin/toggle-monitoring', {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        'X-CSRF-Token': getCsrf()
      },
      body: JSON.stringify({ service: key, enable: enabled })
    });
    showToast(`Monitoring ${enabled ? 'enabled' : 'disabled'} for ${key}`);
    await refresh();
  } catch (err) {
    console.error('toggle failed', err);
    showToast('Failed to toggle monitoring', 'error');
  }
}

let chart;
function renderChart(overall) {
  if(!window.Chart) return;
  const labels = ['server','plex','overseerr'];
  const vals = labels.map(k => +(overall?.[k]??0).toFixed(1));
  const ctx = document.getElementById('uptimeChart').getContext('2d');
  const data = {labels, datasets:[{label:'Uptime %',data:vals,borderWidth:1}]};
  
  if(chart) {
    chart.data = data;
    chart.update();
    return;
  }
  
  chart = new Chart(ctx, {
    type: 'bar',
    data,
    options: {
      responsive: true,
      plugins: {legend: {display:false}},
      scales: {y: {beginAtZero:true, max:100}}
    }
  });
}

function renderIncidents(items) {
  const list = $('#incidents');
  if(!items?.length) {
    list.innerHTML = '<li class="label">No incidents in last 24h</li>';
    return;
  }
  
  list.innerHTML = items.map(i => {
    const ts = new Date(i.taken_at).toLocaleString();
    return `<li><span class="dot"></span><span>${ts}</span><span class="label"> — ${i.service_key} (${i.http_status||'n/a'})</span></li>`;
  }).join('');
}

function updateServiceStats(metrics) {
  const services = ['server', 'plex', 'overseerr'];
  
  services.forEach(key => {
    const uptimeEl = $(`#uptime-24h-${key}`);
    const avgResponseEl = $(`#avg-response-${key}`);
    
    if (uptimeEl && metrics.overall) {
      const uptime = metrics.overall[key] || 0;
      uptimeEl.textContent = `${uptime.toFixed(1)}%`;
      // Green only for 100%, orange for <100%, red for <50%
      uptimeEl.className = 'stat-value ' + (uptime >= 100 ? 'good' : uptime >= 50 ? 'warning' : 'bad');
    }
    
    if (avgResponseEl && metrics.series && metrics.series[key]) {
      const data = metrics.series[key];
      let totalMs = 0;
      let count = 0;
      
      data.forEach(point => {
        if (point.avg_ms && point.avg_ms > 0) {
          totalMs += point.avg_ms;
          count++;
        }
      });
      
      if (count > 0) {
        const avgMs = totalMs / count;
        avgResponseEl.textContent = `${Math.round(avgMs)}ms`;
        avgResponseEl.className = 'stat-value ' + (avgMs < 100 ? 'good' : avgMs < 500 ? 'warning' : 'bad');
      } else {
        avgResponseEl.textContent = '—';
        avgResponseEl.className = 'stat-value';
      }
    }
  });
}

function renderUptimeBars(metrics, days) {
  const daysToShow = days || DAYS;
  const services = ['server', 'plex', 'overseerr'];
  const now = new Date();
  const daysAgo = now.getTime() - (daysToShow * 24 * 60 * 60 * 1000);
  
  // Update global timestamp once
  const globalTimestamp = $('#timestamp-global');
  if (globalTimestamp) {
    const today = now.toLocaleDateString();
    globalTimestamp.textContent = `Tracking from ${today} • Hover over blocks for details`;
  }
  
  services.forEach(key => {
    const bar = $(`#uptime-bar-${key}`);
    const uptimePercent = $(`#uptime-${key}`);
    
    if (!bar) return;
    
    // Add data attribute for CSS styling based on day count
    bar.setAttribute('data-days', daysToShow);
    
    const data = (metrics && metrics.series) ? metrics.series[key] || [] : [];
    
    // Calculate average uptime from the daily/hourly data points (not the overall which dilutes short outages)
    let avgUptime = 0;
    if (data.length > 0) {
      const validPoints = data.filter(p => p.uptime !== null && p.uptime !== undefined);
      if (validPoints.length > 0) {
        avgUptime = validPoints.reduce((sum, p) => sum + p.uptime, 0) / validPoints.length;
      }
    }
    
    // Update uptime percentage
    if (uptimePercent) {
      if (data.length === 0) {
        uptimePercent.textContent = 'N/A';
        uptimePercent.style.color = 'var(--text-dim)';
      } else {
        // Use 2 decimal places to avoid rounding 99.99% to 100%
        // But show "100%" without decimals if truly 100%
        if (avgUptime >= 100) {
          uptimePercent.textContent = '100%';
        } else {
          uptimePercent.textContent = `${avgUptime.toFixed(2)}%`;
        }
        // Green only for 100%, orange for <100%, red for <50%
        uptimePercent.style.color = avgUptime >= 100 ? 'var(--ok)' : avgUptime >= 50 ? 'var(--warn)' : 'var(--down)';
      }
    }
    
    // Clear existing blocks
    bar.innerHTML = '';
    
    // Create blocks for each day - always show DAYS blocks
    // If we have data, use it; otherwise show gray "no data" blocks
    const blocks = [];
    
    if (data.length > 0) {
      // Fill in missing days with null data
      const dataMap = {};
      data.forEach(point => {
        // API returns 'day' field for daily aggregation
        const dayKey = point.day || point.hour?.substr(0, 10);
        if (dayKey) {
          dataMap[dayKey] = point;
        }
      });
      
      // Create all days
      for (let i = daysToShow - 1; i >= 0; i--) {
        const dayTime = new Date(now.getTime() - (i * 24 * 60 * 60 * 1000));
        const dayBin = dayTime.toISOString().substr(0, 10);
        blocks.push(dataMap[dayBin] || { day: dayBin, uptime: null });
      }
    } else {
      // No data yet - create empty blocks
      for (let i = daysToShow - 1; i >= 0; i--) {
        const dayTime = new Date(now.getTime() - (i * 24 * 60 * 60 * 1000));
        const dayBin = dayTime.toISOString().substr(0, 10);
        blocks.push({ day: dayBin, uptime: null });
      }
    }
    
    blocks.forEach((point) => {
      const block = document.createElement('div');
      block.className = 'uptime-block';
      
      const uptime = point.uptime;
      const dayDate = new Date(point.day);
      const formattedDate = dayDate.toLocaleDateString('en-US', { 
        month: 'short', 
        day: 'numeric',
        year: daysToShow > 90 ? 'numeric' : undefined 
      });
      
      let tooltipText = '';
      if (uptime === null || uptime === undefined) {
        block.classList.add('unknown');
        tooltipText = `${formattedDate}\nNo data available`;
      } else if (uptime >= 100) {
        // 100% uptime = green
        block.classList.add('up');
        tooltipText = `${formattedDate}\n${uptime.toFixed(1)}% uptime\n✓ Fully operational`;
      } else if (uptime >= 50) {
        // 50-99% uptime = orange (partial outage)
        block.classList.add('degraded');
        tooltipText = `${formattedDate}\n${uptime.toFixed(1)}% uptime\n⚠ Partial outage`;
      } else {
        // Below 50% = red (major outage)
        block.classList.add('down');
        tooltipText = `${formattedDate}\n${uptime.toFixed(1)}% uptime\n✗ Major outage`;
      }
      
      block.title = tooltipText;
      block.setAttribute('data-tooltip', tooltipText);
      
      // Add mobile-friendly touch feedback
      block.addEventListener('touchstart', (e) => {
        // Show a quick visual feedback on touch
        block.style.transition = 'transform 0.1s';
        
        // Create temporary tooltip for mobile
        const isMobile = window.innerWidth <= 768;
        if (isMobile && tooltipText) {
          showMobileTooltip(block, tooltipText, e.touches[0]);
        }
      });
      
      block.addEventListener('touchend', () => {
        block.style.transition = '';
      });
      
      bar.appendChild(block);
    });
  });
}

// Mobile tooltip function for uptime blocks
let tooltipTimeout;
function showMobileTooltip(element, text, touch) {
  // Remove any existing tooltip
  const existingTooltip = document.querySelector('.mobile-tooltip');
  if (existingTooltip) {
    existingTooltip.remove();
  }
  
  clearTimeout(tooltipTimeout);
  
  const tooltip = document.createElement('div');
  tooltip.className = 'mobile-tooltip';
  tooltip.textContent = text.replace(/\n/g, ' • ');
  tooltip.style.cssText = `
    position: fixed;
    background: rgba(0, 0, 0, 0.9);
    color: white;
    padding: 8px 12px;
    border-radius: 6px;
    font-size: 12px;
    z-index: 10000;
    pointer-events: none;
    max-width: 80vw;
    text-align: center;
    box-shadow: 0 4px 12px rgba(0, 0, 0, 0.3);
    left: 50%;
    top: ${touch ? touch.clientY - 60 : 100}px;
    transform: translateX(-50%);
    animation: fadeIn 0.2s ease-in;
  `;
  
  document.body.appendChild(tooltip);
  
  tooltipTimeout = setTimeout(() => {
    tooltip.style.animation = 'fadeOut 0.2s ease-out';
    setTimeout(() => tooltip.remove(), 200);
  }, 2000);
}

async function refresh() {
  try {
    const live = await j('/api/check');
    $('#updated').textContent = new Date(live.t).toLocaleString();
    updCard('card-server', live.status.server || {});
    updCard('card-plex', live.status.plex || {});
    updCard('card-overseerr', live.status.overseerr || {});
  } catch (e) {
    console.error('live check failed', e);
  }

  // Resources (Glances)
  refreshResources();

  try {
    const metrics = await j(`/api/metrics?days=${DAYS}`);
    $('#window').textContent = `Last ${DAYS} days`;
    
    try {
      renderChart(metrics.overall || {});
    } catch (chartErr) {
      // Chart rendering failed - silent failure
    }
    
    renderIncidents(metrics.downs || []);
    renderUptimeBars(metrics, DAYS);
    
    // Fetch 24h stats for the service cards
    const stats24h = await j('/api/metrics?hours=24');
    updateServiceStats(stats24h);
  } catch (e) {
    // Metrics unavailable - render with no data
    renderUptimeBars(null, DAYS);
  }
}

async function doLoginFlow() {
  const dlg = document.getElementById('loginModal');
  const err = $('#loginError', dlg);
  err.classList.add('hidden');
  err.textContent = '';
  
  // Clear any previous input
  $('#u', dlg).value = '';
  $('#p', dlg).value = '';
  
  dlg.showModal();
}

async function submitLogin() {
  const dlg = document.getElementById('loginModal');
  const u = $('#u', dlg).value.trim();
  const p = $('#p', dlg).value;
  
  if (!u || !p) {
    const el = $('#loginError', dlg);
    el.textContent = 'Username and password are required';
    el.classList.remove('hidden');
    return;
  }
  
  // Disable form while submitting to prevent double submission
  const submitBtn = $('#doLogin');
  submitBtn.disabled = true;
  submitBtn.textContent = 'Logging in...';
  
  try {
    const csrfToken = getCsrf();
    
    const result = await j('/api/login', {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        'X-CSRF-Token': csrfToken
      },
      body: JSON.stringify({username: u, password: p})
    });
    
    dlg.close();
    await whoami();
  } catch (err) {
    console.error('Login error:', err.message);
    
    const el = $('#loginError', dlg);
    
    if (err.status === 403) {
      el.textContent = 'Access denied - too many failed attempts. Try again later.';
    } else if (err.status === 401) {
      el.textContent = 'Invalid username or password';
    } else if (err.name === 'AbortError') {
      el.textContent = 'Request timeout - check your connection';
    } else {
      el.textContent = err.message || 'Login failed. Please try again.';
    }
    
    el.classList.remove('hidden');
    submitBtn.disabled = false;
    submitBtn.textContent = 'Login';
  }
}

async function logout() {
  try {
    await j('/api/logout', {method: 'POST'});
  } catch (_) {}
  await whoami();
}

function getCsrf() {
  return (document.cookie.split('; ').find(s => s.startsWith('csrf=')) || '').split('=')[1] || '';
}

// Custom event for login state changes
const loginStateChanged = new Event('loginStateChanged');

async function whoami() {
  try {
    const me = await j('/api/me');
    
    if(me.authenticated) {
      $('#welcome').textContent = 'Welcome, ' + me.user;
      $('#loginBtn').classList.add('hidden');
      $('#logoutBtn').classList.remove('hidden');
      $('#adminPanel').classList.remove('hidden');
      $$('.adminRow').forEach(e => e.classList.remove('hidden'));
      document.dispatchEvent(loginStateChanged);
      loadAlertsConfig();
      loadResourcesConfig();
    } else {
      $('#welcome').textContent = 'Public view';
      $('#loginBtn').classList.remove('hidden');
      $('#logoutBtn').classList.add('hidden');
      $('#adminPanel').classList.add('hidden');
      $$('.adminRow').forEach(e => e.classList.add('hidden'));
      
      // Reset login form
      const dlg = document.getElementById('loginModal');
      if (dlg) {
        const submitBtn = $('#doLogin', dlg);
        if (submitBtn) {
          submitBtn.disabled = false;
          submitBtn.textContent = 'Login';
        }
        const errorEl = $('#loginError', dlg);
        if (errorEl) {
          errorEl.textContent = '';
          errorEl.classList.add('hidden');
        }
        $('#u', dlg).value = '';
        $('#p', dlg).value = '';
      }
    }
  } catch (e) {
    console.error('Failed to fetch user info:', e.message);
  }
}

async function handleButtonAction(btn, action, successMsg) {
  btn.disabled = true;
  btn.classList.add('loading');
  try {
    await action();
    showToast(successMsg);
  } catch (err) {
    console.error(err);
    showToast(err.message || 'Action failed', 'error');
  } finally {
    btn.disabled = false;
    btn.classList.remove('loading');
  }
}

async function ingestAll() {
  const btn = $('#ingestNowTab') || $('#ingestNow');
  await handleButtonAction(
    btn,
    async () => {
      await j('/api/admin/ingest-now', {
        method: 'POST',
        headers: {'X-CSRF-Token': getCsrf()}
      });
      await refresh();
    },
    'Ingestion completed successfully'
  );
}

async function resetRecent() {
  const btn = $('#resetRecentTab') || $('#resetRecent');
  await handleButtonAction(
    btn,
    async () => {
      await j('/api/admin/reset-recent', {
        method: 'POST',
        headers: {'X-CSRF-Token': getCsrf()}
      });
      await refresh();
    },
    'Recent incidents reset successfully'
  );
}

async function saveAlertsConfig() {
  const statusEl = $('#alertStatus');
  const btn = $('#saveAlerts');
  
  const config = {
    enabled: $('#alertsEnabled').checked,
    smtp_host: $('#smtpHost').value,
    smtp_port: parseInt($('#smtpPort').value) || 587,
    smtp_user: $('#smtpUser').value,
    smtp_password: $('#smtpPassword').value,
    alert_email: $('#alertEmail').value,
    from_email: $('#alertFromEmail').value,
    alert_on_down: $('#alertOnDown').checked,
    alert_on_degraded: $('#alertOnDegraded').checked,
    alert_on_up: $('#alertOnUp').checked
  };
  
  await handleButtonAction(
    btn,
    async () => {
      await j('/api/admin/alerts/config', {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
          'X-CSRF-Token': getCsrf()
        },
        body: JSON.stringify(config)
      });
      
      statusEl.textContent = 'Configuration saved successfully';
      statusEl.className = 'status-message success';
      statusEl.classList.remove('hidden');
      setTimeout(() => statusEl.classList.add('hidden'), 3000);
    },
    'Configuration saved'
  );
}

async function sendTestEmail() {
  const statusEl = $('#alertStatus');
  const btn = $('#testEmail');
  
  await handleButtonAction(
    btn,
    async () => {
      const result = await j('/api/admin/alerts/test', {
        method: 'POST',
        headers: {'X-CSRF-Token': getCsrf()}
      });
      
      statusEl.textContent = result.message || 'Test email sent successfully';
      statusEl.className = 'status-message success';
      statusEl.classList.remove('hidden');
      setTimeout(() => statusEl.classList.add('hidden'), 5000);
    },
    'Test email sent'
  );
}

async function loadAlertsConfig() {
  try {
    const config = await j('/api/admin/alerts/config');
    if (config) {
      $('#alertsEnabled').checked = config.enabled || false;
      $('#smtpHost').value = config.smtp_host || '';
      $('#smtpPort').value = config.smtp_port || 587;
      $('#smtpUser').value = config.smtp_user || '';
      $('#smtpPassword').value = config.smtp_password || '';
      $('#alertEmail').value = config.alert_email || '';
      $('#alertFromEmail').value = config.from_email || '';
      $('#alertOnDown').checked = config.alert_on_down !== false;
      $('#alertOnDegraded').checked = config.alert_on_degraded !== false;
      $('#alertOnUp').checked = config.alert_on_up || false;
    }
  } catch (err) {
    // No alerts config available
  }
}

async function checkNowFor(card) {
  const btn = $('.checkNow', card);
  const key = card.getAttribute('data-key');
  const toggle = $('.monitorToggle', card);
  
  // Don't allow checks on disabled services
  if (toggle && !toggle.checked) {
    showToast('Cannot check disabled services', 'error');
    return;
  }
  
  await handleButtonAction(
    btn,
    async () => {
      const res = await j('/api/admin/check', {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
          'X-CSRF-Token': getCsrf()
        },
        body: JSON.stringify({service: key})
      });
      updCard('card-'+key, res);
      /* also refresh metrics in background */
      refresh();
    },
    `Check completed for ${key}`
  );
}

window.addEventListener('load', async () => {
  // IMPORTANT: Load resources config FIRST before any refresh calls.
  // This prevents hidden tiles from briefly appearing due to race conditions.
  await loadResourcesConfig();
  
  refresh();
  whoami();
  setInterval(refresh, REFRESH_MS);
  
  // Handle both click and touch events for login button
  const loginBtn = $('#loginBtn');
  if (loginBtn) {
    loginBtn.addEventListener('click', doLoginFlow);
    loginBtn.addEventListener('touchstart', (e) => {
      e.preventDefault();
      doLoginFlow();
    });
  }
  
  // Handle both click and touch for doLogin button
  const doLoginBtn = $('#doLogin');
  if (doLoginBtn) {
    doLoginBtn.addEventListener('click', (e) => {
      e.preventDefault();
      e.stopPropagation();
      submitLogin();
    });
    doLoginBtn.addEventListener('touchstart', (e) => {
      e.preventDefault();
      e.stopPropagation();
      submitLogin();
    });
  }

  // Handle cancel button
  const cancelBtn = $('#cancelLogin');
  if (cancelBtn) {
    cancelBtn.addEventListener('click', (e) => {
      e.preventDefault();
      $('#loginModal').close();
    });
    cancelBtn.addEventListener('touchstart', (e) => {
      e.preventDefault();
      $('#loginModal').close();
    });
  }
  
  // Handle both click and touch for logout
  const logoutBtn = $('#logoutBtn');
  if (logoutBtn) {
    logoutBtn.addEventListener('click', logout);
    logoutBtn.addEventListener('touchstart', (e) => {
      e.preventDefault();
      logout();
    });
  }
  
  const ingestBtn = $('#ingestNow');
  if (ingestBtn) {
    ingestBtn.addEventListener('click', ingestAll);
  }
  
  const resetBtn = $('#resetRecent');
  if (resetBtn) {
    resetBtn.addEventListener('click', resetRecent);
  }
  
  // Tab functionality in admin panel
  const ingestBtnTab = $('#ingestNowTab');
  if (ingestBtnTab) {
    ingestBtnTab.addEventListener('click', ingestAll);
  }
  
  const resetBtnTab = $('#resetRecentTab');
  if (resetBtnTab) {
    resetBtnTab.addEventListener('click', resetRecent);
  }
  
  // Tab switching
  const tabBtns = $$('.tab-btn');
  tabBtns.forEach(btn => {
    btn.addEventListener('click', () => {
      const tabName = btn.getAttribute('data-tab');
      
      // Update active tab button
      tabBtns.forEach(b => b.classList.remove('active'));
      btn.classList.add('active');
      
      // Update active tab content
      $$('.tab-content').forEach(content => content.classList.remove('active'));
      const activeContent = $(`#tab-${tabName}`);
      if (activeContent) {
        activeContent.classList.add('active');
      }
      
      // Load data when tabs are clicked
      if (tabName === 'security') {
        const event = new Event('loginStateChanged');
        document.dispatchEvent(event);
      } else if (tabName === 'banners') {
        loadAdminBanners();
      }
    });
  });
  
  // Alerts form handlers
  const saveAlertsBtn = $('#saveAlerts');
  if (saveAlertsBtn) {
    saveAlertsBtn.addEventListener('click', saveAlertsConfig);
  }
  
  const testEmailBtn = $('#testEmail');
  if (testEmailBtn) {
    testEmailBtn.addEventListener('click', sendTestEmail);
  }

  // Resources config handlers
  const saveResourcesBtn = $('#saveResources');
  if (saveResourcesBtn) {
    saveResourcesBtn.addEventListener('click', saveResourcesConfig);
  }
  
  $$('.checkNow').forEach(btn => 
    btn.addEventListener('click', () => checkNowFor(btn.closest('.card')))
  );

  $$('.monitorToggle').forEach(toggle => 
    toggle.addEventListener('change', (e) => toggleMonitoring(e.target.closest('.card'), e.target.checked))
  );
  
  // Uptime filter dropdown
  const uptimeFilter = $('#uptimeFilter');
  if (uptimeFilter) {
    uptimeFilter.addEventListener('change', async (e) => {
      DAYS = parseInt(e.target.value);
      
      // Fetch new metrics and re-render
      try {
        const metrics = await j(`/api/metrics?days=${DAYS}`);
        $('#window').textContent = `Last ${DAYS} days`;
        renderUptimeBars(metrics, DAYS);
      } catch (err) {
        console.error('Failed to fetch metrics for new time range', err);
        renderUptimeBars(null, DAYS);
      }
    });
  }
  
  // Banner management
  const createBannerBtn = $('#createBanner');
  if (createBannerBtn) {
    createBannerBtn.addEventListener('click', createBanner);
  }
  
  // Banner template selection
  const bannerTemplate = $('#bannerTemplate');
  if (bannerTemplate) {
    bannerTemplate.addEventListener('change', () => {
      const msgInput = $('#bannerMessage');
      if (msgInput && bannerTemplate.value) {
        msgInput.value = bannerTemplate.value;
        bannerTemplate.value = ''; // Reset dropdown
      }
    });
  }
  
  // Load banners on page load
  loadBanners();
});

/* Banner Functions */
async function loadBanners() {
  try {
    const banners = await j('/api/status-alerts');
    renderSiteBanners(banners);
    renderServiceBanners(banners);
  } catch (e) {
    console.error('Failed to load banners', e);
  }
}

function getAlertIcon(level) {
  const icons = {
    info: `<svg class="site-alert-icon" viewBox="0 0 20 20" fill="currentColor"><circle cx="10" cy="10" r="9" fill="none" stroke="currentColor" stroke-width="1.5"/><path d="M10 9v4m0-6.5v.5" stroke="currentColor" stroke-width="2" stroke-linecap="round"/></svg>`,
    warning: `<svg class="site-alert-icon" viewBox="0 0 20 20" fill="currentColor"><path d="M10 2L1 18h18L10 2z" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linejoin="round"/><path d="M10 8v4m0 2v.5" stroke="currentColor" stroke-width="2" stroke-linecap="round"/></svg>`,
    error: `<svg class="site-alert-icon" viewBox="0 0 20 20" fill="currentColor"><circle cx="10" cy="10" r="9" fill="none" stroke="currentColor" stroke-width="1.5"/><path d="M7 7l6 6m0-6l-6 6" stroke="currentColor" stroke-width="2" stroke-linecap="round"/></svg>`
  };
  return icons[level] || icons.info;
}

function getServiceAlertIcon(level) {
  const icons = {
    info: `<svg class="service-alert-icon" viewBox="0 0 20 20" fill="currentColor"><circle cx="10" cy="10" r="9" fill="none" stroke="currentColor" stroke-width="1.5"/><path d="M10 9v4m0-6.5v.5" stroke="currentColor" stroke-width="2" stroke-linecap="round"/></svg>`,
    warning: `<svg class="service-alert-icon" viewBox="0 0 20 20" fill="currentColor"><path d="M10 2L1 18h18L10 2z" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linejoin="round"/><path d="M10 8v4m0 2v.5" stroke="currentColor" stroke-width="2" stroke-linecap="round"/></svg>`,
    error: `<svg class="service-alert-icon" viewBox="0 0 20 20" fill="currentColor"><circle cx="10" cy="10" r="9" fill="none" stroke="currentColor" stroke-width="1.5"/><path d="M7 7l6 6m0-6l-6 6" stroke="currentColor" stroke-width="2" stroke-linecap="round"/></svg>`
  };
  return icons[level] || icons.info;
}

function formatBannerTime(isoString) {
  if (!isoString) return '';
  const date = new Date(isoString);
  const now = new Date();
  const diffMs = now - date;
  const diffMins = Math.floor(diffMs / 60000);
  const diffHours = Math.floor(diffMs / 3600000);
  const diffDays = Math.floor(diffMs / 86400000);
  
  if (diffMins < 1) return 'Just now';
  if (diffMins < 60) return `${diffMins}m ago`;
  if (diffHours < 24) return `${diffHours}h ago`;
  if (diffDays < 7) return `${diffDays}d ago`;
  
  return date.toLocaleDateString(undefined, { month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit' });
}

function renderSiteBanners(banners) {
  const container = $('#siteAlerts');
  if (!container) return;
  container.innerHTML = '';
  
  // Only show global banners (no service_key) at the top
  const globalBanners = banners.filter(b => !b.service_key);
  
  globalBanners.forEach(b => {
    const div = document.createElement('div');
    div.className = `site-alert ${b.level}`;
    div.dataset.id = b.id;
    const timeStr = formatBannerTime(b.created_at);
    div.innerHTML = `
      ${getAlertIcon(b.level)}
      <div class="site-alert-content">
        <span class="site-alert-message">${b.message}</span>
        <span class="site-alert-time">${timeStr}</span>
      </div>
    `;
    container.appendChild(div);
  });
}

function renderServiceBanners(banners) {
  // Clear existing service alerts
  document.querySelectorAll('.service-alert').forEach(el => el.remove());
  
  // Filter to only service-specific banners
  const serviceBanners = banners.filter(b => b.service_key);
  
  serviceBanners.forEach(b => {
    const card = $(`#card-${b.service_key}`);
    if (!card) return;
    
    // Check if banner already exists
    const existing = card.querySelector(`.service-alert[data-id="${b.id}"]`);
    if (existing) return;
    
    const alertDiv = document.createElement('div');
    alertDiv.className = `service-alert ${b.level}`;
    alertDiv.dataset.id = b.id;
    const timeStr = formatBannerTime(b.created_at);
    alertDiv.innerHTML = `
      ${getServiceAlertIcon(b.level)}
      <div class="service-alert-content">
        <span>${b.message}</span>
        <span class="service-alert-time">${timeStr}</span>
      </div>
    `;
    
    // Insert before adminRow if present, otherwise at end
    const adminRow = card.querySelector('.adminRow');
    if (adminRow) {
      card.insertBefore(alertDiv, adminRow);
    } else {
      card.appendChild(alertDiv);
    }
  });
}

async function loadAdminBanners() {
  try {
    const banners = await j('/api/admin/status-alerts', {
      headers: { 'X-CSRF-Token': getCsrf() }
    });
    const list = $('#bannersList');
    if (!list) return;
    
    if (banners.length === 0) {
      list.innerHTML = '<div class="muted">No active banners</div>';
      return;
    }
    
    list.innerHTML = '';
    banners.forEach(b => {
      const div = document.createElement('div');
      div.className = 'banner-item';
      const scopeLabel = b.service_key ? b.service_key.charAt(0).toUpperCase() + b.service_key.slice(1) : 'Global';
      div.innerHTML = `
        <span class="banner-item-level ${b.level}">${b.level.toUpperCase()}</span>
        <div class="banner-item-content">
          <span class="banner-item-msg">${b.message}</span>
          <span class="banner-item-service">${scopeLabel}</span>
        </div>
        <button class="banner-delete">Delete</button>
      `;
      div.querySelector('.banner-delete').addEventListener('click', () => deleteBanner(b.id));
      list.appendChild(div);
    });
  } catch (e) {
    console.error('Failed to load admin banners', e);
  }
}

async function createBanner() {
  const msgEl = $('#bannerMessage');
  const levelEl = $('#bannerLevel');
  const serviceEl = $('#bannerService');
  if (!msgEl || !levelEl) return;
  
  const message = msgEl.value.trim();
  const level = levelEl.value;
  const service_key = serviceEl ? serviceEl.value : '';
  
  if (!message) {
    alert('Please enter a message');
    return;
  }
  
  try {
    await j('/api/admin/status-alerts', {
      method: 'POST',
      headers: { 
        'Content-Type': 'application/json',
        'X-CSRF-Token': getCsrf()
      },
      body: JSON.stringify({ message, level, service_key })
    });
    
    msgEl.value = '';
    showToast('Banner created');
    loadBanners();
    loadAdminBanners();
  } catch (e) {
    console.error('Failed to create banner', e);
    showToast('Failed to create banner', 'error');
  }
}

async function deleteBanner(id) {
  if (!confirm('Delete this banner?')) return;
  
  try {
    await j(`/api/admin/status-alerts?id=${id}`, {
      method: 'DELETE',
      headers: { 'X-CSRF-Token': getCsrf() }
    });
    showToast('Banner deleted');
    loadBanners();
    loadAdminBanners();
  } catch (e) {
    console.error('Failed to delete banner', e);
    showToast('Failed to delete banner', 'error');
  }
}

// Handle browser back/forward cache (bfcache) restoration
// When the browser restores from cache, force reload the config to ensure correct visibility
window.addEventListener('pageshow', (event) => {
  if (event.persisted) {
    console.log('[Resources] Page restored from bfcache, reloading config');
    loadResourcesConfig();
  }
});