
function applyAdminUIState() {
  const adminPanel = $('#adminPanel');
  if (isAdminUser) {
    adminPanel?.classList.remove('hidden');
    $$('.adminRow').forEach(e => e.classList.remove('hidden'));
  } else {
    adminPanel?.classList.add('hidden');
    $$('.adminRow').forEach(e => e.classList.add('hidden'));
  }
  if (typeof syncCPUAdminVisibility === 'function') syncCPUAdminVisibility();
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

function batteryClassForPct(p) {
  if (p == null || isNaN(p)) return '';
  const n = Number(p);
  if (n <= 20) return 'bad';
  if (n <= 50) return 'warn';
  return '';
}

function setBatteryMeter(id, pct) {
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
  const clsName = batteryClassForPct(p);
  if (clsName) el.classList.add(clsName);
}

function fmtDurationSeconds(n) {
  if (n == null || isNaN(n)) return '—';
  let seconds = Math.max(0, Math.round(Number(n)));
  const hours = Math.floor(seconds / 3600);
  seconds %= 3600;
  const minutes = Math.floor(seconds / 60);
  seconds %= 60;
  if (hours > 0) return `${hours}h ${minutes}m`;
  if (minutes > 0) return `${minutes}m`;
  return `${seconds}s`;
}

function fmtWatts(n) {
  if (n == null || isNaN(n)) return '—';
  const v = Number(n);
  if (Math.abs(v) >= 1000) return `${(v / 1000).toFixed(2)} kW`;
  return `${v.toFixed(0)} W`;
}

function formatUPSOnBatteryDetail(statusText) {
  const detail = String(statusText || '')
    .replace(/^on battery,?\s*/i, '')
    .trim();
  return detail ? `Running on UPS battery - ${detail}` : 'Running on UPS battery';
}

function upsStatusTokens(ups) {
  return new Set(
    String(ups && ups.status ? ups.status : '')
      .toUpperCase()
      .split(/\s+/)
      .filter(Boolean)
  );
}

function getUPSAlertState(ups) {
  if (!ups) return 'unknown';

  const tokens = upsStatusTokens(ups);
  const charge = Number(ups.battery_charge_percent);
  const hasCharge = ups.battery_charge_percent != null && Number.isFinite(charge);
  const criticalStatus = ['LB', 'FSD', 'OVER', 'RB', 'ALARM', 'OFF'].some(token => tokens.has(token));

  if (criticalStatus || (ups.power_present === false && hasCharge && charge <= 20)) {
    return 'critical';
  }
  if (
    ups.power_present === false ||
    (hasCharge && charge <= 20) ||
    ['OB', 'DISCHRG', 'BYPASS', 'CAL'].some(token => tokens.has(token))
  ) {
    return 'warning';
  }
  if (ups.power_present === true) return 'ok';
  return 'unknown';
}

function getUPSCriticalLabel(ups) {
  const tokens = upsStatusTokens(ups);
  if (tokens.has('LB')) return 'Battery critically low';
  if (tokens.has('OVER')) return 'UPS overload';
  if (tokens.has('RB')) return 'Battery replacement required';
  if (tokens.has('FSD') || tokens.has('OFF')) return 'UPS shutdown warning';
  if (tokens.has('ALARM')) return 'UPS alarm';
  return 'UPS battery critical';
}

function setUPSState(ups) {
  const tileEl = document.querySelector('#card-resources .resource-tile[data-kind="ups"]')
    || document.querySelector('.resource-tile[data-kind="ups"]');
  const stateEl = document.getElementById('res-ups-state');

  if (tileEl) tileEl.classList.remove('ups-warning', 'ups-critical', 'ups-unknown');
  if (stateEl) {
    stateEl.classList.remove('ups-state-ok', 'ups-state-warning', 'ups-state-critical', 'ups-state-unknown');
  }

  if (!ups) {
    if (tileEl) tileEl.classList.add('ups-unknown');
    if (stateEl) stateEl.classList.add('ups-state-unknown');
    setResText('res-ups-state-label', 'UPS state');
    setResText('res-ups-status', 'Unavailable');
    return;
  }

  const statusText = ups.status_text || ups.status || '';
  const alertState = getUPSAlertState(ups);
  if (alertState === 'critical') {
    if (tileEl) tileEl.classList.add('ups-critical');
    if (stateEl) stateEl.classList.add('ups-state-critical');
    setResText('res-ups-state-label', getUPSCriticalLabel(ups));
    setResText('res-ups-status', ups.power_present === false
      ? formatUPSOnBatteryDetail(statusText)
      : (statusText || 'Immediate attention required'));
    return;
  }

  if (alertState === 'warning') {
    const tokens = upsStatusTokens(ups);
    if (tileEl) tileEl.classList.add('ups-warning');
    if (stateEl) stateEl.classList.add('ups-state-warning');
    if (ups.power_present === false || tokens.has('OB') || tokens.has('DISCHRG')) {
      setResText('res-ups-state-label', 'Mains power lost');
      setResText('res-ups-status', formatUPSOnBatteryDetail(statusText));
    } else if (tokens.has('BYPASS')) {
      setResText('res-ups-state-label', 'UPS bypass active');
      setResText('res-ups-status', statusText || 'Load is bypassing battery protection');
    } else if (tokens.has('CAL')) {
      setResText('res-ups-state-label', 'UPS calibration active');
      setResText('res-ups-status', statusText || 'Runtime calibration in progress');
    } else {
      setResText('res-ups-state-label', 'UPS battery low');
      setResText('res-ups-status', statusText || 'Battery reserve is low');
    }
    return;
  }

  if (alertState === 'ok') {
    if (stateEl) stateEl.classList.add('ups-state-ok');
    setResText('res-ups-state-label', 'Line power present');
    setResText('res-ups-status', statusText || 'Online');
    return;
  }

  if (tileEl) tileEl.classList.add('ups-unknown');
  if (stateEl) stateEl.classList.add('ups-state-unknown');
  setResText('res-ups-state-label', 'UPS state unknown');
  setResText('res-ups-status', statusText || 'Status unavailable');
}

function applyResourcesVisibility(config) {
  const section = document.getElementById('card-resources');
  if (!section || !config) return;

  // Cache the config for use elsewhere
  resourcesConfig = config;

  // Resources section only shows if enabled and at least one source is configured.
  const hasGlances = config.glances_configured === true ||
    Boolean(config.glances_url && config.glances_url.trim() !== '');
  const hasUPS = config.ups_configured === true || Boolean(
    config.nut_host && config.nut_host.trim() !== '' &&
    config.ups_name && config.ups_name.trim() !== ''
  );

  // For each tile: if enabled in config, remove 'hidden' class; otherwise ensure it has 'hidden'
  const tiles = $$('.resource-tile', section);
  let visibleTileCount = 0;
  let upsVisible = false;
  tiles.forEach(t => {
    const kind = t.getAttribute('data-kind');
    let show = false;
    if (kind === 'cpu') show = hasGlances && config.cpu !== false;
    else if (kind === 'mem') show = hasGlances && config.memory !== false;
    else if (kind === 'net') show = hasGlances && config.network !== false;
    else if (kind === 'temp') show = hasGlances && config.temp !== false;
    else if (kind === 'storage') show = hasGlances && config.storage !== false;
    else if (kind === 'swap') show = hasGlances && config.swap === true;
    else if (kind === 'load') show = hasGlances && config.load === true;
    else if (kind === 'gpu') show = hasGlances && config.gpu === true;
    else if (kind === 'containers') show = hasGlances && config.containers === true;
    else if (kind === 'processes') show = hasGlances && config.processes === true;
    else if (kind === 'uptime') show = hasGlances && config.uptime === true;
    else if (kind === 'ups') {
      show = hasUPS && config.ups === true;
      upsVisible = show;
    }

    if (show) {
      t.classList.remove('hidden');
      visibleTileCount++;
    } else {
      t.classList.add('hidden');
    }
  });

  // Show/hide the entire section
  const enabled = config.enabled !== false && visibleTileCount > 0;
  if (enabled) {
    section.classList.remove('hidden');
  } else {
    section.classList.add('hidden');
  }

  if ((!enabled || !upsVisible) && typeof clearUPSLineAlert === 'function') {
    clearUPSLineAlert();
  }
}

function hydrateResourcesForm(cfg) {
  if (!$('#resourcesEnabled')) return;

  $('#glancesUrl').value = cfg.glances_url || '';
  if ($('#nutHost')) $('#nutHost').value = cfg.nut_host || '';
  if ($('#upsName')) $('#upsName').value = cfg.ups_name || '';
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
  if ($('#resourcesUPS')) $('#resourcesUPS').checked = cfg.ups === true;
}

function readResourcesFormConfig(overrides = {}) {
  return Object.assign({
    glances_url: $('#glancesUrl').value.trim(),
    nut_host: $('#nutHost') ? $('#nutHost').value.trim() : '',
    ups_name: $('#upsName') ? $('#upsName').value.trim() : '',
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
    ups: $('#resourcesUPS') ? $('#resourcesUPS').checked : false,
  }, overrides);
}

async function loadResourcesConfig() {
  try {
    const configURL = isAdminUser
      ? '/api/admin/resources/config'
      : `/api/resources/config?_=${Date.now()}`;
    const cfg = await j(configURL);
    applyResourcesVisibility(cfg);

    if (isAdminUser) hydrateResourcesForm(cfg);
  } catch (err) {
    // Keep the current visibility state on transient failures. Only hide the
    // section when the first configuration request fails completely.
    if (!resourcesConfig) {
      applyResourcesVisibility({
        enabled: false,
        glances_configured: false,
        ups_configured: false,
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
        uptime: false,
        ups: false
      });
    }
  }
}

async function saveResourcesConfig() {
  const statusEl = $('#resourcesStatus');
  const btn = $('#saveResources');
  if (!btn) return;

  const config = readResourcesFormConfig();

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

  await handleButtonAction(
    btn,
    async () => {
      const result = await j('/api/admin/resources/test', {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
          'X-CSRF-Token': getCsrf()
        },
        body: JSON.stringify({
          source: 'glances',
          glances_url: glancesUrl
        })
      });

      if (statusEl) {
        statusEl.textContent = `✓ Connected to Glances on ${result.host || glancesUrl}`;
        statusEl.className = 'status-message success';
        statusEl.classList.remove('hidden');
        setTimeout(() => statusEl.classList.add('hidden'), 5000);
      }

    },
    'Glances connection successful',
    async (err, msg) => {
      if (statusEl) {
        statusEl.textContent = `✗ Connection failed: ${msg || err.message || 'Could not reach Glances'}`;
        statusEl.className = 'status-message error';
        statusEl.classList.remove('hidden');
      }
    }
  );
}

async function testUPSConnection() {
  const statusEl = $('#resourcesStatus');
  const btn = $('#testUPS');
  const nutHost = $('#nutHost') ? $('#nutHost').value.trim() : '';
  const upsName = $('#upsName') ? $('#upsName').value.trim() : '';

  if (!nutHost || !upsName) {
    if (statusEl) {
      statusEl.textContent = 'Please enter a NUT host:port and UPS name first';
      statusEl.className = 'status-message error';
      statusEl.classList.remove('hidden');
      setTimeout(() => statusEl.classList.add('hidden'), 3000);
    }
    return;
  }

  await handleButtonAction(
    btn,
    async () => {
      const result = await j('/api/admin/resources/test', {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
          'X-CSRF-Token': getCsrf()
        },
        body: JSON.stringify({
          source: 'ups',
          nut_host: nutHost,
          ups_name: upsName
        })
      });

      if (!result.ups) {
        throw new Error('UPS data was not returned');
      }

      if (statusEl) {
        const label = result.ups.model || result.ups.name || upsName;
        statusEl.textContent = `Connected to UPS ${label}`;
        statusEl.className = 'status-message success';
        statusEl.classList.remove('hidden');
        setTimeout(() => statusEl.classList.add('hidden'), 5000);
      }
    },
    'UPS connection successful',
    async (err, msg) => {
      if (statusEl) {
        statusEl.textContent = `UPS connection failed: ${msg || err.message || 'Could not reach NUT'}`;
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
  const upsTile = document.querySelector('#card-resources .resource-tile[data-kind="ups"]');

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
  const upsEnabled = upsTile && !upsTile.classList.contains('hidden');

  // If ALL tiles are disabled, don't fetch data at all
  if (!cpuEnabled && !memEnabled && !tempEnabled && !netEnabled && !storageEnabled && !swapEnabled && !loadEnabled && !gpuEnabled && !containersEnabled && !processesEnabled && !uptimeEnabled && !upsEnabled) {
    if (pill) {
      pill.textContent = 'DISABLED';
      pill.className = 'pill';
    }
    return;
  }

  try {
    const snap = await j('/api/resources');

    if (cpuEnabled) updateCPUTile(snap);

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

    // UPS tile
    let upsAlertState = 'unknown';
    if (upsEnabled) {
      const ups = snap.ups;
      if (ups) {
        if (typeof updateUPSLineAlert === 'function') updateUPSLineAlert(ups);
        upsAlertState = getUPSAlertState(ups);
        const modelText = ups.model || ups.name || 'UPS device';
        const hasLoad = ups.load_percent != null && !isNaN(ups.load_percent);
        const hasRated = ups.realpower_nominal_watt != null && !isNaN(ups.realpower_nominal_watt);
        let outputSub = 'Total connected load on UPS';
        if (hasLoad && hasRated) {
          outputSub = ups.output_power_estimated
            ? `Estimated from ${fmtPct(ups.load_percent)} load of ${fmtWatts(ups.realpower_nominal_watt)} rated output`
            : `${fmtPct(ups.load_percent)} load of ${fmtWatts(ups.realpower_nominal_watt)} rated output`;
        } else if (hasLoad) {
          outputSub = `${fmtPct(ups.load_percent)} UPS load`;
        } else if (hasRated) {
          outputSub = `${fmtWatts(ups.realpower_nominal_watt)} rated output`;
        }

        setUPSState(ups);
        setResText('res-ups-model', modelText);
        setResText('res-ups-output-label', ups.output_power_estimated
          ? 'ESTIMATED CONNECTED OUTPUT'
          : 'CONNECTED OUTPUT');
        setResText('res-ups-watts', fmtWatts(ups.output_power_watt));
        setResText('res-ups-output-sub', outputSub);
        setResText('res-ups-charge', fmtPct(ups.battery_charge_percent));
        setBatteryMeter('meter-ups', ups.battery_charge_percent);
        const powerText = ups.power_present === true
          ? 'Present'
          : (ups.power_present === false ? 'On battery' : 'Unknown');
        setResText('res-ups-power', powerText);
        setResText('res-ups-runtime', fmtDurationSeconds(ups.battery_runtime_seconds));
        setResText('res-ups-load', fmtPct(ups.load_percent));
        setResText('res-ups-rated', fmtWatts(ups.realpower_nominal_watt));
        setResText('res-ups-input', ups.input_voltage != null && !isNaN(ups.input_voltage)
          ? `${Number(ups.input_voltage).toFixed(0)} V`
          : '—');

      } else {
        if (typeof updateUPSLineAlert === 'function') updateUPSLineAlert(null);
        setUPSState(null);
        setResText('res-ups-model', 'UPS data unavailable');
        setResText('res-ups-output-label', 'CONNECTED OUTPUT');
        setResText('res-ups-watts', 'N/A');
        setResText('res-ups-output-sub', 'Unable to read connected load');
        setResText('res-ups-charge', 'N/A');
        setBatteryMeter('meter-ups', null);
        setResText('res-ups-power', '—');
        setResText('res-ups-runtime', '—');
        setResText('res-ups-load', '—');
        setResText('res-ups-rated', '—');
        setResText('res-ups-input', '—');
      }
    }

    // Pill status based on availability and enabled metrics
    if (pill) {
      const hasAny = (snap.cpu_percent != null) || (snap.mem_percent != null) || (snap.temp_c != null) || (snap.net_rx_bytes_per_sec != null) || (snap.net_tx_bytes_per_sec != null) || (snap.ups != null);
      if (upsAlertState === 'critical') {
        pill.textContent = 'UPS ALERT';
        pill.className = 'pill down';
      } else if (upsAlertState === 'warning') {
        pill.textContent = snap.ups && snap.ups.power_present === false ? 'UPS ON BATTERY' : 'UPS WARNING';
        pill.className = 'pill warn';
      } else {
        pill.textContent = hasAny ? 'LIVE' : 'PARTIAL';
        pill.className = hasAny ? 'pill ok' : 'pill warn';
      }
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
      if (section) section.classList.add('hidden');
      if (typeof clearUPSLineAlert === 'function') clearUPSLineAlert();
      return;
    }

    // Genuine error (502 Glances unreachable, network timeout, etc.)
    if (pill) {
      pill.textContent = status === 502 ? 'UNREACHABLE' : 'UNAVAILABLE';
      pill.className = 'pill warn';
    }
    // Only reset meters for enabled tiles
    if (cpuEnabled) updateCPUTile(null);
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
    if (upsEnabled) {
      setBatteryMeter('meter-ups', null);
      setUPSState(null);
      setResText('res-ups-model', '—');
      setResText('res-ups-output-label', 'CONNECTED OUTPUT');
      setResText('res-ups-watts', '—');
      setResText('res-ups-output-sub', '—');
      setResText('res-ups-charge', '—');
      setResText('res-ups-power', '—');
      setResText('res-ups-runtime', '—');
      setResText('res-ups-load', '—');
      setResText('res-ups-rated', '—');
      setResText('res-ups-input', '—');
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
