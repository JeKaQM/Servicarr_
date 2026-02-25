
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
        statusEl.textContent = `âœ“ Connected to Glances on ${result.host || glancesUrl}`;
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
        statusEl.textContent = `âœ— Connection failed: ${err.message || 'Could not reach Glances'}`;
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
      // Example: C0 12% â€¢ C1 6% â€¢ C2 18% ...
      cpuDetail = snap.cpu_per_core_percent
        .map((v, i) => `C${i} ${fmtPct(v)}`)
        .join(' â€¢ ');
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
