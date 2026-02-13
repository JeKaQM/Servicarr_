const REFRESH_MS = 15000;
let DAYS = 30;
let resourcesConfig = null; // Cache the config
let isAdminUser = false;
const $ = (s, r = document) => r.querySelector(s);
const $$ = (s, r = document) => Array.from(r.querySelectorAll(s));
const fmtMs = ms => ms == null ? '—' : ms + ' ms';
const cls = (ok, status, degraded) => {
  if (!ok) return 'pill down'; // Down = red
  if (degraded) return 'pill warn'; // Degraded = amber
  return 'pill ok'; // Up = green
};

function fmtBytes(n) {
  if (n == null || isNaN(n)) return '—';
  const units = ['B', 'KB', 'MB', 'GB', 'TB'];
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

function applyAdminUIState() {
  const adminPanel = $('#adminPanel');
  if (isAdminUser) {
    adminPanel?.classList.remove('hidden');
    $$('.adminRow').forEach(e => e.classList.remove('hidden'));
  } else {
    adminPanel?.classList.add('hidden');
    $$('.adminRow').forEach(e => e.classList.add('hidden'));
  }
}

function shouldSuspendDashboardRefresh() {
  const adminPanel = $('#adminPanel');
  if (!adminPanel || adminPanel.classList.contains('hidden')) return false;
  const rect = adminPanel.getBoundingClientRect();
  return rect.top <= 80;
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

  // Resources section only shows if enabled AND glances_url is configured
  const hasGlances = config.glances_url && config.glances_url.trim() !== '';
  const enabled = config.enabled !== false && hasGlances;

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
    else if (kind === 'swap') show = config.swap === true;
    else if (kind === 'load') show = config.load === true;
    else if (kind === 'gpu') show = config.gpu === true;
    else if (kind === 'containers') show = config.containers === true;
    else if (kind === 'processes') show = config.processes === true;
    else if (kind === 'uptime') show = config.uptime === true;

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
      $('#glancesUrl').value = cfg.glances_url || '';
      $('#resourcesEnabled').checked = cfg.enabled !== false;
      $('#resourcesCPU').checked = cfg.cpu !== false;
      $('#resourcesMemory').checked = cfg.memory !== false;
      $('#resourcesNetwork').checked = cfg.network !== false;
      $('#resourcesTemp').checked = cfg.temp !== false;
      if ($('#resourcesStorage')) $('#resourcesStorage').checked = cfg.storage !== false;
      if ($('#resourcesSwap')) $('#resourcesSwap').checked = cfg.swap === true;
      if ($('#resourcesLoad')) $('#resourcesLoad').checked = cfg.load === true;
      if ($('#resourcesGPU')) $('#resourcesGPU').checked = cfg.gpu === true;
      if ($('#resourcesContainers')) $('#resourcesContainers').checked = cfg.containers === true;
      if ($('#resourcesProcesses')) $('#resourcesProcesses').checked = cfg.processes === true;
      if ($('#resourcesUptime')) $('#resourcesUptime').checked = cfg.uptime === true;
    }
  } catch (err) {
    // If the public endpoint isn't available for some reason, try the admin endpoint
    // (will work when logged in).
    try {
      const cfg = await j('/api/admin/resources/config');
      applyResourcesVisibility(cfg);
      if ($('#resourcesEnabled')) {
        $('#glancesUrl').value = cfg.glances_url || '';
        $('#resourcesEnabled').checked = cfg.enabled !== false;
        $('#resourcesCPU').checked = cfg.cpu !== false;
        $('#resourcesMemory').checked = cfg.memory !== false;
        $('#resourcesNetwork').checked = cfg.network !== false;
        $('#resourcesTemp').checked = cfg.temp !== false;
        if ($('#resourcesStorage')) $('#resourcesStorage').checked = cfg.storage !== false;
        if ($('#resourcesSwap')) $('#resourcesSwap').checked = cfg.swap === true;
        if ($('#resourcesLoad')) $('#resourcesLoad').checked = cfg.load === true;
        if ($('#resourcesGPU')) $('#resourcesGPU').checked = cfg.gpu === true;
        if ($('#resourcesContainers')) $('#resourcesContainers').checked = cfg.containers === true;
        if ($('#resourcesProcesses')) $('#resourcesProcesses').checked = cfg.processes === true;
        if ($('#resourcesUptime')) $('#resourcesUptime').checked = cfg.uptime === true;
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
          storage: false,
          swap: false,
          load: false,
          gpu: false,
          containers: false,
          processes: false,
          uptime: false
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
    glances_url: $('#glancesUrl').value.trim(),
    enabled: $('#resourcesEnabled').checked,
    cpu: $('#resourcesCPU').checked,
    memory: $('#resourcesMemory').checked,
    network: $('#resourcesNetwork').checked,
    temp: $('#resourcesTemp').checked,
    storage: $('#resourcesStorage') ? $('#resourcesStorage').checked : true,
    swap: $('#resourcesSwap') ? $('#resourcesSwap').checked : false,
    load: $('#resourcesLoad') ? $('#resourcesLoad').checked : false,
    gpu: $('#resourcesGPU') ? $('#resourcesGPU').checked : false,
    containers: $('#resourcesContainers') ? $('#resourcesContainers').checked : false,
    processes: $('#resourcesProcesses') ? $('#resourcesProcesses').checked : false,
    uptime: $('#resourcesUptime') ? $('#resourcesUptime').checked : false,
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

async function testGlancesConnection() {
  const statusEl = $('#resourcesStatus');
  const btn = $('#testGlances');
  const glancesUrl = $('#glancesUrl').value.trim();

  if (!glancesUrl) {
    if (statusEl) {
      statusEl.textContent = 'Please enter a Glances host:port first';
      statusEl.className = 'status-message error';
      statusEl.classList.remove('hidden');
      setTimeout(() => statusEl.classList.add('hidden'), 3000);
    }
    return;
  }

  // Save config first so the server uses the new URL
  const config = {
    glances_url: glancesUrl,
    enabled: $('#resourcesEnabled').checked,
    cpu: $('#resourcesCPU').checked,
    memory: $('#resourcesMemory').checked,
    network: $('#resourcesNetwork').checked,
    temp: $('#resourcesTemp').checked,
    storage: $('#resourcesStorage') ? $('#resourcesStorage').checked : true,
    swap: $('#resourcesSwap') ? $('#resourcesSwap').checked : false,
    load: $('#resourcesLoad') ? $('#resourcesLoad').checked : false,
    gpu: $('#resourcesGPU') ? $('#resourcesGPU').checked : false,
    containers: $('#resourcesContainers') ? $('#resourcesContainers').checked : false,
    processes: $('#resourcesProcesses') ? $('#resourcesProcesses').checked : false,
    uptime: $('#resourcesUptime') ? $('#resourcesUptime').checked : false,
  };

  await handleButtonAction(
    btn,
    async () => {
      // Save config first
      await j('/api/admin/resources/config', {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
          'X-CSRF-Token': getCsrf()
        },
        body: JSON.stringify(config)
      });

      // Now test the connection
      const result = await j('/api/resources');

      if (result.error) {
        throw new Error(result.message || 'Connection failed');
      }

      if (statusEl) {
        statusEl.textContent = `✓ Connected to Glances on ${result.host || glancesUrl}`;
        statusEl.className = 'status-message success';
        statusEl.classList.remove('hidden');
        setTimeout(() => statusEl.classList.add('hidden'), 5000);
      }

      // Refresh resources display
      applyResourcesVisibility(config);
    },
    'Testing...',
    async (err) => {
      if (statusEl) {
        statusEl.textContent = `✗ Connection failed: ${err.message || 'Could not reach Glances'}`;
        statusEl.className = 'status-message error';
        statusEl.classList.remove('hidden');
      }
    }
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
  const swapTile = document.querySelector('#card-resources .resource-tile[data-kind="swap"]');
  const loadTile = document.querySelector('#card-resources .resource-tile[data-kind="load"]');
  const gpuTile = document.querySelector('#card-resources .resource-tile[data-kind="gpu"]');
  const containersTile = document.querySelector('#card-resources .resource-tile[data-kind="containers"]');
  const processesTile = document.querySelector('#card-resources .resource-tile[data-kind="processes"]');
  const uptimeTile = document.querySelector('#card-resources .resource-tile[data-kind="uptime"]');

  const cpuEnabled = cpuTile && !cpuTile.classList.contains('hidden');
  const memEnabled = memTile && !memTile.classList.contains('hidden');
  const tempEnabled = tempTile && !tempTile.classList.contains('hidden');
  const netEnabled = netTile && !netTile.classList.contains('hidden');
  const storageEnabled = storageTile && !storageTile.classList.contains('hidden');
  const swapEnabled = swapTile && !swapTile.classList.contains('hidden');
  const loadEnabled = loadTile && !loadTile.classList.contains('hidden');
  const gpuEnabled = gpuTile && !gpuTile.classList.contains('hidden');
  const containersEnabled = containersTile && !containersTile.classList.contains('hidden');
  const processesEnabled = processesTile && !processesTile.classList.contains('hidden');
  const uptimeEnabled = uptimeTile && !uptimeTile.classList.contains('hidden');

  // If ALL tiles are disabled, don't fetch data at all
  if (!cpuEnabled && !memEnabled && !tempEnabled && !netEnabled && !storageEnabled && !swapEnabled && !loadEnabled && !gpuEnabled && !containersEnabled && !processesEnabled && !uptimeEnabled) {
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

    // Swap tile
    if (swapEnabled) {
      setResText('res-swap', fmtPct(snap.swap_percent));
      setMeter('meter-swap', snap.swap_percent);
      setResText('res-swap-detail', (snap.swap_used_bytes != null && snap.swap_total_bytes != null)
        ? `${fmtBytes(snap.swap_used_bytes)} / ${fmtBytes(snap.swap_total_bytes)}`
        : 'Swap unavailable');
    }

    // Load Average tile
    if (loadEnabled) {
      const load1 = snap.load_1 != null ? snap.load_1.toFixed(2) : '—';
      const load5 = snap.load_5 != null ? snap.load_5.toFixed(2) : '—';
      const load15 = snap.load_15 != null ? snap.load_15.toFixed(2) : '—';
      setResText('res-load', load1);
      setResText('res-load-1', load1);
      setResText('res-load-5', load5);
      setResText('res-load-15', load15);
    }

    // GPU tile
    if (gpuEnabled) {
      if (snap.gpu_percent != null) {
        setResText('res-gpu', fmtPct(snap.gpu_percent));
        setMeter('meter-gpu', snap.gpu_percent);
        setResText('res-gpu-name', snap.gpu_name || 'GPU');
        setResText('res-gpu-mem', snap.gpu_mem_percent != null ? fmtPct(snap.gpu_mem_percent) : '—');
        setResText('res-gpu-temp', snap.gpu_temp_c != null ? fmtTempC(snap.gpu_temp_c) : '—');
        setResText('res-gpu-detail', '');
      } else {
        setResText('res-gpu', 'N/A');
        setMeter('meter-gpu', null);
        setResText('res-gpu-name', '');
        setResText('res-gpu-mem', '—');
        setResText('res-gpu-temp', '—');
        setResText('res-gpu-detail', 'No GPU detected or nvidia-smi/AMD tools not available on Glances host');
      }
    }

    // Containers tile
    if (containersEnabled) {
      if (snap.container_count != null) {
        setResText('res-containers', snap.container_running != null ? snap.container_running.toString() : '0');
        setResText('res-containers-running', snap.container_running != null ? snap.container_running.toString() : '0');
        setResText('res-containers-total', snap.container_count.toString());
        setResText('res-containers-detail', 'Docker / Podman');
      } else {
        setResText('res-containers', 'N/A');
        setResText('res-containers-running', '—');
        setResText('res-containers-total', '—');
        setResText('res-containers-detail', 'Docker not installed or Glances lacks access to /var/run/docker.sock');
      }
    }

    // Processes tile
    if (processesEnabled) {
      if (snap.proc_total != null) {
        const procTotal = snap.proc_total;
        const procRunning = snap.proc_running != null ? snap.proc_running : 0;
        const procSleeping = snap.proc_sleeping != null ? snap.proc_sleeping : 0;
        const procThreads = snap.proc_threads != null ? snap.proc_threads : 0;
        setResText('res-processes', procTotal.toString());
        setResText('res-proc-running', procRunning.toString());
        setResText('res-proc-sleeping', procSleeping.toString());
        setResText('res-proc-threads', procThreads.toString());
      } else {
        setResText('res-processes', '—');
        setResText('res-proc-running', '—');
        setResText('res-proc-sleeping', '—');
        setResText('res-proc-threads', '—');
      }
    }

    // Uptime tile
    if (uptimeEnabled) {
      setResText('res-uptime', snap.uptime_string || '—');
    }

    // Pill status based on availability and enabled metrics
    if (pill) {
      const hasAny = (snap.cpu_percent != null) || (snap.mem_percent != null) || (snap.temp_c != null) || (snap.net_rx_bytes_per_sec != null) || (snap.net_tx_bytes_per_sec != null);
      pill.textContent = hasAny ? 'LIVE' : 'PARTIAL';
      pill.className = hasAny ? 'pill ok' : 'pill warn';
    }
  } catch (e) {
    // Distinguish error types to avoid false "UNAVAILABLE" on transient issues
    const status = e.status || 0;
    const errorType = e.body && e.body.error ? e.body.error : '';

    if (status === 429) {
      // Rate limited — keep previous state, don't reset tiles
      // The next poll will succeed once the rate limit window passes
      return;
    }

    if (status === 503 && errorType === 'not_configured') {
      // Resources not configured — hide the entire section
      if (section) section.classList.add('hidden');
      return;
    }

    // Genuine error (502 Glances unreachable, network timeout, etc.)
    if (pill) {
      pill.textContent = status === 502 ? 'UNREACHABLE' : 'UNAVAILABLE';
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
    if (swapEnabled) {
      setMeter('meter-swap', null);
      setResText('res-swap', '—');
      setResText('res-swap-detail', 'Swap unavailable');
    }
    if (loadEnabled) {
      setResText('res-load', '—');
      setResText('res-load-1', '—');
      setResText('res-load-5', '—');
      setResText('res-load-15', '—');
    }
    if (gpuEnabled) {
      setMeter('meter-gpu', null);
      setResText('res-gpu', '—');
      setResText('res-gpu-name', '');
      setResText('res-gpu-mem', '—');
      setResText('res-gpu-temp', '—');
      setResText('res-gpu-detail', 'Unable to fetch GPU data from Glances');
    }
    if (containersEnabled) {
      setResText('res-containers', '—');
      setResText('res-containers-running', '—');
      setResText('res-containers-total', '—');
      setResText('res-containers-detail', 'Unable to fetch container data from Glances');
    }
    if (processesEnabled) {
      setResText('res-processes', '—');
      setResText('res-proc-running', '—');
      setResText('res-proc-sleeping', '—');
      setResText('res-proc-threads', '—');
    }
    if (uptimeEnabled) {
      setResText('res-uptime', '—');
    }
  }
}

async function j(u, opts) {
  const controller = new AbortController();
  const timeoutId = setTimeout(() => controller.abort(), 15000); // 15 second timeout for slow networks

  try {
    const fetchOpts = Object.assign({
      cache: 'no-store',
      credentials: 'include',
      signal: controller.signal
    }, opts || {});

    const r = await fetch(u, fetchOpts);
    clearTimeout(timeoutId);

    // Read response body first, before checking ok
    let result;
    const ct = r.headers.get('content-type') || '';
    try {
      result = ct.includes('json') ? await r.json() : await r.text();
    } catch (parseErr) {
      throw new Error(`Failed to parse response: ${parseErr.message}`);
    }

    if (!r.ok) {
      const err = new Error('HTTP ' + r.status);
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

function updCard(id, data) {
  const el = document.getElementById(id);
  if (!el) {
    console.error('Card element not found:', id);
    return;
  }

  const pill = $('.pill', el);
  const k = $('.kpi', el);
  const h = $('.kpirow .label', el); // More specific selector for status label
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
  
  // Show appropriate status message based on check type
  const checkType = (data.check_type || 'http').toLowerCase();
  if (checkType === 'always_up') {
    h.textContent = data.ok ? 'Always up' : 'Down';
  } else if (checkType === 'tcp') {
    h.textContent = data.ok ? 'Port open' : 'Connection refused';
  } else if (checkType === 'dns') {
    h.textContent = data.ok ? 'DNS resolved' : 'Lookup failed';
  } else {
    // HTTP/HTTPS
    if (typeof data.status === 'number' && data.status > 0) {
      h.textContent = 'HTTP ' + data.status;
    } else if (data.status === 0 && !data.ok) {
      h.textContent = 'No response';
    } else {
      h.textContent = '—';
    }
  }

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
  if (!window.Chart) return;
  // Get service keys from the servicesData array (dynamic)
  const labels = servicesData.map(s => s.key);
  if (labels.length === 0) return;

  const vals = labels.map(k => +(overall?.[k] ?? 0).toFixed(1));
  const ctx = document.getElementById('uptimeChart');
  if (!ctx) return;

  const data = { labels, datasets: [{ label: 'Uptime %', data: vals, borderWidth: 1 }] };

  if (chart) {
    chart.data = data;
    chart.update();
    return;
  }

  chart = new Chart(ctx.getContext('2d'), {
    type: 'bar',
    data,
    options: {
      responsive: true,
      plugins: { legend: { display: false } },
      scales: { y: { beginAtZero: true, max: 100 } }
    }
  });
}

function renderIncidents(items) {
  const list = $('#incidents');
  if (!items?.length) {
    list.innerHTML = '<li class="label">No incidents in last 24h</li>';
    return;
  }

  list.innerHTML = items.map(i => {
    const rawTs = i.taken_at || i.time || '';
    let ts = '';
    if (rawTs) {
      const d = new Date(rawTs);
      ts = Number.isNaN(d.getTime()) ? String(rawTs) : d.toLocaleString();
    }

    const svcRaw = i.service_name || i.service_key || '';
    const svc = escapeHtml(svcRaw);
    const statusCode = Number(i.http_status) || 0;
    const latency = i.latency_ms ?? i.ping;
    let err = '';
    if (typeof i.error === 'string') err = i.error;
    else if (typeof i.msg === 'string') err = i.msg;
    if (err) {
      err = err.trim();
    }

    const parts = [];
    const checkType = String(i.check_type || '').trim();
    if (checkType && checkType.toLowerCase() !== 'http') {
      parts.push(checkType.toUpperCase());
    }
    if (statusCode > 0) parts.push(`HTTP ${statusCode}`);
    if (latency && Number(latency) > 0) parts.push(`${Number(latency)}ms`);
    if (err) parts.push(err);

    const detail = parts.length ? parts.join(' | ') : 'down';
    const summary = detail.length > 90 ? detail.slice(0, 87) + '...' : detail;

    const payload = JSON.stringify({
      time: ts || rawTs,
      service: svcRaw,
      check_type: checkType || 'http',
      status: statusCode > 0 ? `HTTP ${statusCode}` : 'No response',
      latency: latency && Number(latency) > 0 ? `${Number(latency)}ms` : '',
      detail,
      error: err || ''
    }).replace(/'/g, '&#39;').replace(/"/g, '&quot;');

    return `
      <li class="incident-item" data-incident="${payload}">
        <span class="dot"></span>
        <div class="incident-content">
          <span class="incident-time">${escapeHtml(ts)}</span>
          <span class="incident-detail">${svc} (${escapeHtml(summary)})</span>
        </div>
        <span class="incident-action">Details</span>
      </li>
    `;
  }).join('');

  $$('#incidents .incident-item').forEach(item => {
    item.addEventListener('click', () => showIncidentDetails(item));
  });
}

function updateServiceStats(metrics) {
  // Get service keys from the servicesData array (dynamic)
  const services = servicesData.map(s => s.key);

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
  // Get service keys from the servicesData array (dynamic)
  const services = servicesData.map(s => s.key);
  const now = new Date();
  const daysAgo = now.getTime() - (daysToShow * 24 * 60 * 60 * 1000);

  // Find the earliest date with actual data across all services
  let earliestDate = null;
  if (metrics && metrics.series) {
    services.forEach(key => {
      const data = metrics.series[key] || [];
      data.forEach(point => {
        const dayStr = point.day || point.hour?.substr(0, 10);
        if (dayStr && point.uptime !== null && point.uptime !== undefined) {
          const d = new Date(dayStr);
          if (!earliestDate || d < earliestDate) {
            earliestDate = d;
          }
        }
      });
    });
  }

  // Update global timestamp with actual tracking start date
  const globalTimestamp = $('#timestamp-global');
  if (globalTimestamp) {
    if (earliestDate) {
      const startDate = earliestDate.toLocaleDateString();
      globalTimestamp.textContent = `Tracking since ${startDate} • Hover over blocks for details`;
    } else {
      globalTimestamp.textContent = `No data yet • Hover over blocks for details`;
    }
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
  const suspendDashboard = shouldSuspendDashboardRefresh();

  try {
    const live = await j('/api/check');
    $('#updated').textContent = new Date(live.t).toLocaleString();

    // Update cards dynamically based on services returned from API
    if (live.status) {
      latestLiveStatus = live.status;  // cache for matrix view

      Object.keys(live.status).forEach(key => {
        const cardEl = document.getElementById(`card-${key}`);
        if (cardEl) {
          updCard(`card-${key}`, live.status[key] || {});
        }
      });

      // Re-render matrix if active
      if (currentView === 'matrix') renderMatrix();
    }
  } catch (e) {
    console.error('live check failed', e);
  }

  // Resources (Glances)
  if (!suspendDashboard) {
    refreshResources();
  }

  try {
    if (suspendDashboard) {
      return;
    }

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
    // Rate limited — keep previous bars, don't wipe to empty
    if (e.status === 429) return;
    // Genuine error — render with no data
    if (!suspendDashboard) {
      renderUptimeBars(null, DAYS);
    }
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
  submitBtn.textContent = 'Signing in...';

  try {
    const csrfToken = getCsrf();

    const result = await j('/api/login', {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        'X-CSRF-Token': csrfToken
      },
      body: JSON.stringify({ username: u, password: p })
    });

    dlg.close();
    // Reload page to get server-rendered admin elements
    window.location.reload();
  } catch (err) {
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
    submitBtn.textContent = 'Sign In';
  }
}

async function logout() {
  try {
    await j('/api/logout', { method: 'POST' });
  } catch (_) { }
  // Reload page to remove server-rendered admin elements
  window.location.reload();
}

function getCsrf() {
  return (document.cookie.split('; ').find(s => s.startsWith('csrf=')) || '').split('=')[1] || '';
}

// Custom event for login state changes
const loginStateChanged = new Event('loginStateChanged');

async function whoami() {
  try {
    const me = await j('/api/me');

    if (me.authenticated) {
      isAdminUser = true;
      $('#welcome').textContent = 'Welcome, ' + me.user;
      $('#loginBtn').classList.add('hidden');
      $('#logoutBtn').classList.remove('hidden');
      applyAdminUIState();
      document.dispatchEvent(loginStateChanged);
      loadAlertsConfig();
      loadResourcesConfig();
    } else {
      isAdminUser = false;
      $('#welcome').textContent = 'Public view';
      $('#loginBtn').classList.remove('hidden');
      $('#logoutBtn').classList.add('hidden');
      applyAdminUIState();

      // Reset login form
      const dlg = document.getElementById('loginModal');
      if (dlg) {
        const submitBtn = $('#doLogin', dlg);
        if (submitBtn) {
          submitBtn.disabled = false;
          submitBtn.textContent = 'Sign In';
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
    let msg = err?.message || 'Action failed';
    if (err?.body) {
      if (typeof err.body === 'string') {
        msg = err.body;
      } else if (typeof err.body === 'object') {
        msg = err.body.message || err.body.error || msg;
      }
    }
    showToast(msg, 'error');
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
        headers: { 'X-CSRF-Token': getCsrf() }
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
        headers: { 'X-CSRF-Token': getCsrf() }
      });
      await refresh();
    },
    'Recent incidents reset successfully'
  );
}

/* Security Tab Functions */
async function loadSecurityData() {
  await Promise.all([loadBlocks(), loadWhitelist(), loadBlacklist()]);
}

async function loadBlocks() {
  const container = $('#blocksList');
  if (!container) return;

  try {
    const data = await j('/api/admin/blocks');
    const blocks = data.blocks || [];

    if (blocks.length === 0) {
      container.innerHTML = '<div class="muted">No temporary blocks</div>';
      return;
    }

    container.innerHTML = blocks.map(block => `
      <div class="block-item">
        <div class="block-info">
          <strong>${escapeHtml(block.ip)}</strong>
          <span class="muted">Attempts: ${block.attempts} • Expires: ${new Date(block.expires_at).toLocaleString()}</span>
        </div>
        <button class="btn danger small" onclick="unblockIP('${escapeHtml(block.ip)}')">Unblock</button>
      </div>
    `).join('');
  } catch (err) {
    container.innerHTML = '<div class="muted">Failed to load blocks</div>';
  }
}

async function unblockIP(ip) {
  try {
    await j('/api/admin/unblock', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json', 'X-CSRF-Token': getCsrf() },
      body: JSON.stringify({ ip })
    });
    showToast('IP unblocked');
    loadBlocks();
  } catch (err) {
    showToast('Failed to unblock IP', 'error');
  }
}

async function clearAllBlocks() {
  if (!confirm('Are you sure you want to clear all temporary blocks?')) return;
  try {
    await j('/api/admin/clear-blocks', {
      method: 'POST',
      headers: { 'X-CSRF-Token': getCsrf() }
    });
    showToast('All blocks cleared');
    loadBlocks();
  } catch (err) {
    showToast('Failed to clear blocks', 'error');
  }
}

async function loadWhitelist() {
  const container = $('#whitelistList');
  if (!container) return;

  try {
    const data = await j('/api/admin/whitelist');
    const list = data.whitelist || [];

    if (list.length === 0) {
      container.innerHTML = '<div class="muted">No whitelisted IPs</div>';
      return;
    }

    container.innerHTML = list.map(item => `
      <div class="block-item">
        <div class="block-info">
          <strong>${escapeHtml(item.ip)}</strong>
          <span class="muted">${item.note ? escapeHtml(item.note) : 'No note'} • Added: ${new Date(item.created_at).toLocaleDateString()}</span>
        </div>
        <button class="btn danger small" onclick="removeFromWhitelist('${escapeHtml(item.ip)}')">Remove</button>
      </div>
    `).join('');
  } catch (err) {
    container.innerHTML = '<div class="muted">Failed to load whitelist</div>';
  }
}

async function addToWhitelist() {
  const ipInput = $('#whitelistIP');
  const noteInput = $('#whitelistNote');
  const ip = ipInput.value.trim();
  const note = noteInput.value.trim();

  if (!ip) {
    showToast('Please enter an IP address', 'error');
    return;
  }

  try {
    await j('/api/admin/whitelist', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json', 'X-CSRF-Token': getCsrf() },
      body: JSON.stringify({ ip, note })
    });
    ipInput.value = '';
    noteInput.value = '';
    showToast('IP added to whitelist');
    loadWhitelist();
  } catch (err) {
    showToast('Failed to add to whitelist', 'error');
  }
}

async function removeFromWhitelist(ip) {
  try {
    await j('/api/admin/whitelist', {
      method: 'DELETE',
      headers: { 'Content-Type': 'application/json', 'X-CSRF-Token': getCsrf() },
      body: JSON.stringify({ ip })
    });
    showToast('IP removed from whitelist');
    loadWhitelist();
  } catch (err) {
    showToast('Failed to remove from whitelist', 'error');
  }
}

async function loadBlacklist() {
  const container = $('#blacklistList');
  if (!container) return;

  try {
    const data = await j('/api/admin/blacklist');
    const list = data.blacklist || [];

    if (list.length === 0) {
      container.innerHTML = '<div class="muted">No blacklisted IPs</div>';
      return;
    }

    container.innerHTML = list.map(item => `
      <div class="block-item">
        <div class="block-info">
          <strong>${escapeHtml(item.ip)}${item.permanent ? '<span class="badge">PERMANENT</span>' : ''}</strong>
          <span class="muted">${item.note ? escapeHtml(item.note) : 'No note'} • Added: ${new Date(item.created_at).toLocaleDateString()}</span>
        </div>
        <button class="btn danger small" onclick="removeFromBlacklist('${escapeHtml(item.ip)}')">Remove</button>
      </div>
    `).join('');
  } catch (err) {
    container.innerHTML = '<div class="muted">Failed to load blacklist</div>';
  }
}

async function addToBlacklist() {
  const ipInput = $('#blacklistIP');
  const noteInput = $('#blacklistNote');
  const permanentInput = $('#blacklistPermanent');
  const ip = ipInput.value.trim();
  const note = noteInput.value.trim();
  const permanent = permanentInput.checked;

  if (!ip) {
    showToast('Please enter an IP address', 'error');
    return;
  }

  try {
    await j('/api/admin/blacklist', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json', 'X-CSRF-Token': getCsrf() },
      body: JSON.stringify({ ip, note, permanent })
    });
    ipInput.value = '';
    noteInput.value = '';
    permanentInput.checked = false;
    showToast('IP added to blacklist');
    loadBlacklist();
  } catch (err) {
    showToast('Failed to add to blacklist', 'error');
  }
}

async function removeFromBlacklist(ip) {
  try {
    await j('/api/admin/blacklist', {
      method: 'DELETE',
      headers: { 'Content-Type': 'application/json', 'X-CSRF-Token': getCsrf() },
      body: JSON.stringify({ ip })
    });
    showToast('IP removed from blacklist');
    loadBlacklist();
  } catch (err) {
    showToast('Failed to remove from blacklist', 'error');
  }
}

function escapeHtml(str) {
  const div = document.createElement('div');
  div.textContent = str;
  return div.innerHTML;
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
    status_page_url: $('#statusPageUrl').value.trim(),
    smtp_skip_verify: $('#smtpSkipVerify').checked,
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
        headers: { 'X-CSRF-Token': getCsrf() }
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
      $('#statusPageUrl').value = config.status_page_url || '';
      $('#smtpSkipVerify').checked = config.smtp_skip_verify || false;
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
        body: JSON.stringify({ service: key })
      });
      updCard('card-' + key, res);
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

  // Load services dynamically and render them (non-blocking)
  loadServices().then(services => {
    if (services.length > 0) {
      renderDynamicUptimeBars(services);
    }
  }).catch(e => {
    console.error('Failed to load services on init', e);
  });

  // Initialize services management (admin features)
  initServicesManagement();

  // Initialize settings tab (admin features)
  initSettingsTab();

  // Initialize view toggle (Cards / Hive)
  initViewToggle();

  // Start the refresh cycle immediately (don't wait for services)
  refresh();
  whoami();
  setInterval(refresh, REFRESH_MS);

  // Handle both click and touch events for login button
  const loginBtn = $('#loginBtn');
  if (loginBtn) {
    loginBtn.addEventListener('click', doLoginFlow);
  }

  // Handle login form submission (prevents iOS form submit)
  const loginForm = document.querySelector('#loginModal .login-form');
  if (loginForm) {
    loginForm.addEventListener('submit', (e) => {
      e.preventDefault();
      e.stopPropagation();
      submitLogin();
      return false;
    });
  }

  // Handle doLogin button
  const doLoginBtn = $('#doLogin');
  if (doLoginBtn) {
    doLoginBtn.addEventListener('click', (e) => {
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
        loadSecurityData();
      } else if (tabName === 'banners') {
        loadAdminBanners();
        populateBannerScopeDropdown();
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

  const testGlancesBtn = $('#testGlances');
  if (testGlancesBtn) {
    testGlancesBtn.addEventListener('click', testGlancesConnection);
  }

  // Security tab handlers
  const resetBlocksBtn = $('#resetBlocks');
  if (resetBlocksBtn) {
    resetBlocksBtn.addEventListener('click', clearAllBlocks);
  }

  const addWhitelistBtn = $('#addWhitelist');
  if (addWhitelistBtn) {
    addWhitelistBtn.addEventListener('click', addToWhitelist);
  }

  const addBlacklistBtn = $('#addBlacklist');
  if (addBlacklistBtn) {
    addBlacklistBtn.addEventListener('click', addToBlacklist);
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

function normalizeAlertLevel(level) {
  const allowed = ['info', 'warning', 'error'];
  return allowed.includes(level) ? level : 'info';
}

function renderSiteBanners(banners) {
  const container = $('#siteAlerts');
  if (!container) return;
  container.innerHTML = '';

  // Only show global banners (no service_key) at the top
  const globalBanners = banners.filter(b => !b.service_key);

  globalBanners.forEach(b => {
    const level = normalizeAlertLevel(b.level);
    const message = escapeHtml(b.message || '');
    const div = document.createElement('div');
    div.className = `site-alert ${level}`;
    div.dataset.id = b.id;
    const timeStr = formatBannerTime(b.created_at);
    div.innerHTML = `
      ${getAlertIcon(level)}
      <div class="site-alert-content">
        <span class="site-alert-message">${message}</span>
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
    const level = normalizeAlertLevel(b.level);
    const message = escapeHtml(b.message || '');
    const card = $(`#card-${b.service_key}`);
    if (!card) return;

    // Check if banner already exists
    const existing = card.querySelector(`.service-alert[data-id="${b.id}"]`);
    if (existing) return;

    const alertDiv = document.createElement('div');
    alertDiv.className = `service-alert ${level}`;
    alertDiv.dataset.id = b.id;
    const timeStr = formatBannerTime(b.created_at);
    alertDiv.innerHTML = `
      ${getServiceAlertIcon(level)}
      <div class="service-alert-content">
        <span>${message}</span>
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
      const level = normalizeAlertLevel(b.level);
      const message = escapeHtml(b.message || '');
      const div = document.createElement('div');
      div.className = 'banner-item';
      const scopeLabel = escapeHtml(b.service_key ? b.service_key.charAt(0).toUpperCase() + b.service_key.slice(1) : 'Global');
      div.innerHTML = `
        <span class="banner-item-level ${level}">${level.toUpperCase()}</span>
        <div class="banner-item-content">
          <span class="banner-item-msg">${message}</span>
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

function populateBannerScopeDropdown() {
  const select = $('#bannerService');
  if (!select) return;

  // Keep the global option, remove service options
  const globalOption = select.querySelector('option[value=""]');
  select.innerHTML = '';
  if (globalOption) {
    select.appendChild(globalOption);
  } else {
    const opt = document.createElement('option');
    opt.value = '';
    opt.textContent = 'Global (top of page)';
    select.appendChild(opt);
  }

  // Add all services from servicesData
  if (servicesData && servicesData.length > 0) {
    const optgroup = document.createElement('optgroup');
    optgroup.label = 'Services';

    servicesData.forEach(svc => {
      const opt = document.createElement('option');
      opt.value = svc.key;
      opt.textContent = svc.name;
      optgroup.appendChild(opt);
    });

    select.appendChild(optgroup);
  }
}

/* ========================================
   Dynamic Services Management
   ======================================== */

let servicesData = [];
let serviceTemplates = [];
let editingServiceId = null;

// Service Icons SVG - inline for simplicity
const SERVICE_ICONS = {
  server: `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><rect x="2" y="3" width="20" height="6" rx="2"/><rect x="2" y="13" width="20" height="6" rx="2"/><circle cx="6" cy="6" r="1" fill="currentColor"/><circle cx="6" cy="16" r="1" fill="currentColor"/></svg>`,
  plex: `<svg viewBox="0 0 24 24" fill="currentColor"><path d="M12 2L4 7v10l8 5 8-5V7l-8-5zm0 2.5L17 8v8l-5 3-5-3V8l5-3.5z"/></svg>`,
  overseerr: `<svg viewBox="0 0 24 24" fill="currentColor"><circle cx="12" cy="12" r="10"/><path d="M8 12l3 3 5-5" stroke="white" stroke-width="2" fill="none"/></svg>`,
  jellyfin: `<svg viewBox="0 0 24 24" fill="currentColor"><ellipse cx="12" cy="6" rx="4" ry="4"/><ellipse cx="12" cy="18" rx="4" ry="4"/><ellipse cx="12" cy="12" rx="8" ry="4" fill="none" stroke="currentColor" stroke-width="1.5"/></svg>`,
  emby: `<svg viewBox="0 0 24 24" fill="currentColor"><path d="M12 2C6.48 2 2 6.48 2 12s4.48 10 10 10 10-4.48 10-10S17.52 2 12 2zm-2 14.5v-9l6 4.5-6 4.5z"/></svg>`,
  sonarr: `<svg viewBox="0 0 24 24" fill="currentColor"><rect x="3" y="3" width="18" height="18" rx="2"/><path d="M7 15l3-3 2 2 5-5" stroke="white" stroke-width="2" fill="none"/></svg>`,
  radarr: `<svg viewBox="0 0 24 24" fill="currentColor"><circle cx="12" cy="12" r="10"/><circle cx="12" cy="12" r="3" fill="white"/></svg>`,
  prowlarr: `<svg viewBox="0 0 24 24" fill="currentColor"><circle cx="12" cy="12" r="10"/><path d="M8 12h8M12 8v8" stroke="white" stroke-width="2"/></svg>`,
  lidarr: `<svg viewBox="0 0 24 24" fill="currentColor"><circle cx="12" cy="12" r="10"/><circle cx="12" cy="12" r="4" fill="white"/><circle cx="12" cy="12" r="1.5" fill="currentColor"/></svg>`,
  readarr: `<svg viewBox="0 0 24 24" fill="currentColor"><path d="M4 4h6v16H4V4zm10 0h6v16h-6V4z"/></svg>`,
  bazarr: `<svg viewBox="0 0 24 24" fill="currentColor"><rect x="2" y="6" width="20" height="12" rx="2"/><path d="M6 10h12M6 14h8" stroke="white" stroke-width="1.5"/></svg>`,
  tautulli: `<svg viewBox="0 0 24 24" fill="currentColor"><path d="M3 3v18h18V3H3zm16 16H5V5h14v14z"/><path d="M7 17V9l4 4-4 4zm6-8h4v2h-4V9zm0 4h4v2h-4v-2z"/></svg>`,
  sabnzbd: `<svg viewBox="0 0 24 24" fill="currentColor"><path d="M12 2L2 7l10 5 10-5-10-5zM2 17l10 5 10-5M2 12l10 5 10-5"/></svg>`,
  qbittorrent: `<svg viewBox="0 0 24 24" fill="currentColor"><circle cx="12" cy="12" r="10"/><path d="M12 6v12M6 12h12" stroke="white" stroke-width="2"/></svg>`,
  transmission: `<svg viewBox="0 0 24 24" fill="currentColor"><circle cx="12" cy="12" r="10"/><path d="M12 6v6l4 2" stroke="white" stroke-width="2" fill="none"/></svg>`,
  homeassistant: `<svg viewBox="0 0 24 24" fill="currentColor"><path d="M12 3L2 12h3v8h14v-8h3L12 3zm0 12a2 2 0 100-4 2 2 0 000 4z"/></svg>`,
  pihole: `<svg viewBox="0 0 24 24" fill="currentColor"><circle cx="12" cy="12" r="10"/><circle cx="12" cy="12" r="6" fill="white"/><circle cx="12" cy="12" r="3" fill="currentColor"/></svg>`,
  portainer: `<svg viewBox="0 0 24 24" fill="currentColor"><path d="M4 4h7v7H4V4zm9 0h7v7h-7V4zM4 13h7v7H4v-7zm9 0h7v7h-7v-7z"/></svg>`,
  website: `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><circle cx="12" cy="12" r="10"/><path d="M2 12h20M12 2a15.3 15.3 0 014 10 15.3 15.3 0 01-4 10 15.3 15.3 0 01-4-10 15.3 15.3 0 014-10z"/></svg>`,
  custom: `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><circle cx="12" cy="12" r="3"/><path d="M12 1v4m0 14v4M4.22 4.22l2.83 2.83m9.9 9.9l2.83 2.83M1 12h4m14 0h4M4.22 19.78l2.83-2.83m9.9-9.9l2.83-2.83"/></svg>`
};

// Service Icons - use image files for known services, inline SVG for others
const SERVICE_ICON_FILES = ['server', 'plex', 'overseerr'];

// Get icon HTML for a service - supports custom icon URLs
function getServiceIconHtml(serviceTypeOrObj, iconUrl = null) {
  let serviceType = serviceTypeOrObj;
  let customIconUrl = iconUrl;

  // Handle service object
  if (typeof serviceTypeOrObj === 'object' && serviceTypeOrObj !== null) {
    serviceType = serviceTypeOrObj.service_type || 'custom';
    customIconUrl = serviceTypeOrObj.icon_url || null;
  }

  // Use custom icon URL if provided
  if (customIconUrl) {
    // Use a data attribute for fallback, handle error in CSS/JS
    return `<img src="${customIconUrl}" class="icon service-icon-img" alt="${serviceType}" data-fallback="${serviceType}" onerror="this.style.display='none';this.nextElementSibling.style.display='flex';"/><span class="icon icon-fallback" style="display:none;">${SERVICE_ICONS[serviceType] || SERVICE_ICONS.custom}</span>`;
  }

  // Use local image file if available
  if (SERVICE_ICON_FILES.includes(serviceType)) {
    return `<img src="/static/images/${serviceType}.svg" class="icon" alt="${serviceType}"/>`;
  }

  // Fallback to inline SVG for other services
  return `<span class="icon">${SERVICE_ICONS[serviceType] || SERVICE_ICONS.custom}</span>`;
}

async function loadServiceTemplates() {
  try {
    const templates = await j('/api/services/templates');
    serviceTemplates = templates;
    populateTemplateDropdown();
  } catch (e) {
    console.error('Failed to load service templates', e);
  }
}

function populateTemplateDropdown() {
  const select = $('#serviceTemplate');
  if (!select || !serviceTemplates.length) return;

  select.innerHTML = '<option value="">Select a template...</option>';
  serviceTemplates.forEach(t => {
    const opt = document.createElement('option');
    opt.value = t.type; // Templates use 'type' field
    opt.textContent = t.name;
    select.appendChild(opt);
  });
}

async function loadServices() {
  try {
    // Load visible services for public view
    const services = await j('/api/services');
    servicesData = services;
    renderServiceCards(services);
    return services;
  } catch (e) {
    console.error('Failed to load services', e);
    return [];
  }
}

async function loadAllServices() {
  try {
    // Load all services for admin view (includes hidden)
    const services = await j('/api/admin/services', {
      headers: { 'X-CSRF-Token': getCsrf() }
    });
    servicesData = services;
    renderAdminServicesList(services);
    populateBannerScopeDropdown(); // Update banner scope dropdown
    return services;
  } catch (e) {
    console.error('Failed to load all services', e);
    return [];
  }
}

function renderServiceCards(services) {
  const container = $('#services-container');
  if (!container) return;

  container.innerHTML = '';

  services.forEach(svc => {
    const card = document.createElement('section');
    card.className = 'card';
    card.id = `card-${svc.key}`;
    card.setAttribute('data-key', svc.key);

    const iconHtml = getServiceIconHtml(svc);
    const svcName = escapeHtml(svc.name || '');
    const svcLabel = escapeHtml(getServiceLabel(svc.service_type));

    // Match original structure - no clickable link exposing URL
    card.innerHTML = `
      <div class="row">
        <div class="row-left">
          ${iconHtml}
          <div><strong>${svcName}</strong><div class="label">${svcLabel}</div></div>
        </div>
        <span class="pill warn">—</span>
      </div>
      <div class="row kpirow"><div class="kpi">—</div><div class="label">—</div></div>
      
      <div class="stats-grid">
        <div class="stat-item">
          <div class="stat-label">24h Uptime</div>
          <div class="stat-value" id="uptime-24h-${svc.key}">—</div>
        </div>
        <div class="stat-item">
          <div class="stat-label">Avg Response</div>
          <div class="stat-value" id="avg-response-${svc.key}">—</div>
        </div>
        <div class="stat-item">
          <div class="stat-label">Last Check</div>
          <div class="stat-value" id="last-check-${svc.key}">—</div>
        </div>
      </div>
      
      <div class="row adminRow hidden">
        <div class="label">Admin</div>
        <div class="ops">
          <button class="btn mini checkNow">Check now</button>
          <label class="toggle">
            <input type="checkbox" class="monitorToggle" checked>
            <span class="slider"></span>
            <span class="toggleLabel">Monitor</span>
          </label>
        </div>
      </div>
    `;

    container.appendChild(card);
  });

  // Rebind event handlers for new cards
  $$('.checkNow').forEach(btn => {
    btn.removeEventListener('click', checkNowHandler);
    btn.addEventListener('click', checkNowHandler);
  });

  $$('.monitorToggle').forEach(toggle => {
    toggle.removeEventListener('change', toggleMonitoringHandler);
    toggle.addEventListener('change', toggleMonitoringHandler);
  });

  applyAdminUIState();
}

// Get service label based on type
function getServiceLabel(serviceType) {
  const labels = {
    server: 'Health Check',
    plex: 'Media Server',
    overseerr: 'Request Management',
    jellyfin: 'Media Server',
    emby: 'Media Server',
    sonarr: 'TV Shows',
    radarr: 'Movies',
    prowlarr: 'Indexer Manager',
    lidarr: 'Music',
    readarr: 'Books',
    bazarr: 'Subtitles',
    tautulli: 'Plex Stats',
    sabnzbd: 'Usenet Downloader',
    qbittorrent: 'Torrent Client',
    transmission: 'Torrent Client',
    homeassistant: 'Home Automation',
    pihole: 'DNS Filter',
    portainer: 'Container Manager',
    website: 'Website',
    custom: 'Service'
  };
  return labels[serviceType] || 'Service';
}

function checkNowHandler(e) {
  checkNowFor(e.target.closest('.card'));
}

function toggleMonitoringHandler(e) {
  toggleMonitoring(e.target.closest('.card'), e.target.checked);
}

/* ── View Toggle (Cards ↔ Matrix) ──────────────────────── */
let currentView = 'cards';       // 'cards' | 'matrix'
let latestLiveStatus = null;     // cache last /api/check result for matrix
let matrixAnimFrame = null;      // requestAnimationFrame id
let matrixTooltipEl = null;      // shared tooltip element

function initViewToggle() {
  const btnCards  = $('#viewCards');
  const btnMatrix = $('#viewMatrix');
  if (!btnCards || !btnMatrix) return;

  btnCards.addEventListener('click',  () => switchView('cards'));
  btnMatrix.addEventListener('click', () => switchView('matrix'));
}

function switchView(view) {
  currentView = view;
  const cards  = $('#services-container');
  const matrix = $('#matrix-container');
  const btnC   = $('#viewCards');
  const btnM   = $('#viewMatrix');

  if (view === 'matrix') {
    cards  && cards.classList.add('hidden');
    matrix && matrix.classList.remove('hidden');
    btnC   && btnC.classList.remove('active');
    btnM   && btnM.classList.add('active');
    renderMatrix();
  } else {
    matrix && matrix.classList.add('hidden');
    cards  && cards.classList.remove('hidden');
    btnM   && btnM.classList.remove('active');
    btnC   && btnC.classList.add('active');
    stopMatrixAnimation();
  }
}

/* ── Matrix status helpers ──────────────────────────────── */
function matrixStatusOf(svc) {
  let statusClass = 'unknown', statusLabel = 'Unknown', ms = null;
  if (latestLiveStatus && latestLiveStatus[svc.key]) {
    const s = latestLiveStatus[svc.key];
    if (s.disabled)       { statusClass = 'disabled'; statusLabel = 'Disabled'; }
    else if (!s.ok)       { statusClass = 'down';     statusLabel = 'Down';     }
    else if (s.degraded)  { statusClass = 'degraded'; statusLabel = 'Degraded'; }
    else                  { statusClass = 'up';       statusLabel = 'Operational'; }
    if (s.ms != null) ms = s.ms;
  }
  return { statusClass, statusLabel, ms };
}

const MATRIX_COLORS = {
  up:       { r: 34,  g: 197, b: 94  },
  down:     { r: 248, g: 113, b: 113 },
  degraded: { r: 251, g: 191, b: 36  },
  disabled: { r: 100, g: 116, b: 139 },
  unknown:  { r: 100, g: 116, b: 139 },
  hub:      { r: 99,  g: 102, b: 241 }
};

/* ── Canvas line animation engine ───────────────────────── */
function stopMatrixAnimation() {
  if (matrixAnimFrame) { cancelAnimationFrame(matrixAnimFrame); matrixAnimFrame = null; }
}

function animateMatrixLines(canvas, nodePositions) {
  stopMatrixAnimation();
  const ctx = canvas.getContext('2d');
  if (!ctx) return;

  const dpr = window.devicePixelRatio || 1;

  function frame(t) {
    const w = canvas.clientWidth;
    const h = canvas.clientHeight;
    canvas.width  = w * dpr;
    canvas.height = h * dpr;
    ctx.setTransform(dpr, 0, 0, dpr, 0, 0);
    ctx.clearRect(0, 0, w, h);

    const cx = w / 2;
    const cy = h / 2;

    // Draw lines from each node to the centre hub
    nodePositions.forEach(n => {
      const col = MATRIX_COLORS[n.status] || MATRIX_COLORS.unknown;

      // Gradient along the line
      const grad = ctx.createLinearGradient(n.x, n.y, cx, cy);
      grad.addColorStop(0, 'rgba(' + col.r + ',' + col.g + ',' + col.b + ',0.55)');
      grad.addColorStop(1, 'rgba(' + MATRIX_COLORS.hub.r + ',' + MATRIX_COLORS.hub.g + ',' + MATRIX_COLORS.hub.b + ',0.25)');

      ctx.beginPath();
      ctx.moveTo(n.x, n.y);
      ctx.lineTo(cx, cy);
      ctx.strokeStyle = grad;
      ctx.lineWidth = 1.2;
      ctx.stroke();

      // Pulsating particle travelling node → hub
      const speed  = 4000; // ms per full trip
      const prog   = ((t + n.phase) % speed) / speed;
      const px     = n.x + (cx - n.x) * prog;
      const py     = n.y + (cy - n.y) * prog;
      const pulse  = 0.5 + 0.5 * Math.sin(prog * Math.PI);
      const radius = 2 + pulse * 2;

      ctx.beginPath();
      ctx.arc(px, py, radius, 0, Math.PI * 2);
      ctx.fillStyle = 'rgba(' + col.r + ',' + col.g + ',' + col.b + ',' + (0.6 + pulse * 0.4) + ')';
      ctx.fill();

      // Soft glow around particle
      ctx.beginPath();
      ctx.arc(px, py, radius + 4, 0, Math.PI * 2);
      const glow = ctx.createRadialGradient(px, py, 0, px, py, radius + 4);
      glow.addColorStop(0, 'rgba(' + col.r + ',' + col.g + ',' + col.b + ',0.3)');
      glow.addColorStop(1, 'rgba(' + col.r + ',' + col.g + ',' + col.b + ',0)');
      ctx.fillStyle = glow;
      ctx.fill();
    });

    // Draw faint interconnect lines between adjacent nodes
    for (let i = 0; i < nodePositions.length; i++) {
      const j = (i + 1) % nodePositions.length;
      const a = nodePositions[i];
      const b = nodePositions[j];
      ctx.beginPath();
      ctx.moveTo(a.x, a.y);
      ctx.lineTo(b.x, b.y);
      ctx.strokeStyle = 'rgba(99,102,241,0.08)';
      ctx.lineWidth = 0.7;
      ctx.stroke();
    }

    matrixAnimFrame = requestAnimationFrame(frame);
  }

  matrixAnimFrame = requestAnimationFrame(frame);
}

/* ── Render the full matrix view ────────────────────────── */
function renderMatrix() {
  const container = $('#matrix-container');
  if (!container) return;

  if (!servicesData || servicesData.length === 0) {
    container.innerHTML = '<div style="color:#9ca3af;text-align:center;padding:48px;">No services configured</div>';
    stopMatrixAnimation();
    return;
  }

  container.innerHTML = '';

  // Canvas for animated lines
  const canvas = document.createElement('canvas');
  canvas.className = 'matrix-canvas';
  container.appendChild(canvas);

  // Centre hub
  const hub = document.createElement('div');
  hub.className = 'matrix-hub';
  hub.innerHTML = '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><circle cx="12" cy="12" r="3"/><path d="M12 1v4m0 14v4m-9.5-9.5h4m14 0h4M4.2 4.2l2.8 2.8m10 10l2.8 2.8M4.2 19.8l2.8-2.8m10-10l2.8-2.8"/></svg>';
  container.appendChild(hub);

  // Nodes layer
  const nodesLayer = document.createElement('div');
  nodesLayer.className = 'matrix-nodes';
  container.appendChild(nodesLayer);

  // Tooltip
  if (!matrixTooltipEl) {
    matrixTooltipEl = document.createElement('div');
    matrixTooltipEl.className = 'matrix-tooltip';
    document.body.appendChild(matrixTooltipEl);
  }

  // Lay nodes out after container is visible and sized
  requestAnimationFrame(() => {
    const rect = container.getBoundingClientRect();
    const W = rect.width;
    const H = rect.height;
    const cx = W / 2;
    const cy = H / 2;

    // Position hub
    hub.style.left = cx + 'px';
    hub.style.top  = cy + 'px';

    // Calculate positions — elliptical ring around centre
    const count = servicesData.length;
    const rx = Math.min(cx - 50, 260);  // horizontal radius
    const ry = Math.min(cy - 50, 180);  // vertical radius
    const nodePositions = [];

    servicesData.forEach((svc, i) => {
      const angle = (2 * Math.PI * i / count) - Math.PI / 2;
      const nx = cx + rx * Math.cos(angle);
      const ny = cy + ry * Math.sin(angle);
      const { statusClass, statusLabel, ms } = matrixStatusOf(svc);

      // Build icon HTML
      let iconHtml = '';
      if (svc.icon_url) {
        iconHtml = '<img src="' + svc.icon_url + '" class="matrix-node-icon" alt="" onerror="this.style.display=\'none\'">';
      } else {
        const raw = getServiceIconHtml(svc);
        if (raw.includes('<img')) {
          iconHtml = raw.replace(/class="icon[^"]*"/g, 'class="matrix-node-icon"');
        } else {
          iconHtml = '<span class="matrix-node-icon-placeholder">' + raw.replace(/<\/?span[^>]*>/g, '') + '</span>';
        }
      }

      const name = escapeHtml(svc.display_name || svc.name || svc.key || '');
      const msText = ms != null ? ms + 'ms' : '';

      const node = document.createElement('div');
      node.className = 'matrix-node';
      node.style.left = nx + 'px';
      node.style.top  = ny + 'px';
      node.innerHTML =
        '<div class="matrix-node-ring ' + statusClass + '">' + iconHtml + '</div>' +
        '<span class="matrix-node-label">' + name + '</span>' +
        (msText ? '<span class="matrix-node-ms">' + msText + '</span>' : '');

      // Tooltip on hover
      const tipText = name + ' — ' + statusLabel + (msText ? ' (' + msText + ')' : '');
      node.addEventListener('mouseenter', function(e) {
        matrixTooltipEl.textContent = tipText;
        matrixTooltipEl.classList.add('visible');
      });
      node.addEventListener('mousemove', function(e) {
        matrixTooltipEl.style.left = e.clientX + 12 + 'px';
        matrixTooltipEl.style.top  = e.clientY - 30 + 'px';
      });
      node.addEventListener('mouseleave', function() {
        matrixTooltipEl.classList.remove('visible');
      });

      nodesLayer.appendChild(node);

      nodePositions.push({
        x: nx,
        y: ny,
        status: statusClass,
        phase: i * 600  // stagger pulse per node
      });
    });

    // Start canvas animation
    animateMatrixLines(canvas, nodePositions);
  });
}

// Detect the actual protocol from URL and check_type
function getProtocolBadge(svc) {
  const url = (svc.url || '').toLowerCase();
  const checkType = (svc.check_type || 'http').toLowerCase();
  
  if (checkType === 'always_up') {
    return 'DEMO';
  }
  if (checkType === 'tcp' || url.startsWith('tcp://')) {
    return 'TCP';
  }
  if (checkType === 'dns' || url.startsWith('dns://')) {
    return 'DNS';
  }
  // For HTTP check type, detect from URL
  if (url.startsWith('https://')) {
    return 'HTTPS';
  }
  if (url.startsWith('http://')) {
    return 'HTTP';
  }
  // Default fallback
  return checkType.toUpperCase();
}

function renderDynamicUptimeBars(services) {
  const container = $('#uptime-bars-container');
  if (!container) return;

  container.innerHTML = '';

  services.forEach(svc => {
    const protocolBadge = getProtocolBadge(svc);
    const svcName = escapeHtml(svc.name || '');
    const row = document.createElement('div');
    row.className = 'service-uptime';
    row.innerHTML = `
      <div class="service-uptime-header">
        <span class="service-name">${svcName}</span>
        <span class="protocol-badge">${protocolBadge}</span>
        <span class="uptime-percent" id="uptime-${svc.key}">—%</span>
      </div>
      <div class="uptime-bar-container">
        <div class="uptime-bar" id="uptime-bar-${svc.key}"></div>
      </div>
    `;
    container.appendChild(row);
  });
}

function renderAdminServicesList(services) {
  const list = $('#servicesList');
  if (!list) return;

  list.innerHTML = '';
  const totalServices = services.length;

  services.forEach((svc, index) => {
    const item = document.createElement('div');
    item.className = 'service-item';
    item.dataset.id = svc.id;
    item.dataset.index = index;
    item.draggable = true;

    // Use icon HTML (with img for known types or custom icon URL)
    const iconHtml = getServiceIconHtml(svc);

    // Mask the URL for display (only show domain)
    const urlDisplay = escapeHtml(maskUrl(svc.url));
    const svcName = escapeHtml(svc.name || '');

    item.innerHTML = `
      <span class="drag-handle desktop-only">⋮⋮</span>
      <div class="reorder-buttons mobile-only">
        <button class="reorder-btn move-up" ${index === 0 ? 'disabled' : ''} title="Move up">▲</button>
        <button class="reorder-btn move-down" ${index === totalServices - 1 ? 'disabled' : ''} title="Move down">▼</button>
      </div>
      <span class="service-icon-wrap">${iconHtml}</span>
      <div class="service-info">
        <div class="service-name">${svcName}</div>
        <div class="service-url">${urlDisplay}</div>
      </div>
      <div class="service-actions">
        <button class="action-btn visibility-btn ${svc.visible ? 'visible' : 'hidden-svc'}" title="${svc.visible ? 'Hide from dashboard' : 'Show on dashboard'}">
          ${svc.visible ? '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M1 12s4-8 11-8 11 8 11 8-4 8-11 8-11-8-11-8z"/><circle cx="12" cy="12" r="3"/></svg>' : '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M17.94 17.94A10.07 10.07 0 0112 20c-7 0-11-8-11-8a18.45 18.45 0 015.06-5.94M9.9 4.24A9.12 9.12 0 0112 4c7 0 11 8 11 8a18.5 18.5 0 01-2.16 3.19m-6.72-1.07a3 3 0 11-4.24-4.24"/><line x1="1" y1="1" x2="23" y2="23"/></svg>'}
        </button>
        <button class="action-btn edit-btn" title="Edit service">
          <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M11 4H4a2 2 0 00-2 2v14a2 2 0 002 2h14a2 2 0 002-2v-7"/><path d="M18.5 2.5a2.121 2.121 0 013 3L12 15l-4 1 1-4 9.5-9.5z"/></svg>
        </button>
      </div>
    `;

    // Drag and drop events (desktop)
    item.addEventListener('dragstart', handleDragStart);
    item.addEventListener('dragend', handleDragEnd);
    item.addEventListener('dragover', handleDragOver);
    item.addEventListener('drop', handleDrop);
    item.addEventListener('dragenter', handleDragEnter);
    item.addEventListener('dragleave', handleDragLeave);

    // Reorder button events (mobile)
    item.querySelector('.move-up')?.addEventListener('click', () => moveService(svc.id, 'up'));
    item.querySelector('.move-down')?.addEventListener('click', () => moveService(svc.id, 'down'));

    // Visibility toggle
    item.querySelector('.visibility-btn').addEventListener('click', () => toggleServiceVisibility(svc.id, !svc.visible));

    // Edit button
    item.querySelector('.edit-btn').addEventListener('click', () => openServiceModal(svc));

    list.appendChild(item);
  });
}

// Move service up or down
async function moveService(id, direction) {
  const list = $('#servicesList');
  const items = [...list.querySelectorAll('.service-item')];
  const currentIndex = items.findIndex(item => item.dataset.id == id);

  if (currentIndex === -1) return;

  const newIndex = direction === 'up' ? currentIndex - 1 : currentIndex + 1;
  if (newIndex < 0 || newIndex >= items.length) return;

  // Get all service IDs in new order
  const newOrder = items.map(item => parseInt(item.dataset.id));
  [newOrder[currentIndex], newOrder[newIndex]] = [newOrder[newIndex], newOrder[currentIndex]];

  try {
    const orders = {};
    newOrder.forEach((serviceID, index) => {
      orders[serviceID] = index;
    });

    await j('/api/admin/services/reorder', {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        'X-CSRF-Token': getCsrf()
      },
      body: JSON.stringify({ orders })
    });

    await loadAllServices();
    loadServices().then(services => {
      renderDynamicUptimeBars(services);
    });
  } catch (err) {
    console.error('Failed to reorder:', err);
    showToast('Failed to reorder services', 'error');
  }
}

// Drag and drop state
let draggedItem = null;

function handleDragStart(e) {
  draggedItem = this;
  this.classList.add('dragging');
  e.dataTransfer.effectAllowed = 'move';
  e.dataTransfer.setData('text/html', this.innerHTML);
}

function handleDragEnd(e) {
  this.classList.remove('dragging');
  $$('.service-item').forEach(item => item.classList.remove('drag-over'));
  draggedItem = null;
}

function handleDragOver(e) {
  e.preventDefault();
  e.dataTransfer.dropEffect = 'move';
  return false;
}

function handleDragEnter(e) {
  this.classList.add('drag-over');
}

function handleDragLeave(e) {
  this.classList.remove('drag-over');
}

function handleDrop(e) {
  e.stopPropagation();
  e.preventDefault();

  if (draggedItem !== this) {
    const list = $('#servicesList');
    const items = [...list.querySelectorAll('.service-item')];
    const draggedIndex = items.indexOf(draggedItem);
    const targetIndex = items.indexOf(this);

    if (draggedIndex < targetIndex) {
      this.parentNode.insertBefore(draggedItem, this.nextSibling);
    } else {
      this.parentNode.insertBefore(draggedItem, this);
    }

    // Save the new order
    saveServiceOrder();
  }

  this.classList.remove('drag-over');
  return false;
}

async function saveServiceOrder() {
  const list = $('#servicesList');
  const items = [...list.querySelectorAll('.service-item')];

  const orders = {};
  items.forEach((item, index) => {
    orders[parseInt(item.dataset.id)] = index;
  });

  try {
    await j('/api/admin/services/reorder', {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        'X-CSRF-Token': getCsrf()
      },
      body: JSON.stringify({ orders })
    });
    showToast('Order saved');
    // Reload to reflect new order everywhere
    loadServices().then(services => {
      renderDynamicUptimeBars(services);
    });
  } catch (e) {
    console.error('Failed to save order', e);
    showToast('Failed to save order', 'error');
  }
}

// Mask URL for display - show only host, hide path and port
function maskUrl(url) {
  try {
    const parsed = new URL(url);
    return `${parsed.protocol}//${parsed.hostname}`;
  } catch {
    return '***';
  }
}

async function toggleServiceVisibility(id, visible) {
  try {
    await j(`/api/admin/services/${id}/visibility`, {
      method: 'PUT',
      headers: {
        'Content-Type': 'application/json',
        'X-CSRF-Token': getCsrf()
      },
      body: JSON.stringify({ visible })
    });
    showToast(`Service ${visible ? 'shown' : 'hidden'}`);
    loadAllServices();
    loadServices().then(services => {
      renderDynamicUptimeBars(services);
      refresh();
    });
  } catch (e) {
    console.error('Failed to toggle visibility', e);
    showToast('Failed to toggle visibility', 'error');
  }
}

// Update icon preview in the service modal
function updateIconPreview(iconUrl) {
  const preview = $('#iconPreview');
  if (!preview) return;

  if (iconUrl) {
    preview.innerHTML = `<img src="${iconUrl}" class="icon-preview-img" alt="Icon preview" onerror="this.style.display='none';this.nextElementSibling.style.display='block';" /><span class="icon-preview-fallback" style="display:none;">⚠️</span>`;
    preview.classList.remove('hidden');
  } else {
    preview.innerHTML = '';
    preview.classList.add('hidden');
  }
}

function openServiceModal(service = null) {
  const modal = $('#serviceModal');
  if (!modal) return;

  editingServiceId = service?.id || null;

  // Update modal title
  const title = $('#serviceModalTitle');
  if (title) {
    title.textContent = service ? 'Edit Service' : 'Add Service';
  }

  // Show/hide delete button
  const deleteBtn = $('#deleteService');
  if (deleteBtn) {
    deleteBtn.classList.toggle('hidden', !service);
  }

  // Clear any previous error
  const errEl = $('#serviceError');
  if (errEl) {
    errEl.textContent = '';
    errEl.classList.add('hidden');
  }

  // Clear any previous test result
  const testResultEl = $('#testConnectionResult');
  if (testResultEl) {
    testResultEl.textContent = '';
    testResultEl.classList.add('hidden');
  }

  // Populate form
  $('#serviceTemplate').value = service?.service_type || '';
  $('#serviceName').value = service?.name || '';
  $('#serviceUrl').value = service?.url || '';
  $('#serviceToken').value = service?.api_token || '';
  $('#serviceIconUrl').value = service?.icon_url || '';
  $('#serviceCheckType').value = service?.check_type || 'http';
  $('#serviceTimeout').value = service?.timeout || 5;
  $('#serviceInterval').value = service?.check_interval || 60;
  $('#serviceExpectedMin').value = service?.expected_min || 200;
  $('#serviceExpectedMax').value = service?.expected_max || 399;
  $('#serviceVisible').checked = service?.visible !== false;
  $('#serviceId').value = service?.id || '';
  $('#serviceType').value = service?.service_type || '';

  // Update icon preview
  updateIconPreview(service?.icon_url);

  // If editing, disable template selection
  $('#serviceTemplate').disabled = !!service;

  modal.showModal();
}

function closeServiceModal() {
  const modal = $('#serviceModal');
  if (modal) modal.close();
  editingServiceId = null;
}

function handleTemplateChange(e) {
  const templateType = e.target.value;
  if (!templateType) return;

  // Templates use 'type' field from the backend
  const template = serviceTemplates.find(t => t.type === templateType);
  if (!template) return;

  // Auto-fill form fields from template
  $('#serviceName').value = template.name;
  $('#serviceCheckType').value = template.check_type;

  // Auto-fill icon URL from template if available
  if (template.icon_url) {
    $('#serviceIconUrl').value = template.icon_url;
    updateIconPreview(template.icon_url);
  }

  // Set URL placeholder based on template
  if (template.default_url) {
    const urlField = $('#serviceUrl');
    if (!urlField.value) {
      urlField.placeholder = template.default_url;
    }
  }

  // Show help text if available
  const helpEl = $('#templateHelp');
  if (helpEl && template.help_text) {
    helpEl.textContent = template.help_text;
  }

  // Show/hide token field based on whether it's required
  const tokenGroup = $('#tokenGroup');
  const tokenHelp = $('#tokenHelp');
  if (tokenGroup) {
    if (template.requires_token) {
      tokenGroup.classList.remove('hidden');
      if (tokenHelp && template.token_header) {
        tokenHelp.textContent = `Required header: ${template.token_header}`;
      }
    }
  }
}

// Test service connection before saving
async function testServiceConnection() {
  const url = $('#serviceUrl').value.trim();
  const apiToken = $('#serviceToken').value.trim();
  const checkType = $('#serviceCheckType').value;
  const timeout = parseInt($('#serviceTimeout').value) || 5;
  const serviceType = $('#serviceTemplate').value || $('#serviceType').value || 'custom';

  const resultEl = $('#testConnectionResult');
  const btn = $('#testServiceConnection');

  if (!url) {
    if (resultEl) {
      resultEl.textContent = 'Please enter a URL first';
      resultEl.className = 'test-result error';
      resultEl.classList.remove('hidden');
    }
    return;
  }

  // Show loading state
  if (btn) {
    btn.disabled = true;
    btn.textContent = 'Testing...';
  }
  if (resultEl) {
    resultEl.textContent = 'Testing connection...';
    resultEl.className = 'test-result';
    resultEl.classList.remove('hidden');
  }

  try {
    const resp = await j('/api/admin/services/test', {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        'X-CSRF-Token': getCsrf()
      },
      body: JSON.stringify({
        url,
        api_token: apiToken,
        check_type: checkType,
        timeout,
        service_type: serviceType
      })
    });

    if (resultEl) {
      if (resp.success) {
        let msg = '✓ Connection successful';
        if (resp.status_code) {
          msg += ` (${resp.status_code})`;
        }
        if (resp.latency_ms !== undefined) {
          msg += ` - ${resp.latency_ms}ms`;
        }
        resultEl.textContent = msg;
        resultEl.className = 'test-result success';
      } else {
        resultEl.textContent = '✗ ' + (resp.error || 'Connection failed');
        resultEl.className = 'test-result error';
      }
      resultEl.classList.remove('hidden');
    }
  } catch (e) {
    console.error('Connection test failed:', e);
    if (resultEl) {
      resultEl.textContent = '✗ ' + (e.body?.error || e.message || 'Connection test failed');
      resultEl.className = 'test-result error';
      resultEl.classList.remove('hidden');
    }
  } finally {
    if (btn) {
      btn.disabled = false;
      btn.textContent = 'Test Connection';
    }
  }
}

async function saveService() {
  const serviceData = {
    name: $('#serviceName').value.trim(),
    url: $('#serviceUrl').value.trim(),
    key: generateServiceKey($('#serviceName').value),
    service_type: $('#serviceTemplate').value || $('#serviceType').value || 'custom',
    api_token: $('#serviceToken').value.trim(),
    icon_url: $('#serviceIconUrl').value.trim(),
    check_type: $('#serviceCheckType').value,
    timeout: parseInt($('#serviceTimeout').value) || 5,
    check_interval: parseInt($('#serviceInterval').value) || 60,
    expected_min: parseInt($('#serviceExpectedMin').value) || 200,
    expected_max: parseInt($('#serviceExpectedMax').value) || 399,
    visible: $('#serviceVisible').checked
  };

  if (!serviceData.name || !serviceData.url) {
    const errEl = $('#serviceError');
    if (errEl) {
      errEl.textContent = 'Name and URL are required';
      errEl.classList.remove('hidden');
    }
    return;
  }

  try {
    if (editingServiceId) {
      // Update existing service
      await j(`/api/admin/services/${editingServiceId}`, {
        method: 'PUT',
        headers: {
          'Content-Type': 'application/json',
          'X-CSRF-Token': getCsrf()
        },
        body: JSON.stringify(serviceData)
      });
      showToast('Service updated');
    } else {
      // Create new service
      await j('/api/admin/services', {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
          'X-CSRF-Token': getCsrf()
        },
        body: JSON.stringify(serviceData)
      });
      showToast('Service created');
    }

    closeServiceModal();
    loadAllServices();
    loadServices().then(services => {
      renderDynamicUptimeBars(services);
      refresh();
    });
  } catch (e) {
    console.error('Failed to save service', e);
    const errEl = $('#serviceError');
    if (errEl) {
      errEl.textContent = e.body?.error || 'Failed to save service';
      errEl.classList.remove('hidden');
    }
  }
}

async function deleteService() {
  if (!editingServiceId) return;

  if (!confirm('Are you sure you want to delete this service? All monitoring data will be lost.')) {
    return;
  }

  try {
    await j(`/api/admin/services/${editingServiceId}`, {
      method: 'DELETE',
      headers: { 'X-CSRF-Token': getCsrf() }
    });
    showToast('Service deleted');
    closeServiceModal();
    loadAllServices();
    loadServices().then(services => {
      renderDynamicUptimeBars(services);
      refresh();
    });
  } catch (e) {
    console.error('Failed to delete service', e);
    showToast('Failed to delete service', 'error');
  }
}

function generateServiceKey(name) {
  return name.toLowerCase()
    .replace(/[^a-z0-9]+/g, '-')
    .replace(/^-|-$/g, '')
    .substring(0, 32);
}

// Initialize services management
function initServicesManagement() {
  // Load templates
  loadServiceTemplates();

  // Add service button
  const addBtn = $('#addServiceBtn');
  if (addBtn) {
    addBtn.addEventListener('click', () => openServiceModal());
  }

  // Service modal handlers
  const closeBtn = $('#closeServiceModal');
  if (closeBtn) {
    closeBtn.addEventListener('click', closeServiceModal);
  }

  const cancelBtn = $('#cancelService');
  if (cancelBtn) {
    cancelBtn.addEventListener('click', closeServiceModal);
  }

  const saveBtn = $('#saveService');
  if (saveBtn) {
    saveBtn.addEventListener('click', saveService);
  }

  const testBtn = $('#testServiceConnection');
  if (testBtn) {
    testBtn.addEventListener('click', testServiceConnection);
  }

  const deleteBtn = $('#deleteService');
  if (deleteBtn) {
    deleteBtn.addEventListener('click', deleteService);
  }

  const templateSelect = $('#serviceTemplate');
  if (templateSelect) {
    templateSelect.addEventListener('change', handleTemplateChange);
  }

  // Update icon preview when URL changes
  const iconUrlInput = $('#serviceIconUrl');
  if (iconUrlInput) {
    iconUrlInput.addEventListener('input', (e) => {
      updateIconPreview(e.target.value.trim());
    });
  }

  // Close modal on backdrop click
  const modal = $('#serviceModal');
  if (modal) {
    modal.addEventListener('click', (e) => {
      if (e.target === modal) closeServiceModal();
    });
  }

  // Load services when Services tab is clicked
  const tabBtns = $$('.tab-btn');
  tabBtns.forEach(btn => {
    if (btn.getAttribute('data-tab') === 'services') {
      btn.addEventListener('click', loadAllServices);
    }
  });
}

// ============ Settings Tab Handlers ============

// Save App Name
async function saveAppName() {
  const appNameInput = $('#appNameInput');
  const statusEl = $('#appNameStatus');
  const appName = appNameInput?.value?.trim() || 'Service Status';

  try {
    const res = await fetch('/api/admin/settings/app-name', {
      method: 'POST',
      credentials: 'same-origin',
      headers: {
        'Content-Type': 'application/json',
        'X-CSRF-Token': getCsrf()
      },
      body: JSON.stringify({ app_name: appName })
    });

    const data = await res.json();
    if (res.ok) {
      showStatus(statusEl, 'App name saved! Refreshing...', 'success');
      // Update the page title and header immediately
      document.title = data.app_name || appName;
      const appTitle = $('#appTitle');
      if (appTitle) appTitle.textContent = data.app_name || appName;
      // Reload to ensure all references are updated
      setTimeout(() => window.location.reload(), 1000);
    } else {
      showStatus(statusEl, data.error || 'Failed to save app name', 'error');
    }
  } catch (err) {
    showStatus(statusEl, 'Network error: ' + err.message, 'error');
  }
}

// Change Password
async function changePassword() {
  const currentPassword = $('#currentPassword')?.value;
  const newPassword = $('#newPassword')?.value;
  const confirmPassword = $('#confirmPassword')?.value;
  const statusEl = $('#passwordStatus');

  if (!currentPassword || !newPassword || !confirmPassword) {
    showStatus(statusEl, 'Please fill in all fields', 'error');
    return;
  }

  if (newPassword !== confirmPassword) {
    showStatus(statusEl, 'New passwords do not match', 'error');
    return;
  }

  if (newPassword.length < 6) {
    showStatus(statusEl, 'Password must be at least 6 characters', 'error');
    return;
  }

  try {
    const res = await fetch('/api/admin/settings/password', {
      method: 'POST',
      credentials: 'same-origin',
      headers: {
        'Content-Type': 'application/json',
        'X-CSRF-Token': getCsrf()
      },
      body: JSON.stringify({
        current_password: currentPassword,
        new_password: newPassword
      })
    });

    const data = await res.json();
    if (res.ok) {
      showStatus(statusEl, 'Password changed successfully!', 'success');
      $('#currentPassword').value = '';
      $('#newPassword').value = '';
      $('#confirmPassword').value = '';
    } else {
      showStatus(statusEl, data.error || 'Failed to change password', 'error');
    }
  } catch (err) {
    showStatus(statusEl, 'Network error: ' + err.message, 'error');
  }
}

// Export Database
async function exportDatabase() {
  const statusEl = $('#backupStatus');
  try {
    showStatus(statusEl, 'Preparing export...', 'info');

    const res = await fetch('/api/admin/settings/export', { credentials: 'same-origin' });
    if (!res.ok) {
      const data = await res.json();
      showStatus(statusEl, data.error || 'Export failed', 'error');
      return;
    }

    const blob = await res.blob();
    const url = URL.createObjectURL(blob);
    const a = document.createElement('a');
    a.href = url;
    const timestamp = new Date().toISOString().slice(0, 10);
    a.download = `servicarr-backup-${timestamp}.json`;
    document.body.appendChild(a);
    a.click();
    document.body.removeChild(a);
    URL.revokeObjectURL(url);

    showStatus(statusEl, 'Database exported successfully!', 'success');
  } catch (err) {
    showStatus(statusEl, 'Export failed: ' + err.message, 'error');
  }
}

// Import Database
let selectedImportFile = null;

function handleImportFileSelect(event) {
  const file = event.target.files[0];
  if (!file) return;

  selectedImportFile = file;
  const fileNameEl = $('#importFileName');
  if (fileNameEl) fileNameEl.textContent = file.name;

  const dialog = $('#confirmImportDialog');
  if (dialog) dialog.showModal();
}

async function confirmImportDatabase() {
  const statusEl = $('#backupStatus');
  const errorEl = $('#importDbError');

  if (!selectedImportFile) {
    if (errorEl) {
      errorEl.textContent = 'No file selected';
      errorEl.classList.remove('hidden');
    }
    return;
  }

  try {
    const formData = new FormData();
    formData.append('backup', selectedImportFile);

    const res = await fetch('/api/admin/settings/import', {
      method: 'POST',
      credentials: 'same-origin',
      headers: { 'X-CSRF-Token': getCsrf() },
      body: formData
    });

    const data = await res.json();

    const dialog = $('#confirmImportDialog');
    if (dialog) dialog.close();

    if (res.ok) {
      showStatus(statusEl, 'Database imported successfully! Reloading...', 'success');
      // Reload page after import to reflect changes
      setTimeout(() => window.location.reload(), 1500);
    } else {
      showStatus(statusEl, data.error || 'Import failed', 'error');
    }
  } catch (err) {
    showStatus(statusEl, 'Import failed: ' + err.message, 'error');
    const dialog = $('#confirmImportDialog');
    if (dialog) dialog.close();
  }

  // Reset file input
  const fileInput = $('#importDbFile');
  if (fileInput) fileInput.value = '';
  selectedImportFile = null;
}

// Reset Database
function openResetDbDialog() {
  const dialog = $('#confirmResetDialog');
  if (dialog) {
    $('#confirmResetPassword').value = '';
    $('#resetDbError')?.classList.add('hidden');
    dialog.showModal();
  }
}

async function confirmResetDatabase() {
  const password = $('#confirmResetPassword')?.value;
  const errorEl = $('#resetDbError');
  const statusEl = $('#backupStatus');

  if (!password) {
    if (errorEl) {
      errorEl.textContent = 'Password is required';
      errorEl.classList.remove('hidden');
    }
    return;
  }

  try {
    const res = await fetch('/api/admin/settings/reset', {
      method: 'POST',
      credentials: 'same-origin',
      headers: {
        'Content-Type': 'application/json',
        'X-CSRF-Token': getCsrf()
      },
      body: JSON.stringify({ password })
    });

    const data = await res.json();

    const dialog = $('#confirmResetDialog');
    if (dialog) dialog.close();

    if (res.ok) {
      showStatus(statusEl, 'Database reset successfully! Redirecting to setup...', 'success');
      // Redirect to setup page after reset
      setTimeout(() => window.location.href = '/setup', 1500);
    } else {
      if (errorEl) {
        errorEl.textContent = data.error || 'Reset failed';
        errorEl.classList.remove('hidden');
      }
    }
  } catch (err) {
    if (errorEl) {
      errorEl.textContent = 'Network error: ' + err.message;
      errorEl.classList.remove('hidden');
    }
  }
}

function showStatus(el, message, type) {
  if (!el) return;
  el.textContent = message;
  el.className = 'status-message ' + type;
  el.classList.remove('hidden');

  // Auto-hide success messages after 5 seconds
  if (type === 'success') {
    setTimeout(() => el.classList.add('hidden'), 5000);
  }
}

// Initialize Settings Tab
function initSettingsTab() {
  // Save app name
  const saveAppNameBtn = $('#saveAppNameBtn');
  if (saveAppNameBtn) {
    saveAppNameBtn.addEventListener('click', saveAppName);
  }

  // Change password
  const changePasswordBtn = $('#changePasswordBtn');
  if (changePasswordBtn) {
    changePasswordBtn.addEventListener('click', changePassword);
  }

  // Export database
  const exportBtn = $('#exportDbBtn');
  if (exportBtn) {
    exportBtn.addEventListener('click', exportDatabase);
  }

  // Import database
  const importInput = $('#importDbFile');
  if (importInput) {
    importInput.addEventListener('change', handleImportFileSelect);
  }

  const cancelImport = $('#cancelImportDb');
  if (cancelImport) {
    cancelImport.addEventListener('click', () => {
      const dialog = $('#confirmImportDialog');
      if (dialog) dialog.close();
      selectedImportFile = null;
    });
  }

  const confirmImport = $('#confirmImportDb');
  if (confirmImport) {
    confirmImport.addEventListener('click', confirmImportDatabase);
  }

  // Reset database
  const resetBtn = $('#resetDbBtn');
  if (resetBtn) {
    resetBtn.addEventListener('click', openResetDbDialog);
  }

  const cancelReset = $('#cancelResetDb');
  if (cancelReset) {
    cancelReset.addEventListener('click', () => {
      const dialog = $('#confirmResetDialog');
      if (dialog) dialog.close();
    });
  }

  const confirmReset = $('#confirmResetDb');
  if (confirmReset) {
    confirmReset.addEventListener('click', confirmResetDatabase);
  }

  // Close dialogs on backdrop click
  const resetDialog = $('#confirmResetDialog');
  if (resetDialog) {
    resetDialog.addEventListener('click', (e) => {
      if (e.target === resetDialog) resetDialog.close();
    });
  }

  const importDialog = $('#confirmImportDialog');
  if (importDialog) {
    importDialog.addEventListener('click', (e) => {
      if (e.target === importDialog) importDialog.close();
    });
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

// ===== LOGS TAB FUNCTIONALITY =====
let logsOffset = 0;
const LOGS_LIMIT = 50;
let currentLogFilters = { level: '', category: '', service: '' };

async function loadLogStats() {
  try {
    const res = await j('/api/admin/logs/stats');
    if (res && res.success !== false) {
      setResText('logTotalCount', res.total_logs || 0);
      setResText('logErrorCount', res.error_count || 0);
      setResText('logWarnCount', res.warn_count || 0);
      setResText('logInfoCount', res.info_count || 0);
    }
  } catch (err) {
    console.error('[Logs] Failed to load stats:', err);
  }
}

async function loadLogs(append = false) {
  try {
    const params = new URLSearchParams();
    params.set('limit', LOGS_LIMIT);
    params.set('offset', logsOffset);
    if (currentLogFilters.level) params.set('level', currentLogFilters.level);
    if (currentLogFilters.category) params.set('category', currentLogFilters.category);
    if (currentLogFilters.service) params.set('service', currentLogFilters.service);

    const res = await j('/api/admin/logs?' + params.toString());
    if (!res || !res.logs) {
      if (!append) {
        renderLogs('#allLogsList', [], false);
        renderLogs('#errorLogsList', [], true);
      }
      return;
    }

    const logs = res.logs;
    if (!append) {
      renderLogs('#allLogsList', logs, false);
      // Also show errors/warnings in highlights section
      const errorWarnLogs = logs.filter(l => l.level === 'error' || l.level === 'warn');
      renderLogs('#errorLogsList', errorWarnLogs.slice(0, 10), true);
    } else {
      appendLogs('#allLogsList', logs);
    }

    // Show/hide load more button
    const loadMoreBtn = $('#loadMoreLogs');
    if (loadMoreBtn) {
      loadMoreBtn.style.display = logs.length >= LOGS_LIMIT ? 'block' : 'none';
    }
  } catch (err) {
    console.error('[Logs] Failed to load logs:', err);
    showToast('Failed to load logs: ' + err.message, 'error');
  }
}

function renderLogs(selector, logs, isErrorList = false) {
  const container = $(selector);
  if (!container) return;

  if (!logs || logs.length === 0) {
    if (isErrorList) {
      // Success state for error list
      container.innerHTML = `
        <div class="logs-empty">
          <svg fill="none" viewBox="0 0 24 24" stroke="currentColor">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M5 13l4 4L19 7"/>
          </svg>
          <p>No errors or warnings</p>
        </div>`;
    } else {
      container.innerHTML = `
        <div class="logs-empty">
          <svg fill="none" viewBox="0 0 24 24" stroke="currentColor">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 12h6m-6 4h6m2 5H7a2 2 0 01-2-2V5a2 2 0 012-2h5.586a1 1 0 01.707.293l5.414 5.414a1 1 0 01.293.707V19a2 2 0 01-2 2z"/>
          </svg>
          <p>No logs found</p>
        </div>`;
    }
    return;
  }

  container.innerHTML = logs.map(log => renderLogEntry(log)).join('');
}

function appendLogs(selector, logs) {
  const container = $(selector);
  if (!container || !logs || logs.length === 0) return;

  // If there's an empty state, clear it first
  const emptyState = container.querySelector('.logs-empty');
  if (emptyState) {
    container.innerHTML = '';
  }

  const html = logs.map(log => renderLogEntry(log)).join('');
  container.insertAdjacentHTML('beforeend', html);
}

function renderLogEntry(log) {
  const time = new Date(log.timestamp).toLocaleString();
  const level = log.level || 'info';
  const category = log.category || '';
  const service = log.service || '';
  const message = escapeHtml(log.message || '');
  const details = log.details ? escapeHtml(log.details) : '';

  // Level icons
  const levelIcons = {
    error: '<svg fill="none" viewBox="0 0 24 24" stroke="currentColor"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 8v4m0 4h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z"/></svg>',
    warn: '<svg fill="none" viewBox="0 0 24 24" stroke="currentColor"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 9v2m0 4h.01m-6.938 4h13.856c1.54 0 2.502-1.667 1.732-3L13.732 4c-.77-1.333-2.694-1.333-3.464 0L3.34 16c-.77 1.333.192 3 1.732 3z"/></svg>',
    info: '<svg fill="none" viewBox="0 0 24 24" stroke="currentColor"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M13 16h-1v-4h-1m1-4h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z"/></svg>',
    debug: '<svg fill="none" viewBox="0 0 24 24" stroke="currentColor"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M10 20l4-16m4 4l4 4-4 4M6 16l-4-4 4-4"/></svg>'
  };

  // Category labels
  const categoryLabels = {
    check: 'Check',
    email: 'Email',
    security: 'Security',
    system: 'System',
    schedule: 'Scheduler'
  };

  const levelIcon = levelIcons[level] || levelIcons.info;
  const categoryLabel = categoryLabels[category] || category;

  // Escape details for use in data attribute

  return `
    <div class="log-entry ${level}" onclick="showLogDetails(this)" data-log='${JSON.stringify({ time, level, category: categoryLabel, service, message: log.message || '', details: log.details || '' }).replace(/'/g, "&#39;").replace(/"/g, "&quot;")}'>
      <span class="log-time">${time}</span>
      <span class="log-badge level-${level}">${levelIcon}${level.toUpperCase()}</span>
      ${category ? `<span class="log-badge category">${categoryLabel}</span>` : ''}
      ${service ? `<span class="log-service-name">${escapeHtml(service)}</span>` : ''}
      <span class="log-message">${message}</span>
      ${details ? `<span class="log-details">${details}</span>` : ''}
    </div>`;
}

function showLogDetails(el) {
  try {
    const data = JSON.parse(el.dataset.log.replace(/&#39;/g, "'"));

    // Level icons for modal
    const levelIcons = {
      error: '<svg fill="none" viewBox="0 0 24 24" stroke="currentColor"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 8v4m0 4h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z"/></svg>',
      warn: '<svg fill="none" viewBox="0 0 24 24" stroke="currentColor"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 9v2m0 4h.01m-6.938 4h13.856c1.54 0 2.502-1.667 1.732-3L13.732 4c-.77-1.333-2.694-1.333-3.464 0L3.34 16c-.77 1.333.192 3 1.732 3z"/></svg>',
      info: '<svg fill="none" viewBox="0 0 24 24" stroke="currentColor"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M13 16h-1v-4h-1m1-4h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z"/></svg>',
      debug: '<svg fill="none" viewBox="0 0 24 24" stroke="currentColor"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M10 20l4-16m4 4l4 4-4 4M6 16l-4-4 4-4"/></svg>'
    };

    const levelIcon = levelIcons[data.level] || levelIcons.info;

    const modal = document.createElement('div');
    modal.className = 'log-detail-modal';
    modal.innerHTML = `
      <div class="log-detail-content">
        <div class="log-detail-header">
          <div class="log-detail-level level-${data.level}">
            ${levelIcon}
            <span>${data.level.toUpperCase()}</span>
          </div>
          <button class="log-detail-close" onclick="this.closest('.log-detail-modal').remove()">
            <svg fill="none" viewBox="0 0 24 24" stroke="currentColor">
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M6 18L18 6M6 6l12 12"/>
            </svg>
          </button>
        </div>
        <div class="log-detail-body">
          <div class="log-detail-row">
            <span class="log-detail-label">Time</span>
            <span class="log-detail-value">${data.time}</span>
          </div>
          ${data.category ? `<div class="log-detail-row">
            <span class="log-detail-label">Category</span>
            <span class="log-detail-value">${data.category}</span>
          </div>` : ''}
          ${data.service ? `<div class="log-detail-row">
            <span class="log-detail-label">Service</span>
            <span class="log-detail-value">${data.service}</span>
          </div>` : ''}
          <div class="log-detail-row">
            <span class="log-detail-label">Message</span>
            <span class="log-detail-value">${escapeHtml(data.message)}</span>
          </div>
          ${data.details ? `<div class="log-detail-row">
            <span class="log-detail-label">Details</span>
            <pre class="log-detail-details">${escapeHtml(data.details)}</pre>
          </div>` : ''}
        </div>
      </div>
    `;

    modal.addEventListener('click', (e) => {
      if (e.target === modal) modal.remove();
    });

    document.body.appendChild(modal);
  } catch (err) {
    console.error('Failed to show log details:', err);
  }
}

function showIncidentDetails(el) {
  try {
    const data = JSON.parse(el.dataset.incident.replace(/&#39;/g, "'"));
    const levelIcon = '<svg fill="none" viewBox="0 0 24 24" stroke="currentColor"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 8v4m0 4h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z"/></svg>';

    const modal = document.createElement('div');
    modal.className = 'log-detail-modal';
    modal.innerHTML = `
      <div class="log-detail-content">
        <div class="log-detail-header">
          <div class="log-detail-level level-error">
            ${levelIcon}
            <span>INCIDENT</span>
          </div>
          <button class="log-detail-close" onclick="this.closest('.log-detail-modal').remove()">
            <svg fill="none" viewBox="0 0 24 24" stroke="currentColor">
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M6 18L18 6M6 6l12 12"/>
            </svg>
          </button>
        </div>
        <div class="log-detail-body">
          <div class="log-detail-row">
            <span class="log-detail-label">Time</span>
            <span class="log-detail-value">${escapeHtml(data.time || '')}</span>
          </div>
          <div class="log-detail-row">
            <span class="log-detail-label">Service</span>
            <span class="log-detail-value">${escapeHtml(data.service || '')}</span>
          </div>
          <div class="log-detail-row">
            <span class="log-detail-label">Check</span>
            <span class="log-detail-value">${escapeHtml((data.check_type || 'http').toUpperCase())}</span>
          </div>
          <div class="log-detail-row">
            <span class="log-detail-label">Status</span>
            <span class="log-detail-value">${escapeHtml(data.status || 'Down')}</span>
          </div>
          ${data.latency ? `<div class="log-detail-row">
            <span class="log-detail-label">Latency</span>
            <span class="log-detail-value">${escapeHtml(data.latency)}</span>
          </div>` : ''}
          ${data.error ? `<div class="log-detail-row">
            <span class="log-detail-label">Error</span>
            <span class="log-detail-value">${escapeHtml(data.error)}</span>
          </div>` : ''}
          <div class="log-detail-row">
            <span class="log-detail-label">Details</span>
            <span class="log-detail-value">${escapeHtml(data.detail || '')}</span>
          </div>
        </div>
      </div>
    `;

    modal.addEventListener('click', (e) => {
      if (e.target === modal) modal.remove();
    });

    document.body.appendChild(modal);
  } catch (err) {
    console.error('Failed to show incident details:', err);
  }
}

function renderLogsEmpty(selector) {
  const container = $(selector);
  if (!container) return;
  container.innerHTML = `
    <div class="logs-empty">
      <svg fill="none" viewBox="0 0 24 24" stroke="currentColor">
        <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 12h6m-6 4h6m2 5H7a2 2 0 01-2-2V5a2 2 0 012-2h5.586a1 1 0 01.707.293l5.414 5.414a1 1 0 01.293.707V19a2 2 0 01-2 2z"/>
      </svg>
      <p>No logs found</p>
    </div>`;
}

async function refreshLogs() {
  const btn = $('#refreshLogsBtn');
  if (btn) btn.classList.add('loading');

  try {
    logsOffset = 0;
    await Promise.all([loadLogStats(), loadLogs(false)]);
    showToast('Logs refreshed');
  } catch (e) {
    showToast('Failed to refresh logs', 'error');
  } finally {
    if (btn) btn.classList.remove('loading');
  }
}

async function applyLogFilters() {
  const levelSelect = $('#logLevelFilter');
  const categorySelect = $('#logCategoryFilter');
  const serviceSelect = $('#logServiceFilter');

  currentLogFilters.level = levelSelect ? levelSelect.value : '';
  currentLogFilters.category = categorySelect ? categorySelect.value : '';
  currentLogFilters.service = serviceSelect ? serviceSelect.value : '';

  logsOffset = 0;
  await loadLogs(false);
}

async function loadMoreLogs() {
  logsOffset += LOGS_LIMIT;
  await loadLogs(true);
}

async function clearLogs() {
  if (!confirm('Are you sure you want to clear all logs? This cannot be undone.')) {
    return;
  }

  try {
    const res = await j('/api/admin/logs', {
      method: 'DELETE',
      headers: {
        'Content-Type': 'application/json',
        'X-CSRF-Token': getCsrf()
      },
      body: JSON.stringify({ days: 0 })
    });
    if (res && res.success) {
      await refreshLogs();
      showToast('Logs cleared successfully');
    } else {
      showToast('Failed to clear logs', 'error');
    }
  } catch (err) {
    console.error('[Logs] Failed to clear logs:', err);
    showToast('Failed to clear logs', 'error');
  }
}

async function populateServiceFilter() {
  const serviceSelect = $('#logServiceFilter');
  if (!serviceSelect) return;

  try {
    const services = await j('/api/admin/services', {
      headers: { 'X-CSRF-Token': getCsrf() }
    });
    if (Array.isArray(services)) {
      // Clear existing options except the first "All Services" option
      while (serviceSelect.options.length > 1) {
        serviceSelect.remove(1);
      }
      // Add service options
      services.forEach(svc => {
        const opt = document.createElement('option');
        opt.value = svc.key;
        opt.textContent = svc.name || svc.key;
        serviceSelect.appendChild(opt);
      });
    }
  } catch (err) {
    console.error('[Logs] Failed to populate service filter:', err);
  }
}

function initLogsTab() {
  // Filter change handlers
  const levelSelect = $('#logLevelFilter');
  const categorySelect = $('#logCategoryFilter');
  const serviceSelect = $('#logServiceFilter');

  if (levelSelect) levelSelect.addEventListener('change', applyLogFilters);
  if (categorySelect) categorySelect.addEventListener('change', applyLogFilters);
  if (serviceSelect) serviceSelect.addEventListener('change', applyLogFilters);

  // Refresh button
  const refreshBtn = $('#refreshLogsBtn');
  if (refreshBtn) {
    // Remove existing listener to prevent duplicates if init is called multiple times
    refreshBtn.removeEventListener('click', refreshLogs);
    refreshBtn.addEventListener('click', refreshLogs);
  }

  // Clear logs button
  const clearBtn = $('#clearLogsBtn');
  if (clearBtn) {
    clearBtn.removeEventListener('click', clearLogs);
    clearBtn.addEventListener('click', clearLogs);
  }

  // Load more button
  const loadMoreBtn = $('#loadMoreLogs');
  if (loadMoreBtn) {
    loadMoreBtn.removeEventListener('click', loadMoreLogs);
    loadMoreBtn.addEventListener('click', loadMoreLogs);
  }

  // Load data when Logs tab is clicked
  const logsTab = document.querySelector('[data-tab="logs"]');
  if (logsTab) {
    // Ensure we don't attach multiple listeners
    // Create a named handler or check if already attached?
    // Easiest is to just attach it, assuming initLogsTab is called once or on full reload.
    // But to be safe against double-init:
    if (!logsTab._logsInit) {
      logsTab.addEventListener('click', async () => {
        await populateServiceFilter();
        await refreshLogs();
      });
      logsTab._logsInit = true;
    }
  }
}

// Notification provider selector
function initNotificationSelector() {
  const selector = document.querySelector('.notification-selector');
  if (!selector) return;
  
  selector.addEventListener('click', (e) => {
    const btn = e.target.closest('.notification-option');
    if (!btn || btn.disabled) return;
    
    const provider = btn.dataset.provider;
    
    // Update button states
    $$('.notification-option').forEach(b => b.classList.remove('active'));
    btn.classList.add('active');
    
    // Show corresponding panel
    $$('.notification-panel').forEach(p => p.classList.remove('active'));
    const panel = $(`.notification-panel[data-provider="${provider}"]`);
    if (panel) panel.classList.add('active');
  });
}

// Initialize logs tab on DOMContentLoaded
document.addEventListener('DOMContentLoaded', () => {
  initLogsTab();
  initNotificationSelector();
});
